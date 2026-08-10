# Lite / Pro Runtime Rework Plan

## Goal

Rework ClawManager instance modes so users choose between:

- `lite`: shared runtime Pod plus per-instance gateway process.
- `pro`: dedicated desktop Deployment plus Service plus PVC.

Both modes must keep the same user-facing product capabilities: instance access, AI Gateway, channel management, skill management, workspace import/export, and admin operations. The modes differ only in runtime backend, scheduling, and resource/capacity controls.

## Progress Protocol

Implementation should update this plan as each phase completes. Each phase must include:

- Files changed.
- Tests or checks run.
- Known gaps or follow-up work.
- Whether user-visible behavior changed.

Do not revert unrelated existing workspace changes.

## Architecture Decisions

- User-facing mode is `instance_mode`: `lite` or `pro`.
- Internal backend remains `runtime_type`: `gateway`, `desktop`, or `shell`.
- `lite` always maps to `runtime_type = gateway`.
- `pro` always maps to `runtime_type = desktop`.
- Frontend should show Lite/Pro and avoid exposing gateway/desktop as ordinary user choices.
- Scheduler must manage only Lite instances.
- Dedicated K8s lifecycle must manage only Pro and shell instances.
- External access should still proxy through ClawManager, not directly expose Pod IP or runtime Pod gateway ports.
- AI Gateway telemetry must include instance-level and gateway-level identity for Lite so multiple Lite instances on one runtime Pod never mix audit, cost, risk, or trace records.

## Phase 0: Planning

- [x] Create this implementation plan.
- [x] Confirm dirty worktree impact before each implementation phase.

## Phase 1: Instance Mode Data Model And Routing Boundary

### Backend

- [x] Add migration for `instances.instance_mode ENUM('lite','pro') NOT NULL DEFAULT 'lite'`.
- [x] Add `InstanceMode` to `models.Instance`.
- [x] Add constants and helpers:
  - `InstanceModeLite`
  - `InstanceModePro`
  - `RuntimeTypeGateway`
  - `RuntimeTypeDesktop`
  - `NormalizeInstanceMode`
  - `RuntimeTypeForInstanceMode`
  - `InstanceModeForRuntimeType`
- [x] Extend create request validation to accept `mode` or `instance_mode`.
- [x] Preserve backward compatibility:
  - If request has `runtime_type=gateway`, infer `lite`.
  - If request has `runtime_type=desktop`, infer `pro`.
  - If neither is present, default to `lite` for OpenClaw/Hermes unless system config says otherwise.
- [x] Change create flow:
  - Lite -> existing gateway-pool creation path.
  - Pro -> existing dedicated runtime creation path.
- [x] Change `v2RuntimeTypeForInstance` to return true only for Lite/gateway instances.
- [x] Change scheduler selectors to require `runtime_type='gateway'` and `instance_mode='lite'`.
- [x] Change sync service to ignore Lite/gateway and continue managing Pro/shell dedicated workloads.

### Frontend

- [x] Add `mode` / `instance_mode` type fields.
- [x] Show Lite/Pro selection on create page.
- [x] Submit `mode` to backend.
- [ ] Keep runtime image selection compatible with Pro desktop images and Lite runtime pool images.

### Tests

- [x] Unit test Lite create produces `instance_mode=lite`, `runtime_type=gateway`, workspace path.
- [ ] Unit test Pro create produces `instance_mode=pro`, `runtime_type=desktop`, and does not enter gateway scheduler.
- [x] Unit test scheduler ignores Pro instances even if workspace_path exists.

## Phase 2: Pro Dedicated Deployment + Service + PVC

### K8s Services

- [x] Add per-instance deployment service, separate from shared runtime pool deployment service.
- [x] Build Deployment from the existing PodConfig shape:
  - labels: `instance-id`, `instance-name`, `user-id`, `instance-type`, `runtime-type=desktop`, `managed-by=clawreef`
  - selector should be immutable and match Service selector.
  - container name remains `desktop`.
  - probes, resources, env, envFrom, PVC mounts, config map mounts, SHM, and security mode should match current Pod behavior.
- [x] Add operations:
  - Ensure Deployment.
  - Scale Deployment to 1.
  - Scale Deployment to 0.
  - Delete Deployment.
  - Restart/Rollout restart.
  - Get Deployment status.
  - Get active Pod for instance.

### Instance Lifecycle

- [x] Pro create: PVC -> Deployment(replicas=1) -> Service.
- [x] Pro start: scale Deployment to 1 and ensure Service.
- [x] Pro stop: scale Deployment to 0, retain PVC and Service.
- [x] Pro restart: rollout restart or scale 0 -> 1.
- [x] Pro delete: delete Deployment, Service, PVC, Secrets, ConfigMaps.
- [x] Sync Pro status from Deployment available replicas instead of raw Pod phase.

### Tests

- [x] K8s fake-client tests for BuildInstanceDeployment.
- [ ] Lifecycle tests for Pro start/stop/delete paths.
- [ ] Proxy tests still resolve Pro through stable Service.

## Phase 3: Mode-Specific Capacity And Resource Limits

