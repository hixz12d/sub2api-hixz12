# Profile Foundation Verification

Baseline: `670b1844f7334bd4e09dfa28d6f6146c5b8c45cb`.
Commands ran locally on Windows. Go selected the module's 1.27.0 toolchain;
`go.mod` and dependency lockfiles were not modified.

## Passed

- Candidate package unit tests, including an HTTP loopback receiver (not the
  production finalizer, plugin or uTLS stack).
- Focused Codex/Relay Kernel tests in service/repository/handler.
- Full `internal/repository` tests.
- `go vet ./internal/pkg/clientprofile`.
- `git diff --check` for tracked changes.

## Full Suite Not Green

Command:

```sh
go test ./internal/pkg/clientprofile ./internal/service ./internal/repository ./internal/handler -count=1
```

Candidate and repository packages passed. Service had 14 failing top-level
tests, handler had 2. All 16 failures reproduced with a Go build overlay using
HEAD's original `openai_codex_profile.go`, the only pre-existing production Go
file changed in this batch. The candidate package has no production consumer.
The overlay did not change the working tree. This is a focused baseline
reproduction of those failures, not a second full-suite baseline run.

Service failures:

```text
TestCalculateTokenCostContextTierEnablement
TestNewOpenAIStreamFailoverErrorPreservesCapacityReason
TestFetchCodexModelsManifestAPIKeyUsesOfficialOpenAIModelsEndpoint
TestDeepSeekResponsesForwardRestoresClientToolsStreaming
TestDeepSeekAdaptiveResponsesForwardRestoresClientToolsNonStreaming
TestDeepSeekResponsesCompactSkipsClientToolAdaptation
TestOpenAIGatewayService_Forward_PreservesPartialImageResultOnStreamCancellation
TestOpenAIGatewayServiceForwardImages_OAuthPassesNAndReturnsAllImages
TestPluginPackageInstallerInstallUnsignedDevelopmentPackage
TestPluginPackageInstallerAllowsRepeatedIdenticalUpload
TestPluginPackageInstallerVerifiesTrustedSignature
TestPluginPackageInstallerKeepsHostVersionMismatchDisabled
TestPricingOverride_ExplicitZeroThresholdDisablesCatalogLadder
TestPricingOverride_DisablesGPT55LadderOnDefaultCatalog
```

Handler failures:

```text
TestOpenAICompatibleTextTargetAllowsCompositeProviders
TestOpenAIResponsesWebSocketV2PassthroughCyberMarkIsConsumedAfterTurn
```

These unrelated implementation/test expectations were not edited. A production
release cannot be described as fully verified from this result.

## Unavailable / Not Run

`go test -race ./internal/pkg/clientprofile -count=1` failed before test execution:
`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`.
No gcc/clang/clang-cl was found on PATH. No compiler was installed globally.

No paid inference, production HTTP/WS request, credential refresh, deployment,
Redis migration, secret rotation, shared Python bundle integration or full
blueprint acceptance matrix was performed. UI verification is tracked separately.


## UI Follow-Up

The automatic UI child timed out without changes. Parent implementation updated
only existing client profile presentation and strict boolean extraction; no
candidate bundle, installation reset or migration control was exposed.

- 21 profile/settings/schema tests passed, including 12 new capability tests.
- 80 account create/bulk-edit regression tests passed.
- Frontend typecheck and production build passed. Existing Browserslist age,
  mixed dynamic/static imports and large-chunk warnings remain.
- Playwright with installed Microsoft Edge rendered the isolated real Vue
  component at 1280x900 and 390x844, with no horizontal overflow or page errors.
  Profile selector interaction changed WS status from unsupported to pending.
  The isolated harness used the real locale strings as message functions; this
  is not an authenticated end-to-end admin/account-save test.
- Screenshots are local temporary artifacts: `sub2api-profile-desktop.png` and
  `sub2api-profile-mobile.png` under the Windows user temporary directory.
