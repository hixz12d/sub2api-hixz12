# Compatibility And Evidence

| Selector | Application Contract | Device Declaration | Operations | Actual Sender |
| --- | --- | --- | --- | --- |
| cli | codex-0.153.4-exec-f0-r1 | win32 10.0.26200 x64 only | WS Responses; prewarm then turn | websockets/OpenSSL |
| pi | pi-0.57.1-managed-sse-r1 | catalog's 13 managed OS tuples | HTTP Responses SSE | httpx/OpenSSL or configured curl_cffi |
| opencode | opencode-1.2.4-managed-sse-r1 | catalog's 13 managed OS tuples | HTTP Responses SSE | httpx/OpenSSL or configured curl_cffi |
| pi-0.57.1-oauth-sse-r1 | unchanged shared artifact | captured Windows tuple only | HTTP Responses SSE; tls_profile=none | httpx/OpenSSL |
| opencode-1.2.4-oauth-sse-r1 | unchanged shared artifact | captured Windows tuple only | HTTP Responses SSE; tls_profile=none | httpx/OpenSSL |

平台支持在这里表示应用声明可用，不是已验证的原生 runtime 二进制矩阵。普通 Pi 的 UA 没有版本字段；r1 Pi 使用原 artifact 的正文规则，不能用普通 Pi adapter 代替。Compact 不受此合同支持。

## 四层证据

1. Application contract：本轮合成与真实本地发送测试覆盖，结果见 verification。
2. Cross implementation HTTP：现有共享 r1 测试与仓库已冻结的 Go capture fixture 比较；没有重新构建或运行 Sub2API。新 v2 合同尚未适配 Go。
3. Native TLS/H2：本轮未测。HTTP/WS 分别报告 sender；明文 loopback 不证明原生 TLS、H2、原始 JSON 字节或头顺序相等。
4. Authorized upstream：本轮未执行真实账号请求，不报告成功率、风控收益或生产 canary 通过。

catalog 内未引用本轮实验作为已批准 release 的验证证据，因此描述符的验证状态仍为 `untested`。本轮测试结果是独立可追溯的构建证据，不会为改变标签而重写已 pin 的描述符摘要。

## 跨端边界

Sub2API 可继续消费既有 r1 字节。本次新增 profile schema、Persona v2 tag、会话 pin、候选状态与设备拒绝策略均需要文档 02 的 Go/UI 端实现和独立验收。未修改该仓库，也未读取其私有配置。

旧 Python reader 必须拒绝 v2 tag。新 reader 支持 v1 合法记录和 v2；未知版本或字段拒绝。停止服务后才能迁移或复制部署。多进程共写、本机运行时之外的系统、镜像构建及原生资产验证不在本轮已验收范围。
