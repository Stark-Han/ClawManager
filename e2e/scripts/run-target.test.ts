import assert from "node:assert/strict";
import { buildTargetRunOptions } from "./run-target.js";

const defaultOptions = buildTargetRunOptions(["172.16.1.12:39443"], {});

assert.equal(defaultOptions.frontendUrl, "https://172.16.1.12:39443");
assert.equal(defaultOptions.backendUrl, "https://172.16.1.12:39443/api/v1");
assert.deepEqual(defaultOptions.playwrightArgs, [
  "playwright",
  "test",
  "--grep",
  "@p0",
  "--grep-invert",
  "@local-only",
  "--workers=1"
]);
assert.equal(defaultOptions.env.E2E_FRONTEND_URL, "https://172.16.1.12:39443");
assert.equal(defaultOptions.env.E2E_BACKEND_URL, "https://172.16.1.12:39443/api/v1");
assert.equal(defaultOptions.env.E2E_IGNORE_HTTPS_ERRORS, "true");
assert.equal(defaultOptions.env.E2E_HEADLESS, "false");
assert.equal(defaultOptions.env.E2E_DEBUG, "false");
assert.equal(defaultOptions.env.E2E_SLOW_MO_MS, "0");
assert.equal(defaultOptions.debug, false);
assert.equal(defaultOptions.slowMoMs, 0);

const httpOptions = buildTargetRunOptions(["http://172.16.1.12:39080/"]);
assert.equal(httpOptions.frontendUrl, "http://172.16.1.12:39080");
assert.equal(httpOptions.backendUrl, "http://172.16.1.12:39080/api/v1");

const headlessOptions = buildTargetRunOptions(["172.16.1.12:39443", "--headless"], {});
assert.equal(headlessOptions.env.E2E_HEADLESS, "true");

const headedOptions = buildTargetRunOptions(["172.16.1.12:39443", "--headless=false"], {});
assert.equal(headedOptions.env.E2E_HEADLESS, "false");

const ciOptions = buildTargetRunOptions(["172.16.1.12:39443"], { CI: "true" });
assert.equal(ciOptions.env.E2E_HEADLESS, "true");

const debugOptions = buildTargetRunOptions(["172.16.1.12:39443", "--debug"], {});
assert.equal(debugOptions.env.E2E_DEBUG, "true");
assert.equal(debugOptions.env.E2E_SLOW_MO_MS, "500");
assert.equal(debugOptions.env.E2E_HEADLESS, "false");
assert.equal(debugOptions.debug, true);
assert.equal(debugOptions.slowMoMs, 500);

assert.throws(() => buildTargetRunOptions([]), /Usage/);
assert.throws(() => buildTargetRunOptions(["https://a.example", "https://b.example"]), /only accepts one address/);
