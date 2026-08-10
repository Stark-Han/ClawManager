import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const layoutSource = readFileSync(
  path.resolve(scriptDir, "../src/components/UserLayout.tsx"),
  "utf8",
);
const instanceListSource = readFileSync(
  path.resolve(scriptDir, "../src/pages/instances/InstanceListPage.tsx"),
  "utf8",
);
const routerSource = readFileSync(
  path.resolve(scriptDir, "../src/router/index.tsx"),
  "utf8",
);

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(
  !layoutSource.includes("path: '/portal'") &&
    !layoutSource.includes('path: "/portal"'),
  "User sidebar must not expose Portal as a top-level navigation entry.",
);
assert(
  instanceListSource.includes('to="/portal"'),
  "Instance list must keep the Portal entry point.",
);
assert(
  routerSource.includes('path="/portal"'),
  "Portal route must remain available for instance-page entry points.",
);

console.log("User portal entry placement is valid.");
