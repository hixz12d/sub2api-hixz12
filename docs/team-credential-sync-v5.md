# 第五批：刷新协调与受控交回 Team

本地实现和隔离验证已完成；未提交、推送、部署或执行生产迁移。详细跨仓协议见 Team 仓库 `docs/sub2api-refresh-handoff.md`。

## 服务端变化

新增迁移 `250_openai_refresh_fencing.sql`：

- 持久化刷新链、RT 哈希、轮换关系及在途票据。
- 持久化 draining/ready/acknowledged 交接状态、管理员作用域及暂存凭据。
- 记录已交回账号，并以数据库触发器阻止其通过普通更新重新获得 RT。

`OpenAIOAuthService.RefreshTokenWithClientID` 在实际请求前取得持久化票据，统一覆盖裸 RT、原生刷新执行器、请求恢复和新账号导入。已有账号的管理员单个/批量刷新改走协调器，原始账号刷新方法在没有受控上下文时拒绝消费 RT。

OpenAI 刷新成功采用完整原凭据 CAS，并校验票据和 epoch；凭据版本单调推进，与 outbox 同事务提交。新授权已写入时丢弃旧结果，记录旧 RT 的消费事实。取消、持久化不明、过期但未结束的票据都不授权再次刷新。

Redis 锁故障不会绕过 OpenAI 的 PostgreSQL 票据。保留其他平台原有行为；共享 RefreshIfNeeded 回归已执行。

## 安全交接

```text
POST /api/v1/admin/accounts/{id}/credential-refresh-handoff
```

仅管理员 API Key，拒绝浏览器 JWT；no-store，整个请求体不进入审计。prepare/read/ack 复用同一 operation_id、实例、原版本、原 updated_at 和身份前置条件。

- prepare：按刷新链阻止新尝试；已有尝试可完成受控 CAS。结果不明时一直保持阻断，不按时间强行排空。
- ready：在同一事务中移除当前已知链的账号副本 RT、ID token 和会话凭据，推进版本、写 outbox，并暂存最后确定的 AT/RT/client_id。
- read：只有原作用域、身份及操作匹配，且交接 ready 时，向 Team 后台返回必要凭据。浏览器不接收它们。
- ack：Team 本地接管已提交后清除托管内容。可重复确认；确认后 read 不再返回 RT。

交接不自动打开 schedulable、不清认证错误、额度和其他运行限制。委托账号的 AT 到期后，等待 Team 送来新 AT，不把预期的“没有 RT”写成新的永久认证错误。

Team 使用 SQLite 持久化交接编号和状态，在短写事务中再次核对完整本地凭据、绑定、归属、刷新租约与 OAuth 会话，然后写入加密 AT/RT 并切换归属。失败可继续同一交接；本地已提交、远端 ack 未确认时显示部分完成，只重试清理。

Team 后续只推送 AT/client_id/到期信息，远端数据库也拒绝 RT 回灌。交回账号不能通过普通配置推送再次委托；后续重新委托必须设计独立受控步骤。

## 能力与边界

能力 revision=5；普通同步请求 contract_version=1 不变。新增 refresh_fencing、refresh_handoff 和范围 `observed_rt_lineage`。Team 普通同步接受 revision 3/4/5，可信认证恢复接受 4/5，交回要求 5 的完整声明。

协调同一 RT 和由本系统刷新时观察到的轮换关系，不能从不透明 RT、邮箱或未验签 JWT 猜测所有外部授权族。系统外直接上游调用、未升级进程及未记录的外部令牌复制不属于该保证；通用 single_writer 仍为 false，不能借此宣称无条件全局单写。

## 发布及回退

先升级 Team 的协议兼容，再按固定双实例流程升级 Sub2API。全部会消费 RT 的进程/副本已升级且迁移成功后，才能启用交回。新副本的能力声明不能证明旧副本已排空。

迁移不批量移交现有账号。实际交接后保留新表、触发器和审计；不要通过 RT 回灌、删除归属记录、清理不明票据或回滚旧刷新代码来恢复刷新。应用回退不撤销上游已发生的令牌轮换。未确认的托管保留给同一操作继续；ack 清理失败不把归属退回远端。

正式上线仍需备份、双实例滚动和健康/缓存/outbox 检查。第六批自动轮转、生产 canary 与真实上游验证尚未进行。

## 验证

- Go 六个相关包针对性测试/编译通过；共享 TestRefreshIfNeeded、新票据/取消、管理员 API Key 边界及凭据请求体整体不入审计的测试通过。
- PostgreSQL 18.1 + Redis 8.4：14 项隔离集成通过，覆盖前批恢复、并发票据、轮换映射、旧响应 CAS、结果不明不重抢、交接排空、拒绝 RT 回灌、作用域和 ack，以及 outbox 故障回滚。
- Team：118 项 Python、28 项 JavaScript 回归通过。
- Chromium + 隔离 FastAPI/SQLite：预览/取消、等待后继续同一操作、Team 接管、暂停和认证状态保留、AT/RT 不入浏览器、1440/390 布局及抽屉清理通过。
- 两仓 diff 检查通过；Python 的既有 curl_cffi 清理忽略异常与 Starlette/httpx 弃用告警已记录，断言通过。

未运行全仓完整测试、生产迁移、真实 RT 消费或生成请求。测试全部使用合成凭据与隔离数据。
