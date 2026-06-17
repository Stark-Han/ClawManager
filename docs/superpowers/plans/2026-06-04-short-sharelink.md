# Short Share Link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace exposed public instance URLs with `/s/<code>/` short links and add fixed/custom/permanent expiration choices.

**Architecture:** Public links become capability URLs: the plaintext short code is returned only at creation time and only its SHA-256 hash is stored. The backend resolves `/s/:code/*path` to an enabled external access record, rewrites the request to the internal instance proxy path, and proxies the request. The old `/api/v1/public/instances/:slug/proxy` route is removed.

**Tech Stack:** Go/Gin backend, MySQL migrations, React/Vite frontend, existing Kubernetes image build and rollout flow.

---

### Task 1: Backend Short Link Storage and Service Behavior

**Files:**
- Modify: `backend/internal/models/instance_external_access.go`
- Modify: `backend/internal/repository/instance_external_access_repository.go`
- Modify: `backend/internal/services/instance_external_access_service.go`
- Modify: `backend/internal/services/instance_external_access_service_test.go`
- Create: `backend/internal/db/migrations/029_add_short_external_access_links.sql`

- [ ] Write tests that public links return `/s/<code>/`, do not return `token=`, and persist only a code hash.
- [ ] Write tests for preset, custom, and permanent expiration resolution.
- [ ] Add `short_code_hash` storage and lookup repository methods.
- [ ] Generate random short codes, hash them, and return only the short URL.
- [ ] Keep API key mode available, but do not make Public Link depend on a separate query token.

### Task 2: Backend Short Link Routing

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/handlers/instance_handler.go`
- Modify: `backend/internal/handlers/instance_handler_test.go`
- Modify: `deployments/nginx/nginx.conf`

- [ ] Write handler tests proving `/s/<code>/` and `/s/<code>/assets/app.js` resolve without query tokens.
- [ ] Remove old `/api/v1/public/instances/:slug...` public routes.
- [ ] Add root-level `/s/:code` and `/s/:code/*path` routes.
- [ ] Rewrite short-link request paths to `/api/v1/instances/:id/proxy/...` before using the existing proxy service.
- [ ] Add an nginx `/s/` proxy location so the SPA does not consume short links.

### Task 3: Frontend Expiration UI

**Files:**
- Modify: `frontend/src/types/instance.ts`
- Modify: `frontend/src/services/instanceService.ts`
- Modify: `frontend/src/pages/instances/InstanceDetailPage.tsx`

- [ ] Add request types for `preset`, `custom`, and `permanent` expiration modes.
- [ ] Show fixed choices: 1 hour, 24 hours, 7 days, 30 days, custom, permanent.
- [ ] Default Public Link creation to 24 hours.
- [ ] Send expiration selection to `POST /instances/:id/external-access/public-link`.
- [ ] Display the returned short URL and expiration state.

### Task 4: Verification and Deployment

**Files:**
- No source-only changes beyond Tasks 1-3.

- [ ] Run targeted backend tests for external access and handler routing.
- [ ] Run `go test ./... -count=1`.
- [ ] Run frontend lint/build.
- [ ] Build and push image to `172.16.1.12:5010`.
- [ ] Roll out `clawmanager-app`.
- [ ] Verify on the real cluster that generated links use `/s/<code>/`, the long public route is not registered, and `/s/<code>/assets/...` no longer returns 401.
