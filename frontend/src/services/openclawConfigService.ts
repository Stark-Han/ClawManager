import api from './api';
import type {
  OpenClawConfigBundle,
  OpenClawConfigCompilePreview,
  OpenClawConfigPlan,
  OpenClawConfigResource,
  OpenClawInjectionSnapshot,
  OpenClawResourceType,
  UpsertOpenClawConfigBundleRequest,
  UpsertOpenClawConfigResourceRequest,
} from '../types/openclawConfig';

export const openclawConfigService = {
  listResources: async (resourceType?: OpenClawResourceType): Promise<OpenClawConfigResource[]> => {
    const response = await api.get('/openclaw-configs/resources', {
      params: resourceType ? { resource_type: resourceType } : undefined,
    });
    return response.data.data;
  },

  getResource: async (id: number): Promise<OpenClawConfigResource> => {
    const response = await api.get(`/openclaw-configs/resources/${id}`);
    return response.data.data;
  },

  createResource: async (payload: UpsertOpenClawConfigResourceRequest): Promise<OpenClawConfigResource> => {
    const response = await api.post('/openclaw-configs/resources', payload);
    return response.data.data;
  },

  updateResource: async (id: number, payload: UpsertOpenClawConfigResourceRequest): Promise<OpenClawConfigResource> => {
    const response = await api.put(`/openclaw-configs/resources/${id}`, payload);
    return response.data.data;
  },

  deleteResource: async (id: number): Promise<void> => {
    await api.delete(`/openclaw-configs/resources/${id}`);
  },

  cloneResource: async (id: number): Promise<OpenClawConfigResource> => {
    const response = await api.post(`/openclaw-configs/resources/${id}/clone`);
    return response.data.data;
  },

  validateResource: async (payload: UpsertOpenClawConfigResourceRequest): Promise<void> => {
    await api.post('/openclaw-configs/resources/validate', payload);
  },

  listBundles: async (): Promise<OpenClawConfigBundle[]> => {
    const response = await api.get('/openclaw-configs/bundles');
    return response.data.data;
  },

  getBundle: async (id: number): Promise<OpenClawConfigBundle> => {
    const response = await api.get(`/openclaw-configs/bundles/${id}`);
    return response.data.data;
  },

  createBundle: async (payload: UpsertOpenClawConfigBundleRequest): Promise<OpenClawConfigBundle> => {
    const response = await api.post('/openclaw-configs/bundles', payload);
    return response.data.data;
  },

  updateBundle: async (id: number, payload: UpsertOpenClawConfigBundleRequest): Promise<OpenClawConfigBundle> => {
    const response = await api.put(`/openclaw-configs/bundles/${id}`, payload);
    return response.data.data;
  },

  deleteBundle: async (id: number): Promise<void> => {
    await api.delete(`/openclaw-configs/bundles/${id}`);
  },

  cloneBundle: async (id: number): Promise<OpenClawConfigBundle> => {
    const response = await api.post(`/openclaw-configs/bundles/${id}/clone`);
    return response.data.data;
  },

  compilePreview: async (payload: OpenClawConfigPlan): Promise<OpenClawConfigCompilePreview> => {
    const response = await api.post('/openclaw-configs/compile-preview', payload);
    return response.data.data;
  },

  listInjections: async (limit: number = 50): Promise<OpenClawInjectionSnapshot[]> => {
    const response = await api.get('/openclaw-configs/injections', { params: { limit } });
    return response.data.data;
  },

  getInjection: async (id: number): Promise<OpenClawInjectionSnapshot> => {
    const response = await api.get(`/openclaw-configs/injections/${id}`);
    return response.data.data;
  },
};
