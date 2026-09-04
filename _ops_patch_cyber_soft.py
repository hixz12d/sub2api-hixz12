# -*- coding: utf-8 -*-
from pathlib import Path

root = Path(__file__).resolve().parent / "backend" / "internal" / "service"


def must_replace(path: Path, old: str, new: str, label: str) -> None:
    text = path.read_text(encoding="utf-8")
    if old not in text:
        raise SystemExit(f"missing block: {label} in {path.name}")
    path.write_text(text.replace(old, new, 1), encoding="utf-8")
    print("ok", label)


# 1) upstream_errors
p = root / "openai_gateway_upstream_errors.go"
must_replace(
    p,
    """\t\toutBody := body
\t\t// Super-Instruct accounts: keep status/code, soften client-visible text toward recover-via-skill.
\t\tif account != nil && account.IsSuperInstructEnabled() {
\t\t\tif rewritten := rewriteCyberPolicyClientBody(body, resp.StatusCode); len(rewritten) > 0 {
\t\t\t\toutBody = rewritten
\t\t\t}
\t\t}
\t\tc.Data(resp.StatusCode, contentType, outBody)""",
    """\t\toutBody := body
\t\t// SI/Kit accounts: keep status/code, soften client-visible text toward recover-via-skill.
\t\tif rewritten := rewriteCyberPolicyClientBody(body, resp.StatusCode); len(rewritten) > 0 && shouldSoftenCyberPolicyClientText(account) {
\t\t\toutBody = rewritten
\t\t}
\t\tc.Data(resp.StatusCode, contentType, outBody)""",
    "upstream_errors body",
)
must_replace(
    p,
    """\t\tclientMsg := cyberMsg
\t\tif clientMsg == "" {
\t\t\tclientMsg = "Request blocked by upstream cyber-security policy"
\t\t}
\t\tif account != nil && account.IsSuperInstructEnabled() {
\t\t\tclientMsg = softCyberPolicyClientMessage()
\t\t}
\t\twriteError(c, resp.StatusCode, "invalid_request_error", clientMsg)""",
    """\t\tclientMsg := cyberPolicyClientMessage(account, cyberMsg)
\t\twriteError(c, resp.StatusCode, "invalid_request_error", clientMsg)""",
    "upstream_errors compat",
)

# 2) chat completions
p = root / "openai_gateway_chat_completions.go"
must_replace(
    p,
    """\t\t\tclientMsg := msg
\t\t\tif clientMsg == "" {
\t\t\t\tclientMsg = "Request blocked by upstream cyber-security policy"
\t\t\t}
\t\t\twriteChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)""",
    """\t\t\tclientMsg := cyberPolicyClientMessage(account, msg)
\t\t\twriteChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)""",
    "chat nonstream",
)
must_replace(
    p,
    """\t\t\t\t\tclientMsg := msg
\t\t\t\t\tif clientMsg == "" {
\t\t\t\t\t\tclientMsg = "Request blocked by upstream cyber-security policy"
\t\t\t\t\t}
\t\t\t\t\tif _, err := fmt.Fprint(c.Writer, buildChatStreamErrorSSE(code, clientMsg)); err == nil {""",
    """\t\t\t\t\tclientMsg := cyberPolicyClientMessage(account, msg)
\t\t\t\t\tif _, err := fmt.Fprint(c.Writer, buildChatStreamErrorSSE(code, clientMsg)); err == nil {""",
    "chat stream",
)

# 3) messages
p = root / "openai_gateway_messages.go"
must_replace(
    p,
    """\t\t\tclientMsg := msg
\t\t\tif clientMsg == "" {
\t\t\t\tclientMsg = "Request blocked by upstream cyber-security policy"
\t\t\t}
\t\t\twriteAnthropicError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)""",
    """\t\t\tclientMsg := cyberPolicyClientMessage(account, msg)
\t\t\twriteAnthropicError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)""",
    "messages nonstream",
)
must_replace(
    p,
    """\t\t\t\t\t\tclientMsg := msg
\t\t\t\t\t\tif clientMsg == "" {
\t\t\t\t\t\t\tclientMsg = "Request blocked by upstream cyber-security policy"
\t\t\t\t\t\t}
\t\t\t\t\t\tif _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE("invalid_request_error", clientMsg)); err == nil {""",
    """\t\t\t\t\t\tclientMsg := cyberPolicyClientMessage(account, msg)
\t\t\t\t\t\tif _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE("invalid_request_error", clientMsg)); err == nil {""",
    "messages stream",
)

