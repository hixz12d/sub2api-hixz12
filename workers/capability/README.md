# Capability Modules

The degradation-monitoring system owns scheduling, target selection, request
budgets, cancellation, persistence and access control. Detection modules provide
separate evidence, not a shared synthetic confidence score.

## Meow Distribution Module

`meow_adapter.py` adapts the pinned upstream scoring, normalization and payload
builders without launching its web server, updater, key store or HTTP transport.
The source is supplied separately, mounted read-only, and verified against
`meow.lock.json`. No upstream source is redistributed in this directory.

Input and output are bounded JSON on stdin/stdout. Credentials, base URLs and
arbitrary headers are not accepted. Payloads returned by `plan` must still pass
the main service's account, lease, request-contract and per-send budget checks.
The adapter does not execute HTTP requests or manage retries.

An upstream `match` is a calibrated distribution result, not proof of model
identity or proof that a provider never changes routing. A mutated or unknown
request contract cannot produce a strong match/mismatch result. Raw answers are
not returned in evidence.

The upstream license is PolyForm Noncommercial 1.0.0, not an OSI open-source
license. Commercial operation/distribution needs appropriate authorization.
`--admitted` only gates execution; it is not a legal license grant. Production
admission remains disabled by default.

## Question-And-Answer Probes

Question probes are independent of meow's official benchmarks and calibration.
Drafts live in `qa-probes.json`; their exact prompts must not be translated or
silently changed. A reviewed reference answer, provenance, revision and explicit
review policy are needed before enabling automatic assessment. Correct recall,
plausible-sounding identities and keyword matches are not by themselves evidence
of model identity or general intelligence.

The supplied Thibault Sottiaux question is disabled and requires review. The
prompt requests no browsing, but text alone cannot prove an upstream model did
not browse internally. The calling layer must omit tools and reject observed
tool use; unknown upstream behavior remains a limitation.

## Controlled Benchmark Updates

`benchmark_catalog.py` is a local, offline SQLite catalog API. `stage` preserves
exact bytes by ID/version; conflicting replacements are rejected. `approve`
uses the admitted adapter to validate derived data, calibration and payload
compatibility across all tiers. Engine-lock and adapter digests are recorded in
validation receipts. A new engine still requires an audited adapter update;
an upstream version string alone never grants execution admission.

`activate` explicitly selects a release using a revision compare-and-swap.
`freeze` returns the immutable release reference for a new plan. Existing
references do not change when a channel is updated. `handle_release` resolves
that exact reference and executes planning/scoring against approved package
bytes, including packages not in the bootstrap benchmark whitelist.

`withdraw` blocks future resolution of the release and preserves bytes, receipts,
and audit events. It does not automatically select an older release. Every real
outbound dispatch must eventually recheck the shared release authority. Already
sent requests cannot be recalled or have their consumed budget refunded.

This local SQLite catalog is NOT the shared authority for production workers.
The Go application now manages the shared PostgreSQL registry through its
administrator API and the Benchmark Versions tab on the channel monitor page.
The server invokes `--validate-candidate` against the pinned engine and persists
its receipt; clients cannot submit approval receipts. A Python audit hook denies
socket connection operations during CLI validation, in addition to not starting
any upstream transport. This is not an operating-system sandbox.

Server-owned configuration is `LLM_DETECTOR_ENGINE_ALLOWED`,
`LLM_DETECTOR_PYTHON`, `LLM_DETECTOR_ADAPTER`, `LLM_DETECTOR_ENGINE_ROOT`, and
`LLM_DETECTOR_ADAPTER_SHA256`. Paths must be absolute. The engine lock has a
compiled-in canonical digest; changing the engine requires an audited host update.
The source and interpreter dependencies must be provisioned separately. Missing
configuration fails closed and does not prevent listing or withdrawing releases.

Channel-bound manifests carry `channel` and `channel_revision`. Switching a
channel invalidates old unconsumed plans and queued jobs; running jobs retain
their frozen release. Withdrawal rejects further permits/renewals and recovers
remaining reservations. Explicit version-pinned plans without a channel remain
pinned. Automatic propagation into group policy revisions and the complete
capability request executor are still pending.
The legacy `handle`/CLI bootstrap path uses the bundled lock and does not consult
catalog withdrawal state; it must not be used as a catalog-governed worker path.
Do not independently replicate SQLite catalogs across worker instances. No
catalog operation schedules tests or sends model requests.

## Verification

`python -I meow_adapter.py --engine-root PATH --verify-only` checks the pinned
source and benchmark files without importing or running the engine.

`python -I -m unittest discover -s tests -v` runs the adapter's isolated contract
tests. Real-engine parity requires an admitted source and exactly the declared
NumPy/httpx versions; it is a separate gate, not replaced by mock tests.

The offline approval path is connected to Go, PostgreSQL, administrator API/UI,
and real-login Playwright acceptance. The adapter is not yet connected to a live
Go capability request worker. Approval does not schedule or send model requests;
do not treat it as completion of the full monitoring system.
