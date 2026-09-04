import api from './api';
import type { OpenClawUpgradeLabView } from '../types/openClawUpgradeLab';

export const openClawUpgradeLabService = {
  async latest(): Promise<OpenClawUpgradeLabView> {
    const response = await api.get('/admin/openclaw-upgrade-lab');
    return response.data.data;
  },

  async get(id: number): Promise<OpenClawUpgradeLabView> {
    const response = await api.get(`/admin/openclaw-upgrade-lab/${id}`);
    return response.data.data;
  },

  async createBaseline(instanceCount: number): Promise<OpenClawUpgradeLabView> {
    const response = await api.post('/admin/openclaw-upgrade-lab', { instance_count: instanceCount });
    return response.data.data;
  },

  async startUpgrade(id: number, targetImageRef: string, batchSize: number): Promise<OpenClawUpgradeLabView> {
    const response = await api.post(`/admin/openclaw-upgrade-lab/${id}/upgrade`, {
      target_image_ref: targetImageRef,
      batch_size: batchSize,
    });
    return response.data.data;
  },

  async reset(id: number, instanceCount: number): Promise<OpenClawUpgradeLabView> {
    const response = await api.post(`/admin/openclaw-upgrade-lab/${id}/reset`, { instance_count: instanceCount });
    return response.data.data;
  },

  async cleanup(id: number): Promise<void> {
    await api.delete(`/admin/openclaw-upgrade-lab/${id}`);
  },
};
