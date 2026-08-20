# OpenCode Lite / Pro Agent 开发说明

**状态：** ClawManager 平台侧（类型、调度、代理改写、门户）已接入；AgentsRuntime 已增加 `opencode` RuntimeProfile / Dockerfile（钉版本官方 `opencode`）。需构建并推送 `opencode` / `opencode-lite` 镜像后，在集群 apply `opencode-runtime` 再联调。

本文写给 OpenCode runtime / agent 侧开发者，用来区分 ClawManager 中 OpenCode Lite 和 OpenCode Pro 两种形态。先读本文，再分别参考：

- Lite: `docs/agent-runtime-development-spec.md` 和 `docs/clawmanager-agent-v2-contract.md`
- Pro: `docs/agent-control-plane.md` 和 `docs/runtime-agent-integration-guide.md`

一句话原则：

- **OpenCode Lite 是 Runtime Pod Agent V2**：共享 `opencode-runtime` Pod 内的 pod-level agent 管理多个 `opencode web` 子进程。
- **OpenCode Pro 是 Instance Agent Control Plane**：每个实例独立 Webtop 桌面容器；桌面内运行官方 `opencode web`（localhost），Instance Agent 只负责本实例控制面。

OpenCode **使用官方发行物（钉版本）**，不维护 OpenCode fork。Lite 门户子路径适配由 ClawManager `InstanceProxyService` 完成（strip-prefix + HTML/绝对路径改写），镜像内 OpenCode 仍监听根路径。

**镜像配置来源：** 部署清单中 Lite 镜像以 `opencode-runtime` Deployment 的 `CLAWMANAGER_RUNTIME_IMAGE_REF`（及 container `image`）为准，与 openclaw/hermes runtime Deployment 一致；控制面可选通过环境变量 `OPENCODE_RUNTIME_IMAGE` 覆盖 `config.Runtime.OpenCodeImage` 默认值（当前与 hermes/openclaw 相同，部署 yaml 的 `clawmanager-app` 不强制写入该 env）。

## 1. 模式对照

| 项目 | OpenCode Lite | OpenCode Pro |
| --- | --- | --- |
| ClawManager mode | `lite` | `pro` |
| Runtime backend | `gateway` | `desktop` |
| Kubernetes | 共享 `opencode-runtime` Deployment/Pod | 每实例 Deployment + Service |
| 用户进程 | `opencode web --hostname 0.0.0.0 --port {分配端口}` | 桌面内 `opencode web`（如 `127.0.0.1:4096`） |
| 用户访问 | Browser → `/api/v1/instances/{id}/proxy/` → gateway 端口 | Browser → Webtop `3001` → 桌面内浏览器访问 web |
| workspace | `/workspaces/opencode/user-{uid}/instance-{id}` | `/config/.opencode` |
| 持久化 | `{workspace}/home/.opencode` | `/config/.opencode` |

## 2. Lite CreateGateway 要求

- 校验 `agent_type=opencode`，workspace 必须在 `/workspaces/opencode/` 下。
- 使用请求中的 `uid/gid` 与 `environment`。
- 必须设置 `OPENCODE_SERVER_PASSWORD`（平台注入为实例 AccessToken）与可选 `OPENCODE_SERVER_USERNAME`（默认 `opencode`）。
- 启动前写入仅含 ClawManager AI Gateway 的 OpenCode provider 配置（`OPENCODE_CONFIG_CONTENT` 或 `{persistent}/opencode.json`），禁止依赖用户 `/connect` 自带外部 key。
- 健康检查：`GET http://127.0.0.1:{port}/global/health`（根路径）。
- `RUNTIME_GATEWAY_COMMAND` 示例：`/usr/local/bin/opencode web --hostname 0.0.0.0`（端口由 agent 追加）。

## 3. Pro 要求

- Webtop 端口 `3001`，`SUBFOLDER=/api/v1/instances/{id}/proxy/`。
- Instance Agent 随容器启动；`CLAWMANAGER_AGENT_PERSISTENT_DIR=/config/.opencode`。
- 桌面内启动官方 `opencode web`；**不**依赖 ClawManager 对 OpenCode HTML 的子路径改写。
- 同样注入 Gateway env 与锁定 provider 配置。

## 4. 首版非目标

Skill Hub、OpenClaw 配置中心 plan、workspace 归档导入导出、定时任务 bootstrap、Team persona overlay。

## 5. Lite Web 失败保底

若钉版本官方 web 在平台代理改写后仍无法在门户完成加载/会话/Gateway 调用，则 Lite 可改为浏览器终端内官方 TUI；Pro 保持 web。触发条件与验收见平台接入计划。
