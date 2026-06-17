import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import assert from "node:assert/strict";
import { findFailureScreenshots, parseDebugArgs, parseHeadlessArgs } from "./runner-utils.js";

assert.deepEqual(parseHeadlessArgs([], false), {
  headless: false,
  args: []
});

assert.deepEqual(parseHeadlessArgs([], true), {
  headless: true,
  args: []
});

assert.deepEqual(parseHeadlessArgs(["--headless", "--workers=1"], false), {
  headless: true,
  args: ["--workers=1"]
});

assert.deepEqual(parseHeadlessArgs(["--headless=false", "--workers=1"], true), {
  headless: false,
  args: ["--workers=1"]
});

assert.deepEqual(parseHeadlessArgs(["--headless", "false", "--workers=1"], true), {
  headless: false,
  args: ["--workers=1"]
});

assert.deepEqual(parseHeadlessArgs(["--headed", "--workers=1"], true), {
  headless: false,
  args: ["--workers=1"]
});

assert.throws(() => parseHeadlessArgs(["--headless=maybe"], false), /headless/);

assert.deepEqual(parseDebugArgs([], false), {
  debug: false,
  args: []
});

assert.deepEqual(parseDebugArgs(["--debug", "--workers=1"], false), {
  debug: true,
  args: ["--workers=1"]
});

assert.deepEqual(parseDebugArgs(["--debug=false", "--workers=1"], true), {
  debug: false,
  args: ["--workers=1"]
});

assert.deepEqual(parseDebugArgs(["--debug", "false", "--workers=1"], true), {
  debug: false,
  args: ["--workers=1"]
});

assert.deepEqual(parseDebugArgs(["--no-debug", "--workers=1"], true), {
  debug: false,
  args: ["--workers=1"]
});

assert.throws(() => parseDebugArgs(["--debug=maybe"], false), /debug/);

const screenshotRoot = fs.mkdtempSync(path.join(os.tmpdir(), "clawmanager-e2e-screenshots-"));
const nestedDir = path.join(screenshotRoot, "failed-test");
fs.mkdirSync(nestedDir, { recursive: true });
fs.writeFileSync(path.join(nestedDir, "test-failed-1.png"), "");
fs.writeFileSync(path.join(nestedDir, "trace.zip"), "");
fs.writeFileSync(path.join(screenshotRoot, "other.png"), "");

assert.deepEqual(
  findFailureScreenshots(screenshotRoot)
    .map((filePath) => path.basename(filePath))
    .sort(),
  ["other.png", "test-failed-1.png"]
);
