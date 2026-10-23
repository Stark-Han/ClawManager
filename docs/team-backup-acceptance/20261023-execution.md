# Team 备份连续实施记录

- 开始：2026-10-23 20:09:41（Asia/Shanghai）。最长五小时，最晚 2026-10-24 01:09:41 收尾。
- 用户授权：连续实现全部计划；分批测试、commit；仅测试服务器 10.130.14.23，租户 clawmanager-hxc-peer-system，入口 32543。部署使用 /home/hxc/clawmanager-apply.sh。正式环境只读。
- ClawManager 基线：48d00c09deabf3569fd044eaab837c6c94138111；Runtime 基线：770c469feafcc8169719075252bf9012c828bfcb，均 codex/team-personal-integration。
- 初始未跟踪文件：最终计划。未发现其他业务未提交改动。
- 测试环境原 App 镜像：10.130.14.23:5000/clawmanager-hxc-app:team-owner-placement-cards-20261023-v1。
- 初始只读检查：32543 对应指定租户；App 就绪。App 工作区通过 NFS service 挂载，底层受管 PV 为 HostPath；尚未证明符合 strong capture，不冒充 CSI 快照能力。

## 当前结果（持续更新，2026-10-23 22:15）

| 项目 | 状态 | 证据/限制 |
| --- | --- | --- |
| T01 registry | 部分完成 | 75 张业务表六域归属、真实全量 migrations 与字段合同匹配通过；未知表/新增可执行字段拒绝。Runtime 文件/Redis/writer 完整归属尚未闭环 |
| 数据保留/归一化 | 组件验收通过 | 真实 MySQL 全 75 表恢复及独立 SELECT-only verifier；CM/多个 Owner/provider、半创建步骤、已完成子项、部分结果、原始状态证据通过；不等于完整演练流水线验收 |
| 加密与外部发布 | 组件验收通过 | AES-256-GCM 分帧、六 part 签名、真实 MinIO conditional-create/固定版本读取；删除本地 staging 后独立读取、旧版本固定、删除指定分片失败通过。仅 functional_test/best_effort，未生成正式 acceptance/catalog |
| schema ownership | 本地完成并已提交基础迁移 | 11 个 repository 构造 DDL 移到迁移 078；隔离 MySQL 实际迁移通过。目标 schema 仅可信 embedded DDL，业务进程拒绝连接 cm_restore_ 隔离恢复库 |
| Redis / 文件 / 对象 | 组件验收通过 | Redis DUMP 二进制/stream PEL/原 expiry 保留，只恢复到历史命名空间；文件隐藏/部分产物/hash/穿越负测；真实对象产物、metadata、空对象和目标不可覆盖验证通过。Linux 文件元数据和完整 strong checkpoint 待验 |
| K8s/Secret 采集 | 组件验收通过 | 独立真实 GET-only SA：普通采集不能读 Secret，Secret 采集不能读 Pod，两者不能跨命名空间/修改资源。固定 UID 的历史记录仅数据保留，不 Apply 旧 Pod/Secret |
| 控制面/前端 | 部分完成 | 配置 CAS、幂等创建、排队、双租约/fencing、未开始任务取消/过期收尾通过；左侧备份入口、总览/配置/任务详情接真实 API。完整 capture runner、attempt 接管、外部动作、全部前端页面未完成，准入仍拒绝缺少依赖 |
| 六源联合恢复 | 功能联合验收通过 | TestSixRealSourcesPublishAndDataOnlyRestore：真实 MySQL/Redis/MinIO/本地文件/独立 K8s GET-only 账号，外部加密发布后删除 staging，再恢复验证六类数据；运行中任务变 failed，部分结果与产物保留。未证明活跃源的 strong checkpoint，也未完成断开整个源网络的灾备矩阵 |
| 镜像 push / 测试部署 | 未执行 | 本地 Docker daemon 未启动；测试服务器使用 containerd/nerdctl，构建路径待核对 |
| 总体验收 T01～T13 / A01～A24 | 未通过 | 不以局部单测代替完整计划验收 |

