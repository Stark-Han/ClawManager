# Team 整队启停修复交接（2026-11-11）

本文件替代此前的限时交接结论。最终现场验收结果在文末；不能用镜像 Ready 代替任务成功。

## 代码范围和合并方式

- App：`D:/test/ClawManager-team-upstream-merge`；Runtime：`D:/test/AgentsRuntime`。
- 两个仓库均为 `codex/team-personal-integration`。个人项目未修改。
- 保留上游普通实例自动休眠、沙箱/NFS 配置及已合并更新；Team 通过现有生命周期服务协调整队启停，不恢复旧调度器。
- Team 成员不进入普通实例独立自动停机队列。只有整队没有未完成任务、审核、通知、模型请求、工具命令及未确认写入，并且最后一条用户任务正式成功完成、超过既有空闲期限，才允许自动停机。心跳、在线回报和轮询时间不延长业务空闲时间。
- 保留 Leader/Worker 派发、串行执行、依赖、正式交付、Leader 关闭根任务的既有机制；没有增加 Worker 必填字段。
- CM 与北向共用整队状态、启动/停止接口和按钮组件。停止后保留历史和文件，发送任务在启动完成前被拒绝。

## 本次提交及根因

App：

- `fab27c52`：整队启动期间给已启动成员续租；停止时等待原实例的持久化退出证明，沿用同一个整队操作有限续查。
- `e6c707a1`、`ea4f46bb`、`e1cd3d7a`：冷启动沿用原工作区，仅接受同版本、完成整队停止、实际保留 PVC/NFS 来源的可信证明。失败或仅预检的升级记录不冒充已执行迁移；已确认迁移和排除规则仍有效。
- `fb31655f`：显式整队启动重新检查已报错的成员，健康成员保留；存在活跃模型/命令/写入则拒绝恢复转换，原业务任务不改写。
- `f9993de9`、`373ffe5f`：先核实冷启动保留的 PVC/内联 NFS 等价关系，再执行普通存储筛选；同一整队操作未取得绑定的重试沿用原停止来源，任何后续执行/停止记录仍阻止沿用。真实 MySQL 覆盖来源不明、版本不同、普通实例及后续停止记录拒绝。
- `4a15d945`：人工 Lite 停止也在原生退出前读取实际 Pod/保留卷并持久化存储身份。真实 MySQL 验证退出失败时该来源已经保存，原重启恢复回归通过。
- `74dcae84`：真实 MySQL 中复现任务授权检查的 1213 死锁。实例/任务锁顺序统一到同一事务，只对确定回滚的 1213 有限重试，权限撤销、停机与代数检查保留。旧代码并发用例失败，修复后包含原撤销生命周期断言的真实测试通过。
- `dd886db4`：后端单文件镜像覆盖入口，保留已验证的前端和原生资源，避免重复叠加无关镜像层。

Runtime：

- `c8a481c`：OpenCode 原生辅助进程按原目录路径/device/inode 验证 Team 授权，而非依赖子命名空间中不可映射的组号。
- `9fb94d3`：保留可信镜像的 Team/模型治理能力声明，防止实例请求伪造或清除这些能力。
- `f272f82`：OpenClaw 10.2 默认阻止固定沙箱模型中继的私网地址。仅对隔离模式、确切的管理模型中继和相应版本增加 provider 请求策略，并同步现存模型缓存；外部 provider 不放宽。
- `03d4508`：10.2 配置重载校验也使用有效期内的管理代理租约，原始配置不持久化临时信任；过期/缺失继续拒绝。
- `33d0282`：Redis 租约到期会关闭既有连接；旧客户端对已销毁 socket 的后续命令会永久等待。现在新命令写入前合并重连、重新认证/选择数据库；丢失响应的写入不自动重放；显式关闭会拒绝所有等待者。
- `52e3379`：OpenCode 和 DeepSeek Harness 复用同一协议，也发布重连修复。没有分叉业务协议。
- `03eb18d`：固定模型中继原先删除所有 X-ClawManager 请求头，导致已协商治理的 OpenCode 请求丢失任务身份并持续 409。只在固定模型入口保留完整、唯一且与租约代数一致的三个任务坐标；控制/实例身份、公共网络、文件服务及响应仍过滤。旧代码真实 HTTP 中继用例失败，修复后整个 netbroker 包通过。四种 Lite 管理器重新构建；原生模型二进制和功能未更换。

