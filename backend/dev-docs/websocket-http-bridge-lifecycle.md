# WebSocket HTTP bridge connection lifecycle

The HTTP bridge accepts a Responses WebSocket connection and forwards each turn as an HTTP streaming request. Its client reader belongs to the inbound connection and survives account retries. The reader now starts for bridge, pooled WebSocket and passthrough ingress. One application message may be queued while the current turn runs. Further pipelining closes the connection with status 1008 instead of retaining unbounded request bodies or blocking control-frame processing. Pooled WebSocket turns use the same bounded client-disconnect drain and keep their existing upstream read timeout.

## Timeouts and cancellation

- `gateway.openai_ws.read_timeout_seconds` also bounds bridge upstream inactivity, including waiting for response headers and partial SSE lines. The existing default is 900 seconds. Incoming body bytes refresh this deadline.
- `gateway.openai_first_output_timeout_seconds` and the high-effort override apply to the bridge's first semantic output. Zero keeps this optional deadline disabled. SSE comments and metadata do not extend it.
- `gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds` remains the wait for the client's next turn after completion. It does not limit an active model response.
- Client disconnect before output cancels the upstream request immediately. After output has started, the bridge may finish reading terminal usage for billing, for at most 30 seconds (or the configured WS read timeout, if shorter). Upstream traffic cannot extend this drain period. Ordinary request cancellation and ingress lease loss still cancel the upstream request.

These changes do not change Nginx, client retries, or HTTP/SSE ingress configuration. They do not enable unsolicited WebSocket Ping frames; the continuous reader handles incoming Ping/Pong/Close while the bridge waits for its upstream.

## Recovery boundary

Later-turn transport errors, incomplete streams and retryable upstream errors may enter the existing handler failover policy only when the bridge can rebuild the current turn, including matching tool-call context, and no downstream output has been committed. Metadata is staged for these recoverable OpenAI turns. The retry payload includes the completed conversation history but executes only the current turn. Missing tool context remains a closed recovery gate. Existing handler retry budgets and account-selection restrictions still apply.

A disconnected client never starts a recovery attempt. A response that has already emitted output is never silently replayed. A terminal usage event collected during the bounded drain follows the existing successful-turn billing path; usage that the upstream never supplies cannot be guaranteed.

## Regression checks

Targeted tests cover header cancellation, header/body inactivity, partial SSE lines, first-output timeout despite comments, Ping and disconnect while waiting for headers, terminal usage after disconnect, bounded draining despite continued traffic, reader shutdown, bounded pending messages, and current-turn recovery with tool context. Existing bridge isolation, continuation and WebSocket lifecycle tests remain applicable.
