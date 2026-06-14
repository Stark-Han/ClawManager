import path from "node:path";
import { fileURLToPath } from "node:url";

const fixturesDir = path.dirname(fileURLToPath(import.meta.url));
export const e2eRoot = path.resolve(fixturesDir, "..");
export const repoRoot = path.resolve(e2eRoot, "..");
export const reportsDir = path.join(e2eRoot, "reports");
export const testResultsDir = path.join(e2eRoot, "test-results");

function readEnv(name: string, fallback: string): string {
  const value = process.env[name]?.trim();
  return value && value.length > 0 ? value : fallback;
}

function readNumberEnv(name: string, fallback: number): number {
  const raw = process.env[name]?.trim();
  if (!raw) {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed)) {
    throw new Error(`${name} must be a number`);
  }
  return parsed;
}

function readBooleanEnv(name: string, fallback: boolean): boolean {
  const raw = process.env[name]?.trim();
  if (!raw) {
    return fallback;
  }
  return ["1", "true", "yes", "on"].includes(raw.toLowerCase());
}

function withoutTrailingSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

const isCI = process.env.CI === "true" || process.env.E2E_CI === "true";
const debug = readBooleanEnv("E2E_DEBUG", false);

export const env = {
  frontendUrl: withoutTrailingSlash(readEnv("E2E_FRONTEND_URL", "http://127.0.0.1:9002")),
  backendUrl: withoutTrailingSlash(readEnv("E2E_BACKEND_URL", "http://127.0.0.1:9001/api/v1")),
  db: {
    host: readEnv("E2E_DB_HOST", "127.0.0.1"),
    port: readNumberEnv("E2E_DB_PORT", 13307),
    user: readEnv("E2E_DB_USER", "root"),
    password: readEnv("E2E_DB_PASSWORD", "123456"),
    database: readEnv("E2E_DB_NAME", "clawmanager_e2e")
  },
  runtimeAgentToken: readEnv("E2E_RUNTIME_AGENT_TOKEN", "e2e-runtime-agent-token"),
  workspaceRoot: readEnv("E2E_WORKSPACE_ROOT", path.join(repoRoot, ".cache", "e2e", "workspaces")),
  objectStorageFallback: readEnv(
    "E2E_OBJECT_STORAGE_FALLBACK",
    path.join(repoRoot, ".cache", "e2e", "object-storage")
  ),
  ignoreHttpsErrors: readBooleanEnv("E2E_IGNORE_HTTPS_ERRORS", false),
  headless: readBooleanEnv("E2E_HEADLESS", isCI),
  debug,
  slowMoMs: debug ? readNumberEnv("E2E_SLOW_MO_MS", 500) : 0,
  remoteTarget: readBooleanEnv("E2E_REMOTE_TARGET", false),
  isCI
} as const;
