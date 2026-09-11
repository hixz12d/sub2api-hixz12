# v0.2.4 Full Integration — Local Candidate

## Scope and baselines

- Candidate worktree: `sub2api-hixz12-v024-full`, branch `integrate/v0.2.4-full`.
- Merge base checkout: `26228978ee40a1abbb876f8cd17b266ef349c504`.
- Upstream release: `v0.2.4`, commit `5de5e2bed035d43591a2e10e51f420ef6a84eb98`.
- Latest local main incorporated: `8b4f511c8c615ab3683b7184e0322198e021c6ce`.
- Version retained from the existing candidate: `0.2.4-hixz12.1-rc.1`.
- At the source-validation checkpoint only the candidate had been modified;
  neither other worktree nor production had been changed.
- Main's last four commits were composed as a patch onto the existing resolved
  worktree. Publication must retain both main and upstream ancestry via normal
  merge commits and verify the final main tree matches the validated candidate.
- The user subsequently authorized committing/pushing to `origin/main` and
  removing the two integration worktrees. Keep `sub2api-hixz12-main`, its local
  untracked handoff files, and validation evidence. Archive the abandoned dirty
  integration worktree before removal. This does not authorize deployment.

## Compatibility decisions

1. **Model display and admission remain independent.** Preserve
   `models_list_config` and its `enabled/models` JSON contract. Add
   `model_allowlist` independently, disabled by default. Never copy old display
   preferences into admission. Preserve both columns, admin inputs, DTOs,
   repository projections, auth snapshots, listing filters and UI controls.
2. **Do not use the old integration branch's 249 synchronization migration.**
   That script copies values in both directions and would silently enable
   admission restrictions from legacy display-only settings.
3. **Keep historical SQL immutable.** No pre-existing main SQL migration is
   modified or removed. Migration bookkeeping uses full filenames/checksums.
   Upstream `232/233` request-ID and `234` manifest migrations duplicate local
   `234/235/236` DDL, differing only in comments. Both sets are retained; their
   `ADD COLUMN IF NOT EXISTS` / `CREATE INDEX CONCURRENTLY IF NOT EXISTS` clauses
   are idempotent. Fresh-database migration execution passed in the repository
   integration harness. This is not evidence from a production database.
4. **Keep local identity and transport boundaries.** Main's managed client
   presets, opt-in API-key stateless policy, version synchronization and HTTP/2
   recovery changes are included. The final identity tuple is applied together;
   pinned profile/version and `doOpenAIUpstream` protections are retained.
5. **Keep local billing.** Astra's operator card remains input/cache-write/
   cache-read/output `$10/$12.5/$2/$50` per million tokens, with existing tier
   and long-context policies. Upstream tests were adapted to that contract,
   rather than changing production prices to make those tests pass. New Astra
   `ultra` metadata is retained; the local default remains `low`.
6. **Keep fail-closed behavior.** Generic 403 must not bypass the local retry
   budget to rotate accounts. Astra pro/max preservation is tested across 503
   failover; a separate 403 regression asserts no rotation. Model-not-found 400
   failover requires a managed OpenAI-compatible account; bare or incompatible
   forwarding services retain terminal errors. Managed proxy fixtures explicitly
   use active status instead of weakening the egress resolver.

## Minimal additional repairs in this pass

- Composed the four newer main commits: 127 file patches applied cleanly, one
  version-forwarding test already matched, and four overlaps were inspected.
  Two overlaps were already equivalent; HTTP/2 failure-window logic and final
  identity-version handling were completed manually.
- Removed one duplicate `gpt-6-astra` model entry and added a uniqueness regression.
- Restored switch roles and checked-state accessibility attributes on the two
  Codex manifest toggles, preserving existing reactive selection behavior.
- Updated affected test mocks/fixtures for MiniMax, user-ranking visibility,
  active proxies, current todo-guard marker and Astra metadata.
- Added an SQL-level old/new write round-trip test proving display/admission
  independence, including repair replay and disabling admission. Extended the
  durable cache-invalidation test to cover both configuration fields.

## Verification

Completed:

- Windows `go build ./...`.
- Frontend `pnpm typecheck` and `pnpm build` (including locale completeness).
- Frontend initial focused suite: 10 files / 135 tests passed.
- Final frontend suite: `pnpm exec vitest run --maxWorkers=4 --minWorkers=1`,
  all 285 files / 2079 tests passed, with no unhandled errors. The account-list
  test now drains its filter debounce and unmounts before mocks are reset.
- Backend focused model, Astra, raw-chat, failover, identity and pricing regressions.
- Docker PostgreSQL/Redis repository integration tests for model allowlist,
  group repositories, auth invalidation, projections, proxy/session behavior.
- Additional PostgreSQL tests for independent rolling writes, both configuration
  invalidation events, and migration repair all passed.
- Backend all-package unit run passed every package except `internal/service`;
  its assertions were repaired and the 180-second total package budget was too
  short. Final `go test -tags unit ./internal/service -count=1 -timeout=600s`
  passed (207 seconds), including first-output timing checks. No production
  timeout limits were loosened. Focused regressions also passed after repairs.
- `git diff --cached --check` and `git diff --check` passed. All merge resolutions
  are staged; the unmerged-file count is zero. Upstream prompt trailing spaces
  were trimmed solely to satisfy the whitespace check.

The local source candidate is integrated and verified as above, but not a
production release. A Linux/amd64 embedded cross-build was attempted but hit the
180-second command limit; no successful Linux artifact or runtime is claimed.
No full two-binary deployment, production TTL observation or live routing check
was performed. Publishing the source to main does not change those boundaries.

## Evidence and deployment boundary

Session evidence is in the sibling `_integration-20260911/` directory: original
worktree patch/index backup, main increment patch and application inventory,
backend/frontend logs and database regression logs. These are local working
artifacts, not secrets or deployment inputs.

Not verified: two old/new binaries running concurrently against shared production
state, production TTL/outbox convergence, live upstream routing, load/latency,
Linux runtime or a production rolling deployment. SQL round-trip tests must not
be described as a complete mixed-binary rollout. Production deployment requires
separate authorization and the project's prescribed healthy dual-instance
8100/8101 switching sequence.
