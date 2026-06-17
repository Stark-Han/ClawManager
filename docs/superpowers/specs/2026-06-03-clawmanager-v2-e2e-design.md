# ClawManager V2 E2E Test Design

## Overview

ClawManager V2 needs an end-to-end test mechanism that covers user-facing flows across the React frontend, Go backend, MySQL migrations, and selected V2 runtime-pool APIs. The test suite should live in a root-level `e2e/` directory because it validates cross-service behavior rather than frontend-only behavior.

The initial implementation should use Playwright. It fits the current Vite frontend plus Go backend setup, supports multiple local web servers, can persist authenticated browser state, and produces CI-friendly traces, screenshots, videos, and HTML reports.

## Language And Tooling

E2E test code should be written in TypeScript.

- Test specs use Playwright Test with `.spec.ts` files.
- Fixtures, page objects, setup files, API helpers, and environment helpers use `.ts`.
- Optional command wrappers can use PowerShell (`.ps1`) and shell (`.sh`) so Windows and CI/Linux users have clear entry points.
- JavaScript should not be used for hand-written e2e source files unless a tool generates it.

TypeScript is the preferred language because the frontend already uses TypeScript, Playwright has strong TypeScript support, and typed fixtures/page objects reduce test maintenance risk as V2 flows grow.

## Goals

- Add a dedicated root-level `e2e/` test directory.
- Support local developer execution with an isolated database.
- Support CI execution with stable smoke tests.
- Cover login, route protection, core user navigation, admin access control, workspace file flows, and V2 runtime-pool API surfaces.
- Provide readable run logs, clear progress output, structured test results, and a final human-readable test report.
- Integrate the P0 e2e suite into GitHub pull request checks.
- Support manual local execution for P0, P1, P2, and the full suite.
- Keep the first version independent of a real Kubernetes cluster or real runtime agent.
- Leave room for a later full-stack k8s e2e mode that targets the existing kind smoke deployment.

## Non-Goals

- Do not replace backend Go unit or integration tests.
- Do not move frontend tests into `e2e/`.
- Do not require a real OpenClaw or Hermes runtime Pod for the initial suite.
- Do not make heavy k8s browser tests block every pull request in the first iteration.

## Recommended Directory Structure

```text
e2e/
  package.json
  package-lock.json
  playwright.config.ts
  README.md
  .gitignore

  docker/
    docker-compose.e2e.yml

  scripts/
    reset-db.ts
    wait-for.ts
    run-with-log.ts
    e2e-up.ps1
    e2e-down.ps1
    e2e-up.sh
    e2e-down.sh

  setup/
    auth.setup.ts
    db.setup.ts

  fixtures/
    test.ts
    apiClient.ts
    users.ts
    env.ts

  pages/
    LoginPage.ts
    DashboardPage.ts
    InstanceListPage.ts
    InstanceDetailPage.ts
    AdminRuntimePodsPage.ts

  tests/
    smoke/
      login.spec.ts
      navigation.spec.ts
    user/
      dashboard.spec.ts
      instances.spec.ts
      workspace-files.spec.ts
    admin/
      runtime-pods.spec.ts
      access-control.spec.ts
    api/
      auth.spec.ts
      runtime-agent.spec.ts

  reports/
    .gitkeep
```

## Responsibilities

`playwright.config.ts` defines browser projects, base URLs, reporters, traces, retries, timeouts, and the local web server orchestration.

`docker/docker-compose.e2e.yml` starts only test dependencies that should be isolated from a developer machine, beginning with MySQL on a non-default host port such as `13307`.

`scripts/` contains explicit helper commands for resetting the e2e database, waiting for ports or health endpoints, and starting or stopping local dependencies on Windows and Unix-like systems.

`scripts/run-with-log.ts` wraps Playwright execution when needed so console output is also written to `e2e/reports/run.log`.

`setup/` contains Playwright project dependencies. `db.setup.ts` prepares deterministic data. `auth.setup.ts` logs in as seeded users and writes browser storage state files for later specs.

