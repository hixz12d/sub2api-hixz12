# v0.2.5 integration

## Baselines and scope

- Local baseline: `d895da84f` (`main`, previously `0.2.4-hixz12.1-rc.1`).
- Backup branch: `backup/pre-v0.2.5-d895da84f`.
- Official release: `v0.2.5`, commit `86f93c28ee34cc74b629dafb748bd5ac5ca8c5ea`.
- Integrated upstream main: `881f32026` (release contents plus the official version-file update).
- Candidate version: `0.2.5-hixz12.1-rc.1`.
- Scope: local source integration and validation. No remote push or production deployment.

## Compatibility decisions

- Preserve independent model display configuration (`models_list_config`) and model admission (`model_allowlist`), local account priority modes, existing-user group access, private labels, question review and monitor fields.
- Preserve local outbound identity snapshots, account-specific TLS/egress policies, protected session headers, HTTP/2 recovery and retry budgets. OpenCode session derivation receives the new body inputs before final OpenAI identity enforcement.
- Combine upstream execution-scope isolation with local WebSocket ownership and handshake compatibility, keep client cancellation during pool acquisition, and force a fresh connection after an upstream failure. Retain local TLS profiles in all dialers and adapt upstream test doubles to that interface.
- Incorporate upstream reader-loop ping handling, pool wakeups and session preemption. Existing local diagnostic and route-generation protections remain.
- Preserve local monitor interval limits (9,600 seconds / 9,585 seconds jitter) while accepting the new OpenCode provider.
- Preserve local monitor usage fields and use the upstream image cache breakdown for billing. Preserve Astra's local operator prices and existing long-context overrides; incorporate upstream DeepSeek pricing changes and new Gemini fallback entries.
- Preserve 1,000-row usage export pages and missing-cost handling, while freezing filters/sorting/filename at export start. Both pagination and filter-change regressions are retained.
- Keep local account creation templates; add upstream OpenCode UI and provider presets, including the preset selection callback.
- Adopt the official default compact model `gpt-5.5`, retaining explicit configuration overrides and local affinity configuration.
- Restore upstream reactive balance/quota helpers now required by OpenCode and remove the duplicate test-only implementations. Kimi's local recoverable concurrency-403 handling remains.
- Responses stream events now include `sequence_number` even when zero, as required by upstream's compatibility fix. Sanitized error and single-terminal-event assertions remain.
- Apply upstream's raw User-Agent validation before whitespace trimming in the local identity resolver. Preserve the local family/version policy while rejecting malformed canonical or account identity inputs.
- Adapt the execution-scope regression to provide a fresh socket for each locally isolated session. Make the Ollama stale-generation test use distinct reset timestamps even on Windows' coarse clock; production CAS semantics are unchanged.

## Database changes

No previously tracked SQL migration was modified or deleted. The two official migrations retain their full filenames:

- `238_opencode_go_platform.sql` extends platform constraints to OpenCode while retaining MiniMax.
- `238_purge_unlimited_user_platform_quotas.sql` removes rows whose daily, weekly and monthly limits are all NULL. Absence of a row continues to mean unlimited; configured limits remain.

Migration bookkeeping uses full filenames and checksums, so these coexist with local `238_channel_monitor_group_foundation.sql` and later local migrations. Source validation does not prove production mixed-version behavior, cache/outbox convergence or live routing.

## Validation

Local logs and runner scripts are stored in the sibling `_integration-v025/` directory.

Completed:

- Frontend: all 309 Vitest files / 2,324 tests passed, with no unhandled test errors.
- Frontend `pnpm lint:check` and `pnpm build` passed. Build includes locale completeness and TypeScript checking.
- Backend `go build -p=4 ./...` passed on Windows with Go 1.27.0.
- Backend unit checks: the all-package run identified handler/service regressions, which were repaired. The complete handler package rerun passed (64.871s), and the complete service package rerun passed (239.677s); every other package passed in the all-package run.
- Backend integration checks: all packages other than service passed in the all-package run, including PostgreSQL/Redis repository tests, migration application and schema/idempotency checks. After the shared fixes, the complete service package rerun passed (169.032s).
- Deployment shell syntax, Compose security, gateway environment, runtime resource and Caddy cache checks passed.
- The Apple container mock lifecycle harness passed in a local Linux container with equivalent `stat` / JSON-extraction adapters and an LF copy of the Windows CRLF fixtures. No checked-in deployment scripts or assertions were relaxed. Native macOS / Apple container execution remains unverified.

- Final Linux `golangci-lint 2.13.2 run ./...` passed with **0 issues**, rerun against the final source after the identity and test fixes.
- Final staged whitespace checks passed; no unresolved merge entries remain. The only SQL changes are the two added official migrations listed above.

Initial failed logs are retained for traceability. The successful package reruns and builds above are the final validation evidence.

This is source validation, not a production release. No VPS health/routing checks, production migration, old/new dual-instance rollout, TTL/outbox observation or live upstream request validation was performed.