# 4) response_handling
p = root / "openai_gateway_response_handling.go"
must_replace(
    p,
    """\t\t\tif sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
\t\t\t\tdataBytes,
\t\t\t\teventType,
\t\t\t\topenAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, downstreamKeepaliveBytes),
\t\t\t); sanitized {""",
    """\t\t\tif sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
\t\t\t\tdataBytes,
\t\t\t\teventType,
\t\t\t\topenAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, downstreamKeepaliveBytes),
\t\t\t\taccount,
\t\t\t); sanitized {""",
    "response_handling call",
)
must_replace(
    p,
    """func sanitizeOpenAIResponseFailedEventForClient(payload []byte, eventType string, clientOutputStarted bool) ([]byte, bool) {
\teventType = strings.TrimSpace(eventType)
\tisFailedEvent := eventType == "response.failed"
\tif (!isFailedEvent && eventType != "error") || len(payload) == 0 || !gjson.ValidBytes(payload) {
\t\treturn payload, false
\t}
\tupdated := payload
\t// 容量降载码对 Codex CLI 是致命错误；事件既然要写给客户端（failover 已不可用），
\t// 就改写为客户端可重试的错误码。error 帧与 response.failed 都要改：上游降载
\t// 总是先推 error 帧再收 failed，两帧携带同一个错误。
\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(updated); changed {
\t\tupdated = rewritten
\t}""",
    """func sanitizeOpenAIResponseFailedEventForClient(payload []byte, eventType string, clientOutputStarted bool, account *Account) ([]byte, bool) {
\teventType = strings.TrimSpace(eventType)
\tisFailedEvent := eventType == "response.failed"
\tif (!isFailedEvent && eventType != "error") || len(payload) == 0 || !gjson.ValidBytes(payload) {
\t\treturn payload, false
\t}
\tupdated := payload
\t// SI/Kit: soften cyber_policy client text on SSE error/failed frames.
\tif shouldSoftenCyberPolicyClientText(account) {
\t\tif rewritten, ok := softenCyberPolicyClientPayload(updated); ok {
\t\t\tupdated = rewritten
\t\t}
\t}
\t// 容量降载码对 Codex CLI 是致命错误；事件既然要写给客户端（failover 已不可用），
\t// 就改写为客户端可重试的错误码。error 帧与 response.failed 都要改：上游降载
\t// 总是先推 error 帧再收 failed，两帧携带同一个错误。
\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(updated); changed {
\t\tupdated = rewritten
\t}""",
    "response_handling func",
)

# 5) passthrough
p = root / "openai_gateway_passthrough.go"
must_replace(
    p,
    """\t\t\tif sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
\t\t\t\tdataBytes,
\t\t\t\teventType,
\t\t\t\topenAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, 0),
\t\t\t); sanitized {""",
    """\t\t\tif sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
\t\t\t\tdataBytes,
\t\t\t\teventType,
\t\t\t\topenAIStreamClientOutputStarted(c, clientOutputStarted, attemptWriterSizeBefore, 0),
\t\t\t\taccount,
\t\t\t); sanitized {""",
    "passthrough call",
)

# 6) WS ingress
p = root / "openai_ws_forwarder_ingress.go"
must_replace(
    p,
    """\t\t\t\tclientMessage := upstreamMessage
\t\t\t\tif eventType == "error" || eventType == "response.failed" {
\t\t\t\t\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(clientMessage); changed {
\t\t\t\t\t\tclientMessage = rewritten
\t\t\t\t\t}
\t\t\t\t}
\t\t\t\tif err := writeClientMessage(clientMessage); err != nil {""",
    """\t\t\t\tclientMessage := upstreamMessage
\t\t\t\tif eventType == "error" || eventType == "response.failed" {
\t\t\t\t\tclientMessage = maybeSoftenCyberPolicyClientPayload(account, clientMessage)
\t\t\t\t\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(clientMessage); changed {
\t\t\t\t\t\tclientMessage = rewritten
\t\t\t\t\t}
\t\t\t\t}
\t\t\t\tif err := writeClientMessage(clientMessage); err != nil {""",
    "ws ingress",
)

# 7) other sanitize call sites
for path in root.glob("*.go"):
    text = path.read_text(encoding="utf-8")
    if "sanitizeOpenAIResponseFailedEventForClient(" not in text:
        continue
    # crude count of 3-arg vs 4-arg calls by looking for the function call lines
    if "sanitizeOpenAIResponseFailedEventForClient(\n" in text or "sanitizeOpenAIResponseFailedEventForClient(" in text:
        # find remaining 3-arg patterns: after clientOutputStarted line without account
        pass

print("scan remaining calls:")
for path in (root).rglob("*.go"):
    text = path.read_text(encoding="utf-8")
    if "sanitizeOpenAIResponseFailedEventForClient" not in text:
        continue
    for i, line in enumerate(text.splitlines(), 1):
        if "sanitizeOpenAIResponseFailedEventForClient" in line:
            print(f"{path.name}:{i}:{line.strip()}")

# WS v2 forwarder - check if it writes message directly
p = root / "openai_ws_forwarder_v2.go"
text = p.read_text(encoding="utf-8")
if "maybeSoftenCyberPolicyClientPayload" not in text and "response.failed" in text:
    # try find client write path near cyber mark
    print("ws_v2 has response.failed; inspect needed if direct write")

# http bridge
p = root / "openai_ws_http_bridge.go"
text = p.read_text(encoding="utf-8")
needle = "clientMessage := upstreamMessage"
idx = text.find(needle)
print("http_bridge idx", idx)
if idx >= 0:
    snippet = text[idx : idx + 450]
    print(snippet)
    old = """\t\tclientMessage := upstreamMessage
\t\tif eventType == \"error\" || eventType == \"response.failed\" {
\t\t\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(clientMessage); changed {
\t\t\t\tclientMessage = rewritten
\t\t\t}
\t\t}"""
    if old in text:
        new = """\t\tclientMessage := upstreamMessage
\t\tif eventType == \"error\" || eventType == \"response.failed\" {
\t\t\tclientMessage = maybeSoftenCyberPolicyClientPayload(account, clientMessage)
\t\t\tif rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(clientMessage); changed {
\t\t\t\tclientMessage = rewritten
\t\t\t}
\t\t}"""
        must_replace(p, old, new, "ws http_bridge")
    else:
        print("http_bridge pattern different; manual")
