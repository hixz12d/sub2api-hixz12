# Candidate Client Profile Contracts

Status: offline PR-3 foundation only. This package is deliberately NOT imported
by the Relay Kernel. It must not be exposed as a production account selector yet.
Existing `codex_client_profile`, v2 identity derivation, TLS, secret selection and
Redis records are unchanged.

## Packaged Candidates

- `pi-0.57.1-oauth-sse-r1`: historical Pi reference; `session_id`, default
  verbosity `medium`, no version string in UA. This is NOT installed Pi 0.84.3.
- `opencode-1.2.4-oauth-sse-r1`: historical OpenCode reference; `session_id`,
  platform/release/architecture UA. No inferred verbosity default.

The declared Windows environment is `win32 10.0.26200; x64`. This is an
operator-declared candidate environment, not evidence that a Linux VPS runs
Windows. Sources/blob references are transcribed from the supplied blueprint.
Resolved release commits, lockfile hashes, binary hashes and live captures have
not been independently collected. Evidence therefore remains
`blueprint_source_reference / pending / unverified`; approval is rejected.

## Loading And Digest

`LoadCandidate` only reads embedded allowlisted filenames. There is no network,
filesystem override, latest-version fallback or executable behavior field.
`Decode` rejects unknown fields, duplicate JSON keys, trailing documents, input
larger than 16 KiB, nesting beyond eight levels and unreviewed identity tuples.
The Go types plus `Validate` are the current executable schema; a shared
cross-language JSON Schema is still pending.

Digest version 1 is SHA-256 over the EXACT embedded UTF-8 file bytes, including
whitespace and line endings. It is an artifact digest, NOT an RFC 8785 semantic
JSON digest. Consumers must exchange the same bytes to compare this digest.
Changing whitespace changes the digest. Production must pin the actual loaded
artifact digest together with the bundle ID before this package is wired in.

## Adapter Boundary

`AdaptResponses` is only a pure candidate OAuth HTTP/SSE constructor, not a
sender, authenticator, owner lookup, retry engine or general passthrough proxy.
The session argument must already be authenticated and scoped to an account and
conversation by the integrating caller. It cannot validate that ownership itself.

- Clones headers and decodes a separate body; inputs are never mutated.
- Rejects conflicting session spellings and duplicate body keys.
- Emits one version-specific session header; removes Codex identity/version/beta
  headers and replaces all case variants of UA/originator.
- Does not invent installation, thread, window, client metadata, sandbox,
  additional tools or official instructions.
- Preserves instructions, tool definitions, tool-call/output IDs, images,
  reasoning, explicit verbosity and continuation IDs.
- Pi scopes `prompt_cache_key` to the provided session, an explicit gateway
  isolation policy. OpenCode preserves the caller cache key.
- Rejects `client_metadata` and `conversation_id` instead of silently discarding
  caller body data. This candidate restriction requires review before activation.
- Does not assert WS/Compact support, change TLS or impose store/stream policy.

## Activation Gates

1. Resolve upstream commits and dependency evidence; approve the schema/fixtures.
2. Persist desired bundle and pin effective bundle/digest to a conversation.
   Legacy records must continue with the legacy adapter until expiry.
3. Use one adapter at every final body/header sink and registry reconstruction;
   verify the actual local upstream receiver, not only this pure constructor.
4. Introduce separately versioned stable installation migration; never modify v2
   device derivation in place or change the identity secret during this upgrade.
5. Verify authenticated continuation ownership, proxy revision and transport
   scopes, CAS/commit behavior and rollback using local mocks.
6. Only then expose approved bundles in UI and request authorization for live
   canary/deployment. Do not fabricate active counts or installation references.

Run locally from backend:

```sh
go test ./internal/pkg/clientprofile -count=1
go test -race ./internal/pkg/clientprofile -count=1
```
