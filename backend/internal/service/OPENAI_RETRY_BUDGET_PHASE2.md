# OpenAI RetryBudget Phase 2 Evidence

## Repository baseline

- Branch: `feat/openai-cross-account-cache-isolation`
- Phase 2 start HEAD: `7535478ea4647183000b0e222526d5df96be9b44`
- Origin `main` and `feat/openai-cross-account-cache-isolation` at start: `63cc910b4b5ce7cf9c3c081d249ea99cc7be7563`
- Worktree was clean at start.
- Applicable repository rule: parent `AGENTS.md`; no repository-local `AGENTS.md` or `CONTRIBUTING.md`.
- Phase 1 `CodexIdentitySnapshot` and finalizer remain the final managed identity writer.

## Pre-change retry source-to-sink

| Layer | Existing retry/failover behavior before Phase 2 |
|---|---|
| Responses handler | Account-selection loop, pool same-account retry count, account-switch limit, and one pre-output reacquire round |
| HTTP Forward | Agent-task recovery, invalid-encrypted-content recovery, and rejected-field compatibility recovery each used local loop state |
| Passthrough | Agent-task recovery used another local loop |
| WSv2 over HTTP | Up to `openAIWSReconnectRetryLimit + 1` executions (currently 6), plus previous-response and encrypted-content recovery |
| WS ingress | Per-turn reconnect and `previous_response_not_found` recovery used independent counters |
| OAuth token | Pre-expiry refresh already used local/distributed credential locks, but a 401 response did not share the model-request retry count |
| Handler failover | Could wrap all inner retry layers and move through configured account candidates |

The old maximum was therefore configuration-dependent and multiplicative: handler account attempts multiplied by pool retry, HTTP compatibility retry, or up to six WS executions. It was not represented by one enforceable logical-request ceiling.

## Phase 2 ownership and propagation

- Account feature flag: `openai_retry_budget_v2`.
- Default: enabled for OpenAI OAuth accounts when the extra key is absent; an explicit boolean `false` disables it. API-key/custom-upstream paths never activate the OAuth budget even if the extra key is present.
- HTTP owner: handlers may prepare one `*OpenAIRetryBudget` in the Gin request context before account selection; selecting an eligible OAuth account activates it, while selecting an ineligible or explicitly disabled account deactivates it. Bridge, passthrough, internal HTTP retries, handler failover, and selected-account changes reuse that pointer.
- WS ingress owner: every genuine `response.create` turn calls `StartOpenAIRetryBudgetTurn`; `skipBeforeTurn` reconnect/recovery paths retain the same pointer.
- Reserve points are immediately before a real HTTP upstream execution, WS execution/Dial path, ingress turn execution, or WSv2 passthrough turn write. Local validation before those points does not consume an attempt.
- Defaults: `max attempts = 2`, `max elapsed = 20s`; stateful requests allow one account, and strictly classified stateless first turns allow at most two accounts.
- The structure is mutex-protected and safe for concurrent reserve/output/refresh state changes.

## State and distinct-account policy

The Phase 2 classifier is deliberately local and non-persistent. It treats malformed payloads, `previous_response_id`, `function_call_output`, encrypted content, encrypted reasoning, and a nonblank `x-codex-turn-state` header as stateful. Stateful requests cannot reserve a second account. A syntactically valid first-turn body without those state signals may reserve one fallback account, but no request can exceed two total executions or scan the pool.

A credential failure records `credential/account`; a subsequent reserve on a different account is rejected even for a stateless request.

## Failure classifier

| Failure | Class/scope | Same account | Other account |
|---|---|---:|---:|
| 400/404/409/422 and default 4xx | request/request | no | no |
| 401 | credential/account | once after refresh | no |
| 403 | request/request | no | no |
| 408 | transient/transport | yes | stateless first turn only |
| 429 | rate_limit/account | yes | stateless first turn only, within budget |
| 500/502/503/504 | transient/account | yes | stateless first turn only, within budget |
| status 0 transport error | transport/transport | yes | stateless first turn only, within budget |
| client cancel | canceled/request | no | no |
| any failure after output gate | state/state | no | no |

Existing scoped health/cooldown handlers remain responsible for side effects; the budget prevents those handlers from turning one failure into full-pool traversal. Phase 2 adds no health-state schema.

## First-byte / first-event gate

- Existing HTTP Responses and passthrough stream parsers call `MarkOpenAISemanticOutputStarted` on the first semantic event; this now closes the shared replay gate.
- Handler writer-size evidence marks actual emitted HTTP bytes.
- WSv2-over-HTTP marks semantic output and committed bytes after a successful downstream frame write.
- WS ingress marks committed bytes only after `clientConn.Write` succeeds.
- Compact heartbeat remains non-semantic and does not close replay by itself.
- Once `streamStarted` or `bytesEmitted` is set, every later reserve fails with `ErrOpenAIRetryBudgetExhausted`.

## 401 refresh

`OpenAITokenProvider.RefreshAfterUnauthorized` calls the existing `OAuthRefreshAPI` with a forced-refresh option. The existing API still owns:

- process-local credential lock;
- optional distributed cache lock;
- account DB re-read under the lock;
- credential persistence and token version update.

The forced path compares the observed access token and `_token_version` after acquiring the lock. Concurrent waiters reuse the winner's new token instead of refreshing again. Each RetryBudget permits this recovery once; the retried HTTP execution consumes attempt 2 and must stay on the same account.

## WS and semantic recovery

- WSv2-over-HTTP reserves from the shared budget for every execution, reducing the enabled path from a possible six executions to at most two.
- WS ingress creates one budget per real turn; reconnect and retry paths reserve from that same turn budget.
- Successful first downstream WS event closes replay. A later disconnect is surfaced instead of replaying `response.create`.
- `previous_response_not_found` recovery remains same-account, requires no `function_call_output`, is allowed once by both legacy guard and RetryBudget, and consumes the second execution.
- Reconnect keeps the Phase 1 identity snapshot and account binding; this phase does not rewrite scheduler or persistent affinity semantics.

## Compatibility and rollback

- Set account extra `openai_retry_budget_v2: false` to deactivate RetryBudget for that OAuth account. This does not disable independent egress, affinity, stateful-request, or `max_account_switches` safeguards.
- Explicit flag-off retains legacy inner reconnect/retry behavior only where no independent hardening rule blocks it; it is not a complete rollback to unrestricted cross-account replay.
- API-key/custom upstream controls remain outside OAuth RetryBudget v2.
- The RetryBudget mechanism requires no schema migration or persistent binding and does not change API-key/custom-upstream controls.
- Re-enabling legacy retry behavior can restore multiplicative attempts; operations must alert on attempts/request, distinct accounts/request, partial streams, and retries after output before using that escape hatch in production.

## Verification boundary

Required local verification is intentionally focused:

- RetryBudget/classifier/output/distinct-account tests;
- concurrent forced-401 refresh winner test;
- Phase 1 finalizer and selected WS recovery/reconnect tests;
- service and handler compile-only tests;
- `gofmt` and `git diff --check`.

The optional `-tags unit` OAuth refresh subset is currently blocked by a pre-existing duplicate test declaration (`TestGetModelPricing_Grok46OfficialFallback` in `billing_service_test.go`). The new no-tag concurrent forced-refresh test covers the Phase 2 lock/version invariant without changing that unrelated test file.