- [x] Add system-level mode limits:
  - Lite runtime pod count / gateways per pod / max active instances.
  - Pro capacity / max active instances / max CPU / max memory / max storage / max GPU.
- [ ] Consider user-level overrides after system-level support is stable.
- [x] Enforce Lite capacity before accepting Lite create/start.
- [x] Enforce Pro capacity before accepting Pro create/start.
- [x] Treat `pro_capacity = 0` as Pro create/start disabled, while keeping existing Pro instances visible and deletable.
- [ ] Add admin API to read and update mode limits.
- [x] Add tests for capacity edge cases.

## Phase 4: External Instance Access

### Data And API

- [x] Add `instance_external_access` table:
  - `instance_id`
  - `enabled`
  - `auth_mode`
  - `public_slug`
  - `public_token_hash`
  - `api_key_hash`
  - `api_key_prefix`
  - `expires_at`
  - `created_by`
  - `last_used_at`
  - timestamps
- [x] Owner APIs:
  - Enable public link.
  - Disable external access.
  - Rotate public link by enabling again.
  - Create API key.
  - Revoke current API key by disabling external access.
  - Set expiration.
- [ ] Admin APIs:
  - View external access state.
  - Revoke external access.
  - Configure global policy: disabled, api-key-only, public-link-allowed.

### Proxy

- [x] Add public route such as `/public/instances/{slug}/proxy/`.
- [x] Authenticate by capability URL token or API key.
- [x] Resolve target with the same Lite/Pro proxy routing:
  - Lite -> runtime binding and gateway port.
  - Pro -> stable Service.
- [ ] Record access audit:
  - instance_id
  - instance_mode
  - runtime_type
  - gateway_id for Lite
  - api_key prefix/id if applicable
  - source IP
  - user agent

### Tests

- [x] Public link succeeds when enabled and unexpired.
- [x] Public link fails after expiration.
- [x] API key succeeds and disabled key fails.
- [ ] Public link fails after disable/rotate.
- [ ] Lite and Pro proxy targets remain separated.

### Phase 4 Progress Notes

- Files changed:
  - `backend/internal/db/migrations/027_add_instance_external_access.sql`
  - `backend/internal/models/instance_external_access.go`
  - `backend/internal/repository/instance_external_access_repository.go`
  - `backend/internal/services/instance_external_access_service.go`
  - `backend/internal/services/instance_external_access_service_test.go`
  - `backend/internal/handlers/instance_handler.go`
  - `backend/cmd/server/main.go`
- Tests/checks run:
  - `cd backend; $env:GOCACHE='D:\code\github\ClawManagerV2\.cache\go-build'; go test ./internal/services/k8s ./internal/services ./internal/repository ./internal/handlers -run "(InstanceExternalAccess|ExternalAccess|InstanceDeployment|RuntimeDeployment|InstanceMode|V2|Scheduler|BuildV2|UserSafe|WorkspaceArchive|RuntimeLinuxID|RuntimeWorkspacePath|NormalizeV2RuntimeType|Proxy)" -count=1`
- Known gaps:
  - Admin external access policy/revoke APIs are still pending.
  - Public access audit payload is still pending.
  - Public proxy separation needs a handler/proxy-level regression test.
- User-visible behavior:
  - Instance owners can enable public-link or API-key external access.
  - Public routes proxy through ClawManager using the existing Lite/Pro proxy resolver.

## Phase 5: AI Gateway Lite Gateway-Level Attribution

- [x] Extend gateway auth context to include:
  - `instance_id`
  - `instance_mode`
  - `runtime_type`
  - `gateway_id`
  - `runtime_pod_id`
  - `user_id`
- [x] Ensure Lite gateway create injects identity into environment/token.
- [x] Ensure each Lite gateway has a distinct token or claims set.
- [x] Extend model invocation, audit event, cost record, and risk hit payloads where needed.
- [x] Add tests proving gateway attribution fields are populated and stripped before provider calls.
- [ ] Add integration/e2e test proving two Lite instances on one runtime Pod produce separate invocation records.

### Phase 5 Progress Notes

- Files changed:
  - `backend/internal/middleware/auth_middleware.go`
  - `backend/internal/handlers/ai_gateway_handler.go`
  - `backend/internal/aigateway/service.go`
  - `backend/internal/aigateway/service_test.go`
  - `backend/internal/models/model_invocation.go`
  - `backend/internal/models/audit_event.go`
  - `backend/internal/models/cost_record.go`
  - `backend/internal/models/risk_hit.go`
  - `backend/internal/repository/model_invocation_repository.go`
  - `backend/internal/repository/audit_event_repository.go`
  - `backend/internal/repository/cost_record_repository.go`
  - `backend/internal/repository/risk_hit_repository.go`
  - `backend/internal/db/migrations/028_add_ai_gateway_runtime_attribution.sql`
