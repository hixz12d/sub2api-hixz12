# Shared HTTP Bundle Integration

## Status and Scope

The two versioned selectors are wired into Sub2API's real HTTP dispatch and the
local Hixz12-Relay HTTP request builder:

- `pi-0.57.1-oauth-sse-r1`
- `opencode-1.2.4-oauth-sse-r1`

They are explicit opt-ins, not replacements for `auto`, `pi`, `opencode` or CLI.
This delivers a shared APPLICATION contract, not native-client impersonation
certification. The packaged evidence remains candidate/unverified. CLI/WS,
Compact, native TLS/HTTP2 equivalence and third-party plugin senders are outside
this acceptance boundary. Neither artifact is a claim about current/latest Pi.

## Shared Artifacts

Canonical files live in `backend/internal/pkg/clientprofile/profiles`. Exact-byte
copies are packaged in Hixz12-Relay's `fingerprint/profiles`, with `-text` Git
attributes and package-data inclusion. Python verifies these SHA-256 values:

| Bundle | SHA-256 |
| --- | --- |
| Pi | `5029d50328d6cfbfc5d53065e5253979e8e739b2d84bb1e17bcd9121b250afb2` |
| OpenCode | `7f7a5876e3d0dc859b2eb05ceffaf4b2e0e7225be5a8095e66f123f976f7b989` |

Never edit a shipped bundle in place. A new contract needs a new ID/revision,
allowlist entry, matching bytes on both sides and new final-sender evidence.
Go pins bundle ID/digest inside the existing complete profile snapshot; new
optional JSON fields do not alter the digest of existing legacy snapshots.
Python persists selector/digest in conversation records and session slots.
Pinned conversations retain their selector across account edits and restarts.
Old Python records without a selector must warm on their old configuration or
drain before enabling a bundle; they are not relabeled as a known old version.

## Final Sending

Sub2API applies the adapter in `doOpenAIUpstream`, after the Responses,
passthrough, Messages and Chat Completions builders. It updates actual request
headers, body, replay body and content length. Caller-owned metadata is rejected,
not silently deleted; only gateway-added Codex metadata is removed. Original
OpenCode cache keys survive generic gateway rewriting. Pi uses the scoped session
and fills missing verbosity with `medium`. Explicit verbosity/tools/instructions,
reasoning and continuation content remain intact.

The Python builder invokes the corresponding adapter before `StdHttpxClient`.
It bypasses the old Pi defaults, which used `low` verbosity and extra CLI headers.

The permitted canary path is native Go HTTP on Sub2API and `tls_profile=none`
(httpx/OpenSSL) on Python. Go rejects enabled `enable_tls_fingerprint` for these
selectors and rejects accounts routed through an OAuth plugin. Python rejects
other TLS modes. These restrictions avoid silently using an untested transport;
they do NOT make the two native TLS implementations identical.
Do not turn off uTLS on active legacy conversations just to enable a bundle.
Use an unused canary account or an account already on the approved native path;
transport-mode migration is separate from installation-ID migration.

## Local Evidence

`TestCodexSharedBundleFinalSender` and Python `test_shared_bundle_actual_sender`
send through real loopback HTTP receivers using the same fixture. The comparison
checks artifact digests, application headers and the complete decoded JSON body.
Host, Content-Length, Accept-Encoding and Connection are transport-generated and
excluded from the header comparison. Header order, raw JSON encoding, TLS,
HTTP/2 SETTINGS and live upstream acceptance are NOT proven by this HTTP test.

Reproduce from PowerShell with the two repository roots:

```powershell
$env:HIXZ12_BUNDLE_CAPTURE_OUT = "$env:TEMP/sub2api-shared-bundle-go-wire.json"
go -C "$Sub2Api/backend" test ./internal/service -run '^TestCodexSharedBundleFinalSender$' -count=1
$env:HIXZ12_GO_CAPTURE = $env:HIXZ12_BUNDLE_CAPTURE_OUT
Set-Location $PythonRelay
python -m pytest tests/test_shared_bundles.py -q
```

All credentials and IDs in this fixture are synthetic; no paid inference occurs.
The Python worktree already contained user changes. They were preserved, and no
blanket commit/push of that repository is part of this integration.

## Rollout Gate

Do not enable these selectors fleet-wide. Follow the existing dual-instance
rollout and conversation migration runbook, keep legacy defaults during warming,
verify shared secrets without rotation, inspect Redis retention/memory and plugin
bindings, and use an approved account/key/budget for real HTTP/SSE canary. An admin
account connectivity test is not proof that this bundle was sent. Inspect the
actual final request and response, then confirm billing, continuation and cache
convergence. Native transport parity remains an explicit release limitation.