`fixtures/` contains typed shared utilities: environment parsing, API request helpers, seeded user constants, and a custom Playwright `test` export.

`pages/` contains page objects for stable selectors and common user actions. Page objects should stay small and focused on navigation and interactions, not assertions-heavy business logic.

`tests/` groups specs by product surface rather than by technical layer. Smoke tests are intentionally tiny and run first in CI.

`reports/` stores generated run logs and machine-readable report files. Generated report artifacts are ignored by git except for a `.gitkeep` tracked marker.

## Reuse And Common Module Guidelines

E2E implementation must prefer shared modules over repeated setup code. Repeated steps are expected across login, role switching, API calls, database setup, runtime pod seeding, navigation, and reporting, so the first implementation batch must establish reusable foundations before adding many specs.

Required shared modules:

- `fixtures/test.ts` exports the shared Playwright `test` and `expect`, wires common fixtures, and centralizes base URL, storage state, and artifact behavior.
- `fixtures/env.ts` owns environment parsing for frontend URL, backend URL, database connection, runtime agent token, workspace root, and CI flags.
- `fixtures/apiClient.ts` owns typed request helpers for auth, runtime-agent, admin runtime Pods, and authenticated API calls.
- `fixtures/users.ts` owns seeded user constants such as `admin/admin123` and any generated non-admin test user data.
- `setup/auth.setup.ts` performs reusable login setup and writes storage state files instead of logging in manually in every UI spec.
- `setup/db.setup.ts` prepares deterministic database and workspace state for specs that need seeded V2 runtime data.
- `pages/*.ts` page objects encapsulate repeated UI actions and selectors. Tests should describe user intent; page objects should hold selector details.
- `scripts/*.ts` own lifecycle concerns such as database reset, server readiness, process orchestration, logging, and report paths.

Reuse rules:

- Do not duplicate login steps inside every spec when a storage state fixture can be used.
- Do not hard-code backend or frontend URLs inside specs; read them from `fixtures/env.ts`.
- Do not repeat raw runtime-agent request construction in multiple tests; use `apiClient.ts`.
- Do not repeat selectors across specs; move repeated selectors and actions into page objects.
- Do not add test-only helpers to production frontend or backend code for e2e convenience.
- Keep page objects small: one page object should cover one page or stable product surface.
- Keep shared helpers behavior-focused. Avoid large catch-all utility files.
- When a repeated sequence appears in two specs, extract it before adding a third copy.

## Runtime Model

The initial local flow should start:

1. MySQL through `e2e/docker/docker-compose.e2e.yml`.
2. Go backend from `backend/cmd/server`, with environment overrides:
   - `SERVER_ADDRESS=:9001`
   - `DB_HOST=127.0.0.1`
   - `DB_PORT=13307`
   - `DB_USER=root`
   - `DB_PASSWORD=123456`
   - `DB_NAME=clawmanager_e2e`
   - `JWT_SECRET=clawmanager-e2e-secret`
   - `RUNTIME_SCHEDULER_ENABLED=false`
   - `RUNTIME_WORKSPACE_ROOT=<repo>/.cache/e2e/workspaces`
   - `OBJECT_STORAGE_LOCAL_FALLBACK=<repo>/.cache/e2e/object-storage`
   - `SKILL_SCANNER_ENABLED=false`
3. Vite frontend from `frontend`, using the existing `9002` dev port and `/api` proxy to backend `9001`.
4. Playwright tests against `http://127.0.0.1:9002`.

The backend already applies embedded migrations during startup, so e2e does not need a separate migration runner. Database reset can drop and recreate `clawmanager_e2e` before backend startup.

## Run Logging, Progress, And Reports

E2E execution must make progress and results easy to inspect during and after a run.

Console progress:

- Use Playwright's list-style console reporter for local runs so each test is visible as it starts and finishes.
- Test titles should include the priority tag, such as `@p0`, `@p1`, or `@p2`.
- Long tests should use `test.step(...)` for meaningful milestones such as login, seed data creation, navigation, API call, and assertion phases.
- Setup scripts should print clear lifecycle messages for database reset, backend startup, frontend startup, auth setup, and report location.

