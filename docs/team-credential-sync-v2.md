# Team × Sub2API：事务回执与远端状态（第二批）

> 本批事务回执合同继续有效。新增刷新归属与受控回读见 [第三批交付](team-credential-sync-v3.md)。

本批为本地实现与隔离验证，未部署。基于第一批安全凭据同步，能力修订号升级为 `oauth_sync.revision=3`，请求仍为 `contract_version=1`。

## 已实现

1. 凭据 CAS、操作元数据和 scheduler_outbox 同一 PostgreSQL 事务提交。任何一步失败整体回滚；提交后进程退出仍可查到操作。
2. 后台任务自动领取 pending 操作，只删除令牌缓存和刷新当前账号的共享调度快照。进程重启后继续处理未完成步骤，不调用上游 OAuth、不写凭据、不清除运行阻断。
3. 每次领取生成新的租约标识。60 秒租约配合 30 秒任务上下文；旧租约不能提交进度。缓存成功立即单独持久化，调度失败重试时不再执行已确认成功的缓存步骤。
4. 数据库持久化实例 UUID，两个应用副本共享同一身份。提交和手动重试必须携带 expected_instance_id，避免能力检查后目标实例变化。
5. 提供脱敏状态查询。返回当前身份元数据、凭据版本、最近操作、暂停/限额等已知阻断和观察时间；不返回令牌、原始错误文本或请求摘要。
6. Team 保存按绑定隔离的观察记录。验证邮箱、工作区、官方账号及已固定的实例；失败保留旧快照。观察版本 CAS 防止迟到响应覆盖较新状态。

## 接口与语义

所有接口沿用现有 Admin 鉴权，并返回 `Cache-Control: no-store`。

| 方法与路径 | 行为 |
| --- | --- |
| GET `/api/v1/admin/integration/capabilities` | 声明 revision=3、instance_id、atomic_receipts、resumable_followups、remote_state |
| POST `/api/v1/admin/accounts/{id}/sync-oauth-credentials` | 条件写入并产生持久化 pending 回执；同管理员、同账号、同 operation_id、同请求摘要重放 |
| GET `/api/v1/admin/accounts/{id}/credential-sync-operations/{operation_id}` | 查询已提交操作；查询不会触发写入或重试 |
| POST `/api/v1/admin/accounts/{id}/credential-sync-operations/{operation_id}/retry` | 只接受 expected_instance_id；将现有 pending 操作提前排队，不重新提交凭据 |
| GET `/api/v1/admin/accounts/{id}/credential-sync-state` | 当前账号的脱敏状态，以及当前管理员范围内最近的操作 |

写入请求新增必填 `expected_instance_id`，其余字段沿用第一批。实例变化返回 `SYNC_INSTANCE_MISMATCH`；相同操作 ID 配不同请求摘要返回 `IDEMPOTENCY_KEY_CONFLICT`；旧账号快照返回 `CREDENTIAL_VERSION_CONFLICT`。

| 操作 state | 含义 |
| --- | --- |
| pending | 凭据已经提交，缓存/调度尚未全部确认 |
| completed | 本次缓存与共享调度处理已确认；不代表账号实际可用 |
| needs_review | 账号已删除或类型改变等情况，自动后续处理停止 |
| unknown（查询外层） | 没有可确认的回执，不能据此推断请求从未执行 |

操作回执中的 credential_version 是历史写入版本；状态接口的 credential_version 是当前版本。`operation_is_current=false` 明确表示凭据后来又发生了变化，不能把历史完成回执当成当前授权验证。

`token_cache_invalidation=succeeded` 只确认本次删除；`scheduler_refresh=succeeded` 只确认共享 Redis 调度快照处理。运行中的请求是否已排空、旧请求能否回填缓存、上游授权或成员权限是否有效，均不在此确认范围。`availability` 固定为 `not_verified`。

## 后台恢复

启动立即扫描，空闲时每 5 秒扫描；每批最多 20 个。使用 `FOR UPDATE SKIP LOCKED` 领取短事务，Redis 调用期间不持有数据库事务。任务错误仅存稳定错误码，不存 Redis/HTTP 原始错误。

失败从 10 秒起指数退避，最大 2560 秒。手动重试不会抢占有效租约；刚更新的操作至少等待 2 秒。未完成任务由数据库保留；本批不自动删除新回执。后续引入保留策略时必须保留幂等墓碑，不能让过期操作被当成新写入。

第一批存于通用幂等表的历史回执仍可只读查询，响应标记 `source=legacy`，按旧配置过期。本批不会据旧回执自动补写凭据，也不会凭缺失信息重建恢复任务。

## Team 界面与本地数据

新增 `sub2api_sync_observations` 表，由现有 `Base.metadata.create_all` 创建，不改已有账号列。

详情抽屉新增远端状态面板，提供“重新核对状态”和“重试未完成步骤”。普通 GET 只读本地快照；核对操作才读取远端并保存新观察。处理中可在打开的抽屉内短暂自动核对，关闭、切换账号或页面隐藏时停止相应轮询。超过 120 秒的本地成功观察标为旧快照，重试按钮失效；服务端收到重试仍会先重新核对身份和实例。

独立展示“同步步骤完成”和“仍有运行阻断”。人工暂停、账号错误、限额、临时暂停、过载、过期均不会被同步操作清除。该列表是已知字段的观察，不是对所有模型/预算/路由限制的完整枚举。

凭据已提交、最终身份仍匹配且仅缓存传播 pending 时，用户显式提交的名称等配置仍可继续执行；整体仍报告 partial。配置失败保留 credential_write=succeeded，不诱导用户重新提交凭据。后续重试接口只处理缓存与调度，不代替失败配置操作。

## 迁移与发布

新增 `248_oauth_sync_operations.sql`，创建 `integration_identity` 和 `oauth_sync_operations`。该迁移已在临时 PostgreSQL 执行，未在生产执行。

发布顺序：备份并核实生产基线 → 按项目既定双实例滚动流程升级 Sub2API → 确认两个副本都返回相同实例 UUID 和 revision=3 → 再升级 Team。旧 Team 不识别 revision=3，新 Team 不向 revision=2 发送凭据更新；滚动期间凭据同步可能被明确阻止，不能用 PUT 降级绕过。

实例 UUID 代表逻辑数据库，数据库恢复保留它；克隆数据库用于独立环境时必须重新分配实例身份。不要给共享数据库的两个副本生成不同身份。Team 不会自动替换已固定的实例；发生变化必须先核实连接和绑定。

回退前暂停自动凭据推送；保留新增表与历史操作，不删除回执来“解锁”重试。旧应用不会继续处理新表中的 pending 任务，回退期间这些任务保持待处理，恢复新版本后继续。不能通过回滚应用或数据库撤销上游已经发生的令牌轮换。

## 验证

- Go service/repository/admin handler/middleware/routes/server 的针对性测试或编译检查通过，包括 Wire 生命周期接线。
- 临时 PostgreSQL 集成测试通过：并发重放只写一次；outbox 插入失败整体回滚；租约过期后接手，旧租约无法更新结果；实例身份重连不变。
- Team 61 项针对性 Python 回归及 28 项现有 JavaScript 测试通过；真实 Chromium + 隔离 FastAPI/SQLite 界面验证通过。
- 浏览器核对：未登录被拒绝、处理中、完成但暂停、失败保留旧快照、实例漂移阻止重试、窄重试、关闭抽屉停止轮询；1440 和 390 宽度无横向溢出，无页面错误。

未执行完整测试套件、生产迁移、真实上游授权或生产流量验证。跨应用刷新单写、候选授权验证、自动清错、反向令牌同步和轮转仍未开放。
