# 第三批：受控访问令牌读取与 Team 刷新归属

> 安全交回与刷新协调现已在 [第五批](team-credential-sync-v5.md) 实现；本文不开放逆向交接的说明是第三批历史状态。

> 第三批归属和 AT 回读规则继续有效。第四批新增候选验证及按版本认证恢复，能力 revision=4，见 [第四批交付](team-credential-sync-v4.md)。本文“尚未完成”描述的是第三批交付时状态。

本批已完成本地实现与隔离测试，未部署。第二批的事务回执、缓存/调度恢复保持不变；新增能力是供 Team 服务端受控读取访问令牌，用于显式委托后的单向回读。

## 新接口

```text
POST /api/v1/admin/accounts/{id}/credential-sync-access-token
```

只接受现有 Admin 中间件确认的 `auth_method=admin_api_key`，拒绝 JWT 浏览器会话。请求体严格解析，最多 2048 字节：

```json
{
  "expected_instance_id": "9c365a68-bd8d-42be-9b65-8f881b38e91a",
  "expected_credential_version": 7,
  "expected_updated_at": "2026-09-11T12:00:00Z"
}
```

读取要求实例匹配、OpenAI OAuth 非影子账号、正数凭据版本、updated_at 与版本都仍一致。缺少 AT、RT 或明确 client_id 时拒绝，不猜测默认客户端。

成功结果仅包含 schema_version、instance_id、remote_account_id、credential_version、account_updated_at、access_token、client_id、refresh_configured。RT、ID token、会话凭据、原始错误和其他账号字段不返回。

该接口只读数据库，不请求上游，不刷新或修改令牌，不删除缓存、不清错、不修改调度。返回 `Cache-Control: no-store`，请求体完整省略审计；审计仅保留操作元数据。普通账号 GET 的脱敏机制保持不变。

能力声明和状态快照新增 `access_token_readback=true`；原同步能力 revision=3 保持兼容。这个标志不表示上游授权已验证，也不表示该账号的原生刷新已经开启。

## Team 对应行为

Team 新增 `sub2api_refresh_authorities` 表，按本地账号固定远端实例与绑定，记录远端版本、本地版本/加密凭据指纹及归属 epoch。

用户先预览，再确认以本次远端凭据为准。成功后只在 Team 加密保存 AT，清除本地 RT、ID token 和会话凭据。Team 日常刷新入口对已委托账号只回读，不消费 RT、不在失败时回退本地刷新或 OAuth。

本地写入使用 SQLite 写事务及完整凭据 CAS，并与现有 CredentialLease 检查排序。预览后的本地变化、旧代码不递增版本的凭据变化、绑定或实例变化、过期租约都不能覆盖新状态。绑定删除不会自动恢复本地刷新。

活动 OAuth 会话会阻止交接和回读写入。自动重授权入口、自动会话持久化与换票前均核对归属；已委托账号不会通过独立自动任务绕过日常刷新限制。手动授权保留。

多绑定账号暂拒绝委托。开启前应确认 Sub2API 原生刷新配置；回读沿用 Team 现有刷新/探测调度，未静默启用任何自动任务。配置推送不会把回读 AT 再发回 Sub2API。

详细操作、回退约束和完整测试清单见 Team 仓库 `docs/sub2api-refresh-authority.md`。

## 已验证与未验证

- Go 五个相关包的针对性测试/编译通过，验证版本/实例拒绝、AT-only 响应、无凭据写入、API Key/JWT 区分及原有回执行为。
- Team 100 项 Python 回归断言、28 项 JavaScript 回归通过。扩展 Python 回归出现 curl_cffi 清理告警，已在 Team 交付说明记录。
- Chromium + 隔离 FastAPI/SQLite 验证预览、取消、确认、状态更新，且 AT 未进入浏览器响应；桌面/手机宽度无溢出，关闭后停止轮询。
- 本批未修改 Sub2API 数据表或迁移，未重复运行第二批 PostgreSQL 写入集成测试；第一、二批生产迁移仍未执行。

## 仍保留的边界

`single_writer=false` 和 `auth_only=false` 继续保持。当前完成的是 Team 与 Sub2API 的单向职责选择，没有证明 Sub2API 所有原生刷新入口/副本具备 grant 级全局互斥。

候选授权的可信上游验证、认证错误的凭据版本归因、受控清错、原生跨副本 epoch/旧写隔离、刷新排空与安全回交 Team、自动轮转及生产 canary 仍未完成。不会因为成功读到 AT 就声称账号已经恢复可用。
