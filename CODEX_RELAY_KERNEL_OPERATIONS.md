# Codex Relay Kernel Operations Runbook

## Preconditions

- Deploy only from the reviewed `main` commit in `/opt/sub2api/source-main`.
- Confirm `gateway.openai_affinity.secret` is configured with at least 32 UTF-8 bytes on both instances and is identical.
- Confirm Redis health and persistence policy. Relay Kernel fails closed without its distributed registry.
- Confirm both application instances use the same image digest, config and secret before enabling V2.
- Do not rotate the derivation secret during rollout.

## Pre-deployment verification

From the backend source directory:

```bash
gofmt -w internal/handler/openai_gateway_handler.go \
  internal/pkg/tlsfingerprint/*.go \
  internal/repository/*.go \
  internal/service/*.go

go test ./internal/service ./internal/repository -count=1
go test -race ./internal/service ./internal/repository -run 'Codex|RelayKernel|ConversationRegistry|TransportScope' -count=1
go test ./internal/service -run '^$' -bench 'Codex' -benchmem
```

Review the diff for raw token/secret logging and for any account `extra` values outside the documented public keys.

## Deployment

Use the repository's required dual-instance sequence. Do not restart both application containers together and do not include PostgreSQL/Redis in the application rollout.

1. Build a commit-tagged image from `/opt/sub2api/source-main` without stopping running containers.
2. Rebuild only `sub2api` on port 8100 with `--no-deps`; wait for healthy and verify `http://127.0.0.1:8100/health`.
3. Switch Nginx primary to 8100, run `nginx -t`, reload, and verify production HTTPS `/health`.
4. Rebuild only `sub2api-canary` on port 8101 with the same image and `--no-deps`; verify direct health.
5. Restore 8101 as primary, validate and reload Nginx, then verify production health.
6. Confirm both image digests match and persist `SUB2API_IMAGE` only after validation.

## Rollout stages

### Stage 0: Legacy baseline

Leave all accounts on `legacy` + `v1`. Record baseline request success, retry, WS reconnect, connection-pool and latency metrics.

### Stage 1: Shadow canary

Select one low-risk OpenAI OAuth account and set:

```json
{
  "codex_relay_mode": "legacy",
  "codex_identity_policy_version": "v1",
  "codex_client_profile": "auto",
  "codex_relay_shadow_enabled": true,
  "codex_fingerprint_mode": "session"
}
```

Observe at least one normal HTTP stream, one non-stream response, one WS turn/reconnect, one retry before semantic output, one Compact request and one continuation request if available.

Shadow must not increase upstream request count. Investigate every `codex_relay_shadow_mismatch` category; logs must contain no derived IDs or token material.

### Stage 2: Relay Kernel canary

Change only the canary account to:

```json
{
  "codex_relay_mode": "relay_kernel",
  "codex_identity_policy_version": "v2",
  "codex_client_profile": "auto",
  "codex_relay_shadow_enabled": false,
  "codex_fingerprint_mode": "session"
}
```

Verify:

- HTTP and WS use the same conversation identity across retries.
- A pre-semantic-output failure can switch account through registry CAS.
- After `response.created`/output delta, cross-account replay is blocked.
- Heartbeat-only traffic does not commit.
- Compact remains unary and resume stays on the committed account.
- Credential/profile/proxy changes create a new transport scope for new requests.
- Existing WS connections continue until normal close; rollout does not force disconnect.

### Stage 3: Batch expansion

Expand by small account batches. Keep `opencode` and `pi` on HTTP-only traffic unless their Profile capabilities are extended by measured evidence and tests. Do not label any managed profile exact while fidelity is `degraded`/`unsupported`.

## Monitoring

Alert or stop expansion on:

- Increased upstream 401/403/409/429/5xx rates.
- Registry Redis errors, CAS conflict spikes or “committed to account” errors outside deliberate tests.
- WS policy-violation closes, reconnect loops or handshake failures.
- HTTP TLS/H2 handshake errors or unexpected connection-pool growth.
- Shadow mismatch/error rate above the canary's explained baseline.
- Duplicate semantic output or any cross-account replay after commit.
- Secret, token or derived identity values in logs.

## Immediate rollback

Rollback is an account configuration change and affects new attempts/connections only:

```json
{
  "codex_relay_mode": "legacy",
  "codex_identity_policy_version": "v1",
  "codex_relay_shadow_enabled": false
}
```

Do not terminate healthy active WS connections solely to accelerate rollback. They retain an immutable V2 snapshot and close normally. If failures are severe, drain traffic from the affected instance/account through existing scheduler controls before touching containers.

If the application image itself must be rolled back, use the same dual-instance Nginx sequence with the previous known-good image. Never restart both instances together.

Do not delete Redis registry keys as a routine rollback step. Their TTL will expire. Manual invalidation is exceptional and requires confirming revision/account ownership first; deleting committed state can permit unsafe cross-account continuation.

## Secret rotation

Secret rotation changes all HMAC-derived conversation and transport keys. Treat it as a separate migration:

1. Disable Relay Kernel accounts back to legacy.
2. Wait for active V2 WS connections and strong-affinity TTLs to drain, or explicitly accept that continuity will be lost.
3. Update both instances to the same new high-entropy secret without exposing it in command history/logs.
4. Roll both instances with the dual-instance procedure.
5. Repeat Shadow and canary stages before re-enabling V2.

Never run two active instances with different derivation secrets.

## Evidence to retain

- Reviewed commit and image digest.
- Targeted/full/race test outputs and benchmark result.
- Redacted account IDs and rollout timestamps.
- Shadow metric deltas and mismatch categories.
- Health checks for both direct ports and production HTTPS.
- Nginx primary/backup state before and after rollout.
- Rollback decision, scope and residual active-connection risk, if used.
