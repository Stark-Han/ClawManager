# Team Profiles 本地测试与启动说明

本文记录本地 `kind + localhost:30443` 测试 Team 角色模板、Team 群聊、Execution Kanban、共享文件浏览器等功能时的正确启动方式。

## 当前结论

本地测试使用的是：

- 完整基础部署：[deployments/k8s/clawmanager.yaml](D:\test\ClawManager-2\deployments\k8s\clawmanager.yaml)
- 本地 app 镜像覆盖：[deployments/k8s/clawmanager-team-profiles-test.yaml](D:\test\ClawManager-2\deployments\k8s\clawmanager-team-profiles-test.yaml)

`clawmanager-team-profiles-test.yaml` 只用于把 `clawmanager-app` 镜像切换成本地构建的 `clawmanager:team-profiles-test`。它不会替代完整基础部署。

合并 upstream 后，Team 默认走新的 `Lite` runtime pool 逻辑。也就是说，基础部署里必须存在以下资源：

- `workspace-store`
- `openclaw-runtime`
- `hermes-runtime`
- `clawmanager-team-redis`
- `clawmanager-app` 中的 `RUNTIME_*` 环境变量
- `clawmanager-app` 的 `/workspaces` NFS 挂载

如果只 apply 旧的 test override，或者 test override 把这些字段覆盖掉，Team 实例会一直停在 `Starting`。

## 这次修复了什么

已更新 [deployments/k8s/clawmanager-team-profiles-test.yaml](D:\test\ClawManager-2\deployments\k8s\clawmanager-team-profiles-test.yaml)，补齐 upstream 新 runtime pool 需要的 app 配置：

- `PLATFORM_REDIS_URL`
- `TEAM_REDIS_URL`
- `RUNTIME_NAMESPACE`
- `RUNTIME_WORKSPACE_ROOT`
- `RUNTIME_WORKSPACE_NFS_SERVER`
- `RUNTIME_WORKSPACE_NFS_PATH`
- `RUNTIME_MAX_GATEWAYS_PER_POD`
- `RUNTIME_GATEWAY_PORT_START`
- `RUNTIME_GATEWAY_PORT_END`
- `RUNTIME_SCHEDULER_ENABLED`
- `RUNTIME_HEARTBEAT_TIMEOUT`
- `RUNTIME_AGENT_CONTROL_TOKEN`
- `RUNTIME_AGENT_REPORT_TOKEN`
- `OPENCLAW_GATEWAY_TOKEN`
- `/workspaces` NFS volume mount

也就是说，现在 test YAML 仍然只覆盖 app 镜像，但不会再把 runtime pool 必需配置删掉。

## 正确启动流程

必须在项目根目录执行，因为 Dockerfile 在根目录：

```powershell
cd D:\test\ClawManager-2
```

构建本地 ClawManager 镜像：

```powershell
docker build -t clawmanager:team-profiles-test .
```

把镜像加载进 kind 集群：

```powershell
kind load docker-image clawmanager:team-profiles-test --name my-cluster
```

先 apply 完整基础部署。合并 upstream 后，这一步会创建 runtime pool 相关资源：

```powershell
kubectl apply -f deployments/k8s/clawmanager.yaml
```

再 apply 本地测试覆盖文件：

```powershell
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
```

等待 app 重启完成：

```powershell
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

如果你已经 apply 过 test YAML，但想强制重启：

```powershell
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

启动端口转发：

```powershell
kubectl port-forward -n clawmanager-system svc/clawmanager-frontend 30443:443
```

浏览器打开：

```text
https://localhost:30443
```

## 推荐的一键顺序

```powershell
cd D:\test\ClawManager-2

docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster

kubectl apply -f deployments/k8s/clawmanager.yaml
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml

kubectl -n clawmanager-system rollout status deployment/clawmanager-app
kubectl port-forward -n clawmanager-system svc/clawmanager-frontend 30443:443
```

## 修改前端/后端后怎么重启

前端和后端都会被打进 `clawmanager-app` 镜像，所以改完代码后使用同一套流程：

