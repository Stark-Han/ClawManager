export type OpenClawResourceType =
  | 'channel'
  | 'skill'
  | 'session_template'
  | 'log_policy'
  | 'agent'
  | 'scheduled_task';

export type OpenClawConfigMode = 'none' | 'bundle' | 'manual';

export interface OpenClawConfigResourceSummary {
  id: number;
  resource_type: OpenClawResourceType;
  resource_key: string;
  name: string;
  enabled: boolean;
  version: number;
}

export interface OpenClawConfigResource extends OpenClawConfigResourceSummary {
  user_id: number;
  description?: string;
  tags: string[];
  content: unknown;
  created_at: string;
  updated_at: string;
}

export interface UpsertOpenClawConfigResourceRequest {
  resource_type: OpenClawResourceType;
  resource_key: string;
  name: string;
  description?: string;
  enabled: boolean;
  tags: string[];
  content: unknown;
}

export interface OpenClawConfigBundleItem {
  resource_id: number;
  sort_order: number;
  required: boolean;
  resource?: OpenClawConfigResourceSummary;
}

export interface OpenClawConfigBundleSkillItem {
  skill_id: number;
  sort_order: number;
  required: boolean;
  skill?: {
    id: number;
    user_id: number;
    skill_key: string;
    name: string;
    description?: string;
    status: string;
    source_type: string;
    risk_level: string;
    current_version_id?: number;
    last_scanned_at?: string;
    created_at: string;
    updated_at: string;
  };
}

export interface OpenClawConfigBundle {
  id: number;
  user_id: number;
  name: string;
  description?: string;
  enabled: boolean;
  version: number;
  items: OpenClawConfigBundleItem[];
  skill_items?: OpenClawConfigBundleSkillItem[];
  created_at: string;
  updated_at: string;
}

export interface UpsertOpenClawConfigBundleRequest {
  name: string;
  description?: string;
  enabled: boolean;
  items: OpenClawConfigBundleItem[];
  skill_items?: OpenClawConfigBundleSkillItem[];
}

export interface OpenClawConfigPlan {
  mode: OpenClawConfigMode;
  bundle_id?: number;
  resource_ids?: number[];
}

export interface OpenClawConfigCompilePreview {
  mode: OpenClawConfigMode | 'archive';
  bundle?: OpenClawConfigBundle;
  selected_resources: OpenClawConfigResourceSummary[];
  resolved_resources: OpenClawConfigResourceSummary[];
  auto_included: OpenClawConfigResourceSummary[];
  warnings: string[];
  env_names: string[];
  payload_sizes: Record<string, number>;
  total_payload_bytes: number;
  manifest: unknown;
}

export interface OpenClawInjectionSnapshot {
  id: number;
  instance_id?: number;
  user_id: number;
  mode: OpenClawConfigMode;
  bundle_id?: number;
  selected_resource_ids: number[];
  resolved_resources: OpenClawConfigResourceSummary[];
  manifest: unknown;
  env_names: string[];
  payload_sizes: Record<string, number>;
  secret_name?: string;
  status: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
  activated_at?: string;
}

export const OPENCLAW_RESOURCE_TYPES: Array<{ value: OpenClawResourceType; label: string }> = [
  { value: 'channel', label: 'Channel' },
  { value: 'skill', label: 'Skill' },
  { value: 'session_template', label: 'Session Template' },
  { value: 'log_policy', label: 'Log Policy' },
  { value: 'agent', label: 'Agent' },
  { value: 'scheduled_task', label: 'Scheduled Task' },
];
