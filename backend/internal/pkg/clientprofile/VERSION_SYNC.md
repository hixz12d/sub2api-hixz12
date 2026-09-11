# Automatic Client Release Updates

Pi/OpenCode presets now select `pi-managed` / `opencode-managed`. Historical
custom selectors and the two embedded candidate bundles remain unchanged.
This mechanism updates APPLICATION configuration, not native TLS implementations.

## Compatibility Gate

The updater queries the official GitHub stable release endpoint for
`earendil-works/pi` and `anomalyco/opencode`. It rejects draft/prerelease tags,
redirects, oversized responses, truncated trees and malformed manifests. Tags
are resolved to immutable commits before reading source trees and Git blobs.
Downloaded content is parsed as data and never executed.

`release.go` records the source trees and dependency-declaration fingerprints
observed at Pi `v0.57.1` / OpenCode `v1.2.4`. The entrypoint blob references in the
existing candidate artifacts were independently matched to those tags. Only
releases with the same bounded source contract and dependency declarations are
automatically published. Changes outside that boundary may advance the version;
changes inside it produce `needs_review` and leave the effective version alone.
This is intentionally conservative: it is not a promise to use every latest tag.

The gate does NOT establish transitive lockfile, built-binary, HTTP/2 or TLS
parity. Existing candidate evidence and fidelity labels are not upgraded. Pi's
UA remains versionless; OpenCode's version is substituted only in its reviewed
UA template. The fixed Windows tuple is unchanged.

Future adapter changes must add a new immutable contract/adapter and preserve
validation of previously published snapshots. Do not overwrite the shipped
bundles or reinterpret an old contract digest in place.

## Publication And Runtime

The existing version service checks local scheduling state once per minute;
network checks are due every six hours, including freshness checks at startup.
Pi/OpenCode have independent schedules and are not disabled by the Codex-only
switch. Conditional release requests reuse an ETag only after verification has
completed. Failures retain the active release and do not become approvals.

Each family stores active/previous releases, mode, latest observed version,
check time, contract digest and ETag in one settings value:

- `openai_client_profile_pi_update_v1`
- `openai_client_profile_opencode_update_v1`

Publication uses a conditional SQL insert/update. There is no non-atomic
fallback. Concurrent checks may both fetch public metadata, but a stale result
cannot overwrite a newer publication, pause or rollback. No schema migration is
needed. Per-instance caches converge within 30 seconds during normal database
availability; local publications invalidate their own cache immediately.

Database read errors retain previously loaded valid snapshots. A cold request
without a readable release record fails closed rather than inventing a newer
version. A genuinely absent record uses the unchanged packaged baseline.

New conversations capture the release in `CodexClientProfile.ClientRelease`.
Its digest participates in the existing conversation snapshot and transport
cache key. Old conversations, including those reloaded from registry JSON, keep
the old snapshot and transport. The final HTTP dispatch uses the captured UA,
not whichever release became current while the request was in flight.

## Administration

The existing admin client catalog and effective-preview endpoints expose the
current version, update status, latest observed version and previous version.
Account forms retain the three client choices; update information is read-only.

Authenticated administrators can use:

```http
POST /api/v1/admin/accounts/client-profiles/pi/policy
Content-Type: application/json

{"action":"hold"}
```

The family is `pi` or `opencode`; actions are `hold`, `resume`, and `rollback`.
`rollback` swaps to the previous compatible release and enters hold mode, so the
next timer cannot undo it. `resume` clears the validator and scheduling stamp;
the next minute-level poll performs a fresh check. Concurrent policy changes
return HTTP 409 and require a fresh command. Policy changes affect future
conversations, not existing pins. These are release controls, not a guarantee
that an older application binary can read newly introduced snapshot fields.

No automatic update changes the Python relay, runs paid inference, deploys
containers, modifies affinity secrets or migrates existing conversations.
Production rollout and upstream canary validation remain separately authorized.