Run log:

- Every standard script should write a timestamped log to `e2e/reports/run.log`.
- CI should also retain raw job logs, but `run.log` is the repository-level e2e artifact.
- Logs must not print access tokens, refresh tokens, passwords, or full authorization headers.
- On failure, logs should include the failing test name, retry count, browser project, and the Playwright artifact path.

Report artifacts:

- Human-readable HTML report: `e2e/playwright-report/`.
- Raw per-test artifacts: `e2e/test-results/`.
- Machine-readable JSON report: `e2e/reports/results.json`.
- CI-friendly JUnit report: `e2e/reports/junit.xml`.
- Text run log: `e2e/reports/run.log`.

Recommended package scripts:

```json
{
  "scripts": {
    "test:p0": "playwright test --grep @p0",
    "test:p1": "playwright test --grep @p1",
    "test:p2": "playwright test --grep @p2",
    "test:all": "playwright test",
    "ci:p0": "playwright test --grep @p0 --workers=1",
    "test:report": "playwright show-report playwright-report"
  }
}
```

`playwright.config.ts` should configure at least these reporters:

- `list` for clear live progress.
- `html` for the final local/CI report.
- `json` written to `e2e/reports/results.json`.
- `junit` written to `e2e/reports/junit.xml`.

CI must upload `e2e/playwright-report/`, `e2e/test-results/`, and `e2e/reports/` whenever e2e runs fail. Local users can open the final report with `npm run test:report`.

## Test Data Strategy

Use seeded migration data where it is stable. The default admin account is:

```text
username: admin
password: admin123
```

Additional test data should be created through APIs when possible. For flows that require a specific V2 state not naturally reachable without k8s, `db.setup.ts` may insert deterministic rows directly after migrations have run. Direct database setup is acceptable for initial e2e because the target behavior is browser and API behavior, not migration correctness.

Each spec should use unique names with a run prefix when it creates mutable data. Cleanup should happen through API calls where practical, with database reset reserved for suite setup.

## Test Case Inventory And Priority

The suite should be planned from a full test inventory, then implemented in priority batches. Each batch must be developed, run, fixed, and reviewed before starting the next batch. P0 is the first implementation batch and acts as the required core gate.

### P0: Core Gate

P0 proves the product can boot, authenticate, protect routes, and expose the most critical V2 surfaces without requiring a real k8s cluster or runtime agent.

- `smoke/login.spec.ts`
  - Login page renders.
  - Admin can log in with `admin/admin123`.
  - Login failure shows a user-visible error and stays on `/login`.
- `smoke/navigation.spec.ts`
  - Unauthenticated users are redirected to `/login`.
  - Authenticated admin reaches `/dashboard`.
  - Main authenticated navigation can reach instances and settings/admin entry points that are visible for the logged-in role.
- `admin/access-control.spec.ts`
  - Non-admin users cannot open admin routes.
  - Admin users can open the admin runtime Pods route.
- `api/auth.spec.ts`
  - `/api/v1/auth/login` returns access and refresh tokens.
  - `/api/v1/auth/me` returns the current user when called with a valid token.
  - `/api/v1/auth/me` rejects unauthenticated requests.
- `api/runtime-agent.spec.ts`
  - Runtime-agent register persists an `openclaw` runtime pod.
  - Runtime-agent heartbeat updates pod health state.
  - Invalid runtime-agent payloads are rejected with a non-2xx response.
- `admin/runtime-pods.spec.ts`
  - Admin runtime Pods page renders an empty state.
  - Admin runtime Pods page renders a seeded pod row with status and capacity fields.

P0 completion criteria:

- P0 passes locally on Chromium.
- P0 passes in CI with one worker.
- Console progress, `e2e/reports/run.log`, HTML report, JSON report, JUnit report, traces, and screenshots are produced as configured.
- Playwright trace, report, and log artifacts are uploaded on CI failure.
- No P1 or P2 tests are required for the first e2e gate.