- Local Vite preview listens at `http://127.0.0.1:5174`, with production/backend
  access disabled by proxy target `http://127.0.0.1:9`. Actual account login and
  save require a separately configured local backend. File-watching polling is
  process-local to avoid Windows EBUSY on atomic editor temporary files.


## Conversation Migration Follow-Up (2026-09-05)

The earlier sections describe the first batch, not the final implementation.
The second batch adds runtime profile snapshots/digests, frozen legacy catalog
fallback, stable installation opt-in, activity TTL refresh, stronger commit
ownership checks and tenant-scoped response-chain snapshots. The candidate
bundle package remains offline. See
`backend/internal/service/CODEX_CONVERSATION_MIGRATION.md` for compatibility,
preflight, rollout and rollback requirements.

Final local verification:

- Focused candidate/Codex/Relay Kernel tests passed in service, repository,
  handler and clientprofile packages. Fifteen new top-level backend tests cover
  migration, response chains, snapshot validation, CAS races and Lua TTLs.
- Linux Go 1.27 race tests passed in clientprofile, service and repository using
  the existing `golang:1.26.5-bookworm` image with `GOTOOLCHAIN=auto`, `-race -p 2`
  and the focused test selector. Final container exit was 0; run interval was
  `2026-09-05T09:10:39Z` to `2026-09-05T09:13:51Z`. Earlier toolchain/configuration
  and mid-edit compilation attempts are excluded from the passing evidence.
- 157 frontend tests passed across profile capabilities, installation policy,
  relay schema/settings and create/edit/bulk-edit account forms.
- Frontend typecheck/production build passed; `go test -tags embed ./cmd/server
  -run '^$'` compiled the server with embedded frontend assets (no server tests).
- Playwright Edge rerendered the final component at 1280x900 and 390x844 with no
  horizontal overflow or page errors. This remains an isolated UI smoke check,
  not authenticated production account saving.
- Full Windows regression passed clientprofile/repository and reproduced the
  same 14 service + 2 handler top-level failures listed above. No additional
  failing top-level test appeared. The suite is still NOT fully green.

Temporary evidence files are local only: `sub2api-profile-migration-full.jsonl`,
`sub2api-profile-migration-frontend.log`, `sub2api-profile-migration-race.log`,
and the two UI screenshots in the Windows temporary directory.

The user explicitly selected GitHub main push first, production later. No VPS
connection, production credential use/refresh, production Redis write, paid
inference, image release or deployment was performed. Production activation
still requires the approved manifest/CAS executor, shared-secret check, memory
budget, canary and actual observation window. A missing pre-upgrade response
snapshot is not retroactively fabricated; stable policy requires recovery.


## Continuation Experience Fix

The blanket missing-snapshot rejection above is superseded by verified recovery:
response ownership, original account availability and the response-keyed legacy
record must all agree. Existing IDs/profile are preserved; absent/corrupt identity
is not synthesized. Caller-provided root keys are not used to infer ownership.

Connection-only configuration changes retain identity through CAS, and in-flight
old-connection commits cannot roll back newer configuration. Actual account,
proxy and route changes remain protected. Response pin retention now covers the
larger of strong-session and persistent response TTLs (default 72 hours); account
memory preflight must include that increase. Error messages distinguish the
recovery cause without changing terminal retry policy.

- Service/repository/handler focused Codex, validation and OpenAI OAuth refresh
  regression tests passed.
- Five added top-level tests cover verified legacy recovery, ownership rejection,
  missing identity, connection-only refresh/late commit, TTL, error classification
  and expiry between lookup/finalization. New tests passed five repeated runs.
- Seven selected new/existing recovery and CAS tests passed 20 repeated runs in
  a separate invocation with a 20-second test timeout. An earlier parallel outer
  timeout is excluded from passing evidence; no test process remained afterward.
- Embedded server compilation and `git diff --check` passed.
- This follow-up did not rerun Linux race, frontend or full-suite tests; the prior
  16 full-suite baseline failures remain tracked. No production activity occurred.

This is not universal recovery of untracked history, durable snapshot storage or
automatic replay. Truly missing identity still requires explicit context recovery.
