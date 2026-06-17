import path from "node:path";
import { pathToFileURL } from "node:url";
import type { ChildProcessWithoutNullStreams } from "node:child_process";
import { env, e2eRoot, repoRoot } from "../fixtures/env.js";
import { waitForHttpStatus, waitForTcpPort } from "./wait-for.js";
import {
  createLogger,
  DEBUG_SLOW_MO_MS,
  defaultHeadlessFromEnv,
  parseDebugArgs,
  parseHeadlessArgs,
  runCommand,
  startService,
  stopService,
  suiteArgs,
  type Suite,
  writeFailureScreenshotSummary
} from "./runner-utils.js";

const npmCmd = "npm";
const npxCmd = "npx";
const goCmd = "go";

function childEnv(extra: NodeJS.ProcessEnv = {}) {
  return {
    ...process.env,
    E2E_FRONTEND_URL: env.frontendUrl,
    E2E_BACKEND_URL: env.backendUrl,
    E2E_DB_HOST: env.db.host,
    E2E_DB_PORT: String(env.db.port),
    E2E_DB_USER: env.db.user,
    E2E_DB_PASSWORD: env.db.password,
    E2E_DB_NAME: env.db.database,
    E2E_RUNTIME_AGENT_TOKEN: env.runtimeAgentToken,
    E2E_WORKSPACE_ROOT: env.workspaceRoot,
    E2E_OBJECT_STORAGE_FALLBACK: env.objectStorageFallback,
    ...extra
  };
}

export function parseArgs(
  args: string[] = process.argv.slice(2),
  environment: NodeJS.ProcessEnv = process.env
): { suite: Suite; ci: boolean; headless: boolean; debug: boolean; slowMoMs: number; extraArgs: string[] } {
  const [suiteArg = "p0", ...rest] = args;
  if (!["p0", "p1", "p2", "all"].includes(suiteArg)) {
    throw new Error(`unknown e2e suite: ${suiteArg}`);
  }
  const ci = rest.includes("--ci");
  const filteredArgs = rest.filter((value) => value !== "--ci" && value !== "--");
  const parsedHeadless = parseHeadlessArgs(
    filteredArgs,
    ci || defaultHeadlessFromEnv(environment)
  );
  const parsedDebug = parseDebugArgs(parsedHeadless.args, false);
  return {
    suite: suiteArg as Suite,
    ci,
    headless: parsedHeadless.headless,
    debug: parsedDebug.debug,
    slowMoMs: parsedDebug.debug ? DEBUG_SLOW_MO_MS : 0,
    extraArgs: parsedDebug.args
  };
}

function frontendHostAndPort() {
  const url = new URL(env.frontendUrl);
  return {
    host: url.hostname,
    port: url.port || (url.protocol === "https:" ? "443" : "80")
  };
}

async function main() {
  const { suite, ci, headless, debug, slowMoMs, extraArgs } = parseArgs();
  const logger = createLogger();
  let backend: ChildProcessWithoutNullStreams | undefined;
  let frontend: ChildProcessWithoutNullStreams | undefined;

  try {
    logger.log(`starting e2e suite=${suite} ci=${ci} headless=${headless} debug=${debug} slowMoMs=${slowMoMs}`);
    logger.log(`run log: ${logger.logPath}`);

    logger.log("starting MySQL");
    await runCommand(
      "docker",
      ["compose", "-f", "docker/docker-compose.e2e.yml", "up", "-d", "mysql"],
      { cwd: e2eRoot, label: "docker" },
      logger.write
    );
    await waitForTcpPort(env.db.host, env.db.port, 120_000);

    logger.log("resetting e2e database");
    await runCommand(npxCmd, ["tsx", "scripts/reset-db.ts"], { cwd: e2eRoot, label: "db" }, logger.write);

    logger.log("starting backend");
    backend = startService(
      goCmd,
      ["run", "./cmd/server"],
      {
        cwd: path.join(repoRoot, "backend"),
        label: "backend",
        env: childEnv({
          SERVER_ADDRESS: ":9001",
          DB_HOST: env.db.host,
          DB_PORT: String(env.db.port),
          DB_USER: env.db.user,
          DB_PASSWORD: env.db.password,
          DB_NAME: env.db.database,
          JWT_SECRET: "clawmanager-e2e-secret",
          RUNTIME_SCHEDULER_ENABLED: "false",
          RUNTIME_AGENT_REPORT_TOKEN: env.runtimeAgentToken,
          RUNTIME_WORKSPACE_ROOT: env.workspaceRoot,
          OBJECT_STORAGE_LOCAL_FALLBACK: env.objectStorageFallback,
          SKILL_SCANNER_ENABLED: "false"
        })
      },
      logger.write
    );
    await waitForTcpPort("127.0.0.1", 9001, 120_000);

    logger.log("starting frontend");
    const frontendTarget = frontendHostAndPort();
    frontend = startService(
      npmCmd,
      ["run", "dev", "--", "--host", frontendTarget.host, "--port", frontendTarget.port, "--strictPort"],
      { cwd: path.join(repoRoot, "frontend"), label: "frontend", env: childEnv() },
      logger.write
    );
    await waitForHttpStatus(env.frontendUrl, 200, 120_000);

    logger.log("running Playwright");
    const args = ["playwright", "test", ...suiteArgs(suite)];
    if (ci && !extraArgs.some((value) => value.startsWith("--workers"))) {
      args.push("--workers=1");
    }
    args.push(...extraArgs);
    try {
      await runCommand(
        npxCmd,
        args,
        {
          cwd: e2eRoot,
          label: "playwright",
          env: childEnv({
            CI: ci ? "true" : process.env.CI,
            E2E_HEADLESS: String(headless),
            E2E_DEBUG: String(debug),
            E2E_SLOW_MO_MS: String(slowMoMs)
          })
        },
        logger.write
      );
    } catch (error) {
      writeFailureScreenshotSummary(logger.write);
      throw error;
    }

    logger.log("e2e run completed successfully");
  } finally {
    logger.log("cleaning up e2e services");
    await stopService(frontend, "frontend");
    await stopService(backend, "backend");
    await runCommand(
      "docker",
      ["compose", "-f", "docker/docker-compose.e2e.yml", "down", "--remove-orphans"],
      { cwd: e2eRoot, label: "docker" },
      logger.write
    ).catch((error: unknown) => {
      logger.log(`docker cleanup failed: ${error instanceof Error ? error.message : String(error)}`);
    });
    await logger.close();
  }
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  main().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  });
}
