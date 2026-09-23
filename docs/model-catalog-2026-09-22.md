# 2026-09-22 模型目录与账号模型同步

补充 `gpt-6-sol`、`gpt-6-luna`、`claude-opus-5-5`。GPT-6 Terra 未找到官方发布及定价，按用户确认暂缓，不能用 GPT-5.6 Terra 或 Astra 的价格替代。

## 价格口径

单位：美元 / 百万 tokens。

| 模型 | 输入 | 输出 | 缓存读取 | 缓存写入 | 1 小时缓存写入 |
| --- | ---: | ---: | ---: | ---: | ---: |
| GPT-6 Sol | 2 | 10 | 0.2 | 2.5 | — |
| GPT-6 Luna | 0.1 | 0.5 | 0.01 | 0.125 | — |
| Claude Opus 5.5 | 4 | 20 | 0.2 | 5（5 分钟） | 8 |

- Sol/Luna：上下文 1,050,000，最大输出 128,000；推理档位 `none/low/medium/high/xhigh/max`，默认 `medium`。
- Sol/Luna：总输入（含缓存）**超过** 272,000 时，整笔请求输入及缓存价格 ×2、输出 ×1.5。仍遵循既有账号和分组长上下文计费开关。
- Sol/Luna：Fast/priority ×2，Flex ×0.5；价表保存 Batch 半价字段，Batch 执行能力仍由既有接口决定。
- Opus 5.5：上下文 1,000,000，最大输出 128,000；Fast 输入/输出为 8/40，缓存价按 ×2；1 小时缓存写入使用独立价格。没有新增长上下文附加费。
- 修复显式 priority 缓存写价在 5 分钟/1 小时分项计费中被忽略的问题；分项会按所选缓存写价与标准价的比值缩放，避免 Opus 5.5 Fast 少计缓存费用。
- 价格目录、动态目录缺项时的兜底、动态价格服务完全不可用时的兜底保持一致。显式自定义定价仍优先，不改已有 Astra 等模型的业务定价。
- Opus 5 与 Opus 5.5 分别匹配，防止价格表 map 遍历顺序改变旧模型计费。
- 未添加未经核实的 Bedrock 模型 ID 或 Antigravity 官方可用性声明。

官方资料：

- https://developers.openai.com/api/docs/models/gpt-6-sol
- https://developers.openai.com/api/docs/models/gpt-6-luna
- https://platform.claude.com/docs/en/models/opus-5-5/overview
- https://www.anthropic.com/claude-opus-5-5

## 同步报错修复

生产排查发现，同一 OpenAI OAuth Team 账号：

- 账号编辑的 `POST /admin/accounts/:id/models/sync-upstream` 返回 502，安全错误信息为 `Failed to request upstream model list`。
- 已有 `GET /admin/accounts/:id/models` 的 Codex 发现链路可以取得最新 Sol、Luna 等模型。
- 通过账号既有代理、由代理解析域名的上游只读探测返回 HTTP 200。

同步入口现复用 `OpenAIGatewayService.FetchCodexModelsManifest`，沿用账号出口、身份、OAuth 插件与模型缓存策略，然后继续原有能力元数据解析和保存流程。API Key 等其他平台流程不变。

前端统一使用 `extractApiErrorMessage` 读取 API 拦截器返回的普通对象，修复重复显示“同步上游模型失败：同步上游模型失败”、丢失真实错误信息的问题。

回归覆盖 OAuth 同步成功及上游 HTTP 错误、能力保存、前端错误显示及失败后重试，以及模型能力、目录与兜底价、长上下文临界点、Fast/Flex 计费。

## 本地验证

- `internal/service`、`internal/pkg/openai`、`internal/pkg/claude`、`internal/handler/admin` 四个 Go 包的模型、定价及同步定向回归通过。
- 同一范围追加 `-tags=unit`，覆盖既有 `CalculateCost`、缓存分项、ServiceTier 等计费测试，通过。
- 前端 `ModelWhitelistSelector`、`AccountStatusIndicator`、`UseKeyModal`、`useModelWhitelist` 四个测试文件共 51 项通过。
- `vue-tsc --noEmit`、价表 JSON 解析及 `git diff --check` 通过。
- 未执行完整生产构建、上线切换或更新后生产验收。

本文件记录代码变更，不代表已完成生产部署。
