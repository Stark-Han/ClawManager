# Team 备份实现状态与运行边界

本文件说明当前代码实际能力，不替代 `docs/team-backup-restore-team-integration-plan.md` 的全部交付要求。

## 已有可执行能力

- 左侧“控制台”下方新增“备份”，路由 `/admin/backups`。总览、配置读取/部分保存、任务查询/详情/取消调用真实 API，权限和依赖不满足时拒绝，不返回模拟成功。
- `backend/internal/systembackup` 提供六类真实源的加密采集、固定版本外部发布、无源数据库的读取验证、独立数据恢复。联合测试覆盖真实 MySQL、Redis、MinIO、Kubernetes 只读账号及工作区文件。
- 原始数据在不可变加密 artifact 中保留。恢复库内中断任务归一化为失败，已完成工作保留；原/后状态与行 hash 写入外部控制库的加密证据。Redis 放入历史命名空间；部署和 Secret 只读作历史，不执行旧资源。
- 官方业务进程拒绝连接 `cm_restore_<32hex>` 命名的隔离数据恢复库。恢复验证账号只有 SELECT，并实际验证 DML/DDL 被拒绝。
- `system-backup-controller` 是独立的控制协调进程：领导租约、任务租约、数据库时间、排队原因和未开始任务的取消/超时收尾。**尚未实现完整 capture/restore Job 执行和 attempt 接管，不能将此进程称为完整备份执行器。**
- `system-backup-artifact` 提供独立的 capture / verify / restore 命令。verify 仅使用外部对象存储和 bootstrap 密钥文件；restore 仅连接灾备侧预先创建的隔离目标，不连接源 DB、源 Kubernetes 或业务 Runtime。输出完整性结果不自动授予正式恢复资格。

## 独立验证工具

构建入口：`backend/cmd/system-backup-artifact`；容器路径 `/usr/local/bin/system-backup-artifact`。

```text
system-backup-artifact --bootstrap /run/backup-bootstrap/config.json --origin <installation-id> --backup sbk_<32hex> --max-bytes 17179869184 --timeout 30m
```

bootstrap JSON 包含 endpoint、bucket、root、secure、credentials_file、可选 ca_file、verification_key_file、verification_key_version、encryption_key_file、encryption_reference。凭据文件为 access_key/secret_key/可选 session_token；验证公钥是 32 字节原始 Ed25519 public key，KEK 是 32 字节原始数据；encryption_reference 使用与备份一致的 provider/name/version/purpose 四元组。所有文件通过外部 Secret 管理注入，禁止提交到 Git。非 TLS 仅允许本机回环地址，用于已经核验身份的本地测试转发。

工具固定读取签名 marker 指向的 manifest/index/object version，并完整解密校验六 part。错误或缺失历史 key 会失败；不会尝试回源、替换新 key 或宣布“全部备份已发现”。当前按已知 installation/backup identity 精确发现，**尚无正式 catalog 全量扫描**。

## 实际采集和数据恢复命令

以下入口已经接上真实数据适配器，均为显式执行的一次性操作；不是前端完整 Job 编排的替代品。

```text
system-backup-artifact --operation capture --bootstrap /run/backup/bootstrap.json --operation-config /run/backup/source.json --origin <installation-id> --backup sbk_<32hex> --max-bytes 17179869184 --max-items 1000000 --timeout 30m
system-backup-artifact --operation restore --bootstrap /run/backup/bootstrap.json --operation-config /run/backup/target.json --origin <installation-id> --backup sbk_<32hex> --max-bytes 17179869184 --max-items 1000000 --timeout 30m
```

