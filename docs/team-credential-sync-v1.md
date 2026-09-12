# Team × Sub2API：安全凭据同步第一批

> 第一批历史记录。当前协议与恢复行为见 [第二批事务回执与远端状态](team-credential-sync-v2.md)。

本批为本地实现，不代表已部署。代码基线：Sub2API `349a34884`，Team `babad66`。

## 范围

现有 OAuth 同步 v1 增加 `oauth_sync.revision=2` 能力协商。支持有明确管理员意图的 `credentials_only` 条件写入、持久化回执查询和真实缓存删除结果。

不开放 `auth_only`：旧实现仅凭错误文字清错，没有候选授权能力验证及错误所属凭据版本，不能作为自动恢复依据。普通测试、重新授权等其他既有入口不由本协议代替。本批不实现跨应用刷新单写、实例 UUID、可信授权身份验证、反向事件、调度传播确认或轮转排空。

## API

均使用现有 Admin 鉴权，不将管理员密钥放入浏览器。

- `GET /api/v1/admin/integration/capabilities`
- `POST /api/v1/admin/accounts/{id}/sync-oauth-credentials`
- `GET /api/v1/admin/accounts/{id}/credential-sync-operations/{operation_id}`

能力对象：

```json
{
  "schema_version": 1,
  "contracts": {"oauth_sync": [1]},
  "oauth_sync": {
    "revision": 2,
    "available": true,
    "credential_cas": true,
    "operation_receipts": true,
    "recovery_modes": ["credentials_only"],
    "auth_only": false,
    "single_writer": false,
    "metadata_changes": false,
    "instance_identity": false,
    "scheduler_confirmation": false
  }
}
```

`available` / `operation_receipts` 根据幂等协调器是否配置返回；存储实际不可用时请求失败关闭。能力声明不承诺上游或数据库健康。

请求示例（全部为假凭据）：

```json
{
  "contract_version": 1,
  "operation_id": "fixture-operation",
  "expected_updated_at": "2026-09-08T00:00:00Z",
  "expected_identity": {"email": "fixture@example.invalid", "workspace_id": "fixture-workspace"},
  "credentials": {"access_token": "fixture-at", "refresh_token": "fixture-rt", "client_id": "fixture-client"},
  "recovery_mode": "credentials_only"
}
```

前置条件：operation_id 必填；expected_updated_at 必须为账号 API 原样返回的时间；邮箱必须存在且匹配，workspace_id 键必须显式提供。null 仅表示调用方明确提交无工作区预期，不能凭此证明上游个人身份。远端组织/工作区字段互相冲突时拒绝，不能猜测它们语义相同。

只允许 access_token、refresh_token、id_token、expires_at、expired、client_id。RT 必须伴随 client_id；已知客户端不匹配拒绝。新 AT 不得默默继承未知关系的旧 RT。输入检查不消费 RT，也不声称候选授权已通过上游验证。

## 写入与回执语义

数据库以账号 ID、未删除、OpenAI OAuth 类型、原 updated_at、完整原 credentials 做条件写入；凭据修改与 scheduler_outbox 插入处于同一 SQL 语句。冲突不写入、不清错、不修改调度开关。写入递增 `_token_version` 并严格推进 updated_at。

不修改名称、分组、代理、倍率、优先级、并发或任何运行阻断。元数据身份前置条件不能替代未来的 grant/epoch 体系，也不能证明 Team 中的授权在语义上比 Sub2API 新。

幂等域包含管理员身份和目标账号。相同操作/相同请求重放已保存回执；不同请求摘要冲突。摘要和既有幂等记录不保存凭据明文，回执只含步骤元数据。该接口审计完整省略请求体，包括 malformed JSON / 未知字段。

严格缓存删除失败返回 partial；旧的 best-effort 缓存失效调用者保持行为不变。`token_cache_invalidation=succeeded` 仅证明本次删除成功，**不证明旧请求无法回填缓存，也不证明所有副本已经接收调度变化**。`scheduling_assessment=not_assessed` 不得展示成已恢复服务。

凭据事务与幂等回执保存不处于同一事务。若写入后、回执保存前进程退出，查询返回 unknown；不伪造未执行。原 CAS 使相同旧快照不能再次覆盖，但本批没有自动补全该崩溃窗口的回执。完整的事务操作记录与传播重试属于后续工作。

查询响应：`state=recorded` 携带 receipt；缺失、过期、处理中或失败后无法确定结果时为 `state=unknown`。保留期使用现有幂等配置，默认 24 小时；过期不能推断历史未执行。

## Team 适配

先协商 revision=2、CAS 与操作回执；不支持时不写入，取消隐式 PUT 降级。已支持能力后账号 404 为账号缺失，不能判为路由不支持。管理认证错误与账号 OAuth 失效分开。

POST 网络/解析/服务端异常后只查询一次回执，不自动重复 POST，不换 PUT。未知结果及已写入但缓存失败沿 client → application → Operation 保留。普通 `success` / `ok` 不再把 partial 表示为完全完成。

手动重新授权与自动重新授权记录不同 reason，不再伪装为后台刷新；没有可信验证来源时仍只提交 credentials_only。同步状态保存在现有 Operation/OperationStep 与绑定观察字段，移除未映射的动态 Account 同步属性。显式调度修改后重新读取，不能引用修改前快照。

## 验证与发布

本次实际结果：Team 48 项针对性 Python 测试通过；Sub2API service、repository、admin handler、middleware 四个包的针对性测试通过（Go 1.27.0），routes 包编译检查通过。临时 PostgreSQL 18.1 / Redis 8.4 集成测试确认两个并发写入只有一个成功、只有一条调度事件，暂停和限额等字段保持不变。测试容器已自动清理。未执行完整测试套件、真实上游授权、浏览器端到端或生产验证。

发布前核实真实生产镜像、Team 生效 URL、账号上下文和 client_id 元数据。Sub2API 应先发布，待两个应用副本都具备同一能力后再发布 Team。源码构建使用 `/opt/sub2api/source-main`，沿现有 8100 备用实例先升级的滚动顺序，Team 独立部署；本批未执行这些操作。

无需新增表或迁移。回退优先关闭自动推送并保留现有幂等记录；旧 Team 会恢复不安全的隐式降级行为，不应作为自动化继续运行时的无条件回滚方案。数据库备份不能撤销已发生的上游令牌轮换。
