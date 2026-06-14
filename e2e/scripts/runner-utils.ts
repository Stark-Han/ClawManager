import fs from "node:fs";
import path from "node:path";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { reportsDir, testResultsDir } from "../fixtures/env.js";

export type Suite = "p0" | "p1" | "p2" | "all";

export const DEBUG_SLOW_MO_MS = 500;

const isWindows = process.platform === "win32";
const truthyValues = new Set(["1", "true", "yes", "on"]);
const falsyValues = new Set(["0", "false", "no", "off"]);

export function defaultHeadlessFromEnv(environment: NodeJS.ProcessEnv = process.env) {
  return environment.CI === "true" || environment.E2E_CI === "true";
}

function parseBooleanFlag(flag: string, value: string) {
  const normalized = value.trim().toLowerCase();
  if (truthyValues.has(normalized)) {
    return true;
  }
  if (falsyValues.has(normalized)) {
    return false;
  }
  throw new Error(`${flag} must be true or false`);
}

export function parseHeadlessArgs(args: string[], defaultHeadless: boolean) {
  let headless = defaultHeadless;
  const remainingArgs: string[] = [];

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--headless") {
      const nextArg = args[index + 1];
      const nextValue = nextArg?.toLowerCase();
      if (nextValue && (truthyValues.has(nextValue) || falsyValues.has(nextValue))) {
        headless = parseBooleanFlag("--headless", nextArg);
        index += 1;
      } else {
        headless = true;
      }
      continue;
    }
    if (arg === "--headed" || arg === "--no-headless") {
      headless = false;
      continue;
    }
    if (arg.startsWith("--headless=")) {
      headless = parseBooleanFlag("--headless", arg.slice("--headless=".length));
      continue;
    }
    remainingArgs.push(arg);
  }

  return {
    headless,
    args: remainingArgs
  };
}

export function parseDebugArgs(args: string[], defaultDebug: boolean) {
  let debug = defaultDebug;
  const remainingArgs: string[] = [];

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--debug") {
      const nextArg = args[index + 1];
      const nextValue = nextArg?.toLowerCase();
      if (nextValue && (truthyValues.has(nextValue) || falsyValues.has(nextValue))) {
        debug = parseBooleanFlag("--debug", nextArg);
        index += 1;
      } else {
        debug = true;
      }
      continue;
    }
    if (arg === "--no-debug") {
      debug = false;
      continue;
    }
    if (arg.startsWith("--debug=")) {
      debug = parseBooleanFlag("--debug", arg.slice("--debug=".length));
      continue;
    }
    remainingArgs.push(arg);
  }

  return {
    debug,
    args: remainingArgs
  };
}

export function findFailureScreenshots(rootDir = testResultsDir) {
  if (!fs.existsSync(rootDir)) {
    return [];
  }

  const screenshots: string[] = [];
  const visit = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const entryPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        visit(entryPath);
      } else if (entry.isFile() && entry.name.toLowerCase().endsWith(".png")) {
        screenshots.push(entryPath);
      }
    }
  };

  visit(rootDir);
  return screenshots.sort((a, b) => a.localeCompare(b));
}

export function writeFailureScreenshotSummary(write: (line: string) => void) {
  const screenshots = findFailureScreenshots();
  if (screenshots.length === 0) {
    write(`[e2e] no failure screenshots found in ${testResultsDir}\n`);
    return;
  }

  write(`[e2e] failure screenshot evidence:\n`);
  for (const screenshot of screenshots) {
    write(`[e2e]   ${screenshot}\n`);
  }
}

function timestamp() {
  return new Date().toISOString();
}

export function createLogger(fileName = "run.log") {
  fs.mkdirSync(reportsDir, { recursive: true });
  const logPath = path.join(reportsDir, fileName);
  const stream = fs.createWriteStream(logPath, { flags: "w" });

  const write = (line: string) => {
    stream.write(line);
    process.stdout.write(line);
  };

  const log = (message: string) => {
    write(`[${timestamp()}] ${message}\n`);
  };

  return {
    logPath,
    log,
    write,
    close: () =>
      new Promise<void>((resolve) => {
        stream.end(resolve);
      })
  };
}

function pipeChild(
  child: ChildProcessWithoutNullStreams,
  label: string,
  write: (line: string) => void
) {
  child.stdout.on("data", (chunk: Buffer) => {
    write(chunk.toString().replace(/^/gm, `[${label}] `));
  });
  child.stderr.on("data", (chunk: Buffer) => {
    write(chunk.toString().replace(/^/gm, `[${label}] `));
  });
}

function quoteWindowsArg(value: string) {
  if (/^[A-Za-z0-9_./:=@-]+$/.test(value)) {
    return value;
  }
  return `"${value.replace(/"/g, '\\"')}"`;
}

function spawnPortable(
  command: string,
  args: string[],
  options: { cwd: string; env?: NodeJS.ProcessEnv }
): ChildProcessWithoutNullStreams {
  if (!isWindows) {
    return spawn(command, args, {
      cwd: options.cwd,
      env: options.env,
      windowsHide: true
    });
  }

  const commandLine = [command, ...args].map(quoteWindowsArg).join(" ");
  return spawn("cmd.exe", ["/d", "/s", "/c", commandLine], {
    cwd: options.cwd,
    env: options.env,
    windowsHide: true
  });
}

export function runCommand(
  command: string,
  args: string[],
  options: { cwd: string; env?: NodeJS.ProcessEnv; label: string },
  write: (line: string) => void
): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawnPortable(command, args, options);
    pipeChild(child, options.label, write);
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`${options.label} exited with code ${code}`));
      }
    });
  });
}

export function startService(
  command: string,
  args: string[],
  options: { cwd: string; env?: NodeJS.ProcessEnv; label: string },
  write: (line: string) => void
) {
  const child = spawnPortable(command, args, options);
  pipeChild(child, options.label, write);
  return child;
}

async function killWindowsProcessTree(pid: number) {
  await new Promise<void>((resolve) => {
    const child = spawn("taskkill.exe", ["/pid", String(pid), "/t", "/f"], {
      windowsHide: true
    });
    child.on("exit", () => resolve());
    child.on("error", () => resolve());
  });
}

export async function stopService(
  child: ChildProcessWithoutNullStreams | undefined,
  label: string
) {
  if (!child || child.killed) {
    return;
  }

  await new Promise<void>((resolve) => {
    child.once("exit", () => resolve());
    if (isWindows && child.pid) {
      void killWindowsProcessTree(child.pid).then(resolve);
    } else {
      child.kill();
    }
    setTimeout(() => {
      if (!child.killed) {
        child.kill("SIGKILL");
      }
      resolve();
    }, 5000).unref();
  });
  console.log(`[e2e] stopped ${label}`);
}

export function suiteArgs(suite: Suite) {
  switch (suite) {
    case "p0":
      return ["--grep", "@p0"];
    case "p1":
      return ["--grep", "@p1"];
    case "p2":
      return ["--grep", "@p2"];
    case "all":
      return [];
  }
}
