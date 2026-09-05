# Codex Conversation Pinning and Installation Migration

## Delivery Boundary

This runbook covers conversation pinning and installation migration. A subsequent
change adds explicit versioned HTTP/SSE bundle selectors; see
`internal/pkg/clientprofile/INTEGRATION.md` for their separate rollout gates.
Short selectors and existing pins are not automatically migrated. No claim of
native TLS/HTTP2 parity is introduced.

The requested delivery order is source/tests -> push GitHub main -> separately
approved production rollout. No production secret rotation, database migration,
Redis deletion, image release or VPS deployment is part of the source push.

## Runtime Contract

- New registry records contain a full `CodexClientProfile` snapshot, canonical
  SHA-256 digest, fingerprint mode and installation policy. Typed JSON is hashed
  after decoding, so Redis Lua object key reordering does not invalidate it.
- The resolved conversation wins over the account's desired profile, `auto`
  detection, fingerprint mode and installation policy. Capability checks and
  final HTTP/WS identity headers use this pinned profile.
- A concurrent creator adopts the winning snapshot instead of replacing a
  same-account uncommitted profile. Account/route recovery retains the existing
  replayability, availability and CAS gates.
- A commit checks account, route tuple, profile and all four identity IDs, not
  merely account ID. Successful activity CAS-refreshes the pin's TTL even when
  already committed. Corrupt or incomplete snapshots fail closed.
- Historical revision-one profiles are frozen in
  `openai_codex_legacy_profiles.json`. Preserve that file when introducing a new
  current catalog. Old records with no snapshot use this frozen catalog and
  retain their stored identity; successful commits backfill the new fields.
- Old records did not store fingerprint mode. Their first upgraded request uses
  the account's configured mode while retaining stored IDs. Do not change mode,
  profile, relay mode and installation policy together during migration.
- Successful HTTP and WS response ownership boundaries also persist a snapshot
  keyed by HMAC of group, authenticated user and response ID. A continuation
  adopts that snapshot even when no original prompt/session key is available.
  Existing HTTP/WS ownership authorization is still mandatory and unchanged.
- Response snapshots and continuation activity use the greater of strong-session
  and persistent response ownership TTLs (72 hours by default). Primary sessions
  without a previous response keep the strong-session TTL (24 hours by default).
  Include the longer retention in Redis memory preflight. Clock tests are not
  evidence of a production TTL observation window.

## Installation Policies

`extra.codex_installation_policy` is validated on account writes, including
JSONB merge and bulk paths. Account forms expose it only for Relay Kernel.

| Value | Behavior |
| --- | --- |
| absent / `legacy_v2` | Preserve the pre-migration device derivation for new conversations. |
| `stable_v1` | New conversations derive installation ID from account ID and client family. Requires Relay Kernel / identity v2 and managed fingerprint mode. |

CLI, exec and Desktop share the `codex` installation family. Pi, OpenCode and
passthrough have separate families. Stable derivation excludes access tokens,
account timestamps, proxy identity and profile revisions. Account and client
family remain isolation boundaries. Transport pools still isolate credential,
account configuration, route and complete pinned profile digest.

This is not a secret migration. Keep the effective HMAC secret AND its selected
source unchanged across both instances. The existing affinity-secret preference
and JWT fallback remain. Rotating either effective secret changes conversation
keys and stable installation derivation. No automatic secret rotation or
installation-reset API is included.

Old conversations retain their installation IDs, even after enabling or rolling
back the desired policy. Policy changes affect genuinely new bindings, not live
registry records. Bulk rollback writes the explicit `legacy_v2` sentinel; a
JSONB merge cannot remove a setting merely by omitting its key.

## Untracked Continuations

Pre-upgrade responses have no response snapshot. While on `legacy_v2`, these
retain the old continuation behavior for compatibility; their historical wire
version cannot be reconstructed from a response ID alone. This is NOT a promise
that pre-upgrade response chains are retroactively pinned.

On `stable_v1`, a missing response snapshot first checks authenticated response
ownership and the original account's availability. If the response-keyed legacy
registry record still exists, recover its actual IDs/profile and backfill on
success; the account's new policy must not reset that conversation. An expired
record must not be recreated between lookup and finalization. Missing ownership,
a different/unavailable account, corrupt data or absence of the original identity
still stops recovery. Caller-supplied root keys are not trusted substitutes.

Connection configuration version changes on the same account, proxy and route
can CAS-refresh transport scope while retaining identity and profile. Already
in-flight requests can commit without reverting the new configuration. Actual
proxy/route/account changes retain the original stateful recovery restrictions.

