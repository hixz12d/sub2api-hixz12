# OpenAI Codex Identity Phase 0 / Phase 1 Evidence

## Repository baseline

- Remote: `origin=https://github.com/hixz12d/sub2api-hixz12.git`
- Upstream: `upstream=https://github.com/Wei-Shaw/sub2api.git`
- Branch at implementation start: `feat/openai-cross-account-cache-isolation`
- HEAD at implementation start: `63cc910b4b5ce7cf9c3c081d249ea99cc7be7563`
- Historical comparison baseline: `40648249c9f5e9e09935ff6109cdd9e50770be30`
- Repository worktree was clean at implementation start.
- The applicable project instruction file is the parent `AGENTS.md`; the repository contains no additional first-party `AGENTS.md` or `CONTRIBUTING.md`.

## Delta from the historical audit

Already fixed before this change:

- OAuth wire headers use canonical `session-id` / `thread-id`, while ingress accepts underscore aliases.
- JSON `client_metadata` retains underscore keys.
- A single fingerprint ID object is shared between header/body mutation in the main HTTP path.
- `prompt_cache_key` is consumed as a seed and removed from OAuth outbound JSON.
- HTTP, passthrough and WS paths already have account-scoped identity and outbound profile helpers.
- WS ingress already calls `BeforeTurn` before every complete `response.create`; retries of the same logical turn intentionally do not call it twice.

Remaining Phase 0 / Phase 1 gaps addressed here:

- The old ID object had no explicit snapshot/finalizer contract.
- HTTP and passthrough applied fingerprint/account/profile writers in different orders.
- Unknown legacy mode values silently entered `session` mode.
- Malformed managed turn metadata was silently retained beside newly generated outer identity.
- WS session/conversation logs exposed truncated raw values; response IDs used an unkeyed digest.
- Protocol UA/version/originator and HTTP/WS beta/body policy were not represented as one versioned profile.
- Several bridge and WS turn paths could perform compatibility rewrites after the common builder.

Intentionally unchanged in this phase:

- Account selection and sticky scheduling.
- Retry count, reconnect count, failover and `previous_response_id` recovery semantics.
- Database schema and response/session binding persistence.
- API-key/custom-upstream OAuth isolation behavior.
- Legacy `window` / `window40` bucketing algorithms and the historical unset-mode default.

## Final source-to-sink map

| Path | Raw signals | Snapshot point | Body sink | Header/Dial sink | Final writer |
|---|---|---|---|---|---|
| OAuth Responses HTTP stream/non-stream | canonical/legacy session headers, conversation, prompt cache | `finalizeCodexOAuthIdentity` in `Forward` | `finalizeCodexOAuthBody` in `buildUpstreamRequest` | `finalizeCodexOAuthHeaders` after header overrides | Codex identity finalizer |
| Compact | compact session resolver plus the same raw signals | same as Responses | no synthetic `client_metadata` when compact body lacks it | same final header boundary | Codex identity finalizer |
| HTTP passthrough | passthrough request headers and raw body cache key | `finalizeCodexOAuthIdentity` inside passthrough builder | `finalizeCodexOAuthBody` | `finalizeCodexOAuthHeaders` after overrides | Codex identity finalizer |
| Chat Completions bridge | bridge-derived prompt cache/session | bridge stores one snapshot in Gin context | shared Responses builder | shared final boundary; bridge compatibility shaping is followed by the same final boundary | Codex identity finalizer |
| Anthropic Messages bridge | bridge-derived prompt cache/session and turn state | bridge stores one snapshot in Gin context | shared Responses builder | bridge beta/turn-state shaping is followed by the same final boundary | Codex identity finalizer |
| WS/WSv2 initial Dial | HTTP ingress headers plus first `response.create` | first-turn snapshot before header build | create payload applies the same snapshot | `buildOpenAIWSHeaders` applies the snapshot and frozen protocol profile | Codex identity finalizer |
| WS second turn | current complete `response.create` | one new snapshot after `BeforeTurn` | turn body applies current snapshot | existing connection remains fixed; reconnect clones current-turn headers | current-turn snapshot application |
| WSv2 passthrough | each complete filtered `response.create` | one snapshot per accepted turn | actual outgoing Write payload applies current snapshot | Dial headers use the matching context snapshot on connection creation | current-turn snapshot application |
| API-key/custom upstream | caller-compatible headers/body | no OAuth snapshot | unchanged | API-key outbound profile policy only | existing API-key policy |

## Managed field ownership

| Field | Final owner |
|---|---|
| installation/session/thread/conversation/window/turn/client request ID | `CodexIdentitySnapshot` + `finalizeCodexOAuthHeaders` / `finalizeCodexOAuthBody` |
| `turn_started_at_unix_ms` | one clock read in `resolveCodexFingerprintIDs`, copied to all sinks |
| UA/version/originator | frozen `CodexProtocolProfile` in request context |
| HTTP/WS beta and body policy version | `CodexProtocolProfile`, with endpoint compatibility deciding whether HTTP beta is emitted |
| Authorization / ChatGPT-Account-Id | authentication/account resolver; never sourced from the snapshot |
| canonical header spelling | `normalizeCodexOAuthHeaders` inside the final header boundary |
| account stable language/beta features | final header boundary after client passthrough and account overrides |

No identity helper is allowed to generate a new turn ID or timestamp at a sink.

## Compatibility and feature flag

- Account extra flag: `codex_identity_finalizer_v2`.
- Default: disabled, preserving legacy configured-mode migration behavior.
- Historical unset `codex_fingerprint_mode` remains `session` in this phase.
- Known legacy modes remain accepted: `off`, `device`, `session`, `window`, `window40`, `full`.
- Unknown mode with v2 disabled fails safe to `off`, never to a high-impact mode.
- Unknown mode with v2 enabled returns a structured request error.
- Malformed managed turn metadata uses the documented sanitize policy: remove the malformed managed value, then apply the finalized outer identity. It is never silently retained.

## Logging policy

Session, conversation and response identifiers in WS diagnostics use a process-local, domain-separated HMAC-SHA256 short digest. Raw prefixes are no longer logged. Different domains (`session`, `conversation`, `response`) produce different digests for the same input.

## Rollback

- Disable `codex_identity_finalizer_v2` on affected accounts for strict-mode rollback.
- The implementation is additive and has no schema/data migration.
- A code rollback is a normal commit revert; no bindings, retry state or account data need to be rewritten.