### P1: V2 User Workflows

P1 covers the main user paths most likely to catch frontend/backend integration regressions after P0 is stable.

- `user/dashboard.spec.ts`
  - Dashboard loads quota and instance summary.
  - Dashboard handles zero instances.
  - Dashboard links to create instance and instance list.
- `user/instances.spec.ts`
  - Instance list handles empty state.
  - Instance list renders seeded V2 `openclaw` and `hermes` instances.
  - Instance detail shows the simplified V2 layout for a seeded V2 instance.
  - Instance detail shows actionable unavailable state for a stopped or creating instance.
- `user/workspace-files.spec.ts`
  - Workspace file manager lists the seeded workspace root.
  - User can create a folder.
  - User can upload a small text file.
  - User can preview the uploaded file.
  - User can download the uploaded file.
  - User can rename a file or folder.
  - User can delete a file or folder.
  - Path traversal attempts are rejected through the UI/API path.
- `api/runtime-agent.spec.ts`
  - Runtime-agent metrics report updates pod CPU, memory, disk, and network fields.
  - Runtime-agent gateways report updates gateway rows for admin runtime view.

P1 completion criteria:

- P0 and P1 pass locally.
- P0 remains the required PR gate.
- P1 produces the same log and report artifacts as P0.
- P1 can run in CI as required or manual depending on runtime and database timing stability.

### P2: Expanded Operations And Full-Stack Confidence

P2 validates broader operational flows and should be introduced only after P0 and P1 are reliable.

- `admin/runtime-pods.spec.ts`
  - Admin runtime Pods page renders seeded gateway rows.
  - Runtime pod drain action calls the expected API and reflects the response.
  - Runtime rollout action can be submitted with a test image and shows rollout feedback.
- `user/instances.spec.ts`
  - Create instance form validates required fields.
  - Create instance submits a minimal V2 instance request.
  - Start, stop, restart, and sync actions show expected UI transitions for seeded or mocked backend state.
- `admin/access-control.spec.ts`
  - User-scoped instance list excludes another user's seeded instances.
  - Admin instance list includes cross-user seeded instances.
- `k8s/smoke.spec.ts`
  - Browser can load the app through `https://127.0.0.1:8443` after kind deployment.
  - Admin can log in through the deployed nginx endpoint.
  - Health page and frontend route fallback work through the deployed endpoint.

P2 completion criteria:

- P0 and P1 remain stable.
- P2 can run manually or on a schedule.
- P2 produces the same log and report artifacts as P0.
- k8s browser tests are not required for every pull request until deployment timing and runtime image availability are proven stable.

## Local Manual Runs

Developers must be able to run e2e locally without GitHub.

First-time setup:

```bash
cd e2e
npm ci
npx playwright install chromium
```

Standard local commands:

```bash
cd e2e
npm run test:p0
npm run test:p1
npm run test:p2
npm run test:all
npm run test:report
```

Windows helpers should provide equivalent entry points:

```powershell
.\scripts\e2e-up.ps1
npm run test:p0
npm run test:report
.\scripts\e2e-down.ps1
```

Unix-like helpers should provide equivalent entry points:

```bash
./scripts/e2e-up.sh
npm run test:p0
npm run test:report
./scripts/e2e-down.sh
```

Local runs should produce the same log and report outputs as CI. The local default is P0. P1, P2, and `test:all` are explicit commands so longer tests are intentional.

## GitHub Actions Integration

Add an e2e PR check to GitHub Actions. The recommended implementation is to add an `e2e-p0` job to the existing `.github/workflows/build.yml` workflow so it appears alongside the existing frontend, backend, Docker, and k8s smoke checks on pull requests.

The PR check requirements are:

- Trigger on `pull_request`.
- Also run on pushes to the same protected branch patterns as the existing build workflow.
- Depend on successful `frontend` and `backend` jobs.
- Run only the P0 suite initially.
- Use Chromium with one worker for stability.
- Upload logs and reports on failure.
- Fail the PR check when P0 fails.