原凭据仅从已忽略的连接材料读取，不写入本文、代码、日志或镜像。维护/备份/清理只作用于已登记的测试资源；不触碰其他租户。

## 已复现并修复的问题

- SQL 空字节切片经 JSON omitempty 变成 nil，驱动把非 NULL 空字符串写成 NULL：补真实 NOT NULL 空字符串回归，改用显式非 nil 空 bytes。
- 实际 schema 存在两个生成列：冻结允许的表达式，恢复 INSERT 跳过生成列，verifier 仍对比其值；未知生成列拒绝。
- JSON 重复 key / 未知字段会被静默接受：先失败回归，增加有界递归重复 key 检测与严格 DTO 解码。
- 当前升级状态 prepared/snapshotted/restart_ready/gateway_verified/restored 未登记：先失败回归，纳入中断态归一化；保留 before 状态，清除旧 operation token。
- 并发备份错误返回 conflict_active_job，与原文排队合同不符：改为 pending/global_capture_queue；幂等重放仍只创建一次。
- 失败分类允许任意字符串与取消码：改为 v12 闭集；失败状态拒绝 cancellation/API-only conflict 分类。
- 联合流水线漏传 Redis inventory budget；首次联合测试失败后补齐有界预算并回归。
- 直接行归一化与流式归一化采用不同时间精度，导致同一原状态证据发生冲突；联合测试复现后按列声明统一 finished_at 精度，重试与只读对账通过。
- 前端全局 401 拦截器会把备份强认证挑战当成登录过期，刷新普通 token 不能解决该挑战；先用实际拦截器回归复现，针对备份 strong_auth_* 返回独立处理，普通登录刷新仍通过。
- 测试 MySQL 初次启动遭遇节点 io_setup EAGAIN：只在独立 fixture Pod 禁用 native AIO，没有修改节点或其他租户。

## 隔离资源与完成边界

- 测试 namespace：`cm-backup-test-20261023-1209`，owner label `cm-20261023-1209`。内含临时 MySQL、MinIO、两个 Redis、按职责拆开的采集 SA/Role；无 NodePort。
- 临时连接经主机指纹固定的 SSH 转发；原服务器凭据和短期 SA token 均仅在仓库外临时文件中，结束时清理。
- P0 恢复目标完全不启动业务进程。`cm_restore_<32hex>` 是受保留的数据库名，官方 app/gateway 的统一连接入口拒绝该名字。此防线只声明 P0 数据演练，不能据此宣称 P1 生产启动恢复已完成。
- Runtime 停写、全部业务 writer 接入、正式 checkpoint、独立最终发布者、外部 catalog 完整扫描、strong-auth/nonce、prune/GC/promotion、四矩阵/large-smoke 和全部 UI 操作仍是未通过项。没有删除原计划这些要求，也没有把当前 functional_test 提升为正式恢复点。
- 本地基础 commit：`4ce7c273`。之后改动需通过对应验收后再提交；未 push Git，未部署 App，正式环境未操作。

## 已执行测试

- `go test ./... -count=1`：全后端通过；本次启用了 systembackup 的真实 MySQL、MinIO、Redis、Kubernetes fixture，systembackup 包耗时 218.711 秒。其他依赖专用环境变量的历史集成测试仍按原项目规则 skip，不算真实全功能验收。
- 完整数据库恢复用独立 SELECT-only MySQL 账号核对所有行，并真实确认 UPDATE/DELETE/CREATE 被拒绝。
- Linux 交叉编译测试程序在 10.130.14.23 的本次专用目录运行，文件元数据、隐藏文件、目录穿越、篡改与加密往返测试通过；没有在其他租户目录执行。
- 前端构建通过，保留现有 Node 22.11 / Vite 版本提示；备份 strong-auth 与普通 session refresh 两个行为回归通过。新增键盘焦点处理后会再构建。
