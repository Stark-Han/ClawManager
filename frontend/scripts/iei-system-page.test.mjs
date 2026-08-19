import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const read = (relativePath) => readFileSync(path.resolve(scriptDir, relativePath), "utf8");
const router = read("../src/router/index.tsx");
const listPage = read("../src/pages/instances/IEISystemListInstancesPage.tsx");
const detailPage = read("../src/pages/instances/IEISystemInstancePage.tsx");
const service = read("../src/services/ieiSystemService.ts");

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

assert(
  router.includes('path="/ieisystem/list-instances"') &&
    router.includes('path="/ieisystem/instances/:id"') &&
    !router.includes('path="/northbound/owners/:owner/lite-instances"'),
  "IEI pages must use the fixed public routes and remove the old owner URL.",
);

assert(
  service.includes('axios.create({') &&
    service.includes("withCredentials: true") &&
    !service.includes('from "./api"') &&
    !service.toLowerCase().includes("share"),
  "IEI access must use a dedicated cookie client and must not reuse ShareLink or normal JWT auth.",
);

assert(
  listPage.includes('params.get("token")') &&
    listPage.includes("window.history.replaceState") &&
    listPage.includes("exchangeSession(token)") &&
    listPage.includes('meta[name="referrer"]'),
  "The one-time SSO token must be exchanged and removed from the browser URL with no-referrer policy.",
);

assert(
  listPage.includes("InstanceTypeIcon") &&
    listPage.includes('src="/openclaw.png"') &&
    listPage.includes('src="/hermes.png"') &&
    !listPage.includes("instance.description"),
  "IEI instance cards must show their runtime icon without the Created by description line.",
);

assert(
  detailPage.includes("getInstance(instanceID)") &&
    detailPage.includes("generateAccess(instanceID)") &&
    detailPage.includes('referrerPolicy="no-referrer"') &&
    detailPage.includes("WorkspaceFileManager") &&
    detailPage.includes("ieiSystemWorkspaceService") &&
    detailPage.includes("xl:grid-cols-[minmax(0,1fr)_minmax(360px,28rem)]"),
  "Entering an IEI instance must validate detail ownership and request a separate access capability.",
);

assert(
  service.includes("/workspace/files") &&
    service.includes("/workspace/preview") &&
    service.includes("/workspace/download") &&
    service.includes("/workspace/upload") &&
    service.includes("/workspace/folders") &&
    service.includes("/workspace/entries"),
  "The IEI detail page must expose the same workspace file operations as the ShareLink page.",
);

console.log("IEI system page contract is valid.");
