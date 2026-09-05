# Sub2API Runtime Egress Map

This is the initial baseline audit, preserved for provenance. The later runtime
pinning/installation changes and their rollback requirements are documented in
`backend/internal/service/CODEX_CONVERSATION_MIGRATION.md` and the follow-up
section of `docs/audit/profile-foundation-verification.md`. The initial gates
below are not a statement that those later source changes are still absent.

## Evidence Boundary

Local source baseline: `670b1844f7334bd4e09dfa28d6f6146c5b8c45cb`.
The worktree was clean before this implementation. This is a SOURCE audit;
production account settings, plugin bindings, secrets, endpoints and listeners
were not queried. No production request, OAuth refresh or live inference was run.

The supplied 2026-09-05 study reports Python relay A/B traffic through local
`:18790` (since stopped). It does not prove that Sub2API chains to that process.
Neither `:8787` nor Docker `:18787` is evidence of this repository's last hop.
The Python repository has not been modified by this task.

## Source Routes

```text
OpenAIGatewayHandler.prepareCodexRequestPlan
  -> OpenAIGatewayService account scheduling / token provider
  -> finalizeCodexOAuthIdentity
     -> legacy identity OR FinalizeCodexAttempt (explicit relay_kernel + v2)
     -> Redis conversation ResolveOrCreate / CAS
  -> finalizeCodexOAuthBody / finalizeCodexOAuthHeaders
  -> finalizeCodexAttemptHTTPWire
  -> doOpenAIUpstream
     -> enabled OpenAI OAuth plugin (if handled, plugin owns final transport)
     -> otherwise HTTPUpstream.DoWithTLS (only when TLS fingerprint is enabled)
     -> otherwise HTTPUpstream.Do
```

WS has a separate final sink, `finalizeCodexAttemptWSWire`, and pool scope.
HTTP evidence must not be generalized to the WS dialer or plugin internals.

## Ownership And Files

| Responsibility | Source owner |
|---|---|
| Immutable request plan | `backend/internal/handler/openai_gateway_handler.go`, `prepareCodexRequestPlan` |
| Attempt identity/scope | `backend/internal/service/openai_codex_relay_kernel.go`, `FinalizeCodexAttempt` |
| Final body/header snapshots | `backend/internal/service/openai_codex_identity_finalizer.go` |
| Distributed binding/CAS | `backend/internal/service/openai_codex_conversation_registry.go` |
| HTTP dispatch/plugin precedence | `backend/internal/service/openai_plugin_transport.go`, `doOpenAIUpstream` |
| OAuth token refresh | `backend/internal/service/openai_token_provider.go`, `OpenAITokenProvider` |
| Account write validation | `backend/internal/service/admin_account.go` calls `ValidateCodexRelayAccountExtra` on create, update and merge paths |

HTTP finalizer call sites include `openai_gateway_forward.go`,
`openai_gateway_passthrough.go`, `openai_gateway_messages.go` and
`openai_gateway_chat_completions.go`. Adapting only the kernel constructor does
not prove these final sinks are free from later generic Codex rewrites.

## Configuration Inventory

| Key/control | Source behavior | Production value |
|---|---|---|
| `codex_relay_mode` | default `legacy`, explicit `relay_kernel` | not collected |
| `codex_identity_policy_version` | kernel requires `v2` | not collected |
| `codex_client_profile` | catalog selector; `auto` resolves inbound metadata | not collected |
| `codex_relay_shadow_enabled` | local candidate comparison, no extra upstream call | not collected |
| `codex_fingerprint_mode` | managed identity required by kernel | not collected |
| `enable_tls_fingerprint` | gates managed HTTP uTLS in final dispatcher | not collected |
| OpenAI OAuth plugin binding | takes precedence over built-in HTTP sender | not collected |
| identity derivation secret | affinity secret preferred, JWT fallback | source only; no value read |

Do not rename the secret or assume both production instances share the same
source without a separately authorized deployment check.

## This Batch

Added `backend/internal/pkg/clientprofile`: embedded, offline candidate Pi
0.57.1 and OpenCode 1.2.4 contracts, strict loader, artifact digest, pure HTTP/SSE
application adapter and tests. It is NOT connected to the live finalizer and
NOT an approved bundle selector. Existing v2/catalog/registry behavior remains.

Fixed save-time selector type validation: explicit null, boolean, numeric,
array and object values no longer silently become default string settings.
Absent fields and existing empty-string defaults remain compatible.

## Remaining Gates

- Verify tenant-scoped conversation aliases and previous-response ownership
  across HTTP/WS. `NewCodexRequestPlan` prioritizes previous response, cache key,
  SessionHash, then logical request; a random identifier is not authorization.
- Pin bundle ID/digest before auto detection can affect an established session.
- Preserve old registry records and v2 installation identities; separately design
  v3 installation migration and credential/proxy scope revisions.
- Integrate adapter into EVERY body/header sink and registry reconstruction.
- Resolve upstream commit/lockfile evidence and approve shared schema/fixtures.
- Run local final-sender mock tests, then separately authorized live canary.

Rollback of this batch does not require Redis deletion, secret rotation or
account changes: candidates have no runtime consumer. Roll back the code build
through the existing dual-instance procedure only if deployment is authorized.
