# Team Profiles 本地测试与重启流程

本文档用于记录本地测试 Team 角色模板、Team 执行过程面板时的操作步骤。

kubectl port-forward -n clawmanager-system svc/clawmanager-frontend 30443:443

## 当前测试方式

保留原始部署文件不动：

```powershell
deployments/k8s/clawmanager.yaml
```

使用测试覆盖文件只替换 ClawManager app 镜像：

```powershell
deployments/k8s/clawmanager-team-profiles-test.yaml
```

这只会更新 ClawManager 前端/后端容器，不会更改 OpenClaw/Hermes 成员运行镜像。

## 第一次部署

如果集群里还没有安装 ClawManager，先应用原始完整 YAML：

```powershell
cd D:\test\ClawManager-2
kubectl apply -f deployments/k8s/clawmanager.yaml
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

如果 ClawManager 已经在运行，不需要重复执行这一步。

## 构建本地 ClawManager 测试镜像

必须在项目根目录执行，因为 Dockerfile 在根目录：

```powershell
cd D:\test\ClawManager-2
docker build -t clawmanager:team-profiles-test .
```

如果在 `deployments/k8s` 目录执行 `docker build ... .` 会失败，因为那里没有 Dockerfile。

## kind 集群加载镜像

当前集群节点名类似 `my-cluster-control-plane`，通常是 kind 集群。

kind 看不到本机 Docker 镜像，需要手动加载：

```powershell
kind load docker-image clawmanager:team-profiles-test --name my-cluster
```

如果集群名不是 `my-cluster`，查看：

```powershell
kind get clusters
```

然后把 `--name my-cluster` 换成实际名称。

## 应用测试 YAML

```powershell
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

如果卡在：

```text
old replicas are pending termination
```

先看 Pod：

```powershell
kubectl -n clawmanager-system get pods -l app=clawmanager-app -o wide
```

如果新 Pod 是 `ImagePullBackOff`，说明镜像没有加载进 kind：

```powershell
kind load docker-image clawmanager:team-profiles-test --name my-cluster
kubectl -n clawmanager-system delete pod -l app=clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

## 修改前端代码后怎么重启

前端代码会被打包进 ClawManager 镜像，所以修改前端后需要重新构建并重启 ClawManager app：

```powershell
cd D:\test\ClawManager-2
docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

浏览器强刷：

```text
Ctrl + F5
```

## 修改后端代码后怎么重启

后端代码同样在 ClawManager 镜像里。修改后端后也使用同一套流程：

```powershell
cd D:\test\ClawManager-2
docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app
```

不需要重启 OpenClaw/Hermes 镜像，除非修改的是它们自己的 runtime 镜像代码。

## 重启电脑后怎么恢复

1. 打开 Docker Desktop，等待 Kubernetes/kind 节点恢复。
2. 进入项目根目录：

```powershell
cd D:\test\ClawManager-2
```

3. 检查 ClawManager 是否还在：

```powershell
kubectl -n clawmanager-system get pods
```

4. 如果 ClawManager app 还在运行，但不是测试镜像，重新应用测试 YAML：

```powershell
docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout restart deployment/clawmanager-app
kubectl -n clawmanager-system rollout status deployment/clawmanager-app

kubectl port-forward -n clawmanager-system svc/clawmanager-frontend 30443:443
```

5. 如果整个系统资源都没了，先重新安装原始 YAML，再应用测试 YAML：

```powershell
kubectl apply -f deployments/k8s/clawmanager.yaml
kubectl -n clawmanager-system rollout status deployment/clawmanager-app

docker build -t clawmanager:team-profiles-test .
kind load docker-image clawmanager:team-profiles-test --name my-cluster
kubectl apply -f deployments/k8s/clawmanager-team-profiles-test.yaml
kubectl -n clawmanager-system rollout status deployment/clawmanager-app



kubectl port-forward -n clawmanager-system svc/clawmanager-frontend 30443:443
```

## 验证 Team 角色模板注入

创建 Team 后，查看成员 Pod：

```powershell
kubectl -n clawmanager get pods --show-labels
```

进入成员 Pod 检查环境变量：

```powershell
kubectl -n clawmanager exec -it <member-pod> -- sh -lc 'printenv | grep AGENTS_JSON'
kubectl -n clawmanager exec -it <member-pod> -- sh -lc 'cat /etc/clawmanager/team/team.json'
```

能看到下面任意变量，说明 ClawManager 注入成功：

```text
CLAWMANAGER_RUNTIME_AGENTS_JSON
CLAWMANAGER_OPENCLAW_AGENTS_JSON
CLAWMANAGER_HERMES_AGENTS_JSON
```

## 验证 Team 执行过程事件

查看 Redis events stream：

```powershell
kubectl -n clawmanager-system exec deploy/clawmanager-team-redis -- redis-cli XRANGE claw:team:<team-id>:events - + COUNT 20
```

查看成员 inbox：

```powershell
kubectl -n clawmanager-system exec deploy/clawmanager-team-redis -- redis-cli XRANGE claw:team:<team-id>:inbox:leader - + COUNT 20
kubectl -n clawmanager-system exec deploy/clawmanager-team-redis -- redis-cli XRANGE claw:team:<team-id>:inbox:worker - + COUNT 20
```

如果 Redis 里只有最终结果，没有 `task_started`、`progress`、`reply`、`outbound` 等事件，前端执行过程面板也只能显示有限过程。

## 常用排查命令

查看 ClawManager app：

```powershell
kubectl -n clawmanager-system get deploy clawmanager-app -o wide
kubectl -n clawmanager-system get pods -l app=clawmanager-app -o wide
kubectl -n clawmanager-system describe pod -l app=clawmanager-app
kubectl -n clawmanager-system logs deploy/clawmanager-app --tail=120
```

查看最近事件：

```powershell
kubectl -n clawmanager-system get events --sort-by=.lastTimestamp
```

查看当前镜像：

```powershell
kubectl -n clawmanager-system get deploy clawmanager-app -o jsonpath="{.spec.template.spec.containers[0].image}"
```

应显示：

```text
clawmanager:team-profiles-test
```
