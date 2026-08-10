# OpenClaw Gateway + Windows Node Exec Configuration

本文说明一个常见 ClawManager 使用场景：

```text
用户浏览器
  -> ClawManager Web / OpenClaw WebChat
  -> 服务端 OpenClaw 实例里的 Gateway
  -> 本机 Windows 上运行的 OpenClaw node
  -> 在 Windows 本机执行 system.run / system.which
```

目标是：用户访问 ClawManager 创建出的 OpenClaw 实例 Web UI，在聊天中让 Agent 操作个人 Windows 电脑上的命令或自动化任务。

## 1. 基本判断

这个场景可行，但要区分两类授权：

| 授权类型 | 作用 | 是否每次变化 |
| --- | --- | --- |
| Device pairing | 允许 Windows node 接入服务端 Gateway | 通常只需要一次 |
| Exec approval | 允许某条 `system.run` 在 Windows node 上执行 | ask 模式下每条命令都会生成新的 requestId |

如果错误是：

```json
{
  "status": "error",
  "tool": "exec",
  "error": "UNAVAILABLE: SYSTEM_RUN_DENIED: approval required"
}
```

说明链路已经打到 Windows node，问题不是网络，而是 `system.run` 被 Windows node 的 exec approval 策略拦住。

## 2. 前置条件

服务端 OpenClaw 实例里的 Gateway 必须满足：

- Gateway WebSocket 端口能被 Windows 电脑访问，例如 `192.168.30.133:31889`。
- Gateway 不能只监听容器内 `127.0.0.1`。
- 如果使用明文 `ws://`，建议只在内网、VPN、Tailscale 或 SSH tunnel 内使用。
- Windows node 需要拿到 Gateway 的认证 token 或 password。

Windows 本机需要满足：

- 已安装 OpenClaw CLI。
- 能访问服务端 Gateway 地址和端口。
- Node 进程能持久运行，建议调通后改为 service 模式。

## 3. 启动 Windows Node

在服务端 OpenClaw 实例里查看 Gateway token：

```bash
openclaw config get gateway.auth.token
```

在 Windows PowerShell 中设置 token 并启动 node：

```powershell
$env:OPENCLAW_GATEWAY_TOKEN="<gateway-token>"
openclaw node run --host 192.168.30.133 --port 31889 --display-name "windows-node"
```

如果要后台常驻：

```powershell
$env:OPENCLAW_GATEWAY_TOKEN="<gateway-token>"
openclaw node install --host 192.168.30.133 --port 31889 --display-name "windows-node" --force
openclaw node restart
openclaw node status
```

如果 Gateway 使用 TLS，再加：

```powershell
openclaw node run --host 192.168.30.133 --port 31889 --tls --display-name "windows-node"
```

## 4. 在 Gateway 侧完成 Node Pairing

进入服务端 OpenClaw 实例终端，查看待审批设备：

```bash
openclaw devices list
```

找到 role 为 `node` 的请求，批准最新的 `requestId`：

```bash
openclaw devices approve <requestId>
```

确认 node 已连接并 paired：

```bash
openclaw nodes status
openclaw nodes describe --node "windows-node"
```

如果 `devices list` 每次看到新的 pairing `requestId`，通常说明 node 身份或认证信息变化了。优先检查：

- Windows 上是否反复使用了不同的 `--node-id`。
- Windows 上 `~/.openclaw/node.json` 是否被删除或重建。
- Gateway token/password 是否变更。
- Node 是否确实使用同一个 `OPENCLAW_GATEWAY_TOKEN` 启动。

## 5. 将 Exec 指向 Windows Node

在服务端 OpenClaw 实例里配置默认 exec host：

```bash
openclaw config set tools.exec.host node
openclaw config set tools.exec.node "windows-node"
```

如果有多个 Agent，也可以按 Agent 配置：

```bash
openclaw config get agents.list
openclaw config set agents.list[0].tools.exec.node "windows-node"
```

