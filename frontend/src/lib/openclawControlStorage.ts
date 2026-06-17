const OPENCLAW_SETTINGS_KEY = "openclaw.control.settings.v1";
const OPENCLAW_SETTINGS_PREFIX = `${OPENCLAW_SETTINGS_KEY}:`;
const OPENCLAW_TOKEN_KEY = "openclaw.control.token.v1";
const OPENCLAW_TOKEN_PREFIX = `${OPENCLAW_TOKEN_KEY}:`;
const OPENCLAW_DEVICE_AUTH_KEY = "openclaw.device.auth.v1";
const CLAWMANAGER_OPENCLAW_INSTANCE_KEY = "clawmanager.openclaw.instanceId";
const CLAWMANAGER_OPENCLAW_GATEWAY_KEY = "clawmanager.openclaw.gatewayUrl";

function storageKeys(storage: Storage) {
  const keys: string[] = [];
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index);
    if (key) {
      keys.push(key);
    }
  }
  return keys;
}

function canonicalOpenClawGatewayUrl(embedUrl: string) {
  const url = new URL(embedUrl, window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.search = "";
  url.hash = "";
  url.pathname = url.pathname.replace(/\/+$/, "");
  return url.toString();
}

export function prepareOpenClawControlUIStorage(instanceId: number, embedUrl: string) {
  if (typeof window === "undefined") {
    return embedUrl;
  }

  try {
    const storage = window.localStorage;
    const gatewayUrl = canonicalOpenClawGatewayUrl(embedUrl);
    const previousInstanceId = storage.getItem(CLAWMANAGER_OPENCLAW_INSTANCE_KEY);
    const previousGatewayUrl = storage.getItem(CLAWMANAGER_OPENCLAW_GATEWAY_KEY);
    const instanceChanged =
      previousInstanceId !== null && previousInstanceId !== String(instanceId);
    const gatewayChanged = previousGatewayUrl !== null && previousGatewayUrl !== gatewayUrl;

    for (const key of storageKeys(storage)) {
      if (
        key === OPENCLAW_SETTINGS_KEY ||
        key.startsWith(OPENCLAW_SETTINGS_PREFIX) ||
        key === OPENCLAW_TOKEN_KEY ||
        key.startsWith(OPENCLAW_TOKEN_PREFIX)
      ) {
        storage.removeItem(key);
      }
    }

    if (instanceChanged || gatewayChanged) {
      storage.removeItem(OPENCLAW_DEVICE_AUTH_KEY);
    }

    const settings = {
      gatewayUrl,
      sessionKey: "main",
      lastActiveSessionKey: "main",
      theme: "claw",
      themeMode: "system",
      chatFocusMode: false,
      chatShowThinking: true,
      chatShowToolCalls: true,
      splitRatio: 0.6,
      navCollapsed: false,
      navWidth: 220,
      navGroupsCollapsed: {},
      borderRadius: 50,
      sessionsByGateway: {
        [gatewayUrl]: {
          sessionKey: "main",
          lastActiveSessionKey: "main",
        },
      },
    };
    const serialized = JSON.stringify(settings);

    storage.setItem(OPENCLAW_SETTINGS_KEY, serialized);
    storage.setItem(`${OPENCLAW_SETTINGS_PREFIX}${gatewayUrl}`, serialized);
    storage.setItem(CLAWMANAGER_OPENCLAW_INSTANCE_KEY, String(instanceId));
    storage.setItem(CLAWMANAGER_OPENCLAW_GATEWAY_KEY, gatewayUrl);
  } catch {
    return embedUrl;
  }

  return embedUrl;
}
