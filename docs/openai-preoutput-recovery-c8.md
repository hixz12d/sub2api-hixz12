# OpenAI Pre-output Recovery — C8 联调清单与生产配置草案

适用范围：本 fork 的 OpenAI/Codex HTTP 流（`/v1/responses`、`/v1/messages` bridge、passthrough）在**首个语义输出前**的失败恢复。

相关实现：C1–C7（H2 流健康、结构化归因、Messages first-output stage、`OutputCommitSnapshot`、bounded preoutput 预算、Proto/TTFT 相位）。

---

## 1. 生产配置草案

### 1.1 推荐默认（兼容历史行为）

不改 `.env` 时行为与 F02 后一致：

| 项 | 默认 | 说明 |
| --- | --- | --- |
| `GATEWAY_OPENAI_FIRST_OUTPUT_TIMEOUT_SECONDS` | 现网值（常见 30） | 单次 attempt 首语义输出超时；`0` 禁用 |
| `GATEWAY_OPENAI_HIGH_EFFORT_FIRST_OUTPUT_TIMEOUT_SECONDS` | 现网值或 `0` | high/xhigh/max 推理可单独放宽 |
| `GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MODE` | 空 / `legacy` | 总墙钟预算固定 **110s** 下限 |
| `GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MAX_ELAPSED_SECONDS` | `0` | 使用 mode 默认 |
| `GATEWAY_STREAM_KEEPALIVE_INTERVAL` | 现网值（建议 ≥1） | 本地 ping/comment；**不得**关闭 pre-output 恢复窗 |

### 1.2 建议启用（C6 bounded，两阶段 canary）

在 canary 分组或 canary 实例先开：

```bash
# 单次 first-output（与现网对齐；勿低于 30）
GATEWAY_OPENAI_FIRST_OUTPUT_TIMEOUT_SECONDS=30

# 可选：重推理
# GATEWAY_OPENAI_HIGH_EFFORT_FIRST_OUTPUT_TIMEOUT_SECONDS=120

# C6：总 pre-output 恢复墙钟 = max(110s, 2*first_output + 50s)
GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MODE=bounded_preoutput
# 显式封顶（可选；0=按 mode 计算）。例：first=30 → 默认 110；first=90 → 230
# GATEWAY_OPENAI_PREOUTPUT_RECOVERY_MAX_ELAPSED_SECONDS=180

GATEWAY_STREAM_KEEPALIVE_INTERVAL=15
```

校验规则（启动失败即拒绝配置）：

- `openai_preoutput_recovery_mode` ∈ `{"" , legacy, bounded_preoutput}`
- `openai_preoutput_recovery_max_elapsed_seconds` ∈ `[0] ∪ [30, 3600]`
- first-output timeout：`0` 或 `[30, 600]`（high-effort 上限 1800）

### 1.3 映射到 compose / `.env`

若现网用 `mapstructure` 嵌套而非全大写 env，等价键为：

```yaml
gateway:
  openai_first_output_timeout_seconds: 30
  openai_high_effort_first_output_timeout_seconds: 0
  openai_preoutput_recovery_mode: bounded_preoutput
  openai_preoutput_recovery_max_elapsed_seconds: 0
  stream_keepalive_interval: 15
```

**本草案不修改生产 `/opt/sub2api/.env`；上线前按 AGENTS 无感更新流程单独确认。**

---

## 2. C8 联调清单

### 2.1 构建与静态

- [ ] `go test ./internal/service/ -count=1 -run 'OutputCommit|AnnotateOpenAI|RetryBudget|AffinityStateConstrains|MessagesStage'`
- [ ] `go test ./internal/service/ -tags=unit -count=1 -run 'PreOutputUnexpectedEOF|KeepalivePing|RetryBudgetAllowsSecond'`
- [ ] `go test ./internal/repository/ -count=1 -run 'OpenAIHTTP2|Streaming'`
- [ ] `go test ./internal/handler/ ./internal/config/ -count=1`
- [ ] 镜像从 `/opt/sub2api/source-main` 构建，标签含版本 + 短 commit；**不** `compose down`

### 2.2 Canary 实例（8100）只读 / 流量前

- [ ] 备份 Nginx `sub2api.conf` 与当前 `.env` 中 gateway 相关键
- [ ] 仅重建 `sub2api`（8100）`--no-deps`，`healthy` 且 `http://127.0.0.1:8100/health` = 200
- [ ] 配置已加载：进程日志或 admin 配置页可见 `openai_preoutput_recovery_mode`

### 2.3 请求级场景（canary 直打 8100 或临时切主）

| ID | 场景 | 期望 |
| --- | --- | --- |
| S1 | Messages 流，上游 headers 200 后 body 立即 UnexpectedEOF | failover 到另一账号；客户端无半截 text；H2 stream failure 计数 +1 |
| S2 | Messages 流，仅本地 `event: ping` 后上游断 | 仍可 failover；`semantic_output_started=false`；可有 heartbeat |
| S3 | Responses 流，首语义前 EOF | 同 S1；`SafeToFailoverAfterWrite` 在仅 keepalive 字节时为 true |
| S4 | 首输出 timeout（压低 canary timeout 或 mock） | 结构化 `failure_phase=pre_output` `failure_cause=first_output_timeout`；预算允许第二次 attempt |
| S5 | 正常流，有 `response.output_text.delta` | 账号头在语义提交后出现；`first_frame_ms`/`first_semantic_ms`/`first_visible_ms` 单调非降（0=未置） |
| S6 | sticky/stateful Codex 续写 | `max_distinct_accounts=1`；不因 local transcript 误放宽切号 |
| S7 | 200 SSE 后中途断流 | **不得**因 headers 成功清零 H2 失败窗口（F01） |

### 2.4 日志断言（`openai.gateway_attempt`）

每条 attempt 的 `forward_complete` 应可见：

- `actual_proto_major`（2 或 1；未知可为 0）
- `first_frame_ms` / `first_semantic_ms` / `first_visible_ms`
- `failure_phase` / `failure_cause` / `retry_decision_reason`（失败路径）
- `wire_state` / `semantic_output_started` / `heartbeat_only`
- `first_token_ms`（与 TTFT mode 一致：semantic 或 visible）

### 2.5 回滚

- [ ] Nginx 主备切回健康旧镜像实例
- [ ] 或仅将 `openai_preoutput_recovery_mode` 置空 / `legacy` 热重启应用（若配置支持热更则优先）
- [ ] 确认无双实例同时重建；DB/Redis 不动

### 2.6 观察窗

- 默认：切换后观察 **≥ 2 小时** 活动流量与错误率（项目规则）
- 若缩短：书面记录残余风险（短观察 ≠ 完整 TTL 验证）

---

## 3. 已知边界

- WS passthrough 的 first-output 超时路径已有独立逻辑；本波 C5 stage 主攻 HTTP Messages bridge。
- `bounded_preoutput` 只拉长/对齐**总墙钟**；单 attempt 仍受 first-output timeout 约束。
- Strong affinity 会话在 rebuildable transcript 下仍强制 `maxDistinctAccounts=1`。

---

## 4. 推送 / 发布检查（main）

- [ ] 提交仅含 pre-output recovery 相关 diff（排除无关 admin credential sync WIP）
- [ ] CI / 本地 package 测试通过
- [ ] PR 或 push 说明引用本文档 S1–S7
- [ ] 生产发布走双实例无感顺序，**禁止**同时动 8100+8101
