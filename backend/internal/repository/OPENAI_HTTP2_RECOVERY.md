# OpenAI HTTP/2 Recovery Defaults

## Behavior

- The default `gateway.openai_http2.fallback_error_threshold` is now `1`.
  A classified HTTP/2 transport or stream failure on a configured proxy can make
  the next eligible attempt use HTTP/1.1 within the existing two-attempt OAuth
  budget. No transport-level request replay was added.
- Explicit positive thresholds still take precedence. A configured value of `2`
  still requires two failures. `0` selects the transport default of `1`.
- Successful streams and non-stream responses no longer erase failures in the
  active observation window. Success clears only an expired window. The window
  remains the existing fixed window, starting with the first failure, not a
  rolling failure-rate calculation.
- The existing default window is 60 seconds and fallback TTL is 600 seconds.
  HTTP/1.1 outcomes and stale HTTP/2 outcomes cannot extend the quarantine.

## Safety And Scope

The policy remains OpenAI-profile and proxy scoped. Direct connections, other
providers, unaffected proxies and explicit fallback opt-outs retain their existing
behavior. Timeout, cancellation and business errors are not newly classified as
HTTP/2 failures.

No account-switch permissions, credential handling, conversation binding, retry
budgets, billing or post-output replay gates were changed. An already interrupted
partial answer is still reported to the client; this change cannot resume its
upstream stream. The trade-off is earlier temporary HTTP/1.1 use on a failing
proxy, potentially reducing multiplexing efficiency.

## Deployment Boundary

Source defaults, the example environment/configuration files and all four Compose
templates use the same threshold. Existing production files are not rewritten.
An existing `.env` value of `GATEWAY_OPENAI_HTTP2_FALLBACK_ERROR_THRESHOLD=2` or an
explicit YAML value continues to override the new default.

Before a separately authorized deployment, verify the effective threshold,
`enabled` and `allow_proxy_fallback_to_http1` settings, the image commit and the
request's actual proxy/transport. Follow the project's two-instance rolling
update procedure. Assess client-visible failures, attempt counts, protocol
selection and proxy concurrency under real traffic; local fault tests are not
production validation. Restoring threshold `2` alone does not restore the old
success-clears-window behavior.

## Regression Coverage

- Real local HTTP CONNECT, SOCKS5 and SOCKS5h proxy failures, default two-attempt
  recovery and explicit two-failure thresholds.
- Linux isolated-process uTLS/ALPN coverage for both threshold policies.
- Interleaved and concurrent success/failure, expiration, generation isolation,
  duplicate reports, scope and opt-out behavior.
- Configuration defaults and environment/YAML overrides.
- Existing service/handler pre-output, retry budget and commit-guard regressions.
