import api from "./api";
import type {
  RuntimeGateway,
  RuntimePod,
  RuntimeType,
  StartRuntimeRolloutRequest,
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

  async startRollout(data: StartRuntimeRolloutRequest): Promise<void> {
    await api.post("/admin/runtime-rollouts", data);
  },
};
