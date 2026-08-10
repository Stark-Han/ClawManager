import { test, expect } from "../fixtures/test.js";
import { defaultRuntimePod, registerRuntimePod } from "../fixtures/apiClient.js";

test("@p0 @local-only db setup registers runtime pod", async ({ request }) => {
  await test.step("register deterministic runtime pod", async () => {
    const pod = await registerRuntimePod(request);
    expect(pod.pod_name).toBe(defaultRuntimePod.pod_name);
    expect(pod.runtime_type).toBe("openclaw");
    expect(pod.state).toBe("ready");
  });
});