## 6. Exec Approval 配置方案

### 方案 A：稳定自动化，免审批执行

仅建议在完全可信的内网或 VPN 环境中使用。

服务端 Gateway 请求策略：

```bash
openclaw config set tools.exec.security full
openclaw config set tools.exec.ask off
openclaw gateway restart
```

同时把 Windows node 本地 approvals 策略也打开：

```bash
openclaw approvals set --node "windows-node" --stdin <<'EOF'
{
  "version": 1,
  "defaults": {
    "security": "full",
    "ask": "off",
    "askFallback": "full"
  }
}
EOF
```

注意：只改 `tools.exec.*` 不够。OpenClaw 会取 Gateway 请求策略和 node 本地 `~/.openclaw/exec-approvals.json` 中更严格的一方。如果 node 本地仍是 `ask: "always"` 或 `askFallback: "deny"`，还是会继续提示授权。

### 方案 B：更安全，使用 allowlist

服务端 Gateway 请求策略：

```bash
openclaw config set tools.exec.security allowlist
openclaw config set tools.exec.ask off
openclaw gateway restart
```

给 Windows node 添加允许执行的命令：

```bash
openclaw approvals allowlist add --agent "*" --node "windows-node" "powershell.exe"
openclaw approvals allowlist add --agent "*" --node "windows-node" "pwsh.exe"
openclaw approvals allowlist add --agent "*" --node "windows-node" "cmd.exe"
```

更推荐只放行具体脚本或具体工具，例如：

```bash
openclaw approvals allowlist add --agent "*" --node "windows-node" "C:\\Tools\\automation\\daily-task.ps1"
openclaw approvals allowlist add --agent "*" --node "windows-node" "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
```

Windows 上如果命令被包装成 `cmd.exe /c ...`，allowlist 模式下仍可能需要审批。这是 OpenClaw 的安全行为。若需要完全无提示自动化，使用方案 A。

## 7. 验证

在服务端 OpenClaw 实例里执行：

```bash
openclaw nodes status
openclaw nodes describe --node "windows-node"
openclaw approvals get --node "windows-node"
```

然后在 WebChat 里让 Agent 执行一个简单命令：

```text
请在 node 上执行 hostname，并告诉我输出。
```

如果仍然报 `SYSTEM_RUN_DENIED`：

1. 确认 `openclaw nodes status` 中 node 是 connected/paired。
2. 确认 `openclaw nodes describe --node "windows-node"` 中包含 `system.run`。
3. 确认 `openclaw approvals get --node "windows-node"` 的 effective policy 是预期值。
4. 如果 effective policy 仍然是 ask 或 deny，说明 node 本地 approvals 文件没有被更新。
5. 如果是 allowlist miss，说明命令没有命中 allowlist，改成 full 或补 allowlist。

## 8. requestId 每次不同怎么处理

如果 requestId 来自 `devices list`：

- 这是 node pairing 请求。
- 应该只需要批准一次。
- 如果每次都变，说明 node 身份被重置或认证信息变化。

如果 requestId 来自 exec approval：

- 这是命令执行审批。
- 每条新命令都有自己的 requestId，变化是正常现象。
- 要避免反复授权，需要配置 `tools.exec.ask off`，并同步配置 node 本地 approvals。

## 9. 安全建议

- 不要把 Gateway WebSocket 端口直接暴露公网。
- 生产环境优先使用 VPN、Tailscale、WireGuard 或 SSH tunnel。
- 如果只是做固定自动化，优先使用 allowlist 放行具体脚本。
- `security full + ask off` 相当于允许 WebChat 控制 Windows 本机命令，只有在完全可信环境使用。
- 定期检查 Windows node 的 `~/.openclaw/exec-approvals.json`。
- 不再使用时，及时停掉 node：

```powershell
openclaw node stop
openclaw node uninstall
```
