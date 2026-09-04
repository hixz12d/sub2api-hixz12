# OpenAI Codex Relay Kernel

## Status

Relay Kernel is an opt-in OpenAI OAuth/setup-token egress policy. The default remains `legacy` + identity policy `v1`. No request enters V2 unless the selected account explicitly has:

```json
{
  "codex_relay_mode": "relay_kernel",
  "codex_identity_policy_version": "v2"
}
```

Shadow mode runs V1 authoritatively and computes a local V2 comparison. It never sends a second upstream request.

## Layered architecture

1. **Request Plan**: created once at ingress from immutable request headers/body/operation/transport. It holds logical request and conversation digests, not credentials.
2. **Attempt State**: created after account/proxy/profile selection. Every retry gets a new state; body, application headers, identity headers, transport key and attempt ID come from that one snapshot.
3. **Conversation Registry**: Redis-backed CAS state keyed by HMAC conversation digest. An uncommitted record may be atomically replaced during failover. Once committed, account/profile/proxy/transport changes fail closed.
4. **Profile Catalog**: application identity, transport declaration, header order, capabilities, revision and fidelity metadata are one object. `auto` is a selector, not a seventh profile.
5. **Transport**: HTTP V2 uses the profile's Chrome Auto uTLS + Chrome HTTP/2 settings and a pool scope derived from account/credential/proxy/profile/transport configuration. WS cache and handshake keys include the same scope.
6. **Commit Guard**: heartbeats do not commit. First semantic output (`response.created`, output/content deltas, terminal semantic events, or non-stream success) commits the conversation. Stateful requests are treated as committed before replay.

## Profiles

| ID | Declared app version | HTTP | WS | Fidelity |
|---|---|---:|---:|---|
| `passthrough` | caller supplied | yes | yes | passthrough/degraded |
| `codex_cli` | 0.148.0 | yes | yes | degraded |
| `codex_exec` | 0.148.0 | yes | yes | degraded |
| `codex_desktop` | 0.148.0 | yes | yes | degraded |
| `opencode` | 1.2.4 | yes | no | degraded |
| `pi` | not asserted | yes | no | unsupported strict parity |

The version strings are catalog pins, not proof of exact wire parity. Strict native-client SChannel/Electron/Node profiles were not measured and verified in this change. Managed profiles therefore declare `degraded`; the implementation uses the repository's Chrome Auto uTLS and Chrome HTTP/2 profile. Unsupported WS claims are rejected.

`auto` inspects immutable inbound `originator` and `User-Agent` values. Recognized Codex CLI/Exec/Desktop/OpenCode/Pi callers select the corresponding profile; unknown callers select `passthrough`.

## Identity derivation

All managed values use HMAC-SHA256 namespace separation with `gateway.openai_affinity.secret` (minimum 32 bytes):

- Device identity: account version + credential version + profile revision.
- Conversation identity: canonical conversation digest + device identity.
- Turn/request identity: logical request + conversation + timestamp.
- Internal attempt identity: logical request + attempt number.
- Transport key: account + credential version + proxy + egress route + profile revision + TLS/H2 config version.

Stable identities use deterministic UUIDv4 formatting. Time-bearing request/attempt values use deterministic UUIDv7 formatting. The UUID version is semantic, not a substitute for entropy; the HMAC secret is the trust root.

No raw token, secret, installation ID, session ID, thread ID, turn ID, transport key or conversation digest is logged by Relay Kernel.

## Conversation state machine

```text
absent
  -> active, uncommitted (first attempt)
  -> active, uncommitted (CAS replacement on pre-output failover)
  -> active, committed   (first semantic output or stateful request)
  -> expired             (TTL)
```

Committed records cannot change account, profile, identity policy, proxy identity, egress route or transport config version. CAS conflict is fail closed. Redis errors are returned; there is no process-local fallback when Redis is configured.

## Finalization boundaries

- HTTP: the final request body and ordered identity headers are captured immediately before the upstream call. V2 attaches the attempt state and transport pool scope to the request context.
- WS/WSv2: the final `response.create` payload and ordered handshake headers are captured from the same attempt state. The Dial key includes transport scope.
- Retry: body and headers are rebuilt from the original Request Plan. A previous attempt's transformed body is never used as input.

The plugin transport remains first in the dispatch chain. If no plugin handles the request, V2 managed profiles use `DoWithTLS`; passthrough and legacy behavior keep the existing path.

## Configuration contract

Public account `extra` keys:

```text
codex_relay_mode: legacy | relay_kernel
codex_identity_policy_version: v1 | v2
codex_client_profile: auto | passthrough | codex_cli | codex_exec | codex_desktop | opencode | pi
codex_relay_shadow_enabled: boolean
codex_fingerprint_mode: off | device | session | window | window40 | full
```

Create, full update and JSONB merge update paths validate the same contract. Relay Kernel requires V2 and a valid global secret. Runtime/sensitive identity keys are stripped from account input.

## Observability

Shadow comparison records atomic totals for compared, mismatched and errors, and writes `codex_relay_shadow_mismatch` only for mismatches/errors. The log contains account ID and category names only.

Commit behavior is observable through existing retry/response handling tests. Operators should alert on:

- Registry/CAS errors.
- Stateful cross-account retry blocks.
- Shadow mismatch/error rate.
- WS policy-violation closes for unsupported profiles.
- HTTP profiled TLS errors and connection churn after profile/credential changes.

## Limits

- Exact native client TLS parity is not claimed.
- OpenCode and Pi do not declare WS support.
- Active connections retain their creation snapshot; configuration changes only affect new connections/attempts.
- Conversation registry data is operational state with TTL, not a permanent audit record.
