import { describe, expect, it } from 'vitest'
import {
  CODEX_CLIENT_PROFILES,
  extractCodexRelayState,
  serializeCodexRelayToBulkExtra,
  serializeCodexRelayToExtra,
  validateCodexRelayState
} from '../codexRelaySchema'

describe('shared client bundles', () => {
  for (const id of ['pi-0.57.1-oauth-sse-r1', 'opencode-1.2.4-oauth-sse-r1']) {
    it(`preserves ${id} through account and bulk saves`, () => {
      const state = extractCodexRelayState({ codex_client_profile: id, codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device' })
      expect(state.codex_client_profile).toBe(id)
      expect(validateCodexRelayState(state, (key) => key).valid).toBe(true)
      const single: Record<string, unknown> = {}
      const bulk: Record<string, unknown> = {}
      serializeCodexRelayToExtra(state, single)
      serializeCodexRelayToBulkExtra(state, bulk)
      expect(single.codex_client_profile).toBe(id)
      expect(bulk.codex_client_profile).toBe(id)
      expect(CODEX_CLIENT_PROFILES.find((profile) => profile.id === id)).toMatchObject({ http: true, ws: false, compact: false, fidelity: 'degraded' })
      state.codex_relay_mode = 'legacy'
      expect(validateCodexRelayState(state, (key) => key).errors.codex_client_profile).toBeTruthy()
    })
  }
})
