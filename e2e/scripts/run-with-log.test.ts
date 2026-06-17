import assert from "node:assert/strict";
import { parseArgs } from "./run-with-log.js";

assert.deepEqual(parseArgs(["p0"], {}), {
  suite: "p0",
  ci: false,
  headless: false,
  debug: false,
  slowMoMs: 0,
  extraArgs: []
});

assert.deepEqual(parseArgs(["p1", "--headless", "--workers=1"], {}), {
  suite: "p1",
  ci: false,
  headless: true,
  debug: false,
  slowMoMs: 0,
  extraArgs: ["--workers=1"]
});

assert.deepEqual(parseArgs(["p2", "--headed", "--workers=1"], { CI: "true" }), {
  suite: "p2",
  ci: false,
  headless: false,
  debug: false,
  slowMoMs: 0,
  extraArgs: ["--workers=1"]
});

assert.deepEqual(parseArgs(["all", "--ci"], {}), {
  suite: "all",
  ci: true,
  headless: true,
  debug: false,
  slowMoMs: 0,
  extraArgs: []
});

assert.deepEqual(parseArgs(["p0", "--ci", "--headless=false"], {}), {
  suite: "p0",
  ci: true,
  headless: false,
  debug: false,
  slowMoMs: 0,
  extraArgs: []
});

assert.deepEqual(parseArgs(["p0", "--debug", "--workers=1"], {}), {
  suite: "p0",
  ci: false,
  headless: false,
  debug: true,
  slowMoMs: 500,
  extraArgs: ["--workers=1"]
});
