# Codex Relay Kernel UI 优化方案

## 1. 范围与入口

本方案只描述前端实现，不在本次后端改动中修改 Vue 页面。

入口放在“账号管理 -> 新建/编辑账号 -> 高级设置”。仅当账号满足以下条件时显示：

- `platform === "openai"`
- `type === "oauth" || type === "setup-token"`
- 非凭据影子账号

高级设置新增一个无嵌套卡片的全宽区块“Codex Relay”。表单继续沿用账号编辑页现有的 label、select、segmented control、switch、inline alert 和表单校验组件，不新建独立设计系统。

## 2. 字段布局

按以下顺序排列，桌面端使用两列 label/value grid，窄屏降为单列：

| UI 字段 | 控件 | `extra` 键 | 可选值 |
|---|---|---|---|
| Relay Mode | segmented control | `codex_relay_mode` | `legacy`、`relay_kernel` |
| Identity Policy | select | `codex_identity_policy_version` | `v1`、`v2` |
| Client Profile | searchable select | `codex_client_profile` | `auto`、`passthrough`、`codex_cli`、`codex_exec`、`codex_desktop`、`opencode`、`pi` |
| Shadow Compare | switch | `codex_relay_shadow_enabled` | boolean |
| Identity Scope | select | `codex_fingerprint_mode` | `off`、`device`、`session`、`window`、`window40`、`full` |

不要把派生 secret、installation ID、session ID、thread ID、turn ID、transport key、conversation digest 或 credential version 做成输入框。这些值属于运行时，不是账号配置。

## 3. 联动规则

- `legacy`：默认保持 `v1`；允许打开 Shadow Compare，以便只计算 V2 对照结果。
- `relay_kernel`：自动切到 `v2`，禁用 `v1` 选项。提交 `relay_kernel + v1` 时后端也会拒绝。
- `auto`：根据入站 `originator` 和 `User-Agent` 识别 Codex CLI、Exec、Desktop、OpenCode、Pi；无法识别时选择 `passthrough`。
- `opencode`、`pi`：Profile 下方显示 warning，“仅声明 HTTP 能力；WebSocket 请求会失败关闭，不会降级伪装为官方客户端”。
- `passthrough`：显示 info，“保留调用方应用身份；Relay Kernel 仍可管理会话身份，但不会伪造官方应用版本”。
- Shadow Compare 打开时：显示只读状态“仅本地对照，不发送第二次上游请求”。
- Identity Scope 设为 `off`：显示 info，“仅适用于 Legacy/passthrough 回滚验证；Relay Kernel 必须选择托管 identity scope”。

切换字段只影响后续新请求。已经开始的 HTTP attempt 或已建立的 WS 连接使用其创建时的不可变快照，UI 不显示“立即重连”或“强制应用”操作。

## 4. Profile 展示

Profile 下拉项使用“名称 + fidelity 状态 + transport 图标”，不要把完整 header 列表平铺在表单中。选中项下方可展开只读详情。

| Profile | Fidelity | HTTP | WS | 应用身份 |
|---|---|---:|---:|---|
| passthrough | passthrough | 是 | 是 | 调用方原值 |
| codex_cli | degraded | 是 | 是 | Codex CLI |
| codex_exec | degraded | 是 | 是 | Codex Exec |
| codex_desktop | degraded | 是 | 是 | Codex Desktop |
| opencode | degraded | 是 | 否 | OpenCode |
| pi | degraded | 是 | 否 | Pi Coding Agent |

`degraded` 必须可见，不得显示为 exact/verified。Tooltip 文案：“应用层字段来自受控 Profile；未取得并验证目标客户端完整 TLS/HTTP2/WS 指纹，因此不声明 exact parity。”

## 5. API 请求

沿用账号创建/更新 API，只写入已公开字段：

```json
{
  "extra": {
    "codex_relay_mode": "relay_kernel",
    "codex_identity_policy_version": "v2",
    "codex_client_profile": "auto",
    "codex_relay_shadow_enabled": false,
    "codex_fingerprint_mode": "session"
  }
}
```

