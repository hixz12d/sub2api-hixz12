# OpenAI Phase 3 durable affinity evidence

## Scope and defaults

Phase 3 adds database-backed OpenAI OAuth session and response ownership without changing the frozen Phase 1 Codex identity finalizer or the default-off Phase 2 RetryBudget contract.

The path is disabled unless both configuration switches are explicitly enabled:

```yaml
gateway:
  openai_affinity:
    enabled: false
    writes_enabled: false
    secret: ""
```

`secret` must be stable across process restarts and contain at least 32 bytes. Owner scope includes group, user, and API-key identity before HMAC derivation. Raw session IDs, response IDs, prompt cache keys, user IDs, API-key IDs, or legacy session inputs are never persisted by the new tables; database keys use domain-separated HMAC-SHA256 digests.

## Persistent model

Additive migration `backend/migrations/222_openai_affinity_bindings.sql` creates:

- `gateway_session_bindings`: owner/provider/capability-scoped weak, explicit, or strong account ownership with TTL and CAS version.
- `gateway_session_binding_aliases`: bounded HMAC aliases for compatible explicit, prompt-cache, and legacy session signals.
- `gateway_response_bindings`: response-ID ownership that outranks session and ordinary scheduling.
- `gateway_affinity_migration_audit`: immutable reason/version/account evidence for manual CAS migration.

No prior migration is modified. No migration was executed while implementing this phase.

## Source-to-sink precedence

1. `GenerateSessionHash` / `GenerateSessionHashWithFallback` attaches a unified `SessionIdentity` before account selection.
2. `previous_response_id` produces strong, non-replay-safe state and first resolves `gateway_response_bindings`.
3. Persistent session ownership resolves next through primary and bounded alias hashes.
4. Legacy Redis sticky state remains a temporary compatibility read path.
5. Only a true cold miss reaches ordinary load-aware selection.
6. Before any cold candidate can execute upstream (including a wait-plan candidate), `CreateOrGetSession` atomically elects one durable winner. A losing candidate releases its acquired slot and is replaced by the winner.
7. A strong binding bypasses transient cooldown, quota, profit, soft-spillover, and health-score account switching. Permanent incompatibility or manual account unavailability fails closed instead of drifting.
8. Phase 2 reads the attached `SessionIdentity` before its first `Reserve`; strong/stateful identity limits the request to one distinct account.

## Response ownership final wire

Persistent response ownership is written before the first client-visible response wherever the upstream response ID is observable:

- Responses HTTP streaming and non-streaming, including SSE-to-JSON and compact bridge paths.
- OAuth passthrough streaming and non-streaming.
- Anthropic Messages bridge events.
- WebSocket ingress and WebSocket v2 upstream events.

`BindResponseAndUpgrade` transactionally writes response ownership and upgrades the related session binding to strong ownership. Legacy in-memory/Redis response bindings continue as compatibility evidence, but durable ownership has higher precedence.

## TTL and expiry

TTL values are bounded in code. Defaults are:

- response: 72 hours;
- strong session: 24 hours;
- explicit session: 12 hours;
- weak session: 30 minutes;
- refresh-on-hit minimum interval: 5 minutes.

Expired rows can be atomically re-elected. Unexpired strong ownership does not drift from transient failures. Refresh-on-hit is write-throttled by the configured minimum interval.

## Manual migration tool

`go run ./cmd/openai-affinity-migrate` is preview-only by default. It previews exactly one binding per plan so apply remains one transaction and one CAS decision.

Preview example (not executed during implementation):

```bash
go run ./cmd/openai-affinity-migrate \
  -database-url "$DATABASE_URL" \
  -from-account 101 -to-account 202 \
  -reason "operator-approved account drain" \
  -plan-out affinity-plan.json
```

Apply requires the saved plan and its exact digest:

```bash
go run ./cmd/openai-affinity-migrate \
  -database-url "$DATABASE_URL" \
  -apply -plan-in affinity-plan.json \
  -confirm '<digest from plan>'
```

The target is revalidated as active, schedulable OpenAI OAuth before apply. CAS rejects stale plans. Session ownership, linked response ownership, and migration audit are changed in the same transaction.

## Rollback

1. Set `gateway.openai_affinity.writes_enabled=false` to stop new persistent writes while retaining durable reads.
2. Set `gateway.openai_affinity.enabled=false` to return to legacy Redis sticky and existing scheduler behavior.
3. Keep migration 222 tables in place; rollback does not require destructive DDL or binding deletion.
4. Phase 1 `codex_identity_finalizer_v2` and Phase 2 per-account `openai_retry_budget_v2` flags remain independent.

## Verification boundary

Completed locally:

- focused `TestOpenAIAffinity*` tests, including 100 concurrent cold candidates converging on one winner, HMAC owner/group isolation, response-to-strong upgrade, before-output binding, RetryBudget state constraint, migration digest/CAS, and default-off control;
- compile-only checks for service, repository, handler, and migration CLI packages;
- formatting and `git diff --check`.

Blocked/not claimed:

- PostgreSQL migration execution, restart recovery against a real database, and database-backed 100-concurrent-cold-start acceptance were not run because no local test PostgreSQL URL/server was available. Production or real database use was prohibited. The in-memory concurrency test verifies scheduler arbitration semantics but is not presented as database integration evidence.
- full-package, race, full-repository, push, merge, release, and deployment verification are outside this phase's authorized minimal-test boundary.
