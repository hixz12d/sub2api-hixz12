import { CODEX_CLIENT_PROFILES, type ClientProfileCatalogItem } from '../codexRelaySchema'
import type { ClientPreset, ClientProfileCatalog } from '@/api/admin/clientProfiles'

export const clientProfileCatalogFixture: ClientProfileCatalog = {
  presets: (['codex', 'pi', 'opencode'] as const).map<ClientPreset>((id) => {
    const profile = id === 'codex' ? 'codex_cli' : id === 'pi' ? 'pi-managed' : 'opencode-managed'
    return { id, profile, extra: {
      codex_client_preset: id, codex_client_profile: profile, codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v2', codex_installation_policy: 'stable_v1',
      codex_fingerprint_mode: 'device', codex_relay_shadow_enabled: false, enable_tls_fingerprint: id === 'codex',
      openai_oauth_responses_websockets_v2_mode: id === 'codex' ? 'ctx_pool' : 'off'
    } }
  }),
  revision: 'test-catalog-revision',
  relay_contract: 'relay-profile-contract/1',
  relay_digest: 'test-reviewed-digest',
  new_managed_default_enabled: false,
  activation_requirements: ['distributed-registry'],
  profiles: CODEX_CLIENT_PROFILES.map<ClientProfileCatalogItem>(({ id }) => {
    const family = id.startsWith('codex_') ? 'codex' : id.startsWith('opencode') ? 'opencode' : id.startsWith('pi') ? 'pi' : 'caller'
    const auto = id === 'auto'
    return {
      id, family, variant: id.endsWith('-managed') ? 'managed' : id.endsWith('-r1') ? 'shared-r1' : 'legacy',
      version_policy: id.endsWith('-managed') ? 'auto' : undefined,
      update_status: id.endsWith('-managed') ? 'current' : undefined,
      appVersion: auto ? 'dynamic' : id === 'pi' ? 'not asserted' : 'remote-test-version',
      http: auto ? null : true,
      ws: auto ? null : family === 'codex' || id === 'passthrough',
      compact: auto ? null : family === 'codex' || id === 'passthrough',
      fidelity: auto ? 'caller-resolved' : id === 'pi' ? 'unsupported strict parity' : 'degraded',
      recipe: id, digest: 'test-profile-digest', native_validation: 'untested'
    }
  })
}
