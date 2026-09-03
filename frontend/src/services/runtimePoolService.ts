import api from "./api";
import type {
  RuntimeGateway,
  RuntimePod,
  RuntimeRollout,
  RuntimeType,
  RuntimeUpgradeDetails,
  StartRuntimeRolloutRequest,
  RuntimeUpgradePreflightResult,
} from "../types/runtimePool";

export const runtimePoolService = {
  async listPods(runtimeType?: RuntimeType): Promise<RuntimePod[]> {
    const response = await api.get("/admin/runtime-pods", {
      params: runtimeType ? { runtime_type: runtimeType } : undefined,
    });
    return response.data.data.pods;
  },

  async listGateways(podId: number): Promise<RuntimeGateway[]> {
    const response = await api.get(`/admin/runtime-pods/${podId}/gateways`);
    return response.data.data.gateways;
  },

  async drainPod(podId: number): Promise<void> {
    await api.post(`/admin/runtime-pods/${podId}/drain`);
  },

	async startRollout(data: StartRuntimeRolloutRequest): Promise<RuntimeRollout> {
		const response = await api.post("/admin/runtime-rollouts", data);
		return response.data.data.rollout;
  },

	async getRollout(id: number): Promise<RuntimeUpgradeDetails> {
		const response = await api.get(`/admin/runtime-rollouts/${id}`);
		return response.data.data;
	},

  async preflightOpenClawRollout(data: Omit<StartRuntimeRolloutRequest, "runtime_type" | "preflight_id">): Promise<RuntimeUpgradePreflightResult> {
    const response = await api.post("/admin/runtime-rollouts/preflight", data);
    return response.data.data;
  },
};
