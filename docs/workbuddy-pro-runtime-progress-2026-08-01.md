# Workbuddy Pro Runtime Progress

> 团队版迁移说明：这是个人版历史实现记录。团队版不再展示或新建 WorkBuddy，也不构建对应 Runtime；仅保留已有实例的数据和生命周期兼容。

> Historical implementation note (2026-08-01), updated for the IEI northbound integration. The northbound API and IEI owner portal support Linux WorkBuddy Pro only. Windows compatibility code may remain for existing administrative workflows, but northbound callers cannot request Windows, choose an image, or change the fixed resource preset. Use `northbound-upgrade-guide.md` for deployment.

## Scope

Workbuddy is a first-class, Pro-only desktop runtime. It uses a dedicated Kubernetes Deployment, Service, and PVC rather than the shared Lite runtime pool.

## Implemented

- Added `workbuddy` to the instance type schema and create API validation.
- Added a fixed `Workbuddy Pro` image card in system settings and the instance creation wizard.
- Reused the managed runtime environment injection used by OpenClaw and Hermes, including ClawManager LLM gateway and instance Agent variables.
- Enabled Webtop desktop defaults, instance-specific `SUBFOLDER`, HTTPS/WSS upstream proxying, clipboard settings, and the `/config` persistent mount.
- Enabled the Portal and instance detail workspace browser for Workbuddy Pro. The browser accesses the mounted `/config` directory through the runtime agent and does not require a server-side `workspace_path` for Pro desktop instances.
- Added the Workbuddy runtime icon to the instance creation wizard.
- Added managed runtime labels, LLM session attribution, and server-side skill scan eligibility.
- Added a migration that converts an existing custom desktop image card named `workbuddy` into the fixed Workbuddy runtime card.

## Compatibility

Existing instances remain on their recorded instance type. A legacy custom Workbuddy instance must be recreated as `workbuddy`, or explicitly migrated and restarted, before it receives the new proxy and environment behavior.

Workbuddy is not registered as a Lite runtime type and is not scheduled into the OpenClaw or Hermes shared runtime pools.
