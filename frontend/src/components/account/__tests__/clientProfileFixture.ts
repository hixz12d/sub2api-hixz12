import { CODEX_CLIENT_PROFILES, type ClientProfileCatalogItem } from '../codexRelaySchema'
import type { ClientProfileCatalog } from '@/api/admin/clientProfiles'

export const clientProfileCatalogFixture: ClientProfileCatalog = {
  revision: 'test-catalog-revision',
  relay_contract: 'relay-profile-contract/1',
  relay_digest: 'test-reviewed-digest',
  new_managed_default_enabled: false,
  activation_requirements: ['distributed-registry'],
  profiles: CODEX_CLIENT_PROFILES.map<ClientProfileCatalogItem>(({ id }) => {
    const family = id.startsWith('codex_') ? 'codex' : id.startsWith('opencode') ? 'opencode' : id.startsWith('pi') ? 'pi' : 'caller'
    const auto = id === 'auto'
    return {
      id, family, variant: id.endsWith('-r1') ? 'shared-r1' : 'legacy',
      appVersion: auto ? 'dynamic' : id === 'pi' ? 'not asserted' : 'remote-test-version',
      http: auto ? null : true,
      ws: auto ? null : family === 'codex' || id === 'passthrough',
      compact: auto ? null : family === 'codex' || id === 'passthrough',
      fidelity: auto ? 'caller-resolved' : id === 'pi' ? 'unsupported strict parity' : 'degraded',
      recipe: id, digest: 'test-profile-digest', native_validation: 'untested'
    }
  })
}
