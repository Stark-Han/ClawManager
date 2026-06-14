import path from "node:path";
import { defineConfig, devices } from "@playwright/test";
import { env, e2eRoot, reportsDir } from "./fixtures/env.js";

const setupProjects = [
  {
    name: "auth setup",
    testMatch: /setup\/auth\.setup\.ts/
  },
  ...(
    env.remoteTarget
      ? []
      : [
          {
            name: "db setup",
            testMatch: /setup\/db\.setup\.ts/
          }
        ]
  )
];

export default defineConfig({
  testDir: ".",
  outputDir: "test-results",
  timeout: env.remoteTarget ? 90_000 : 30_000,
  expect: {
    timeout: env.remoteTarget ? 45_000 : 10_000
  },
  fullyParallel: false,
  retries: env.isCI ? 1 : 0,
  workers: env.isCI ? 1 : undefined,
  reporter: [
    ["list"],
    ["html", { outputFolder: "playwright-report", open: "never" }],
    ["json", { outputFile: path.join(reportsDir, "results.json") }],
    ["junit", { outputFile: path.join(reportsDir, "junit.xml") }]
  ],
  use: {
    baseURL: env.frontendUrl,
    headless: env.headless,
    ignoreHTTPSErrors: env.ignoreHttpsErrors,
    launchOptions: env.debug ? { slowMo: env.slowMoMs } : undefined,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure"
  },
  projects: [
    ...setupProjects,
    {
      name: "chromium",
      testMatch: /tests\/.*\.spec\.ts/,
      dependencies: env.remoteTarget ? ["auth setup"] : ["auth setup", "db setup"],
      use: {
        ...devices["Desktop Chrome"]
      }
    }
  ],
  metadata: {
    e2eRoot,
    frontendUrl: env.frontendUrl,
    backendUrl: env.backendUrl
  }
});