- capture 的 bootstrap 额外指定 `signing_key_file`（64 字节原始 Ed25519 私钥，必须匹配验证公钥）。source.json 指定 `mysql_dsn_file`、`redis`、`objects`、`buckets`、`workspace`（id/path）、两个独立 Kubernetes 只读客户端、namespace/namespace_uid、部署/Secret 固定 UID 清单、空的 `stage_directory`。成功发布后仅删除本次生成的六个密文暂存文件；失败不递归删除任何目录。
- restore 的 target.json 指定 `run_id`（sdr_32hex）、`mysql_dsn_file`、`verifier_dsn_file`、`control_dsn_file`、`control_installation_id`、`redis`、`objects`、`bucket_mapping`、空的 `directory`。目标 MySQL 必须是已有可信 schema、无业务数据的 `cm_restore_<32hex>`；验证身份必须与恢复身份分离；外部控制库不能是恢复目标库。目标 Redis 必须为空且不能是源 Redis 服务器；对象桶必须预先创建、为空且使用 cm-restore-32hex 名称。
- 对象连接包含 endpoint/secure/credentials_file/可选 ca_file；Redis 连接包含 address/database/secure/credentials_file/可选 ca_file/server_name，凭据文件为 username/password。Kubernetes 连接包含 server/server_name/ca_file/token_file，不支持外部 exec 认证插件。非 TLS 对象/Redis 连接只允许回环地址转发。
- 恢复先完整认证源产物，然后归一化导入 SQL、使用独立只读连接流式逐行校验、恢复工作区/历史 Redis/对象，保存加密 Kubernetes 历史。它不应用旧部署/Secret。目标目录保存 `.restore-owner`、`restore-report.json`、workspace 和 encrypted-history；报告包含进度、失败阶段、表数量/hash、文件数量及加密历史元信息，不包含明文 Secret。
- 任何阶段失败都不启动业务服务、不自动覆盖重跑。保留本次隔离资源供核验，重新执行必须使用新目标。**尚未自动创建/销毁目标资源、验证完整网络隔离或销毁恢复写凭据，因此不能作为正式合格演练。**

## 控制协调进程

构建入口：`backend/cmd/system-backup-controller`；不调用 app 初始化或 schema migration。部署前由独立 migration 身份建立 078～081 所需结构。

- `SYSTEM_BACKUP_CONTROL_DSN_FILE`：只含控制数据库 DSN 的挂载文件。
- `SYSTEM_BACKUP_INSTALLATION_ID`：稳定安装标识，首次登记后不能静默更换。
- `SYSTEM_BACKUP_CONTROLLER_ID`：当前控制器实例标识。

API 的安装标识使用相同 `SYSTEM_BACKUP_INSTALLATION_ID`。`SYSTEM_BACKUP_ROLE_BINDINGS` 是离线引导映射 `用户ID:角色,...`；角色为 viewer/operator/config_admin/security_admin，非空配置要求至少两个 security_admin。仍须通过原应用管理员鉴权。此引导映射不代替原计划完整权限管理与近期强认证。

## 当前不能宣布完成的内容

完整业务 writer 接入、OpenClaw/Hermes 模型/工具/子进程停写、strong checkpoint、独立 finalizer/acceptance、外部 catalog、近期强认证/nonce、完整恢复执行器和报告、prune/GC/promotion、全部前端操作、四部署矩阵与 large-smoke 验收仍未完成。

因此当前功能测试产物固定为 `functional_test + best_effort`；API 创建正式备份仍受依赖门禁阻止，不能手动把健康状态勾绿来绕过。测试镜像即使部署，也只证明当前代码已上测试环境，不能等同完整 Team 备份功能已交付。

## 2026-10-23 23:15 增量：业务 SQL 门禁

- 可选 `SYSTEM_BACKUP_SQL_GATE_SCOPE` 将官方业务连接池接入数据库行锁门禁，涵盖 upper repository、后台 SQL、预编译语句和显式事务。现有写事务结束后才能进入停写；维护中读取继续，新的写入失败。Query 接口使用真实只读事务，不能通过换用 Query 绕过。
- 启用前需离线完成全部 migration、建立 scope，并通过 `SYSTEM_BACKUP_CONTROL_DSN_FILE` 提供独立控制连接。启用后应用拒绝自行迁移或执行 DDL/事务控制 SQL。该开关尚未在 32543 启用；它不证明 Redis、文件、对象或 Runtime 已停写，也不覆盖另行创建的裸连接池。
- 真实 MySQL 验证已覆盖写事务排空、维护拒写、只读访问、预编译语句、缺失 gate 拒绝，以及现有 TeamRepository / upper ORM 接入。恢复维护后可以继续正常写入。
- AgentsRuntime 本地提交 `9e9a641` 修复 Hermes 未检查 maintenance、OpenClaw 同批次中途进入 maintenance 仍派发后续消息的问题。真实 Redis 回归通过；尚未构建或部署 Runtime 镜像，也未完成所有工具/子进程/presence 排空。
- 32543 当前实际部署 ClawManager `7159197e` 镜像 `10.130.14.23:5000/clawmanager-hxc-app:team-backup-data-20261023-7159197e`。浏览器已验证入口和未就绪提示；新增 SQL 门禁代码仍属本地开发。
