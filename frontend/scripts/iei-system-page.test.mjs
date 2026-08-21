import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const read = (relativePath) => readFileSync(path.resolve(scriptDir, relativePath), "utf8");
const router = read("../src/router/index.tsx");
const listPage = read("../src/pages/instances/IEISystemListInstancesPage.tsx");
const detailPage = read("../src/pages/instances/IEISystemInstancePage.tsx");
const service = read("../src/services/ieiSystemService.ts");
const workspaceManager = read("../src/components/WorkspaceFileManager.tsx");
const translations = read("../src/lib/i18n.ts");
const runtimeCatalog = read("../src/lib/ieiRuntimeCatalog.ts");
const mockServer = read("./iei-mock-server.mjs");

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
    listPage.includes('src="/opencode.png"') &&
    listPage.includes('src="/deepseek-harness.svg"') &&
    !listPage.includes("instance.description"),
  "IEI instance cards must show their runtime icon without the Created by description line.",
);

assert(
  listPage.includes("我的实例") &&
    listPage.includes("实例详情") &&
    listPage.includes("运行时说明") &&
    listPage.includes("实例模式") &&
    listPage.includes("selectedRuntime.lite") &&
    listPage.includes("selectedRuntime.pro") &&
    runtimeCatalog.includes('"deepseek-harness"') &&
    runtimeCatalog.includes("Developer Preview"),
  "The owner portal must use the three-column runtime-aware master/detail layout with neutral Lite and Pro descriptions.",
);

assert(
  ["openclaw", "hermes", "opencode", "deepseek-harness"].every(
    (type) => (mockServer.match(new RegExp(`type: "${type}"`, "g")) ?? []).length === 2,
  ),
  "The local IEI test server must provide two Lite instances for every supported runtime presentation.",
);

assert(
  detailPage.includes("getInstance(instanceID)") &&
    detailPage.includes("generateAccess(instanceID)") &&
    detailPage.includes('referrerPolicy="no-referrer"') &&
    detailPage.includes("WorkspaceFileManager") &&
    detailPage.includes("ieiSystemWorkspaceService") &&
    detailPage.includes('localeOverride="zh"') &&
    detailPage.includes("xl:grid-cols-[minmax(0,1fr)_minmax(360px,28rem)]"),
  "Entering an IEI instance must validate ownership, request separate access, and default its workspace labels to Chinese.",
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

assert(
  workspaceManager.includes("useI18n") &&
    workspaceManager.includes("localeOverride") &&
    workspaceManager.includes('translateLabel("workspaceFileManager.workspace")') &&
    workspaceManager.includes('translateLabel("workspaceFileManager.name")') &&
    workspaceManager.includes('translateLabel("workspaceFileManager.size")') &&
    workspaceManager.includes('translateLabel("workspaceFileManager.modified")') &&
    workspaceManager.includes('translateLabel("workspaceFileManager.actions")') &&
    translations.includes("workspaceFileManagerTranslations") &&
    translations.includes('workspace: "工作区"') &&
    translations.includes('workspace: "ワークスペース"') &&
    translations.includes('workspace: "작업 공간"') &&
    translations.includes('workspace: "Arbeitsbereich"'),
  "The shared IEI workspace file manager headings must follow the selected application language.",
);

console.log("IEI system page contract is valid.");
