# ClawManager E2E

This directory contains Playwright-based TypeScript e2e tests.

## First-Time Setup

```bash
npm ci
npx playwright install chromium
```

## Run Locally

```bash
npm run test:p0
npm run test:report
```

The default local gate is P0. P1, P2, and all tests are explicit:

```bash
npm run test:p1
npm run test:p2
npm run test:all
```

## Run Against A Target Environment

Pass only the ClawManager address. The runner defaults to the P0 suite, derives
the backend API as `/api/v1`, runs serially, skips local-only data setup, and
writes a dedicated target log.

```bash
npm run target -- 172.16.1.12:39443
```

The address may include a protocol. When omitted, `https://` is used.

## Reports

- Live progress appears in the terminal.
- Local run log: `reports/run.log`
- Target run log: `reports/target-run.log`
- JSON: `reports/results.json`
- JUnit: `reports/junit.xml`
- HTML: `playwright-report/`
- Raw artifacts: `test-results/`
