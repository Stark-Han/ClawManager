import { pathToFileURL } from "node:url";
import { e2eRoot } from "../fixtures/env.js";
import {
  createLogger,
  DEBUG_SLOW_MO_MS,
  defaultHeadlessFromEnv,
  parseDebugArgs,
  parseHeadlessArgs,
  runCommand,
  suiteArgs,
  writeFailureScreenshotSummary
} from "./runner-utils.js";

export interface TargetRunOptions {
  frontendUrl: string;
  backendUrl: string;
  headless: boolean;
  debug: boolean;
  slowMoMs: number;
  playwrightArgs: string[];
  env: NodeJS.ProcessEnv;
}

function withoutTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}

function normalizeTargetAddress(rawAddress: string) {
  const trimmed = rawAddress.trim();
  if (!trimmed) {
    throw new Error("Usage: npm run target -- <clawmanager-address>");
  }

  const withProtocol = /^[a-z][a-z\d+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
  const url = new URL(withProtocol);
  url.hash = "";
  url.search = "";
  return withoutTrailingSlash(url.toString());
}

export function buildTargetRunOptions(
  args: string[] = process.argv.slice(2),
  environment: NodeJS.ProcessEnv = process.env
): TargetRunOptions {
  const parsedHeadless = parseHeadlessArgs(args, defaultHeadlessFromEnv(environment));
  const parsedDebug = parseDebugArgs(parsedHeadless.args, false);
  const remainingArgs = parsedDebug.args;

  if (remainingArgs.length === 0) {
    throw new Error("Usage: npm run target -- <clawmanager-address>");
  }
  if (remainingArgs.length > 1) {
    throw new Error("target runner only accepts one address plus optional --headless/--headed/--debug");
  }

  const frontendUrl = normalizeTargetAddress(remainingArgs[0]);
  const backendUrl = `${frontendUrl}/api/v1`;
  const slowMoMs = parsedDebug.debug ? DEBUG_SLOW_MO_MS : 0;

  return {
    frontendUrl,
    backendUrl,
    headless: parsedHeadless.headless,
    debug: parsedDebug.debug,
    slowMoMs,
    playwrightArgs: ["playwright", "test", ...suiteArgs("p0"), "--grep-invert", "@local-only", "--workers=1"],
    env: {
      ...environment,
      E2E_FRONTEND_URL: frontendUrl,
      E2E_BACKEND_URL: backendUrl,
      E2E_CI: "true",
      E2E_HEADLESS: String(parsedHeadless.headless),
      E2E_DEBUG: String(parsedDebug.debug),
      E2E_SLOW_MO_MS: String(slowMoMs),
      E2E_IGNORE_HTTPS_ERRORS: "true",
      E2E_REMOTE_TARGET: "true"
    }
  };
}

async function main() {
  const options = buildTargetRunOptions();
  const logger = createLogger("target-run.log");

  try {
    logger.log(`target frontend: ${options.frontendUrl}`);
    logger.log(`target backend: ${options.backendUrl}`);
    logger.log(`target headless: ${options.headless}`);
    logger.log(`target debug: ${options.debug} slowMoMs=${options.slowMoMs}`);
    logger.log("running remote Playwright P0 suite");
    try {
      await runCommand(
        "npx",
        options.playwrightArgs,
        {
          cwd: e2eRoot,
          label: "playwright",
          env: options.env
        },
        logger.write
      );
    } catch (error) {
      writeFailureScreenshotSummary(logger.write);
      throw error;
    }
    logger.log("target e2e run completed successfully");
  } finally {
    await logger.close();
  }
}

if (process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url) {
  main().catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  });
}
