# Super-Instruct（账号级 bridge 注入）

## 行为

- **默认关闭**。
- 账号 `extra.super_instruct = true` 时，网关在 Codex 协议路径把 bridge 文本写入顶层 `instructions`。
- `extra.super_instruct_mode`：
  - `prepend`（默认）：`bridge + "\n\n" + existing`
  - `replace`：仅 bridge
- bridge 文件：`gateway.super_instruct_bridge_file`，按 **mtime 热加载**（约 2s 检查间隔）。
- 已含 `[Super-Instruct` 标记的 instructions 不会重复注入。

## 配置

```yaml
gateway:
  super_instruct_bridge_file: /app/data/super-instruct/bridge.md
```

环境变量：`GATEWAY_SUPER_INSTRUCT_BRIDGE_FILE`（若部署侧已按 mapstructure 绑定惯例透出）。

示例 bridge：`deploy/super-instruct-bridge.md`。

## 开关方式

Admin API / WebUI：

```http
POST /api/v1/admin/accounts/bulk-update
{
  "account_ids": [1, 2],
  "extra": {
    "super_instruct": true,
    "super_instruct_mode": "prepend"
  }
}
```

`UpdateAccountExtra` / bulk-update 均为 JSONB key 级合并，不会整表覆盖其它 extra。

调度缓存已白名单 `super_instruct` / `super_instruct_mode`，选号路径可读到开关。

## 注入路径

| 路径 | 文件 |
|------|------|
| `/v1/responses` | `openai_gateway_forward.go` |
| Anthropic `/v1/messages` → Codex | `openai_gateway_messages.go` |
| Chat Completions → Responses | `openai_gateway_chat_completions.go` |

实现：`backend/internal/service/openai_super_instruct.go`。

## WebUI

独立目录：`../super-instruct-webui`（同级 VPS 目录）。公网 `:8020` 控制白名单与 bridge 文本。

## 上线顺序建议

1. 推送本仓库（代码就绪）
2. 先上 WebUI，可先写 extra（补丁未滚动前不注入，安全）
3. canary 构建/滚动 sub2api 后再对测试账号验证 `instructions` 前缀
