import type { DetectorTarget, DetectorTier } from './llmDetector'

export type MonitorSelectionMode = 'random' | 'fixed'

export interface GroupProbeConfig {
  enabled: boolean
  interval_seconds: number
  jitter_seconds: number
  timeout_seconds: number
  selection_mode: MonitorSelectionMode
  fixed_account_ids: number[]
  sample_size: number
  include_extra_models: boolean
  daily_request_limit: number
}

export interface GroupCapabilityConfig {
  enabled: boolean
  interval_seconds: number
  jitter_seconds: number
  selection_mode: MonitorSelectionMode
  fixed_account_ids: number[]
  sample_size: number
  tier: DetectorTier
  execution_timeout_seconds: number
  result_ttl_seconds: number
  daily_request_limit: number
  retest_on_mismatch: boolean
  retest_delay_seconds: number
  retest_limit_per_execution: number
  targets: DetectorTarget[]
}

// Admin-only configuration, never a public group-card DTO.
export interface ChannelMonitorGroupPolicy {
  id: number
  group_id: number
  display_name: string
  primary_model: string
  extra_models: string[]
  enabled: boolean
  probe_config: GroupProbeConfig
  capability_config: GroupCapabilityConfig
  version: number
  evaluation_revision: string
  next_probe_at: string | null
  next_capability_at: string | null
  created_by: number
  updated_by: number
  created_at: string
  updated_at: string
}

export function defaultGroupProbeConfig(): GroupProbeConfig {
  return {
    enabled: false, interval_seconds: 60, jitter_seconds: 15, timeout_seconds: 45,
    selection_mode: 'random', fixed_account_ids: [], sample_size: 1,
    include_extra_models: false, daily_request_limit: 2000,
  }
}

export function defaultGroupCapabilityConfig(): GroupCapabilityConfig {
  return {
    enabled: false, interval_seconds: 43200, jitter_seconds: 6480,
    selection_mode: 'random', fixed_account_ids: [], sample_size: 1, tier: 'low',
    execution_timeout_seconds: 1800, result_ttl_seconds: 86400,
    daily_request_limit: 1000, retest_on_mismatch: true, retest_delay_seconds: 1800,
    retest_limit_per_execution: 1, targets: [],
  }
}