```powershell
cd D:\test\ClawManager-2

docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster

kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

通常不需要重新 apply `clawmanager.yaml`。只有以下情况需要重新 apply 基础部署：

- upstream 更新了 Kubernetes 资源。
- runtime pool 相关 YAML 变了。
- `workspace-store`、`openclaw-runtime`、`hermes-runtime` 不存在。
- 本地 kind 集群被重建或资源丢失。

## 验证 runtime pool 是否正常

```powershell
kubectl -n clawmanager-system get pods
```

至少应能看到：

```text
clawmanager-app
clawmanager-team-redis
workspace-store
openclaw-runtime
hermes-runtime
mysql
minio
skill-scanner
```

查看 app 是否带有 runtime 环境变量：

```powershell
kubectl -n clawmanager-system exec deploy/clawmanager-app -- printenv | findstr RUNTIME
kubectl -n clawmanager-system exec deploy/clawmanager-app -- printenv | findstr REDIS
```

查看 runtime pod 是否向后端注册：

```powershell
kubectl -n clawmanager-system logs deploy/openclaw-runtime --tail=120
kubectl -n clawmanager-system logs deploy/hermes-runtime --tail=120
kubectl -n clawmanager-system logs deploy/clawmanager-app --tail=160
```

## Team 一直 Starting 时怎么查

先看 runtime pool 是否存在：

```powershell
kubectl -n clawmanager-system get deploy workspace-store openclaw-runtime hermes-runtime
kubectl -n clawmanager-system get pods -l clawmanager.io/runtime-type=openclaw
kubectl -n clawmanager-system get pods -l clawmanager.io/runtime-type=hermes
```

再看 app 日志里的 scheduler：

```powershell
kubectl -n clawmanager-system logs deploy/clawmanager-app --tail=200 | findstr /i "runtime scheduler gateway binding creating"
```

再看实例数据库状态和页面状态是否一致。页面里如果实例显示 `Lite` 且 `Starting`，一般表示 gateway runtime 还没有成功分配。

常见原因：

- 没有重新 apply `deployments/k8s/clawmanager.yaml`，导致 `openclaw-runtime/hermes-runtime/workspace-store` 不存在。
- apply 了旧版 `clawmanager-team-profiles-test.yaml`，把 app 的 `RUNTIME_*` 环境变量覆盖没了。
- runtime 镜像拉取失败。
- `workspace-store` NFS 未启动，导致 runtime pod 挂载失败。
- app 没有重启，仍在跑旧环境变量。

## 验证当前 app 镜像

```powershell
kubectl -n clawmanager-system get deploy clawmanager-app -o jsonpath="{.spec.template.spec.containers[0].image}"
```

应显示：

```text
clawmanager:team-profiles-test
```

如果不是，重新 apply test YAML：

```powershell
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

## 验证 Team 角色模板注入

创建 Team 后，查看 Team 成员实例：

```powershell
kubectl -n clawmanager-system logs deploy/clawmanager-app --tail=200
kubectl -n clawmanager-system exec deploy/clawmanager-team-redis -- redis-cli XRANGE claw:team:<team-id>:events - + COUNT 30
```

如果是 Lite 模式，成员不会再是传统独立桌面 Pod，而是通过 runtime gateway 调度。重点看 Redis event stream、Team 群聊、Execution Kanban 和实例状态。

## 与服务器 tenant 部署的区别

本地 localhost 测试使用：

```text
deployments/k8s/clawmanager.yaml
deployments/k8s/clawmanager-team-profiles-test.yaml
namespace: clawmanager-system
port-forward: 30443:443
```

服务器多租户部署使用：

```text
deployments/k8s/clawmanager-tenant.yaml
deployments/k8s/clawmanager-apply.sh
namespace: clawmanager-hxc-system 或其他租户 namespace
NodePort: 32443 等
```

两套部署文件不同。localhost 的 `Starting` 问题应优先检查 `clawmanager-team-profiles-test.yaml` 是否覆盖掉 upstream 新增 runtime 配置。