## 必要验收证据

已通过的检查不因每次打包重复运行：

- 真实临时 MySQL：整队空闲/心跳无关、活跃任务和请求拒绝、任务与停止竞争、启动续租、部分恢复、迟到证明、冷启动来源与版本/迁移保护；新增显式整队启动恢复用例通过。
- 实际服务器无业务卷的 rootless 原生辅助进程复现及修复后检查通过；原目录和内容保留。
- 普通非 Team Hermes #608 从 generation 1 重启到 2，实际原生状态 running，随后通过正常停止释放绑定，数据保留。
- 实际 Redis TCP 断线及真实 Redis CLIENT KILL：有界失败、合并重连、重新 AUTH/SELECT、下一条消息只到一次、停止可退出；原忙时上下文/通知串行测试通过。
- 安装后的 10.2 重载校验实际通过，过期/缺失策略拒绝；安装后的 OpenCode/DSH 协议均包含同一重连实现。
- Team #182 原实例仍为 603/604/605/606，分别为 OpenClaw/Hermes/OpenCode/DeepSeek Harness。
- 原任务 #395 最终报告仍为 3541 bytes，SHA256 `3309f56d5edf73193021c62ce9e2685ca51d55c8f3f844497cd16984ad72cd17`。
- #396 是现场发现模型中继和旧 Redis 连接问题的失败验收任务；没有 Worker 派发。操作员用已有 Runtime `failActiveTask` 上报真实失败并保留诊断文件，未把它改成成功、未直接修改 SQL 状态。这不作为自主模型验收通过证据。
- 忙时停止真实 API 返回 409，原任务和成员未被停止。
- 整队停止操作 `43d10cdd-e418-4151-b0cc-997fdf199131` 于 `2026-11-11T02:17:18.571767Z` 完成 4/4，所有原绑定释放。接口返回 stopped、can_start=true；四成员显示已停止。

## 实际发布与构建

App 已更新至 `team-lifecycle-20261111-v8`，digest `sha256:0a1c7420f08eab24f0123aa6d597554f0674b449ac990b80606bae397508320b`，业务二进制来源 `74dcae84`。Core/Gateway 实际 Ready，用户指定 apply 命令 exit=0。此前中间镜像不作为最终验收。

已执行用户指定的 `/home/hxc/clawmanager-apply.sh`：APP_IMAGE 为上述 App，TENANT_SUFFIX=-hxc-peer，NODE_PORT=32543。Core/Gateway Ready；最终 apply exit=0。首次切换期间 challenge 端口临时不可达，待两个服务 Ready 后串行重跑成功，未并行发布。

最终 Lite 镜像固定为：

| Runtime | 镜像与 digest |
| --- | --- |
| OpenClaw | `10.130.14.23:5000/agentsruntime/openclaw-lite@sha256:5a4dfd45c579a89916d3e482d04598b7dfcdabd1e7cd92c26799aec6e910e477` |
| Hermes | `10.130.14.23:5000/agentsruntime/hermes-lite@sha256:a428a39f803619ec5793cd69947bd51acbdfccfff591fd6a080c40e5df163771` |
| OpenCode | `10.130.14.23:5000/agentsruntime/opencode-lite@sha256:a44c2e261a8aeeb653bdd2ce2665ce21378b47aa600c580a77ae3c936aed3641` |
| DeepSeek Harness | `10.130.14.23:5000/agentsruntime/deepseek-harness-lite@sha256:0470e202435e067e3ff6ebb60c5909cc542993b6928a5637a94f11c57b455ff3` |