Client messages now distinguish ownership, missing identity, account mismatch,
account unavailability, route changes and OAuth refresh failure; terminal 409 and
no-cross-account-retry semantics remain. Automatic full-history replay and durable
snapshot storage are not implemented in this fix. Truly untracked chains still
need warming or explicit context recovery, not a fabricated legacy identity.

## Production Preflight (Not Yet Executed)

1. Obtain approval for the concrete account/group manifest, canary key, inference
   budget, observation duration and rollout window. User-visible reports must
   use full group names, not naked numeric IDs.
2. Inspect at least two hours of activity, classify HTTP/WS/Compact/resume and
   account types, check balances and response chains, and exclude unresolved or
   active untracked continuations from the activation manifest.
3. Verify shared effective identity secret without printing credentials or raw
   environment output. No new secret, Redis flush or affinity-key rotation.
4. Check Redis memory headroom and eviction policy. Each response snapshot adds
   registry state; budget using observed request rate, serialized record size,
   configured TTL and existing memory. Pins must not be evicted to make room.
5. Verify `/opt/sub2api/source-main` is clean, then update only that worktree with
   `git pull --ff-only origin main`. Protect `/opt/sub2api` Compose, `.env`, data,
   PostgreSQL and Redis directories from build contexts and cleanup.
6. Prebuild the image from the reviewed main commit with a version + short-commit
   tag. Check source HEAD, image ID and disk headroom before switching traffic.
7. Prepare one fresh full backup, manifest/CAS checks, cache/outbox assertions and
   a policy rollback rehearsal before the formal window. Do not enable stable
   policy during a mixed old/new-binary phase.

## Rolling Deployment (Requires Approval)

1. Recreate only `sub2api` on 8100 using `--no-deps`, keeping the current 8101
   primary up. Wait for Docker healthy and direct `127.0.0.1:8100/health` 200.
2. Back up `/etc/nginx/sites-enabled/sub2api.conf`, make 8100 primary and 8101
   backup, run `nginx -t`, reload, and verify production HTTPS `/health`.
3. Recreate only `sub2api-canary` on 8101 with the same image and `--no-deps`.
   Wait for Docker healthy and direct `127.0.0.1:8101/health` 200.
4. Restore 8101 primary / 8100 backup, validate and reload Nginx, and verify HTTPS.
5. Verify both healthy, identical image IDs/digests, correct primary order and no
   non-200 health requests during switching. Only then persist `SUB2API_IMAGE`
   in `/opt/sub2api/.env` and check `docker compose config --images`.

Never stop both instances, use `compose down`, include PostgreSQL/Redis in app
rebuilds, or assume `proxy_next_upstream off` retries a failed primary request.
If a health check fails, leave traffic on the healthy instance and stop rollout.

## Canary Acceptance

- Enable `stable_v1` only on the approved canary account after both readers are
  upgraded. Use a reviewed executor that validates merged extra and atomically
  CAS-checks the manifest's account snapshot. A normal UI PATCH alone is not a
  migration CAS. Preserve the existing mode and profile.
- Verify a new conversation and a warmed legacy conversation, then continue the
  returned response chain on the other instance. Verify account routing,
  installation/session/thread/window continuity and new per-turn request IDs.
- Exercise only supported HTTP/WS/Compact/resume paths; Pi/OpenCode WS and Compact
  are unsupported, not silently degraded successes.
- Check the final transmitted profile tuple as well as registry snapshots.
  Profile fixtures or UI screenshots alone are not production wire evidence.
- Observe a credential refresh and benign account edit without stable installation
  drift. Do not induce secret rotation as a test.
- Check cache/outbox convergence, quota/billing, 4xx/5xx, recovery/CAS conflicts,
  Redis memory and TTL renewal. Keep identity/request evidence hashed or redacted.
- Complete at least the approved two-hour activity window. A shorter smoke test
  must be labeled partial; it does not prove the configured full TTL (default
  24 hours). Expand to the batch manifest only after canary acceptance.

## Rollback

Keep the upgraded binary and use the reviewed CAS executor to restore desired
`legacy_v2` for new conversations. Verify cache/outbox convergence and retain all
existing snapshots. Stable conversations must finish on a snapshot-aware reader.
Do not delete Redis records, clear response ownership or rotate secrets.

A binary downgrade after stable activation is a separate operation: old binaries
ignore new fields and can drop them on CAS. First stop creating stable bindings,
drain affected conversations and observe the actual effective TTL, or use a
reviewed compatibility binary. The two-hour activity gate is not a substitute for
that drain. Ordinary health rollback during the pre-activation rolling update
may restore Nginx or the old image because stable policy has not been enabled.
