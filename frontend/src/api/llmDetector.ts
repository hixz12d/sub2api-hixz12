// Persistence contracts only. Network endpoints arrive with the corresponding service phase.
export type DetectorJobState =
  | 'queued' | 'running' | 'cancelling' | 'completed'
  | 'failed' | 'cancelled' | 'interrupted' | 'skipped'

export type DetectorVerdict = 'match' | 'mismatch' | 'insufficient' | 'not_evaluated'
export type DetectorContractStatus = 'exact' | 'mutated' | 'unsupported' | 'unknown'
export type DetectorTier = 'low' | 'medium' | 'high'
export type DetectorSource = 'platform_group' | 'external_api' | 'site_api_key'

export interface DetectorTarget {
  benchmark_channel?: string
  request_model: string
  claimed_model: string
  benchmark_id: string
  benchmark_version: string
  benchmark_sha256: string
}

export interface DetectorBenchmarkManifest {
  channel?: string
  channel_revision?: number
  release_id?: string
  engine_lock_sha256?: string
  id: string
  version: string
  sha256: string
  engine_commit: string
  engine_version: string
  scoring_version: string
  sample_policy_version: string
  request_contract_hash: string
}
