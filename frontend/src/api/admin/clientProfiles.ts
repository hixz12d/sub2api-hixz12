import { apiClient } from '../client'
import type { ClientProfileCatalogItem, CodexClientPreset, CodexClientProfile, CodexRelayFormState } from '@/components/account/codexRelaySchema'

export interface ClientPreset {
  id: CodexClientPreset
  profile: CodexClientProfile
  extra: Partial<CodexRelayFormState> & {
    enable_tls_fingerprint: boolean
    openai_oauth_responses_websockets_v2_mode: 'off' | 'ctx_pool'
  }
}

export interface ClientProfileCatalog {
  presets: ClientPreset[]
  revision: string
  relay_contract: string
  relay_digest: string
  profiles: ClientProfileCatalogItem[]
  new_managed_default_enabled: boolean
  activation_requirements: string[]
}

export interface ClientProfilePreview {
  valid: boolean
  scope: string
  profile?: ClientProfileCatalogItem
  transport?: {
    owner: string
    sender: string
    tls_recipe: string
    http2_recipe: string
    native_validation: string
  }
  conflicts: string[]
  requirements: string[]
  session_effect: string
  plugin_status: string
}

export interface ClientProfilePreviewInput {
  account_id?: number
  platform: 'openai'
  type: string
  extra: Record<string, unknown>
  operation: 'responses' | 'compact' | 'resume'
  transport: 'http' | 'ws'
  catalog_revision: string
}

export async function getClientProfiles(signal?: AbortSignal): Promise<ClientProfileCatalog> {
  const { data } = await apiClient.get<ClientProfileCatalog>('/admin/accounts/client-profiles', { signal })
  return data
}

export async function previewClientProfile(input: ClientProfilePreviewInput, signal?: AbortSignal): Promise<ClientProfilePreview> {
  const { data } = await apiClient.post<ClientProfilePreview>('/admin/accounts/client-profile-preview', input, { signal })
  return data
}
