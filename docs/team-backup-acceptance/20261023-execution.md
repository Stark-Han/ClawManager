# Team 备份连续实施记录

- 开始：2026-10-23 20:09:41（Asia/Shanghai）。最长五小时，最晚 2026-10-24 01:09:41 收尾。
- 用户授权：连续实现全部计划；分批测试、commit；仅测试服务器 10.130.14.23，租户 clawmanager-hxc-peer-system，入口 32543。部署使用 /home/hxc/clawmanager-apply.sh。正式环境只读。
- ClawManager 基线：48d00c09deabf3569fd044eaab837c6c94138111；Runtime 基线：770c469feafcc8169719075252bf9012c828bfcb，均 codex/team-personal-integration。
- 初始未跟踪文件：最终计划。未发现其他业务未提交改动。
- 测试环境原 App 镜像：10.130.14.23:5000/clawmanager-hxc-app:team-owner-placement-cards-20261023-v1。
- 初始只读检查：32543 对应指定租户；App 就绪。App 工作区通过 NFS service 挂载，底层受管 PV 为 HostPath；尚未证明符合 strong capture，不冒充 CSI 快照能力。

## 当前结果（持续更新）

| 项目 | 状态 | 证据/限制 |
| --- | --- | --- |
| T01 registry | 开发中 | 已列出六域显式表归属；migration 覆盖测试通过，运行库覆盖待验 |
| 数据保留/归一化 | 开发中 | 原始行、binary/NULL、大整数、部分结果、终态子项与未知状态单测通过；尚未完成全部依赖策略和 SQL 集成 |
| 加密分帧 | 开发中 | AES-256-GCM 有界帧、身份绑定、认证 EOF；往返/篡改/截断/尾随/取消单测通过，KMS/provider/内存安全门禁待验 |
| schema ownership | 开发中 | 静态测试复现 11 个 repository 构造建表；迁移到 embedded schema，保留历史行为并补兼容字段/索引；真实迁移待验 |
| 镜像 push / 测试部署 | 未执行 | 本地 Docker daemon 未启动；测试服务器使用 containerd/nerdctl，构建路径待核对 |
| 总体验收 T01～T13 / A01～A24 | 未通过 | 不以局部单测代替完整计划验收 |

原凭据仅从已忽略的连接材料读取，不写入本文、代码、日志或镜像。维护/备份/清理只作用于已登记的测试资源；不触碰其他租户。