- Tests/checks run:
  - `cd backend; $env:GOCACHE='D:\code\github\ClawManagerV2\.cache\go-build'; go test ./internal/aigateway ./internal/services/k8s ./internal/services ./internal/repository ./internal/handlers ./internal/middleware -run "(RuntimeAttribution|BuildProvider|InstanceExternalAccess|ExternalAccess|InstanceDeployment|RuntimeDeployment|InstanceMode|V2|Scheduler|BuildV2|UserSafe|WorkspaceArchive|RuntimeLinuxID|RuntimeWorkspacePath|NormalizeV2RuntimeType|Proxy|GatewayAuth)" -count=1`
- Known gaps:
  - Full multi-gateway e2e attribution proof is still pending.
- User-visible behavior:
  - Admin AI audit/cost records can carry Lite gateway identity instead of only instance ID.

## Phase 6: Unified Channel, Skill, Import/Export For Lite And Pro

- [x] Treat channel and skill management as instance capabilities, not runtime backend features.
- [x] Lite:
  - Apply config through runtime agent to gateway.
  - Inventory reports include `instance_id` and `gateway_id`.
  - Import/export through shared workspace backend.
- [x] Pro:
  - Apply config through instance agent/control plane.
  - Inventory reports include `instance_id`.
  - Import/export through dedicated PVC workspace backend or instance agent.
- [x] Add a workspace backend abstraction:
  - Lite shared workspace path.
  - Pro dedicated PVC/workspace path.
- [x] Keep existing OpenClaw/Hermes import/export API shape.
- [x] Add tests for Lite and Pro import/export workspace selection.

### Phase 6 Progress Notes

- Files changed:
  - `backend/internal/services/openclaw_transfer_service.go`
  - `backend/internal/services/openclaw_transfer_service_test.go`
  - `backend/cmd/server/main.go`
- Tests/checks run:
  - `cd backend; $env:GOCACHE='D:\code\github\ClawManagerV2\.cache\go-build'; go test ./internal/aigateway ./internal/services/k8s ./internal/services ./internal/repository ./internal/handlers ./internal/middleware -run "(RuntimeAttribution|BuildProvider|OpenClawTransfer|WorkspaceSpec|InstanceExternalAccess|ExternalAccess|InstanceDeployment|RuntimeDeployment|InstanceMode|V2|Scheduler|BuildV2|UserSafe|WorkspaceArchive|RuntimeLinuxID|RuntimeWorkspacePath|NormalizeV2RuntimeType|Proxy|GatewayAuth)" -count=1`
- Known gaps:
  - Real runtime import/export smoke test still needs a running cluster.
- User-visible behavior:
  - Lite and Pro keep the same OpenClaw/Hermes import/export endpoints.
  - Lite import/export targets its per-instance workspace inside the shared runtime pod.

## Phase 7: Admin Monitoring And Management

### Admin Instances

- [x] Show mode: Lite/Pro.
- [x] Show runtime backend: gateway/desktop for admins only.
- [ ] Show external access status.
- [x] Show AI usage/cost/risk by instance through existing observability surfaces with new attribution fields.
- [x] Support start, stop, restart, delete, sync for Lite/Pro instances.
- [ ] Support export, logs, revoke external access directly from admin list.

### Runtime Pools

- [x] Keep Lite runtime Pod view:
  - runtime Pod status.
  - used/capacity.
  - gateway distribution.
  - drain/migrate/failover.
- [ ] Add Pro capacity view:
  - pro_capacity.
  - running Pro instances.
  - Deployment status.
  - Service/PVC summary.
  - resource usage.
- [x] Add Pro running/creating/total summary in admin instance page.

### Tests

- [x] Frontend build and targeted lint for changed files.
- [x] Admin instance page covers both Lite and Pro.
- [ ] E2E smoke navigation includes mode labels and admin operations.

### Phase 7 Progress Notes

- Files changed:
  - `frontend/src/pages/admin/InstanceManagementPage.tsx`
  - `frontend/src/pages/instances/InstanceDetailPage.tsx`
  - `frontend/src/services/instanceService.ts`
  - `frontend/src/types/instance.ts`
- Tests/checks run:
  - `cd frontend; npx eslint src/pages/admin/InstanceManagementPage.tsx src/pages/instances/InstanceDetailPage.tsx src/services/instanceService.ts src/types/instance.ts`
  - `cd frontend; npm run build`
- Known gaps:
  - Full frontend lint still fails on pre-existing unrelated files.
  - Admin external access status/revoke actions need a dedicated UI/API policy pass.
- User-visible behavior:
  - Admin instance page now has Lite/Pro and backend filters plus mode summary.
  - Instance detail page now lets owners generate public links/API keys and disable external access.

## Phase 8: Verification

- [x] `cd backend && go test ./... -count=1`
- [x] `cd frontend && npm run lint` attempted; fails on pre-existing unrelated lint debt.
- [x] `cd frontend && npx eslint <changed frontend files>`
- [x] `cd frontend && npm run build`
- [ ] YAML parse/dry-run for deployment manifests if changed.
- [ ] Manual smoke:
  - Create Lite OpenClaw.
  - Create Pro OpenClaw.
  - Access both through ClawManager.
  - Enable external access for both.
  - Verify AI Gateway attribution for Lite is instance/gateway-specific.
  - Verify admin can see/manage both modes.
