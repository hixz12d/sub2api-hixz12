# Selective Upstream Stability Backports

## Baselines

- Local baseline: `e8a357e75`, version `0.2.1-hixz12.8`.
- Upstream repository: https://github.com/Wei-Shaw/sub2api
- Latest fetched release tag: `v0.2.4` (`5de5e2bed`).
- This is a selective backport, not a full v0.2.3/v0.2.4 merge. VERSION is intentionally unchanged.

## Included

| Upstream commit | Integration |
| --- | --- |
| `99eba19ff` | Upgrade go-redis v9.17.2 to v9.22.0 for the connection-pool nil-context panic fix. Use ZRangeArgs for concurrency and user message queue expiry cleanup, preserving score bounds and batch limits. Transitive cpuid and atomic versions follow the dependency resolver. |
| `4e5d67df3` | Accept max_tokens=1 Claude Code probes for any model only after validating the User-Agent. Preserve max_tokens in the parsed-request projection. Test integer/JSON representations, invalid values, missing bodies, invalid User-Agent and both handler paths. |
| `c8deeb0b0` | Filter unsupported external_web_access fields on Grok raw Chat Completions. Reuse the existing Responses sanitizer without renaming it or introducing a mutable function alias. Request-level tests cover nested fields, retained text/tools, and unchanged non-Grok forwarding. |
| `c7343d2aa` | Include explicit user role in Gemini monitor probes. Test role and prompt preservation. |

## Existing Equivalent Fix

- `335fcdc1d`: plugin_package.go already closes the ZIP reader before committing the package, including checking the close error. No duplicate change was applied.

## Deferred

- Group model allowlist schema replacement/repair: the fork retains its existing group model configuration and custom migration sequence. Importing upstream migrations 235/236 alone would not constitute a compatible upgrade.
- MiniMax platform introduction, proxy relationship changes, broad UI changes, Image 2.5 and remaining Astra/model-catalog changes: outside this bounded stability pass; require separate contract and configuration review.
- Existing custom monitor workflows, OpenAI client profiles, continuation/pre-output recovery, relay kernel and pricing remain in place.

## Verification

Run in `backend` with Go 1.27.0 on Windows:

```text
go build ./...
go test -tags unit ./internal/service ./internal/handler ./internal/repository -run "ClaudeCode|RawChat|GeminiMonitorBody|Concurrency|UserMsgQueue|ReconcileExpired|PluginPackage" -count=1 -timeout=120s
go test -tags unit ./internal/service ./internal/handler ./internal/repository -run "Grok|ClaudeCode|ChannelMonitor|Concurrency|UserMsgQueue|ReconcileExpired|PreOutput|Continuation|ClientProfile" -count=1 -timeout=180s
```

All three commands passed. Changed Go files were formatted with gofmt; `git diff --check` passed (only local LF/CRLF conversion warnings).

No frontend changes were made. This pass did not run the complete test suite, Docker/PostgreSQL/Redis integration tests, Linux runtime tests or live upstream requests. No credentials were read or used, no Git push was performed, and no production deployment or migration was executed.
