import type { Instance } from './instance';
import type { RuntimeUpgradeDetails } from './runtimePool';

export interface OpenClawUpgradeLabRun {
  id: number;
  actor_user_id?: number;
  status: string;
  phase: string;
  baseline_image_ref: string;
  baseline_image_digest: string;
  source_deployment: string;
  target_image_ref?: string;
  rollout_id?: number;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
}

export interface OpenClawUpgradeLabCheck {
  name: string;
  passed: boolean;
  expected?: string;
  actual?: string;
  message?: string;
}

export interface OpenClawUpgradeLabView {
  run?: OpenClawUpgradeLabRun;
  instance_ids: number[];
  instances: Instance[];
  checks: OpenClawUpgradeLabCheck[];
  rollout?: RuntimeUpgradeDetails;
  baseline_tag: string;
}