The first GitHub PR command should be:

Recommended CI commands:

```bash
cd e2e
npm ci
npx playwright install --with-deps chromium
npm run ci:p0
```

P1 can be added to CI after it is stable locally. P2 should begin as manual or scheduled CI. The e2e job should not run the full k8s browser suite initially. A later workflow can reuse the existing kind deployment smoke path and then run a small Playwright suite against `https://127.0.0.1:8443`.

GitHub manual runs should also be supported. The existing build workflow can add `workflow_dispatch` with an `e2e_suite` input:

```text
p0 | p1 | p2 | all
```

The manual run should map the input to:

- `p0`: `npm run ci:p0`
- `p1`: `npm run test:p1 -- --workers=1`
- `p2`: `npm run test:p2 -- --workers=1`
- `all`: `npm run test:all -- --workers=1`

Artifact upload must include:

- `e2e/playwright-report/`
- `e2e/test-results/`
- `e2e/reports/run.log`
- `e2e/reports/results.json`
- `e2e/reports/junit.xml`

The GitHub summary should include the suite name, pass/fail status, total test count, failed test names, and a pointer to the uploaded HTML report artifact.

## Phased Implementation

Implementation must proceed batch by batch. A later batch should not start until the current batch is implemented, verified, and reviewed.

Batch 0 prepares the harness:

- Create the root `e2e/` package.
- Add Playwright config, isolated MySQL compose file, setup projects, fixtures, scripts, and README.
- Add local manual run commands and helper scripts.
- Add GitHub artifact wiring for logs and reports.
- Verify that the empty harness can start MySQL, backend, frontend, and one minimal smoke test.

Batch 1 implements P0:

- Add P0 smoke, auth API, access-control, runtime-agent, and admin runtime Pods tests.
- Add an `e2e-p0` GitHub Actions PR check to the existing build workflow.
- Make `npm run ci:p0` the first GitHub PR e2e command.
- Fix selectors, setup data, and server orchestration until P0 passes locally and in CI.
- Stop after P0 and review results before adding P1.

Batch 2 implements P1:

- Add dashboard, instance list/detail, workspace file, and runtime metrics/gateway reporting tests.
- Add any deterministic V2 seed helpers required by those specs.
- Keep P0 as the required PR gate until P1 has passed repeatedly.
- Review P1 stability before making it required in CI.

Batch 3 implements P2:

- Add expanded admin operations, instance action, cross-user visibility, and k8s deployed smoke tests.
- Keep k8s browser tests manual or scheduled first.
- Promote P2 subsets into PR CI only after timing and runtime image stability are proven.

## Acceptance Criteria

- `e2e/` exists at the repository root and owns all browser e2e test assets.
- Developers can run the suite without modifying local frontend or backend configuration files.
- Developers can manually run P0, P1, P2, or all e2e tests locally from `e2e/`.
- The suite uses an isolated database and workspace cache path.
- GitHub Actions runs the P0 e2e suite as a pull request check.
- GitHub Actions can also run e2e manually through `workflow_dispatch`.
- CI can run the P0 suite and upload failure artifacts.
- E2E source files are written in TypeScript.
- E2E runs show readable live progress in the terminal.
- E2E runs generate `e2e/reports/run.log`.
- E2E runs generate a final HTML report in `e2e/playwright-report/`.
- E2E runs generate JSON and JUnit result files for CI consumption.
- Repeated login, API, setup, navigation, and runtime-agent operations are implemented through shared fixtures, helpers, setup files, or page objects.
- Specs do not duplicate URLs, credentials, selectors, or raw runtime-agent payload construction when a shared module exists.
- Tests are grouped by product surface, assigned P0/P1/P2 priority, and can be expanded without changing the directory model.
- The full necessary test inventory is documented before implementation begins.
- Each implementation batch is verified before the next batch starts.
- Initial tests do not require a real k8s cluster or runtime agent.
