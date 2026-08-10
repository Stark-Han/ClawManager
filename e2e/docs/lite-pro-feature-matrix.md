# Lite / Pro Feature Matrix

This matrix is the source checklist for the Playwright coverage in
`tests/instances/lite-pro-modes.spec.ts`.

| Area | Lite | Pro | E2E coverage |
| --- | --- | --- | --- |
| Create wizard | Selectable `Lite` mode. Runtime backend is derived as `Gateway`. | Selectable `Pro` mode. Runtime backend is derived as `Desktop`. | `create wizard exposes the lite/pro mode contract` |
| Resource selection | Does not require or show dedicated CPU, memory, storage, or GPU sizing. Dedicated resource quota checks are not applied. | Shows dedicated resource presets and GPU toggle. CPU, memory, storage, and GPU quota checks are applied. | `create wizard exposes the lite/pro mode contract` |
| Runtime ownership | Uses the shared gateway runtime pool and keeps gateway-level identity separate. | Uses a dedicated desktop runtime backed by Kubernetes Deployment + Service. | `admin instances page surfaces mode and backend management controls` |
| Detail layout | Shows the service frame and workspace manager without desktop-only management panels. | Shows the service frame plus a merged Runtime Overview card, Runtime Events, Instance Skills, and workspace manager rooted at `/config`. | `lite detail keeps gateway workspace layout`, `pro detail keeps desktop management layout` |
| Channel / skill management | Supports OpenClaw injection, channel selection, skill injection, import, and export at create/manage time. | Supports OpenClaw injection, channel selection, skill injection, import, export, and instance skill attach/remove. | Create wizard assertions plus pro detail skill panel assertion |
| External access | Supports short Share Link and Password access with fixed, custom, or permanent expiration. | Supports short Share Link and Password access with fixed, custom, or permanent expiration. | `password share link opens a password gate` |
| Admin monitoring | Admin instance management can show/filter Lite records and identify them as `Gateway binding` / `Gateway`. | Admin instance management can show/filter Pro records and identify them as `Deployment` / `Desktop`. | `admin instances page surfaces mode and backend management controls` |
