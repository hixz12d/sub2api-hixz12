# N3 Isolated Acceptance

This follow-up supersedes the open functional gates in openai-n3-acceptance.md.
Evidence is for the current uncommitted worktree, not a deployed image.
No production database, real API key, customer balance, or paid upstream was used.

## Functional Gates

| Gate | Verification | Result |
| --- | --- | --- |
| Candidate ownership | Real HTTP handler with injectable affinity repository: failed A preamble is not persisted; successful B persists before public output | PASS |
| Ownership write failure | Repository failure before public output: one dispatch, no message_start/text leakage | PASS |
| Persistent rollback | Actual PostgreSQL: response ownership conflict rolls back session strength/version changes | PASS, three runs |
| Concurrent ownership | Actual PostgreSQL with fixture-only insert delay: two competing accounts produce one winner and one explicit conflict | PASS, three runs |
| Unknown/hosted events | Real HTTP handler, account switching enabled: unknown event, hosted search event and unknown item cannot dispatch a replay | PASS, three race runs |
| Local staging creation | Linux Messages parser pipeline, invalid TMPDIR: local error, no public bytes, no H2 failure report, no scheduler initialization | PASS, three race runs |
| Staging write/seek/read | Closed-file write/seek and truncated read; temporary path removed and repeated Close safe | PASS, three race runs |
| Billing rollback | Actual PostgreSQL and production Apply: fail after balance/key updates, assert balance, quota and dedup all unchanged | PASS, three runs |
| Concurrent billing | Twelve concurrent copies of one request: one Apply, one dedup row, balance 100 -> 98.75 and quota 0 -> 1.25 | PASS, three runs |
| Existing billing paths | Balance/subscription dedup, fingerprint conflict, OAuth/account quota and scheduler outbox tests | PASS, three runs |

The HTTP handler tests use a fault-injected repository; PostgreSQL transaction
properties are checked separately using the production repository and schema.
This is layered isolated verification, not a claim that the HTTP fixture runs
against a live production database or that failed upstream attempts incur no cost.

## Defects Corrected

- BindResponseAndUpgrade now rechecks the account after its conditional upsert.
  A concurrent winner can no longer be returned as another account's success.
- Messages response IDs remain candidates until the first public commit. A
  persistence failure closes replay without publishing output. A successfully
  persisted binding is retained if a later public write fails; deleting it could
  orphan an ID already partially delivered to the client.
- Unknown/provider-hosted activity closes replay permission without claiming
  successful downstream semantic delivery. Ordinary converted output can continue.
- Local staging and ownership failures have a distinct error identity and bypass
  account-health/scheduler failure penalties; spool read/seek failures preserve
  local attribution instead of being labeled client disconnects.

## Repository Checks

The first full backend unit run found six failing top-level tests. Review found
stale assertions for canonical tool IDs, empty-ID removal, enforced client version
and internal error evidence. They were updated to existing contracts, keeping
paired-tool alignment and no-public-leak assertions. An existing cancellation path
also dropped an already generated image result; the fix preserves partial image
billing data plus the original cancellation error without enabling image retries.
All six failing groups pass when rerun independently.

Final go vet ./... and backend server build passed (exit 0). Final full unit
execution completed with exit 1 and 18,749 passing test/subtest events, but nine
failing top-level tests: four PluginPackageInstaller tests (Windows open-file
rename), TestShouldClearStickySession, and four IsUpstreamModelRestrictedByChannel
mapping tests. They are NOT accepted or silently waived.
Correction: the first full unit attempt also hit its 180-second timeout, so its
six observed failures were not the complete failure inventory. The final run used
600 seconds and reached the later tests. Structured logs remain in the temporary
directory: sub2api-n3-final-unit.jsonl, sub2api-n3-final-vet.log,
sub2api-n3-final-build.log, with corresponding .exit files.

The original golangci-lint v2.9.0 could not read Go 1.27 export data even when
rebuilt with Go 1.27. A Go 1.27-built v2.13.2 was installed only under a
versioned temporary GOBIN. Project go.mod/go.sum and global tools are unchanged.
It reported 42 findings before its five-minute timeout (exit 4): errcheck 11,
gofmt 3, gosec 1, ineffassign 1, staticcheck 15, unused 11. This is neither a
passing nor necessarily exhaustive lint report. The new n3_stage_fault_test.go
unchecked deferred Close was corrected after this report. The remaining findings
are tracked separately; unrelated worktree changes were not rewritten.

## Commands And Evidence

```sh
CI=true go test -tags=integration \
  -run 'TestN3|TestUsageBillingRepositoryApply' -count=3 \
  -timeout=90s ./internal/repository

go test -tags=unit -race -p 2 -timeout 180s \
  -run 'TestN3|TestMessagesAcceptance|TestForwardAsAnthropic|TestHandleAnthropicStreamingResponse|TestMessagesStage|TestOpenAIRecoverySafety|TestCopyOpenAIUsage|TestBlueprintV2|TestOpenAIAffinity' \
  -count=3 ./internal/service ./internal/handler
```

PostgreSQL/Redis integration: 24.783s, exit 0. CI=true prevents silently skipping
when Docker is unavailable. Containers are created by the repository harness.
Linux race container sub2api-n3-isolated-final: service 42.706s, handler 40.790s,
both PASS. The separate sub2api-n3-image-regression race check covers the later
partial-image cancellation fix: three repetitions, 1.168s, exit 0. All launched
test, build, vet and lint processes have ended.

## Deployment Boundary

No commit, push, deployment, production migration or cleanup was performed.
Production topology/permissions/proxy behavior and rolling-release health checks
remain release-time checks, not a substitute for these local fault tests.
Frontend checks are not claimed; this change and its full-package checks concern
the backend. No unrelated credential-sync or frontend worktree changes were reverted.
