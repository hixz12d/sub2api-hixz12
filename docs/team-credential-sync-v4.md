# 第四批：候选验证与按凭据版本恢复认证

> 第五批已补充持久化刷新协调和安全交回 Team，见 [第五批交付](team-credential-sync-v5.md)。本文未完成项描述的是第四批交付时状态；验证范围和受控清错规则继续有效。

本批已实现服务端可信验证、版本化认证错误归因及同事务恢复。仅完成本地和隔离测试，未部署。

## 合同与迁移

- 同步能力 revision=4，请求 contract_version=1。
- 新能力：auth_only、candidate_validation、versioned_auth_errors；single_writer 仍为 false。
- 新迁移 `249_oauth_auth_recovery.sql` 创建独立错误归因表，并向操作回执表追加验证/恢复字段。不得用历史错误文本回填凭据证据。
- `recovery_mode=auth_only` 仍使用现有 `POST /api/v1/admin/accounts/{id}/sync-oauth-credentials`，带完整实例、身份、updated_at 和 operation_id 前置条件。
- 普通 credentials_only、后续步骤重试、第三批 AT 读取均不会触发授权验证或清错。

## 验证及恢复边界

候选 AT 直接请求固定 `https://chatgpt.com/backend-api/wham/usage` 和 `/backend-api/codex/models?client_version=0.101.0`。要求官方返回的 email/account_id 与目标一致，user_id 非空且在已有目标用户ID时匹配；额度状态明确允许，模型清单非空。工作区元数据不一致、返回缺字段、挑战页、401/403/429、重定向或网络失败都拒绝恢复。

复用现有出口与代理/TLS transport，禁止跳转、令牌刷新、候选 RT 消费、原始响应日志及无界响应。范围标签 `codex_identity_usage_catalog` 只证明身份、额度与模型清单访问；不等于模型生成、计费、Team 管理权限或长期可用性。没有真实生产上游验证结果时不宣称生产可用。

HTTP 401 使用实际发送到官方 HTTPS 主机的 bearer 与账号快照核对；只为明确的凭据失效代码保存错误版本。原生刷新拒绝使用已有完整凭据 CAS 隔离入口保存证据。来源不明、没有正数版本或没有实际请求凭据的路径保持无可恢复归因，不从文本猜测归因。

凭据 CAS、严格匹配的认证错误清除、证据移除、操作回执及 outbox 在同一 PostgreSQL 事务中完成。新 AT 必须与出错 AT 不同；验证在入库时须保持新鲜。找不到匹配错误返回 AUTH_ERROR_UNATTRIBUTED，全部回滚。

不会恢复人工 schedulable 开关，也不清限流、过载、权限/工作区停用或被管理员替换的冷却。版本化认证冷却缓存回查数据库，防止旧缓存继续阻断已恢复状态；避免盲删其他新冷却。重放既有操作只返回持久化回执，不再次验证或写凭据。

## Team 操作

账号详情增加“验证新授权并恢复认证”，先预览再确认。后台使用 revision 4 能力，保存脱敏操作记录，浏览器不收到 AT/RT。Team 本地认证状态与刷新归属保持分别处理，远端 Codex 验证不替代本地管理接口验证。

建议先升级兼容 revision 3/4 的 Team，再按固定双实例顺序更新 Sub2API；迁移成功、全部副本更新后才启用恢复操作。回退应用保留追加表列与审计，不能把镜像回滚描述成已撤销远端凭据写入。

详细 Team 交付说明位于 Team 仓库 `docs/sub2api-auth-recovery.md`。

## 验证

- Go 六个相关包的针对性测试/编译通过；另复核原生刷新隔离 repository 测试。
- PostgreSQL 18.1 + Redis 8.4 隔离集成：新迁移、认证恢复和其他阻断保留、原生刷新拒绝归因、迟到 401、无归因错误回滚、相同 AT 拒绝、outbox 故障整体回滚、人工冷却保留、第二批并发/CAS/租约路径。
- Team 106 项 Python、28 项 JavaScript 回归通过；真实 Chromium + 隔离 FastAPI/SQLite 验证新入口的鉴权、预览/取消/确认、暂停保留、无秘密入浏览器及桌面/手机布局。
- Python 仍有既有 curl_cffi 关闭循环忽略异常及 Starlette/httpx 弃用告警；完整断言通过。未执行全量测试、生产迁移或真实账号生成请求。

第五批跨副本刷新排空/旧写隔离/安全回交、以及第六批自动轮转和生产验收仍待完成。