仅在停止 4/4 且实际绑定为零后更新受影响的保护池。Core 批准镜像、站点覆盖配置、runtime-images.json 和 Lite 目录同步；Pro 目录完整身份比对不变。

构建入口：App `Dockerfile.team-server-overlay`；Claw `openclaw/Dockerfile.team-model-relay-overlay`；Code/DSH 各自 `Dockerfile.team-transport-overlay`。Code/DSH 的 `team-protocol` 构建上下文必须先用同一提交的 `opencode/team/build-protocol.mjs`、`deepseek-harness/team/build.mjs` 生成，不复用旧生成文件。正式重建可以使用各自完整 Dockerfile，无需保留测试 overlay 链。

## 数据、回退和运行前提

- 私密备份、原始镜像/配置、迁移日志、停机证明保存在服务器 `/home/hxc/team-lifecycle-20261110/`。不得提交凭据或备份正文。
- 原实例、Team、任务历史和工作区保留；没有清库、删除绑定冒充停机、chmod777 或递归改写目录权限。
- 保留上游命名空间、安全和 NFS 设置。当前节点原生 user namespace 前提已验证；只使用已部署对应 Localhost AppArmor profile 的 node1，集群全局限制未关闭。
- 不在保护沙箱内直接发 Gateway 自重启信号：旧 10.2 自重启可能离开原进程组，管理端会保守报 gateway_stop_failed。此次最终恢复使用正式整队退出证明，不把信号当成停机证明。
- 旧人工停止遗漏的存储字段曾导致旧 Pod 替换后缺失来源。此次针对 #603 generation 4 的一条已完成停止记录，仅修复空的 `storage_identity`：核对原数据库 Pod UID、该 UID 的集群退出事件、部署前快照与保留 ReplicaSet 完整 spec 一致、同一 Deployment UID、早于停止创建的 Bound PVC/Retain RWX PV。证据及条件更新结果保存在服务器 `openclaw-stop-storage-repair-{evidence,result}-20261111.json`；未修改任务、整队状态、绑定或停止结果。未来停止由正式代码保存，不依赖此一次性修复。
- 临时的旧 Gateway 重载配置和内存/容器插件过渡不属于最终方案：停止后已删除临时 loopback trustedProxies/managerUrl，正式镜像包含完整修复。
- 回退必须使用认识 Team 生命周期和原生停止证明的兼容 App/Runtime。先核实原操作，停止确认后切镜像；不回滚覆盖新产生的数据库和文件，不清除证明或重新执行旧任务。

## 操作诊断与补偿审计

- #397 在模型中继缺失任务头、持续 409 后，通过现有 Runtime 失败报告保留失败诊断；原生 OpenCode 已中断并读回实际 idle，再受控停止整队。它不作为成功验收。
- 操作员选择根任务诊断上下文时误取了 Leader 收件箱中的 A1 结果，导致 #497 的 owner/显示结果被诊断投影覆盖。我已核对数据库中原始 `completion_proposed`（成员 575、原 completion ID、原 revision）并仅恢复这条已成功工作项的投影；未改变根 #397 的 failed 或制造新完成。原 Worker 正文、产物和正式回执均保留。条件更新只命中 1 行；服务器审计 `diagnostic-projection397-before-and-repair.txt` SHA256 `cd7fb5709d280572fa6e7d9b40f90f30a5387cdee49120ab96fdb300867d6da1`。以后诊断必须定位根任务原始派发上下文，不能取收件箱首条 taskId 匹配。
- 整队停止 `e284097f-3056-483b-b698-91b95658a1aa` 于 `2026-11-11T03:02:45.26388Z` 确认 4/4，四原实例 603/604/605/606 的新停止记录均完成且 `storage_identity` 为 252 bytes；没有再补写存储字段。仅在实际绑定数为 0 后更换四个 Lite 管理器。

## 最终现场闭环

正在执行：原四成员启动、全新四引擎短任务、正式回执和最终文件、跨 Redis 租约边界在线状态、再一次整队停止/启动。该段完成后写入实际任务、操作和时间；当前不能把这部分标成通过。
