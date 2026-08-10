import {
  defaultRuntimePod,
  listRuntimePods,
  login,
  postRuntimePodRegisterRaw,
  registerRuntimePod,
  sendRuntimeHeartbeat
} from "../../fixtures/apiClient.js";
import { test, expect } from "../../fixtures/test.js";

test("@p0 @local-only runtime-agent register persists an openclaw runtime pod", async ({ request }) => {
  const pod = await registerRuntimePod(request, {
    pod_name: "e2e-openclaw-runtime-api-register"
  });
  const tokens = await login(request);
  const pods = await listRuntimePods(request, tokens.access_token);

  expect(pod.runtime_type).toBe("openclaw");
  expect(pod.capacity).toBe(defaultRuntimePod.capacity);
  expect(pod.used_slots).toBe(defaultRuntimePod.used_slots);
  expect(pod.draining).toBe(defaultRuntimePod.draining);
  expect(pods.some((item) => item.pod_name === "e2e-openclaw-runtime-api-register")).toBe(true);
});

test("@p0 @local-only runtime-agent heartbeat updates pod health state", async ({ request }) => {
  const pod = await registerRuntimePod(request, {
    pod_name: "e2e-openclaw-runtime-api-heartbeat"
  });

  await sendRuntimeHeartbeat(request, { pod_id: pod.id }, { state: "draining", used_slots: 3, draining: true });

  const tokens = await login(request);
  const pods = await listRuntimePods(request, tokens.access_token);
  const updated = pods.find((item) => item.id === pod.id);

  expect(updated?.state).toBe("draining");
  expect(updated?.used_slots).toBe(3);
  expect(updated?.draining).toBe(true);
});

test("@p0 @local-only runtime-agent register rejects unsupported runtime type", async ({ request }) => {
  const response = await postRuntimePodRegisterRaw(request, {
    ...defaultRuntimePod,
    runtime_type: "ubuntu",
    pod_name: "e2e-unsupported-runtime-api"
  });

  expect(response.status()).toBe(400);
});