不要提交 UI 未编辑的脱敏值或任何运行时身份值。后端会丢弃以下敏感/派生键：

```text
codex_relay_secret
codex_identity_derivation_secret
codex_identity_installation_id
codex_identity_session_id
codex_identity_thread_id
codex_identity_turn_id
codex_identity_transport_key
codex_identity_conversation_digest
codex_identity_credential_version
```

后端错误码按字段落位：

| 错误码 | UI 行为 |
|---|---|
| `CODEX_RELAY_ACCOUNT_INVALID` | 在区块顶部显示账号类型错误 |
| `CODEX_RELAY_SETTINGS_INVALID` | 根据 message 标记 Mode、Policy 或 Profile |
| `CODEX_RELAY_SHADOW_INVALID` | 标记 Shadow Compare |
| `CODEX_FINGERPRINT_MODE_INVALID` | 标记 Identity Scope |
| `CODEX_RELAY_SECRET_INVALID` | 显示全局配置 warning，并阻止保存 |
| `CODEX_RELAY_IDENTITY_REQUIRED` | 标记 Identity Scope，并阻止 Relay Kernel 保存 |

## 6. 全局派生 Secret

Relay Kernel 复用后端 `gateway.openai_affinity.secret`，要求 UTF-8 编码后至少 32 bytes。它是全局高熵 secret，不存储在账号 `extra`。

若以后在系统设置页增加编辑入口：

- 使用 password input，默认只显示“已配置/未配置”，绝不回显原值。
- 更新 API 使用 write-only 字段；空值表示“不修改”，另设明确的“清除”确认操作。
- 页面日志、前端 store、埋点、错误消息和网络调试摘要都不得记录该值。
- 变更旁显示 warning：“轮换会改变派生身份与 conversation key，仅影响新建连接；应先启用 Shadow Compare 并在迁移窗口执行。”

## 7. 状态与可观测性

账号列表可以显示紧凑状态列：

- `Legacy`
- `Shadow / v2 / <profile>`
- `Kernel / v2 / <profile>`

不要显示具体 installation/session/thread/turn ID。Shadow 对照结果只从后端指标或结构化日志聚合展示：compared、matched、mismatched、error category；不得触发第二次请求来“测试”。

加载中禁用整个区块；保存失败保留用户输入；成功后以服务端返回的账号 `extra` 重建表单，避免前端认为敏感字段已经保存。

## 8. Vue 实现拆分

建议按现有账号表单目录放置以下组件/模块，具体路径以仓库现有结构为准：

- `CodexRelaySettings.vue`：纯表单区块，接收账号平台、类型和 `extra`。
- `codexRelaySchema.ts`：枚举、默认值、前端依赖校验、API 序列化白名单。
- `CodexProfileDetails.vue`：只读 profile 能力详情；用现有 icon 库表示 HTTP、WS、warning。

组件发出一个完整的公开配置对象，由账号编辑页合并到现有 `extra`。序列化函数必须白名单复制五个公开键，不能 `...form` 全量展开。

## 9. 验收清单

1. 非 OpenAI OAuth/setup-token 账号不显示且不能通过 UI 提交配置。
2. `relay_kernel` 必然提交 `v2`；未知枚举能显示后端字段错误。
3. `auto` 对未知客户端回退 passthrough，不伪装成 Codex CLI。
4. `opencode`/`pi` 的 WS 限制明确可见。
5. Shadow 开关文案明确“无第二次上游请求”。
6. secret 永不回显、永不进入账号请求、日志或 store。
7. 保存后只影响新请求；活动 WS 不被 UI 自动断开。
8. 键盘可操作，warning 与错误不仅依赖颜色，移动端无横向溢出。
9. 新建、编辑、重新授权和 JSONB merge 路径提交相同配置时结果一致。
10. 回滚只需选择 `legacy + v1` 并关闭 Shadow Compare；无需清除历史运行时身份。
