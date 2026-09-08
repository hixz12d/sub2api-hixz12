# Messages Acceptance Checkpoint

Scope: current uncommitted worktree; local fixtures only. This is a targeted
acceptance checkpoint, not complete N3 or production billing approval.

## Confirmed Defects Fixed

1. Empty successful response.completed could leave message_start/message_stop
   buffered and return an empty response. Completed empty streams now publish the
   staged terminal batch without manufacturing a text TTFT.
2. Visible TTFT and FirstTokenMs were recorded before a successful stage write.
   Zero-byte and partial write errors no longer record a successful visible TTFT.
   An attempted public write still closes replay permission; usage is drained when
   the upstream remains readable.
3. Finalization ignored stage commit errors. Phase failures now propagate;
   downstream write failure is tracked as a disconnect rather than success.
4. N4 preparation cancellation checks also affected a non-streaming compatibility
   path. Those new checks now run only when preparation protection is active.

The old MissingTerminalAfterClientDisconnect fixture sent only a preamble, which
is no longer written publicly. It now sends a text delta to actually reach the
failing writer. Its original no-replay/no-upstream-blame assertions remain.

## Verified Locally

- TestMessagesAcceptanceEmptyCompleted.
- TestMessagesAcceptanceFailedWriteDoesNotRecordVisibleTTFT: zero and partial
  write failures, no replay error, no successful visible TTFT, final usage retained.
- TestMessagesAcceptanceHTTPFailoverUsageIsolation: real HTTP client, handler,
  conversion and staging; two actual dispatches; attempt A's preamble/header do
  not leak into B's response; one local usage row for B with expected token counts.
- TestMessagesAcceptanceHTTPPingThenFatalError: the client observes a real ping
  before the upstream emits a non-retryable failure; one dispatch, SSE error,
  no staged message_start or private attempt header leakage.
- Existing ForwardAsAnthropic, MessagesStage, streaming error, canonical usage,
  recovery-safety and BlueprintV2 regressions pass in the unit-tag targeted run.

The HTTP usage fixture runs SIMPLE mode and injects the real pricing service plus
an in-memory UsageLogRepository. It proves local usage attribution and insertion,
not wallet debits, transactional billing idempotency, subscription/quota updates,
provider-side billing, or the absence of upstream costs on failed attempt A.

## Release Gates Still Open

- Candidate response ownership persistence/failure/rollback is not yet verified
  through a fault-injected persistent repository in the full handler path.
- Unknown and hosted-tool event replay safety still needs the complete real-input
  matrix, not only the existing generic recovery-safety unit tests.
- Local staging I/O failures need end-to-end attribution/account-health assertions.
- Transactional billing and complete unit/integration/lint remain separate gates.
- No commit, push, deployment, or production configuration change was performed.

## Repeat Verification

The first broad Linux Go 1.27 unit-tag race run (count=3, container
sub2api-n3-acceptance-20260908) passed handler in 38.380s but failed service:
parallel legacy tests in openai_compat_model_test.go called gin.SetMode after
entering t.Parallel. This is a real fixture race, not a passing run.
The 26 same-file mode initializations now occur before t.Parallel, preserving
parallel request testing without concurrent writes to Gin's global mode.
Service rerun in sub2api-n3-acceptance-service-20260908 passed all three repetitions
in 32.805s, exit code 0. Combined with the unchanged handler pass (38.380s), the
targeted acceptance set is race-verified. Both test containers have exited.
Scoped git diff --check also passed. The open release gates above remain open.

```sh
go test -tags=unit -race -p 2 -timeout 180s \
  -run 'TestMessagesAcceptance|TestForwardAsAnthropic|TestHandleAnthropicStreamingResponse|TestMessagesStage|TestOpenAIRecoverySafety|TestCopyOpenAIUsage|TestBlueprintV2' \
  -count=3 ./internal/service ./internal/handler
```
