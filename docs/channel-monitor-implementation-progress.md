# 渠道监控与降智检测实施进度

## 2026-09-10 最新：人工标签、独立停用与平台链路补接

- 人工评估批量 API 已接账号列表与管理员渠道卡片。按账号/请求模型取最后审核，未审核的新问题不覆盖旧评估，unlabeled 显式撤销标签；渠道按当前成员关系汇总，删除/移出成员不再计入。仅管理员可查，不公开题目、答案、依据或内部账号。审核成功刷新标签；明确标为历史人工评估，不覆盖连通性/自动报告。
- QuestionReviewPanel 已接独立停用确认，取消不写入，确认只调用当前冻结账号的既有 status API，成功刷新账号列表。审核本身仍不改变账号状态。
- 新增 PlatformCapabilityResolver/Scheduler：显式 allowed_group_ids 准入，冻结随机/固定账号集合，多目标逐账号执行；仅 OpenAI APIKey、HTTPS、无模型映射变化/协议转换/指纹覆盖等可原样请求路径。每次发送复核账号/分组/凭据及价格摘要。OAuth、其他协议仍拒绝，不宣称已验证支持。
- runtime 增加独立平台规划循环，不阻塞恢复；managed claim 分别遵守平台/站内开关且不领取 external_api。平台入队与预算预留原子提交，完成/取消/失效清理活动指针并安排下次时间；加入随机间隔和最多一次 mismatch 复测标记。复测新增分支尚未有独立真实请求验收。
- 平台策略 API/UI 已支持创建、CAS 更新、停用、软删除、报告分页。执行配置改变触发旧任务 guard/recovery；纯改名保留调度及现代快照任务。目标新增可选 benchmark_channel：调度时冻结活动发布和 channel revision，激活通道更新关联策略 version 并唤醒空闲策略；排队失效、运行冻结、撤回停发仍分别处理。不按名称猜测通道。
- 必要验证：人工汇总 PostgreSQL 集成通过；人工审核/停用/相邻前端 23 测试通过（提速指令前）；平台生命周期 PostgreSQL 验证入队去重、改名续租、通道切换保留运行版本、配置变更旧租约拒绝、恢复结算、软删除通过。首次 SQL 参数类型冲突修复后复测通过；平台准入/既有 capability/runtime 定向单测通过。最新前端 typecheck、后端相关包编译通过，最后 Go -run '^$' 仅算编译，不算执行测试。
- 用户要求提速后停止非必要全量回归、截图与额外浏览器检查。新增 frontend/e2e/question-review-acceptance.mjs 未执行，不记浏览器签收。本轮新增平台完整 HTTP/评分端到端、复测付费去重、并发策略/基准锁序及最终部署候选仍需必要验收；不以编译通过代替这些门禁。
- 已核实既有 20 美元/1000 次双上限及有条件滚动发布授权，不重复索要。未使用生产凭据/专用验收 Key，未启用生产 runtime，模型调用 0、费用 0、生产写入 0。专用站内验收 Key、可信价格上界、备份/回滚空间及生产验收尚未落实；整体任务仍 in_progress，不能宣称已上线或全部签收。


## 2026-09-10 最新：问答审核入口及站内任务入口

- AccountHandler.Test 已接服务端 QuestionCapture，AccountTestService.sendEvent 采集实际事件；结束后以独立 5 秒保存窗口归档，返回 question_record SSE，保存失败显式通知。保留原题，限制答案大小，不接受浏览器上传伪造答案。
- 已接管理员问答列表、审核历史、CAS 审核提交 API；实时复核管理员状态。账号测试弹窗接入 QuestionReviewPanel：原题/答案、normal/degraded/unlabeled、依据、分页历史、冲突后禁止盲目重试。审核不修改账号启停。迁移 247 仍未应用生产。
- 已接管理员站内 Key 计划/幂等创建/查询报告/取消 API 及渠道监控基准页 DetectorTaskPanel。计划由批准通道和真实引擎生成，服务端冻结摘要与请求数；每用户有效计划上限 20。未知创建响应重试复用幂等键。当前仅管理员本人且 deployment allowlisted 的站内 Key，不宣称普通用户/外部 Key/平台多账号全支持。
- 新增平台策略 CAS 停用接口，停用提交后后续 dispatch/renew 被既有 guard 拒绝，恢复循环独立结算任务；不宣称已发送请求可撤销。不反转 job->policy 结算锁顺序。完整启用、多账号执行、配置变更/展示标签传播尚未完成。
- 修复 runtime 价格配置不能解析 gpt-5.4 等模型键的问题；保留未知字段、尾随 JSON 拒绝，并给价格字段添加 snake_case 标签。
- 验证：前端类型检查通过；QuestionReviewPanel/AccountTestModal/DetectorTaskPanel 共 11 测试通过；后端 handler/service/repository/server/cmd 定向 unit 通过；server/cmd 完整 unit 通过；DetectorPlanCreation/QuestionReview/Capability/MonitorSpend/MonitorJob/MonitorPolicy integration 通过 50.798s。
- Chrome CDP 离线浏览器验收通过 1440x1000 和 375x900，选模式、原题发送、显示答案、加载记录、审核提交、历史展示无运行时异常/横向溢出；mobile-review.png 已检查。脚本 handoff/run-question-review-check.mjs 启停隔离 Vite，截图 frontend/handoff/manual-question-screenshots。不是 Playwright，也不是真实模型/生产验收。首次 Vite 被无关旧 fixture 的缓存导入阻断，限定扫描到本次页面后重跑通过。
- **仍未完成**：平台完整启用/多账号采样/策略编辑传播、人工标签到账号和渠道视图、独立账号停用确认、真实协议合同证明、生产部署/模型/容量/恢复验收。生产 URL/专用站内 Key/可信价格配置尚未核实，运行配置未启用。真实模型请求 0、生产变更 0，任务 5 保持 in_progress。旧段落为历史快照，以本节为准。


## 2026-09-10 后续增量：站内目标解析及人工审核基础

- 新增 SiteCapabilityResolver：仅使用部署指定的 HTTPS Sub2API 地址与 allowlisted 站内 Key ID；逐次查询 owner/Key/group、过期/额度/开关和凭据配置摘要。换 Key、换组、价格变化及 IP 限制失败关闭。15 场景 unit 通过。没有要求另购外部 Key。
- CapabilityExecutor 已通过显式 `LLM_DETECTOR_RUNTIME_CONFIG` 配置装配到 runtime/Wire，默认缺配置仍关闭。新增 ClaimSiteCapability 在领取 SQL 内排除平台/外部任务，运行时不宣称支持平台定时检测。配置尚未在生产设置。
- 网关转换的合同状态按 unknown 处理，报告必须 not_evaluated；禁止以未经证明的站内请求合同发布 match/mismatch。底层许可请求一旦尝试即不可 fallback 再试，防止提交响应丢失后的不确定重发。
- 新增本机真实 HTTP + PostgreSQL 串联测试：3 次请求、3 条 attempt、3 次费用扣账、报告及终态结算一致。此处引擎/目标是 fixture，不宣称真实上游验收。TestCapability/TestMonitorSpend integration 通过 10.879s。
- 新增迁移 247、QuestionReviewRepository/Service：问答记录与 append-only 审核历史，CAS revision，标签 normal/degraded/unlabeled；不修改账号启停。并发审核与保留原题测试通过。新增 bounded QuestionCapture 测试覆盖跨分片凭据回显、超限与迟到事件。
- **人工审核尚未闭环**：采集器尚未接 AccountTestService，审核 API/UI、账号/渠道标签及独立停用确认未完成。不能把数据库/服务基础称为已可用的人工判定功能。
- 最新相邻 integration（QuestionReview/Capability/MonitorSpend/MonitorJob/Benchmark）通过 49.295s；Service/Repository/cmd/server 定向 unit 通过；gofmt、diff --check 通过。未跑本轮全项目回归、race、浏览器或生产容量验收。
- 仍缺 plan/job API/UI 与可信创建入口、站内合同证明、平台多账号执行/策略传播、人工审核闭环、全协议及生产容量/恢复/发布观察。未启用 runtime 配置，未创建生产 Key/campaign，真实模型请求与生产变更仍为 0。任务 5 继续 in_progress。


## 2026-09-10 增量：能力执行内部链路，尚未装配

- 新增 capability_engine.go / capability_executor.go / capability_ports.go：固定包的真实规划、解码、评分子进程入口；原始 payload 字节及摘要校验；逐物理发送 guard、结果分类及报告提交。新增 meow_adapter.py --execute-frozen，使用共享注册表提供的原始包，不退回 bootstrap 白名单。
- 新增迁移 246 和 monitor_spend_repository.go：累计 campaign 请求数与美元微单位上限，默认关闭，逐次最坏费用扣账，跨日/重启不清零，未知结果不退款。尚未配置任何生产 campaign，价格上界仍须绑定可信配置及供应商/站内专用 Key 限额，不能宣称实际美元预算已端到端落实。
- monitor_capability_repository.go 将 attempt 创建、日请求预算消费、campaign 扣账置于同一事务；重复 sample 拒绝并回滚费用，结果/报告按租约 fencing 写回，未完成样本拒绝报告，报告补入冻结基准。
- 实际验证：四个官方基准经 Go→Python 规划/评分及错误合同拒绝通过；TestMonitorSpendCampaign/TestCapabilityDispatchTransactionPostgres 通过（10.458s）；相邻 TestCapability/TestMonitorSpend/TestMonitorJob/TestMonitorBudget/TestBenchmark integration 通过（38.159s）；Python 真实引擎及本地注册表 13 测试通过（6.826s）；Service/Repository/cmd/server 定向 unit 通过。不是本轮全量、race 或浏览器验收。
- 首次费用测试失败于隔离 schema fixture 没有应用 246，已加入并重跑通过。ReadSeek diagnostics 参数互斥，未计为通过，以 Go 编译测试验证。
- **仍未完成且继续保持任务 5 in_progress**：生产 TargetResolver、owner/Key 每次权限验证、价格准入、runtime/Wire 装配、plan/job API/UI、平台多账号选择；人工判定、完整策略传播与生产验收仍未交付。runtime 的 capability executor 仍为 nil，没有开启模型请求。不能把本轮内部实现标成完整 worker 已上线。
- 用户澄清验收应使用 Sub2API 的 URL/API Key，不需要另购外部 Key。专用验收 Key 应是站内隔离限额 Key；尚未创建或使用任何凭据。此前 20 美元/1000 次双上限及有条件发布授权不变，累计模型调用 0、费用 0、生产变更 0。


## 2026-09-10 14:21 增量：真实离线批准、管理入口与版本切换

本节为最新状态。**完整项目仍未完成，未部署、未开启能力请求 worker；不能以本轮管理链路验收替代全部业务验收。**

### 本轮已交付

- `service/benchmark_registry.go`、`benchmark_validator.go` 接通真实固定 meow 适配器与共享 PostgreSQL；批准时校验部署准入、绝对路径、适配器 SHA256、编译固定的 canonical engine-lock SHA256、包摘要及真实验证收据。客户端不能提交批准收据；慢验证结束后重新检查管理员角色。子进程限制 60 秒、单并发、输入/输出上限并过滤继承环境，错误不回显路径或正文。
- `meow_adapter.py --validate-candidate` 实际验证包的内容摘要、所有档位校准/题量和 payload 兼容性；CLI audit hook 禁止网络连接操作，不启动引擎服务器或模型 transport。这不是操作系统级沙箱。
- 管理员 API `/api/v1/admin/monitor-benchmarks` 支持分页列表、导入、验证批准、CAS 激活和撤回；依赖管理认证/审计/合规中间件，Handler 和 Service 均检查管理员身份。列表 DTO 不返回原始包、引擎锁或收据。缺配置保持失败关闭，默认不启用任何模型请求。
- 管理员 `/admin/channels/monitor` 新增基准版本页签，支持上述操作、明确撤回原因、版本冲突提示、刷新/分页及卸载取消。移动端页签缩短，保留完整标题和基准名称。
- manifest 新增可选 `channel`/`channel_revision`。通道绑定计划在消费时锁定复核；过期通道引用的 queued job 不领取且回收预算，切回旧版本不重置修订号。running job 保留冻结版本，普通切换不打断；withdrawal 仍阻止后续许可/续租并恢复任务。未绑定通道的显式固定版本计划不受通道切换影响。
- Wire 真实重新生成完成。缺失工具依赖 `github.com/google/subcommands` 仅通过 handoff 临时 mod/sum 获取；项目 `backend/go.mod`、`go.sum` 未变。前端新增开发依赖 `@playwright/test@1.63.0` 和锁文件，复用已安装 Chrome，没有安装浏览器或全局工具。

### 实际验证

- 固定引擎 + Go Service + 真实 PostgreSQL：四个官方包全部走完导入、真实批准、CAS 激活、冲突拒绝和撤回；加上通道计划/queued/running/switch-back、注册表与撤回场景均通过。日志 `handoff/benchmark-channel-serial-20260910-135020.log`，28.627s，退出 0；真实引擎测试使用 `integration,meow` 标签，要求显式隔离 Python/engine 环境，不静默跳过。
- 全项目 Go unit：退出 0；`handoff/benchmark-admin-full-unit-20260910-135713.log`，Service 196.263s。全项目 Go integration：退出 0；`handoff/benchmark-admin-full-integration-20260910-140606.log`，Service 135.922s，真实 Docker/PostgreSQL/Redis，CI=true。
- 前端全量 Vitest：267 文件、1934 用例通过，350.49s；`handoff/benchmark-admin-full-vitest-20260910-135713.log`。typecheck/lint 均退出 0，`handoff/benchmark-admin-{typecheck,lint}-20260910-141629.log`；build 退出 0，`handoff/benchmark-ui-rebuild-20260910-135400.log`，保留分包/Browserslist 警告。
- 全项目 unit 标签 vet 退出 0：`handoff/benchmark-admin-final-vet-20260910-141629.log`。embed 后端可执行文件构建通过。git diff --check 通过。
- Linux Go 1.27.0 CGO=1 的事务模块定向 race 通过，22.670s；`handoff/benchmark-revocation-race-20260910-141254.log`，脚本源列表新增通道准入/切换测试，仍为独立 PostgreSQL 薄 harness，不宣称全项目 race。
- **真实登录 Playwright**：`frontend/e2e/benchmark-acceptance.mjs` 启动隔离应用、DB、Redis，管理员和普通用户经实际表单登录，不注入 JWT。真实导入/批准/激活/并发 CAS 冲突/刷新/撤回及普通用户管理 API 403 全通过；没有 mock 核心 API。深浅主题 1280/768/375px 均无横向溢出，pageErrors 为空。证据：`handoff/benchmark-browser-2026-09-10T06-20-24-894Z/`，含截图、acceptance.json 和脱敏 trace。登录后才开始 trace，令牌与本地路径脱敏后归档；临时环境自动回收。
- 本轮较早测试失败均已修复并保留记录：i18n runtime-only 单测需预编译消息函数；测试无密码 DSN 触发旧安装代码空值解析，改用随机临时密码；关闭新手引导后完成实际点击。并行构建等待超时不计作通过，后续串行重跑通过。

### 新授权，覆盖此前只读/零费用边界

本轮用户通过结构化确认明确同意：真实模型验收总上限 **20 美元且最多 1000 次请求**，任一上限先到即停止，使用限额可撤销专用 Key，不使用普通业务 Key。**当前累计模型调用 0、费用 0。**

用户同时授权：全部发布准入通过、空间充足且回滚资源落实后，执行一次新鲜备份、迁移、备用先行的双实例滚动切换和至少两小时观察。不授权生产数据清理、付费扩容或破坏性数据库恢复。授权不替代开发/验收门禁，本轮没有读取或使用 VPS 登录凭据、没有生产变更。

### 仍未完成

1. 完整 capability worker、物理请求/attempt/评分报告链路、用户/Key 逐请求权限和美元硬预算；当前 runtime 的 capability executor 仍为 nil，不能发送请求。
2. 完整 plan/job 用户与管理员操作入口，以及活动基准到 group policy 的自动 evaluation_revision 传播；本轮只完成通道绑定的冻结计划/排队任务失效，不宣称完整策略编辑链路已接通。
3. 人工问答记录持久化、人工判定、账号/渠道标注、审核历史和独立停用确认。
4. 完整观测页/权限/全协议/异步审计、生产规模性能/容量、迁移锁耗时与恢复演练；本轮浏览器验收只覆盖基准管理业务，不覆盖全部 /monitor 功能。
5. 生产资源与备份恢复方案、专用 Canary Key 准入、最终固定发布候选及滚动发布/两小时观察。不得重复索要本节已有授权，但新增清理、扩容或扩大费用范围仍需另行确认。


## 2026-09-10 12:48 增量：基准撤回的执行门禁与恢复

本节为当前最新增量，覆盖历史记录中“逐请求预算不检查基准撤回、撤回任务未回收”的状态；**完整能力 worker/API/UI 尚未完成，未部署，也未开启检测。**

- `benchmark_release_admission.go` 新增冻结快照的逐次核验；能力任务每次预算 Consume 均检查所有基准的批准状态、包摘要、引擎锁摘要和版本字段。缺少绑定、空/非法基准列表和不匹配版本失败关闭；探活保持独立，不要求能力基准。
- 事务按 job、排序后的 release、预算作用域加锁，release 共享锁保持到许可事务提交。撤回与许可建立数据库顺序；已发放许可不等价于物理请求已发送，也不能撤销已在途网络请求。完整能力传输桥仍未实现，不宣称实现网络 exactly-once 或即时中止在途请求。
- Claim、Renew、成功 Finish 增加基准有效性与事务内复核；撤回后不再领取/续租/成功收尾。RecoverNext 自动回收不可用基准任务：queued -> skipped、running -> interrupted，记录 `benchmark_unavailable`，失效旧租约并释放未消费预留，保留已派发计数。复用既有 execution/attempt 中断与 uncertain 逻辑。
- 新增 `benchmark_dispatch_admission_integration_test.go`：8 类撤回/绑定/摘要/版本/多基准/空值拒绝及恢复，撤回与许可事务锁排序，跨日拒绝不生成新预算桶，queued/running 回收与旧 worker 拒绝。通用预算及外部 Key 亲和测试夹具补入已批准基准，未放宽生产门禁。
- 最终 Repository 全量 integration：退出码 0，128.426s，`handoff/benchmark-final-repository-integration-20260910-124404.log`；真实项目 PostgreSQL/Redis harness，CI=true，完整迁移链。
- 最终 Repository 全量 unit：退出码 0，6.066s，`handoff/benchmark-final-unit-20260910-124404.log`；integration 标签 Repository vet 退出码 0，`handoff/benchmark-final-vet-20260910-124404.log`。本轮较早 Repository/Service 全量 unit 通过，日志 `handoff/benchmark-dispatch-unit-20260910-123227.log`，Service 192.893s。
- Linux Go 1.27.0、CGO=1 定向 race：退出码 0，40.137s，`handoff/benchmark-revocation-race-20260910-124412.log`。脚本 `handoff/benchmark-revocation-race-20260910.{ps1,sh}` 记录精确源文件及正则；使用薄 harness、隔离 PostgreSQL、内部 Docker 网络和缓存镜像，不是全项目 race。临时容器/网络已确认回收。
- gofmt、git diff --check 通过。ReadSeek 在调用时遇到必填参数组合互斥，未将其 diagnostics 计为通过，实际使用 Go 编译、测试及 vet 验证。首次新增测试因整个 snapshot=null 被现有数据库约束拦截而失败，已改为合法对象内 benchmarks=null，失败日志 `handoff/benchmark-dispatch-integration-20260910-123008.log` 保留。
- 本轮没有新增迁移或依赖，没有改前端，没有执行生产登录、迁移、发布或模型请求。候选仍是既有 HEAD 上的未提交工作树，不冒充固定发布提交；本轮未重跑前端或整个后端质量门禁。

### 下一步必须完成

1. 真实离线适配器批准流程接共享 PostgreSQL、服务层权限与管理员 API/UI；不能接受客户端自行提供的批准收据。
2. 完整 capability worker、物理请求/attempt 桥、评分报告、取消及 owner/Key 权限逐请求核验；当前新增门禁只是其必要条件。
3. 活动 channel 切换与策略 revision、排队计划失效合同；当前只实现撤回/无效冻结引用的任务回收，不把切换等同撤回。
4. 人工问答记录持久化、人工判定、账号/渠道标注及独立停用确认；现有逐账号问答不等于该闭环。
5. 真实登录 Playwright、全协议/异步/安全完整审计、生产规模性能与迁移锁耗时、最终固定候选全量门禁。
6. 生产空间/备份恢复方案、明确生产写入授权及付费模型次数/费用上限，再进行滚动部署与两小时活动观察。下载目录原验收复选框不得因此批量勾选。

## 2026-09-10 全项目质量检查重跑

本节替代历史记录中“本轮未跑全量”的质量检查状态，不替代尚未实现功能的验收。

- Go 1.27.0，GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off、GOWORK=off；`go test -p 3 -mod=readonly -tags=unit ./... -count=1 -timeout=300s` 全项目通过，退出码 0。日志 `handoff/full-unit-20260910-115632.log`。
- CI=true；`go test -p 2 -mod=readonly -tags=integration ./... -count=1 -timeout=300s` 全项目通过，退出码 0。Repository 65.507s，Service 134.672s；包含隔离数据库完整迁移链。日志 `handoff/full-integration-20260910-115632.log`。这不是用筛选正则缩小的定向运行。
- `pnpm test:run`：266 个文件、1930 项测试通过，110.42s，退出码 0；`handoff/full-vitest-20260910-115632.log`。日志保留 i18n compiler/缺翻译警告和预期失败场景的错误输出。
- `pnpm typecheck`、`pnpm lint:check`、`pnpm build` 全部退出码 0；日志分别为 `handoff/full-typecheck-20260910-120447.log`、`full-lint-check-20260910-120447.log`、`full-build-20260910-120447.log`。构建更新本地 embedded frontend dist；保留大分包及静态/动态导入混用警告，不为消警告调整阈值。
- `go vet -p 3 -mod=readonly -tags=unit ./...` 及 integration 标签全项目均退出码 0，日志 `handoff/full-vet-{unit,integration}-20260910-120447.log`。前端构建后 `go build -p 3 -mod=readonly ./...` 退出码 0。
- 本轮没有业务源码修复，因为上述检查未发现失败项。没有发布、生产迁移或主动模型调用。
- **尚未完成**：本轮 Go race、真实登录浏览器端到端、生产规模性能/容量与完整协议人工审计；真实适配器批准流程、管理员版本管理入口、逐请求撤回及完整能力 worker 仍有功能缺口。全量质量检查通过不能据此开启检测或批准部署。

## 增量：共享基准注册表与创建门禁

- 新增迁移 245，仅新增 PostgreSQL 包、release、channel 和 audit 表；没有替换旧迁移或自动批准任何版本。原始包与引擎锁保存为 BYTEA，避免 JSONB 重排改变字节摘要。
- 新增 `benchmark_registry_repository.go`：不可变候选入库、幂等创建、内部可信验证收据批准、共享 channel CAS 激活、冻结引用、撤回和事务审计。撤回保留版本/收据/channel revision，不自动回退。审核及管理员权限尚需服务层装配，不能直接把内部批准方法暴露为接收客户端收据的接口。
- `DetectorBenchmarkManifest` 新增 release_id/engine_lock_sha256。`CreateFromPlan` 现在要求有效引用，在计划消费与预算预留同一事务内锁定并检查 release 状态、包摘要、引擎摘要和版本字段。无引用的旧计划、新版本摘要不匹配和已撤回计划拒绝创建；已创建任务的幂等返回保持不变，不重新预留预算。
- 真实 PostgreSQL 定向验证通过：共享注册表生命周期/并发激活/审计失败回滚，3 个拒绝创建场景，4 个原子计划消费场景；日志 `handoff/benchmark-admission-*.log`。隔离完整迁移链包含 245。
- 最终相邻 integration：`TestBenchmark|TestMonitorJob|TestMonitorBudget|TestMonitorProbe` 全通过，106.744s；repository/service 的 `Monitor|Detector|Benchmark` unit 全通过，0.658s/0.637s；git diff --check 通过。不是全项目验收或本轮 race。
- 初次测试命令超时未取得结果；再次运行发现候选的 NULL 收据不能直接扫描至当前 Go 的 json.RawMessage，已改为可空字节切片并通过重跑，失败日志未覆盖。
- 剩余：将真实离线适配器批准流程接服务端、管理员 API/UI、活动版本切换后的排队计划失效与策略 revision 传播、逐发送撤回检查和既有排队任务恢复。能力执行仍不开放。生产迁移、付费模型调用和部署均未执行。

## 增量：meow 离线适配与基准候选更新

- 已新增 `workers/capability/meow_adapter.py`，复用固定提交的原始 payload builder、normalizer、calibration 检查和 score_counts，不启动上游 HTTP transport/server。33 个固定源码/基准文件摘要校验通过。输出不包含样本原文或可能含回显内容的类别字符串；请求合同改变时公开结果为 not_evaluated。
- 新增本地 `benchmark_catalog.py`：SQLite 事务实现候选、批准、撤回、显式激活 CAS、冻结引用及审计；同 ID/版本的不同内容拒绝覆盖。批准时用实际适配器检查所有档位的校准、题量与 payload 兼容性；保留旧包和验证收据。
- `handle_release` 可读取已批准、但不在旧 bootstrap 白名单中的新基准；更新活动引用不改变旧引用，撤回后拒绝继续解析该版本。新版引擎仍需适配器兼容性审计，不自动加载任意提交。
- 本轮最终 12 个 unittest 全部通过，无跳过，4.368s。包括 9 个本地注册表状态/并发测试和 3 个真实固定引擎测试；覆盖 4 个官方基准的低/中/高档 payload 与有样本/无样本评分对照，以及新增版本 fixture 的批准/执行/撤回。新增版本 fixture 不是官方新版发布。
- 测试使用项目 `handoff/meow-adapter-venv` 内 NumPy 2.3.5、httpx 0.28.1；未改系统 Python、Go 或前端依赖。日志 `handoff/meow-catalog-final-*.log`；第一次新增版本 fixture 因非规范版本号被引擎拒绝，失败日志保留，修正 fixture 后重跑通过。
- 当前仅为本地离线组件，**没有接入生产共享 PostgreSQL 准入、管理员 API/UI、排队计划失效、策略 revision 传播或逐发送撤回检查**。bootstrap CLI 不读取注册表撤回状态，不能作为受注册表控制的线上 worker 入口。没有执行模型调用、生产变更或发布；浏览器/全协议/性能完整验收仍未完成。

## 2026-09-10 增量：逐账号人工问答

本节覆盖下面历史记录中与本轮改动冲突的状态；整体系统仍未完成、未部署。

- 管理员账号测试新增 `question` 模式，支持直接 OpenAI API key/OAuth 文本账号；拒绝影子账号、Agent Identity、插件 OAuth 传输和图片模型。默认填入用户原题 `don't search the internet, who is Thibault Sottiaux on X`，允许编辑，原文（包括首尾空白）送往所选账号。
- Responses 分支在问答模式使用自定义题目，不再发送固定 `hi`；Chat Completions 同样保留原文。无工具声明，45 秒超时，API key 请求限制输出 token。没有证明上游内部不联网，没有把该链路冒充 meow 基准请求契约。
- 问答模式不执行现有 401 错误标记、429 限流/套餐更新、OAuth 响应头快照更新或 Handler 成功后账号恢复。常规/compact 连接测试保持原行为；不基于回答自动停用或恢复账号。
- 同一页面切换账号保留题目、清空回答、不自动发送；取消后的迟到响应或错误不能覆盖新测试。题目/回答未写入浏览器持久存储。
- 修复新增 MonitorJobRuntime 后 `cmd/server/wire_gen_test.go` 未补齐 provideCleanup 参数导致的编译失败。
- 定向 Go 测试通过：cmd/server、service 的 `TestAccountQuestion|TestAccountTestService_OpenAI|TestProvide|TestMonitorProbe|TestMonitorJobRuntime`；admin handler 编译通过，但筛选下无 handler 用例，不能宣称已覆盖 handler 行为。前端两个账号测试文件 8 项通过；vue-tsc --noEmit 通过。
- Chrome CDP（非 Playwright）隔离浏览器夹具通过 1440x1000/375x900 检查，原题、显式发送、固定账号路径、回答展示，无横向溢出或 runtime exception；截图和收据在 `frontend/handoff/manual-question-screenshots/`。浏览器和 Go 均使用 fixture，真实模型调用 0。
- 全量 integration 上次运行的 Repository/service 等包通过，但整体因 cmd/server 编译失败而失败；修复后只完成定向回归，不能称全量门禁通过。全量 unit 被中断，未取得最终结果。
- 仍缺：问答检测记录持久化、人工答题判断、正常/降智/未标注的账号及渠道标注、审核历史、独立停用确认流程、meow adapter 实际实现与完整 worker/API/UI 接入、最终全量/生产验收。`workers/capability/` 的说明与锁文件不代表引擎已实现。

## 最新状态：2026-09-10 任务与预算事务基础

本轮承接 Downloads/降职问题 中的三份交接文件，保留全部既有工作树改动。**任务与预算的 Repository 基础已实现并通过本轮验证；主动执行完整链路仍未完成，不能部署或开启探活/检测。** 下方 2026-09-09 的全量门禁是历史基线，不代替本轮候选验收。

### 本轮实现

- 新增 `backend/internal/repository/monitor_budget_repository.go`：全局及用户/分组必需作用域校验、稳定顺序加锁、最坏请求数原子预留、重复预留幂等、出站计数 CAS、租约/取消/截止检查、UTC 跨日重新预留、终态幂等释放。旧日已消费量保留，重新预留失败时整体回滚；日额度不能通过请求参数直接抬高。
- 新增 `monitor_job_create.go`：从已有私有 plan 冻结 snapshot，owner 与配置 hash 校验；计划一次性消费、幂等 job 和全部预算预留处于同一事务。预算不足或计划过期不留下 job/预留；相同幂等键配不同计划或配置拒绝。
- 新增 `monitor_job_repository.go`：按 availability/capability 分池的 `FOR UPDATE SKIP LOCKED` 领取、90 秒租约、generation fencing、外部凭据所属实例亲和、owner-scoped 取消、完成证据检查及过期/配置失效恢复。领取拒绝无预算任务；续租在取得行锁后重新检查数据库时间，不复活已过期租约。
- 恢复事务同时更新 job、未终结 execution、attempt 与预算。派发中且无可靠终态的 attempt 转为 `uncertain`，未派发转为 `cancelled_before_dispatch`；不透明重发，不返还已经计为派发的请求数。终止后 generation 提升，旧 worker 不可写回。
- 完成任务不自动改变检测 verdict；缺报告不能完成，已有 `insufficient` 不被改成 match。此处测试报告是数据库 fixture，不是引擎验收。
- 未改迁移、Ent、Wire、前端、go.mod/go.sum，没有接入公网接口或启动 worker，所有原执行开关保持不变。

### 本轮验证与证据

环境：HEAD `2866c5a80fbc8f1dbdf7294b5801124bd5f29d13`，既有脏工作树上增量开发；Go 1.27.0，Docker Desktop 29.6.2，PostgreSQL 18.1，真实项目 integration harness 另启 Redis 8.4。Windows 日志文件名采用本机 +08:00 时间，Go 日志初始化后显示 UTC。GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off、GOWORK=off、-mod=readonly；集成测试设置 CI=true，未静默跳过。

- 新增 22 组 PostgreSQL 场景全部通过：预算 10、计划消费 4、状态/配置门禁 3、生命周期 5。覆盖多 worker、相同/不同幂等键争抢、跨 owner 拒绝、额度不足、重复结算、跨日、取消、过期、SQL 故障整体回滚和 uncertain 恢复。
- `go test -mod=readonly -tags=integration ./internal/repository -run 'TestMonitorBudgetPostgres|TestMonitorJob' -count=1 -v -timeout=120s`：退出码 0，15.597s；`handoff/monitor-jobs-integration-20260910-074842.log`。
- Repository 全量 unit：退出码 0，4.545s；`handoff/monitor-jobs-repository-unit-20260910-075302.log`。
- Repository 全量 integration：退出码 0，68.520s；`handoff/monitor-jobs-repository-integration-20260910-075302.log`。使用真实项目 TestMain，包含完整新库迁移链；不是只运行新测试。
- Linux `CGO_ENABLED=1 -race`：退出码 0，8.831s；`handoff/monitor-jobs-race-20260910-075308.log`。使用未改写的本轮测试/Repository 源文件、独立 PostgreSQL 和 `handoff/monitor-jobs-race/main_test.go` 薄 harness；22 组数据库场景及 3 个 unit 顶层函数通过。命令与准确文件列表由 `run-race.ps1` / `run-race.sh` 和日志记录。这是新事务模块定向 race，不是全项目或完整 integration harness 的 race。
- Linux race 使用已缓存镜像和 Go 依赖、internal Docker 网络、只读源码挂载；临时测试容器/网络已回收，没有连接 VPS 或模型上游。
- Repository integration `go vet` 已退出 0；本轮源码 gofmt 及 git diff --check 通过。ReadSeek content/diagnostics 遇到工具参数互斥/范围校验错误，未计为通过；以 Go 编译、vet 和真实测试补充验证。
- 首两次数据库运行失败于新增 fixture 的 UUID/text 参数复用、日期加法参数未明确整数类型，已修正并保留 `handoff/monitor-budget-integration-20260910-072048.log`、`...-072332.log`，没有覆盖旧日志。首次 PowerShell 测试包装命令的引号错误未运行 Go，不计作测试结果。

### 尚未解决的边界

1. 仍缺平台 occurrence 原子派发、完成后间隔调度、真实 worker pool 及启动/停止装配；当前构造器不启动后台活动。
2. `Consume` 是预算记账许可，不是完整 attempt/出站桥接。attempt 创建、冻结请求验证、物理发送、结果写回还没有同事务/受控执行入口；不能据此承诺上游 exactly-once 或实际固定账号路由。
3. owner/Key 权限、运行开关、引擎准入、配置版本和传输取消仍需在应用/bridge 层逐请求校验。Repository 的 guard 不替代 SSRF、DNS/重定向防护、密钥生命周期或费用硬上限。
4. 探活 challenge、固定 meow 引擎实际适配、离线基准核验、真实模型验收，以及全协议/异步、浏览器登录、性能容量与最终候选全项目门禁仍未完成。
5. 生产磁盘/备份恢复资源、付费模型次数与费用上限、生产写入/迁移/部署授权仍未落实。本轮没有读取登录文件内容、使用生产凭据、调用模型、迁移或部署生产。

## 2026-09-09 历史验收状态

更新于 2026-09-09 晚间。本节覆盖下方历史阶段记录中的旧状态，不代表全部 27 项已经完成。

**全量质量门禁已通过，完整业务验收和发布准入仍未完成。** usage/ops 来源、Ent 持久化、V2 观测聚合及页面卡片已有实现，隔离 PostgreSQL 迁移已实测。没有开启检测，没有部署。

### 本轮实际验收

- Go 1.27.0：`go test -mod=readonly -tags=unit ./... -count=1 -timeout=300s`，退出码 0。
- Go 1.27.0：`go test -mod=readonly -tags=integration ./... -count=1 -timeout=300s`，退出码 0；设置 `CI=true`，Docker 不可用时不能静默跳过。新增恢复测试后，Repository 全量再次通过，35.909s。
- Vitest：266 个文件、1927 个测试全部通过；前端 typecheck、lint:check、build 及 Go vet 全部退出码 0。
- Linux Go 1.27.0 容器、禁用网络、CGO=1：监控相关 service/repository/handler 定向 `-race` 通过，不等价于整个后端全量 race。
- 新增 `backend/internal/repository/channel_monitor_recovery_integration_test.go`：25 组保留周期边界、回填期间不清理、重复清理幂等；注入最终水位 SQL 故障，验证分钟事实、rollup、TPS、可见 TTFT 和水位整体回滚，解除故障后两次重算恢复通过。
- `gofmt`、`git diff --check` 通过。ReadSeek diagnostics 因工具参数互斥失败，未计为通过；Go 编译和真实数据库测试已执行。
- 日志：`handoff/monitor-acceptance-*-current.log`、`handoff/monitor-acceptance-repository-final.log`。

### 生产只读与授权

用户明确只授权生产只读检查，同时确认检测引擎已有适用授权。没有生产写入、迁移、部署或付费模型调用。

- SSH 使用已有 known_hosts 严格校验；凭据仅在内存用于连接，不复制进源码或验收文件。
- 生产源码 `/opt/sub2api/source-main` 无未处理改动，HEAD `2866c5a80fbc8f1dbdf7294b5801124bd5f29d13`。
- `sub2api` 与 `sub2api-canary` 均 healthy，镜像 `sub2api-hixz12:0.2.1-hixz12.8-2866c5a80`，image ID 一致：`sha256:15cb0c40e4309ee4a715fae00b52d65d2e481cadb8755ba62700e9bc9a4873e7`。
- 8100、8101 直连及生产 HTTPS `/health` 均成功；Nginx 为 8101 主、8100 backup，`proxy_next_upstream off`。
- 生产最大迁移为 237。数据库报告 8571 MB；磁盘可用 5.5 GB，尚未证明可容纳新鲜备份、恢复和新镜像，发布阻塞。
- PostgreSQL 只读会话、10 秒 statement_timeout：usage_logs 估算 963883 行、2255 MB；ops_error_logs 估算 101329 行、250 MB。不是精确 COUNT 或完整容量评估。
- 候选 `handoff/monitor-migration-candidate.json` 包含 238-243 的依赖、原文件与 runner 摘要、锁风险和回滚原则，六文件十二个摘要已复核。不是已获批执行计划；生产耗时仍未知。
- 固定提交 `chen-006/meow-llm-detector@fdb89c99852e0d5558551168835387b835265942` 的 LICENSE、README 和基准索引已在线读取，确认版本 4.5.2 及 PolyForm Noncommercial 1.0.0；用户适用授权是用户确认，不等价于代理完成法律审查。尚未引入或运行引擎。

### 尚未完成

全协议完整审计、真实登录浏览器端到端、生产规模 EXPLAIN/容量与迁移锁耗时、调度/租约/预算/检测执行链路、完整引擎适配及实际模型请求均未完成。生产迁移、备份、Canary、双实例滚动和两小时观察未执行。新增数据库恢复用例不能替代以上验收。

`.gitignore` 已为本文件增加精确例外，P0 所述“文档被忽略”属于历史状态；既有 handoff 内容保留，本轮新增验收日志和迁移候选。

## P0 历史记录

本次按蓝图第 0.3、17、22 节只执行 P0，不进入功能开发。用户要求减少测试、加快速度，因此选择少量定向基线检查，不运行全量测试、构建或集成环境。

**状态：本地核心路径核对与改动清单已记录；P0 验证存在阻塞，不能标记全部通过。P1 尚未开始，需负责人确认。**

- 蓝图来源：`C:/Users/Lovewell/Downloads/Sub2api_渠道监控与降智检测_实施蓝图.md`，版本 1.0，文档日期 2026-09-08；全文已阅读。
- 工作树：`C:/Projects/VPS/速维云-美国-8H8G-三网/sub2api-hixz12-main`。
- 分支：`main`。
- 实施基线：`21173706d672611051a8bb926ec5745fe0b2642c`。
- 最近提交：`fix(openai): harden pre-output recovery and isolated acceptance`。
- origin：`https://github.com/hixz12d/sub2api-hixz12.git`。Go module 仍为 `github.com/Wei-Shaw/sub2api`，不能仅凭 module 名误判为另一个工作树。
- 蓝图基线 `6e7cff538ab19f7b6bbc3dafdd74d9443c9be55c` 是当前 HEAD 的祖先。后续以当前 HEAD 为准，不退回蓝图旧提交。
- 初始未跟踪内容：`frontend/handoff/`、`handoff/`。没有已跟踪文件差异；这两个目录未修改、未清理。
- 目标源码树内未发现额外 `AGENTS.md`。遵守已加载的个人规则与父项目规则。
- 本次唯一新增文件：`docs/channel-monitor-implementation-progress.md`。
- Git 可见性：`.gitignore:135` 的 `docs/*` 会忽略该新文档，`git check-ignore -v docs/channel-monitor-implementation-progress.md` 已确认。文件已写入磁盘，但默认 `git status` 不显示；本次未修改忽略规则或暂存文件。后续提交时需为该进度文件添加精确例外或单独强制添加。
- 真实模型 API 调用：**0**。没有使用凭据、连接生产数据库、SSH、部署、迁移、生成 Ent/Wire、引入引擎或修改依赖。

## P0：本地事实与兼容边界

### 已确认的关键事实

1. 旧 TPS 是时间窗口总吞吐换算。`frontend/src/features/channel-monitor-v2/monitorFormat.ts` 的 `tokensPerSecondFromTpm` 返回 `Number(tpm) / 60`，`ChannelStatusV2View.vue` 在概览和表格调用该链。不能复用为单请求输出速率 P50。
2. V1 的 `GroupName` 是字符串展示字段。`channel_monitor_types.go` 不提供本蓝图所需的实际分组策略绑定，禁止按同名字符串自动迁移。
3. V1 门禁位于 `setting_public.go` 的 `ChannelMonitorRuntime.ActiveProbesAllowed`，条件为 `Enabled && Mode == ChannelMonitorModeV1`。`ChannelMonitorRunner.fire` 和 `ChannelMonitorService.RunCheck` 都有检查。新主动检测不能通过移除此门禁接入。
4. V2 被动聚合门禁为 `Enabled && Mode == ChannelMonitorModeV2`。已有聚合器使用持久化水位、leader lock、最近十分钟重叠重算及分段历史回填，不能另建无限全表扫描服务。
5. 旧卡片 identity 是 `platform + group_id + model`。`repository/channel_monitor_v2_cards.go` 从 rollup 选取 identity，并使用 `HAVING SUM(m.success_requests+m.error_requests)>0`，没有流量的策略不会出现。新接口必须从策略按组分页，再左连接指标。
6. 旧卡片查询使用只读 RepeatableRead 事务，读取水位、coverage 和聚合数据；已有 `as_of` 校验不能直接当成新业务/探活/检测三来源快照协议。
7. 用户权限入口是 `ChannelMonitorV2Handler.scopeFilter`：从当前登录用户调用 `GetAvailableGroups`，设置 `RestrictGroups=true` 和 `AllowedGroupIDs`。Repository 的 `channelMonitorV2ScopedGroupIDs` 对配置、用户可见组、请求组求交，受限空列表返回空范围。新列表和详情必须保留这条服务端边界。
8. `gateway_usage_billing.go:recordUsageCore` 在强制缓存计费时直接将 `InputTokens` 加入 `CacheReadInputTokens` 并把前者归零。新观测必须在改写前独立保存，不能从现有计费值反推缓存读取率。
9. 在已检查的 usage Ent schema、UsageLog 服务模型、功能开关文件中未找到本蓝图的新请求来源和输出 TPS 观测字段。此结论仅针对已检查范围，不等于完成整个仓库的所有协议审计。
10. 用户监控实际路由是 `/monitor`，不是 `/channel-status`；管理员是 `/admin/channels/monitor`。`ChannelStatusView.vue` 根据 V1 模式渲染 V1，否则渲染 V2。应在此增量接入新展示开关。
11. 监控核心文件的 `git diff --name-only <蓝图基线> HEAD -- <监控路径>` 本次没有输出；这不代表网关、工具链或整个仓库没有变化。

### 实际路径映射

以下路径均相对于仓库根。区分已读取相关实现与仅定位的后续入口，不能把本表当成全文件审计证明。

| 职责 | 实际路径 / 符号 | 核对程度与后续处理 |
| --- | --- | --- |
| 管理端监控 | `frontend/src/views/admin/ChannelMonitorView.vue` | 已定位；P3/P9 修改前完整阅读 |
| Legacy 表单 | `frontend/src/components/admin/monitor/MonitorFormDialog.vue` | 蓝图指定；修改前复核表单与调用方，不改旧 payload |
| V2 设置 | `frontend/src/features/channel-monitor-v2/MonitorSettingsPanel.vue` | 已定位；保留全局设置 |
| 用户包装入口 | `frontend/src/views/user/ChannelStatusView.vue` | 已读；新展示开关入口 |
| 用户旧分析页 | `frontend/src/views/user/ChannelStatusV2View.vue` | 已核对 TPS 调用、路由引用；保留分析入口 |
| 前端旧 API | `frontend/src/api/channelMonitorV2.ts` | 蓝图指定，P1/P4 前复核类型全貌；新 group API 独立 |
| 菜单与路由 | `frontend/src/components/layout/AppSidebar.vue`、`frontend/src/router/index.ts` | 已定位菜单项与 `/monitor` 注册 |
| 前端开关 | `frontend/src/utils/featureFlags.ts` | 已核对设置同步链说明及现有监控标志 |
| 国际化 | `frontend/src/i18n/locales/{zh,en}/channelMonitorV2.ts`、`common.ts`、`admin/channels.ts`、各语言 `index.ts` | 已定位；后续按实际键归属双语修改 |
| V1 服务/任务 | `backend/internal/service/channel_monitor_{types,service,runner}.go` | 已核对 GroupName、RunCheck、fire、inFlight 与停止逻辑 |
| V2 服务与卡片 | `backend/internal/service/channel_monitor_v2.go`、`channel_monitor_v2_cards.go` | 已定位；后续独立 group DTO，不改旧 identity |
| V2 Repository | `backend/internal/repository/channel_monitor_v2_repo.go` | `channelMonitorV2Repository`、`metricAccumulator`；已核对权限范围函数 |
| V2 卡片 Repository | `backend/internal/repository/channel_monitor_v2_cards.go` | 已读查询入口、快照和 identity 分页 |
| V2 重算 | `backend/internal/repository/channel_monitor_v2_aggregation.go` | 已读 `RecomputeRange`、usage/error SQL、直方图、固定 rollup、保留与水位 |
| V2 调度 | `backend/internal/service/channel_monitor_v2_aggregator.go` | 已读启动、停止、门禁、leader lock、重算与恢复水位 |
| 用户 Handler | `backend/internal/handler/channel_monitor_v2_handler.go`、`channel_monitor_v2_cards_handler.go` | 已读；复用 scope 和响应包装 |
| API 注册 | `backend/internal/server/routes/user.go`、`admin.go` | 已定位监控路由注册；新接口复用鉴权路由组 |
| 组合路由 | `backend/internal/service/composite_route_resolver.go` | 已读 Resolve：显式路由、账号模型归属、内置平台识别；未验证主动固定账号兼容 |
| 业务 Handler | `backend/internal/handler/gateway_handler.go`、`openai_gateway_handler.go` | 已定位账号选择、Forward、异步 RecordUsage 调用 |
| 网关转发 | `backend/internal/service/gateway_forward.go`、`openai_gateway_forward.go` | 已定位 Forward；P2/P6 按具体协议进一步阅读 |
| OpenAI 流处理 | `backend/internal/service/openai_gateway_response_handling.go` | 已定位 handleStreamingResponseWithReasoning、首输出/终态观察；尚未证明每条协议符合新 TTFT |
| 协议转换 | `gateway_forward_as_chat_completions.go`、`gateway_forward_as_responses.go`、`openai_gateway_responses_anthropic_native.go`，均在 `backend/internal/service/` | 已定位，P2 必须分别验证文本/工具/推理事件 |
| Usage 服务模型 | `backend/internal/service/usage_log.go` | 已定位 GroupID、FirstTokenMs；新增独立 nullable 观测字段 |
| Usage 记账 | `backend/internal/service/gateway_usage_billing.go`、`openai_gateway_usage.go` | 已核对强制缓存改写点，定位 OpenAI RecordUsage |
| Usage 异步落库 | `backend/internal/service/usage_record_worker_pool.go`、`backend/internal/repository/usage_log_repo_insert.go` | 已定位 Create/CreateBestEffort/批量插入；P2 修改需审查插入列与批处理映射 |
| Usage Ent | `backend/ent/schema/usage_log.go` | 已定位；不是手改 `backend/ent/usagelog*.go` |
| Ops 写入 | `backend/internal/handler/ops_error_logger.go`、`backend/internal/service/ops_models.go`、`backend/internal/repository/ops_repo.go` | 已定位 OpsErrorLog 与原生 INSERT；不可假定另有 `ops_error_log.go` Ent schema |
| Settings | `backend/internal/service/{domain_constants,settings_view,setting_public,setting_service}.go` | 已定位；`PublicSettingsInjectionPayload` 实际在 setting_public.go |
| Settings DTO 与前端映射 | `backend/internal/handler/dto/settings.go`、`frontend/src/types/index.ts` | featureFlags 注释标出的关联入口；P1 需继续核对更新/默认值全链 |
| 装配 | `backend/internal/{repository,service,handler}/wire.go`、`backend/cmd/server/wire.go` | 已定位 ProvideChannelMonitorRunner/ProvideChannelMonitorV2Aggregator 与停止回调 |
| 生成入口 | `backend/ent/generate.go`、`backend/cmd/server/main.go` | 已核对 go:generate；生成 `backend/cmd/server/wire_gen.go`，不手改 |
| 数据迁移 | `backend/migrations/`、`backend/internal/repository/migrations_runner.go` | 已读迁移 README，定位 runner；当前 SQL 最大前缀为 237，P1 前重新确认空闲编号 |

### 实际调用链与待验证边界

```text
server/routes 的业务协议路由
  -> gateway_handler.go / openai_gateway_handler.go
  -> 组合目标解析与模型路由（CompositeRouteResolver.Resolve）
  -> SelectAccountWithLoadAwareness / SelectAccountWithSchedulerForCapability
  -> GatewayService.Forward / OpenAIGatewayService.Forward
  -> 提示/模型/effort 等请求策略与协议转换
  -> 使用所选账号发送，解析流事件与终态 usage
  -> Handler 提交 usage 异步任务
  -> gateway_usage_billing.go / openai_gateway_usage.go 的 RecordUsage
  -> UsageLog Repository / CreateBestEffort / 批量插入
  -> usage_logs

错误支路：ops_error_logger.go -> OpsErrorLog -> ops_repo.go -> ops_error_logs

两侧事实 -> ChannelMonitorV2Aggregator（门禁、leader lock、水位）
  -> channelMonitorV2Repository.RecomputeRange
  -> 1m 事实、固定 rollup、延迟直方图、水位
  -> scopeFilter + Repository 分组权限交集
  -> V2 API -> ChannelStatusView -> ChannelStatusV2View
```

这是已定位的主链，不表示所有 Provider、WebSocket、工具输出及重试路径已逐条审阅。P2 必须证明可见文本首末事件与可靠 token 口径；P6 必须以实际发送验证固定账号，不能仅记录 account_id。现有错误 SQL 按 request_id 去重，但最终失败与成功重试的跨事实一致性仍需定向用例，不能只凭静态阅读宣布已正确。

### 迁移与生成约束

- SQL 文件由自定义 runner 管理，校验历史文件 checksum；普通 SQL 事务执行，`_notx.sql` 用于并发索引。
- 迁移 README 同时存在过时的 down/make 示例；后续以 runner 和实际 Makefile 为准，本次没有执行这些示例。
- 新表 forward-only，旧业务列不改含义；不填造历史 TPS/缓存观测。
- Ent：`go generate ./ent`；Wire：`go generate ./cmd/server`，工作目录均为 `backend`。本次未运行。
- 不在 P0 添加任何关闭态空壳、假接口、mock 生产结果或后台任务。

## 实际检查记录

命令中的目录切换由 PowerShell 包装执行，输出编码设为 UTF-8。以下列出关键独立检查结果；复合搜索脚本的整体退出码不能冒充其中每条 rg 成功。

| 工作目录 | 实际命令 / 操作 | 退出码 / 结果 |
| --- | --- | --- |
| 仓库根 | `git status --short` | 0；仅两个既有未跟踪目录 |
| 仓库根 | `git branch --show-current`、`git rev-parse HEAD`、`git diff --stat` | 所在包装命令 0；main、上述 HEAD、无已跟踪差异 |
| 仓库根 | `git log -1 --format="%h %s"` | 所在包装命令 0；上述最近提交 |
| 仓库根 | `git merge-base --is-ancestor 6e7cff538ab19f7b6bbc3dafdd74d9443c9be55c HEAD` | 明确捕获 0 |
| 仓库根 | `git config --get remote.origin.url` | 所在包装命令 0；上述 origin |
| 仓库根 | `Get-ChildItem -LiteralPath . -Filter AGENTS.md -Recurse -File`，过滤 node_modules/.git | 无额外规则文件；所在包装命令 0 |
| 仓库根 | `Test-Path frontend/node_modules` | True；没有重新安装依赖 |
| backend | 设置进程级 `GOTOOLCHAIN=local` 后 `go version` | `go1.26.5 windows/amd64` |
| backend | `go test -tags=unit ./internal/service -run "TestChannelMonitorV2\|TestChannelMonitor.*Probe\|TestChannelMonitor.*Runtime" -count=1 -timeout=45s` | 1；go.mod 要求 Go >= 1.27.0，未开始运行测试 |
| frontend | `pnpm exec vitest run src/views/user/__tests__/ChannelStatusView.mode.spec.ts src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts src/features/channel-monitor-v2/__tests__/observedMetrics.spec.ts` | 工具 90 秒超时；只有 RUN v2.1.9，无用例终态，未提供进程退出码 |
| frontend | 上条命令附加 `--pool=threads --maxWorkers=1 --minWorkers=1 --no-file-parallelism` | 工具 45 秒超时；只有 RUN，未提供进程退出码；不再重复跑 |
| 宿主机 | 查询 CommandLine 包含 vitest 的 Node 进程 | 最后检查没有发现遗留 Vitest Node 进程 |
| 仓库根 | `git diff --check`（文档新增前） | 0；仅证明当时已跟踪差异检查，无功能测试含义 |

搜索过程还出现以下工具问题，未据此推导错误源码结论：

- Windows 原生 rg 不展开作为路径传入的 `channel_monitor_v2*` / `setting*`；返回 `os error 123`，后续改用目录加 `-g` 并读取实际文件。
- 尝试的 `setting_service_public.go`、`setting_defaults.go`、`setting_keys.go`、`cmd/server/generate.go` 不存在。实际已定位 `setting_public.go`、`domain_constants.go` 和 `cmd/server/main.go`；没有创建同名替代文件。
- 最后一轮补充搜索脚本在 20 秒时超时；已返回生成入口与 settings 文件名，后续子命令没有完整结果，不能算验证通过。
- 源码读取/文件发现使用本地工具；本文件不是工具调用逐行转储。

### 基线结论

- **环境阻塞**：本地 Go 版本不足。没有降低 go.mod 或触发自动下载新 Go。
- **测试未完成**：前端三个定向测试文件两次超时；原因未查明，不能认定为断言失败，也不能认定通过。
- **已确认断言失败**：本次没有取得用例断言失败结果。
- **本次引入失败**：没有改功能代码；不能据此声称整个仓库健康。
- **未执行**：全量 unit/integration、race、typecheck、lint:check、build、迁移、数据库/多实例验证及浏览器验收。
- 新增测试：无，P0 不实现功能。

## meow 准入与外部核验

蓝图指定 `chen-006/meow-llm-detector@fdb89c99852e0d5558551168835387b835265942`，应用版本 4.5.2，并记载许可证为 PolyForm Noncommercial 1.0.0、启动 sample_ratio=.6、档位由实际包读取。

本次尝试只读获取该固定提交的 `LICENSE` 与 `gpt56_vnext/server.py`，两个 URL 均返回工具 `fetch failed`。因此：

- 上述版本、许可和启动参数目前是**蓝图记载，不是本次独立复核结果**。
- 基准 manifest、摘要、实际档位、请求构造、原测试命令仍待核验。
- 未 clone、安装、vendor 或打包 meow，没有声明获得商用授权。
- 真实引擎准入状态为未确认。P7 真实引擎接入受阻；P1 自有 schema/关闭态设计不以引擎打包为前提。

## 下一阶段改动清单

仅在负责人确认后进入 P1，不能一次推进全部阶段。

1. 先读完 settings 默认值、更新入口、公开 DTO/HTML 注入、前端类型/开关的完整链；复核 actual migration runner 与当前空闲迁移编号。
2. 新增分组策略及有限检测任务的具名类型、实际存储 schema、乐观锁和唯一约束。策略以真实 group_id 唯一绑定；旧 V1 记录原样保留。
3. 策略、plan、job、execution、attempt、report、probe 与预算根据实际职责实现，不机械创建蓝图所有建议文件。
4. 七个新开关均默认 false；引擎准入由部署配置管理，不用公开用户复选框代表许可。
5. 公开设置只暴露必要可见性，不暴露 worker 地址、内部认证、账号列表或凭据。
6. 使用匹配仓库的 Go 工具链再运行必要生成；审查生成差异，不手改自动生成文件。
7. 精简测试优先默认关闭/缺失字段兼容、唯一约束、乐观锁和旧记录可读；实际数据库语义必须在隔离数据库验证，不能用纯 mock 代替后声称通过。

### 预计后续接口变化（尚未实现）

- 新管理员 `group-policies` API，独立于旧 ChannelMonitor CRUD。
- 新用户 `group-cards` API，identity 仅 group_id，模型保留为配置/筛选/详情维度；旧 `/cards` 结构保持不变。
- 独立用户私有 detector plans/runs/report API；本人权限逐端点落实。
- 独立内部 listener 与 execution 限权 bridge，不挂到公网通用代理。
- Usage/Ops 可信 origin 与 nullable 观测值，以及新 TPS 直方图；不可改变计费金额含义。

## 待确认与剩余工作

- 负责人确认下一次是否进入 P1。
- 准备匹配 Go >= 1.27.0 的既有开发/隔离 CI 环境；本次不安装系统级工具。
- 前端 Vitest 挂起原因仍未解决。后续先定位启动/transform/setup，不扩展全量测试。
- 完成固定版本 meow 源码和 manifest 核验，商用场景准入由负责人确认。
- P0 未进行全协议发送链、全配置链和每个管理员表单的完整审阅；已定位入口留给对应阶段开始前补齐，不把此进度文档作为全量审计签收。
- 本次没有启动应用服务，避免触发迁移、调度或读取开发环境中的真实上游配置。


## 阶段 P1：默认关闭配置与持久化基础

### 实施边界与适配决策

- 基线仍为 `21173706d672611051a8bb926ec5745fe0b2642c`，分支 main；不提交、不切分支、不回退原有内容。
- 复核了 settings 默认值/解析/保存、管理员请求合并/审计/响应、公开 API、HTML 注入、前端 store/types/featureFlags，以及管理员设置页面的显式保存 payload。旧表单没有发送新字段时仍保留原值。
- 新监控表沿用 V2 的 SQL-owned Repository 模式，不引入第二套 Ent 实体。没有改 Ent/Wire 输入，故不运行生成，也不手改生成文件。Repository 尚未装配到公开 HTTP 接口，构造器没有启动行为。
- 新增迁移 `238_channel_monitor_group_foundation.sql`，只创建新表、索引并插入缺失的关闭态设置，不修改历史迁移、usage_logs 或 ops_error_logs。P2 才增加真实流量观测列。
- 六个可编辑设置通过既有设置链落地：group view、group probe、output TPS、detector、user testing、scheduled。缺失/异常值不启用；默认全部 false。
- 第七个 `llm_detector_engine_allowed` 是部署配置，环境变量为 `LLM_DETECTOR_ENGINE_ALLOWED`，默认 false。管理员 GET 只读显示；PUT 携带该字段返回 400，服务层设置写入也不保存此值。公开设置不暴露该字段。
- 新运行门禁单独读取，设置读取失败返回全关闭；不修改 V1 的历史门禁或默认值。私有检测门禁不依赖监控页面；真实执行还必须显式传入 executorReady，并在后续阶段叠加身份、目标与预算校验。
- 本阶段没有基准目录、执行器、检测接口、任务派发或新增页面；没有 mock 生产结果，不声称用户已能运行检测。

### 已落地文件

配置链修改：

- `backend/internal/config/config.go`
- `backend/internal/service/domain_constants.go`、`settings_view.go`、`setting_parse.go`、`setting_public.go`、`setting_update.go`
- `backend/internal/handler/dto/settings.go`、`backend/internal/handler/setting_handler.go`
- `backend/internal/handler/admin/setting_handler.go`、`setting_handler_update.go`、`setting_handler_audit.go`
- `frontend/src/types/index.ts`、`frontend/src/api/admin/settings.ts`、`frontend/src/stores/app.ts`、`frontend/src/utils/featureFlags.ts`

新增基础与测试：

- `backend/internal/service/channel_monitor_group_types.go`、`channel_monitor_group_types_test.go`
- `backend/internal/service/channel_monitor_feature_flags.go`、`channel_monitor_feature_flags_test.go`
- `backend/internal/service/channel_monitor_group_settings.go`、`channel_monitor_group_settings_test.go`
- `backend/internal/service/llm_detector_types.go`、`llm_detector_types_test.go`
- `backend/internal/repository/channel_monitor_group_repository.go`、`channel_monitor_group_repository_test.go`
- `backend/internal/repository/llm_detector_repository.go`、`llm_detector_repository_test.go`
- `backend/internal/repository/channel_monitor_group_migration_integration_test.go`
- `backend/migrations/238_channel_monitor_group_foundation.sql`
- `frontend/src/api/channelMonitorGroups.ts`、`frontend/src/api/llmDetector.ts`、`frontend/src/api/__tests__/channelMonitorGroups.spec.ts`
- `.gitignore` 只增加本进度文档的例外。

### 存储与契约

- 九张新表：分组策略、plan、job、execution、attempt、report、普通探活结果、预算 bucket、预算 reservation。
- 分组策略有未删除 group_id 唯一索引、version CAS、独立计划时间字段。当前 Repository 只提供 inactive 策略创建/读取/更新，拒绝探活或能力检测开启；更新也拒绝已有调度/活动任务的策略，不能绕过后续取消与版本失效流程。
- 策略结构校验覆盖模型去重/数量、随机与固定集合、时间/抖动/TTL、档位、目标范围、基准摘要格式、预算及复测上限。JSON 解码拒绝未知字段、重复键、null、非整数和尾随 JSON。
- `CapabilityRevision` 当前版本 1 排除名称、CRUD version、调度时间和普通探活设置；包括分组和规范化能力配置。实际引擎/评分/请求契约还须在 P3/P7 加载准入资料后冻结到 execution，不把当前 hash 当成已完成生产契约核验。
- job 有作用域内幂等键唯一、plan_id 唯一消费、occurrence 唯一、同策略同类活动任务唯一、租约/终态/出站计数约束。
- 私有 job 到 plan 的组合外键包含 owner 与 source；execution 到 job 校验 source。平台账号与私有目标使用不同部分唯一索引，不依赖含 NULL 的普通 UNIQUE。
- 计划消费只以 `monitor_jobs.plan_id` 为权威，`DetectorPlan.ConsumedJobID` 从关联查询导出，不维护两个可漂移的链接字段。
- 外部 Key 没有持久化字段；snapshot 是具名类型，不接受任意 headers/body。此设计不代表 P6 的 SSRF、回显脱敏、内存托管已经实现。
- 私有 plan/job/report 的 Repository 查询均带 owner，报告还同时约束 job；未提供不受限的用户查询接口。
- 预算仅落地非负/上限/结算约束。原子预留、逐请求消费、跨日处理、fencing 与调度属于 P5/P6，尚未实现。
- 活动任务指针留作 scheduler-owned 元数据；当前防重权威是 monitor_jobs 的部分唯一索引，跨表指针一致性尚待调度事务实现。

### 实际验证与退出码

所有测试均离线，没有真实模型 API 调用。

1. `gofmt -w <本次修改/新增的 Go 文件列表>`：0。只格式化本次 Go 文件；复查并收敛了 config.go 因对齐产生的无关差异。
2. 新增两个 TS 契约的独立检查：0。

```bash
node frontend/node_modules/typescript/bin/tsc --noEmit --skipLibCheck --target ES2020 --module ESNext --moduleResolution bundler frontend/src/api/channelMonitorGroups.ts frontend/src/api/llmDetector.ts
```

实际调用使用上述文件的绝对路径。

3. 新增 Go 纯标准库文件的隔离测试：首次工具 40 秒超时；随后相同文件集合加 `-v`、`-timeout=10s` 得到退出码 0，9 个顶层测试函数及其子用例通过，Go 报告执行时间 0.963s。

```bash
GO111MODULE=off GOTOOLCHAIN=local go test -v \
  <service>/channel_monitor_group_types.go <service>/channel_monitor_group_types_test.go \
  <service>/channel_monitor_feature_flags.go <service>/channel_monitor_feature_flags_test.go \
  <service>/llm_detector_types.go <service>/llm_detector_types_test.go \
  -count=1 -timeout=10s
```

`<service>` 实际为当前仓库 `backend/internal/service` 的绝对路径。进程级 GO111MODULE=off 仅用于这些无第三方依赖文件的隔离执行；未修改 go.mod，不代表项目已能用 Go 1.26 构建。

4. 新增前端定向测试：0，1 个文件、3 个用例通过，Vitest 报告总时间 6.52s。

```bash
node frontend/node_modules/vitest/vitest.mjs run --root <frontend绝对路径> src/api/__tests__/channelMonitorGroups.spec.ts --pool=threads --maxWorkers=1 --minWorkers=1 --no-file-parallelism
```

5. 完整包上下文中的定向后端测试：使用本机缓存 Go 1.27.0，退出码 0；`internal/service`、`internal/repository`、`internal/handler/dto` 定向包通过。

```bash
GOTOOLCHAIN=local go -C <backend绝对路径> test -tags=unit ./internal/service ./internal/repository ./internal/handler/dto -run 'TestMonitor|TestDetector|TestPublicSettingsInjectionPayload' -count=1 -timeout=20s
```

实际错误：`go.mod requires go >= 1.27.0 (running go 1.26.5; GOTOOLCHAIN=local)`。因此 settings/Repository/mock/DTO schema drift 测试未执行，不能记为通过。

6. 全前端类型检查：`node frontend/node_modules/vue-tsc/bin/vue-tsc.js --noEmit --diagnostics -p <frontend绝对路径>/tsconfig.json`，退出码 0，无错误；845 个文件、总耗时 76.71s，内存峰值约 1.89GB。此前 35 秒超时只是窗口不足。
7. `git diff --check`：0。此命令不覆盖未跟踪文件的功能语义。
8. ReadSeek diagnostics 因当前工具 schema 强制提供的 vision/range 参数与源码 diagnostics 模式不兼容而未取得结果；已使用 gofmt 的语法解析和上述隔离编译补充检查，不伪称 ReadSeek 通过。

### 测试覆盖边界

- 实际通过：关闭态默认、结构输入校验、模型去重、TTL/采样约束、语义 hash 的名称/顺序稳定性、状态与 verdict 分离、snapshot 不接受 api_key、V1/V2/用户自测开关独立性、前端缺失字段关闭态与默认配置。
- 已添加但未运行：设置公开/HTML 注入一致性、设置失败关闭、SQLmock 的策略 CAS/重复创建和 owner 查询约束；隔离 PostgreSQL 的重复迁移、默认配置、唯一约束、租约状态、预算约束和跨用户计划关联。
- PostgreSQL 集成用例复用仓库 `integrationDB`，另建事务内临时 schema 并最终 rollback，未连接或操作生产数据库。
- 尚未完成：完整新库/现有库升级及旧版本回退实测、完整包编译、并发事务/真实 DB 行锁验证、生成/装配阶段回归。这些是 P1 正式验收前的剩余项。
- 本轮没有已知失败断言；有工具链阻塞与检查超时。不能据此宣布整体测试通过。

### 下一步

先在已有匹配 Go 1.27 的隔离环境完成 P1 包测试与 PostgreSQL 验收。没有授权时不安装全局工具、不改低 Go 版本、不执行生产迁移。验收后再进入 P2：可信请求来源与真实可见文本观测，不提前接入真实 meow。

收尾验证：完成空集合归一化修正后，以相同 6 个 Go 文件、`-count=1 -timeout=10s` 再跑一次隔离测试，退出码 0，耗时 0.948s；再次 `git diff --check` 退出码 0。

真实模型 API：0。许可/worker：未准入、未引入、未部署。运行时页面行为：旧 V1/V2 保持原入口，本轮没有新增用户页面。


## P1 后续加固

本轮响应“继续优化”，仅处理 P1 的具体边界问题，保留前次全部改动，不进入 P2。

### 修正内容

1. `channel_monitor_group_types.go`：配置键限定小写 ASCII，阻止 Go JSON 大小写折叠带来的别名与重复字段绕过，包括转义大写及 Unicode 长 s 别名；合法转义小写仍可使用。输入增加 UTF-8 与嵌套深度上限检查，保留 64 KiB 上限。
2. 同文件：模型/基准标识增加 UTF-8、控制字符、首尾空白及长度约束。claimed model/benchmark ID 上限 200 字节，benchmark version 上限 100 字节；故意采用比 PostgreSQL VARCHAR 字符上限更保守的字节限额。防止超长配置通过写入、随后因读取上限失败，以及带空白的 reference-only 候选绕过字面校验。
3. `llm_detector_repository.go`：私有 plan/job/report 查询先验证标准 36 字符 UUID，非法 ID 与无权限/不存在一致返回 `sql.ErrNoRows`，不把参数交给 PostgreSQL 的 UUID 转换。报告同时验证 job 与 report 两个 ID。
4. 新迁移 `239_monitor_plan_scope_constraints.sql`：补上 plan/job 的 source 外键，以及平台 policy/revision 外键。原因是 PostgreSQL 默认 MATCH SIMPLE 会在 owner 为 NULL 时跳过原有组合外键，不能仅依赖 owner/source 三列关联保证平台任务的计划范围。未改写迁移 238。
5. 补充结构输入、UUID 无数据库拦截，以及跨 source/policy/revision 的回归用例。数据库拒绝用例会断言 SQLSTATE 23503 和本次新增约束名称，避免被无关错误误判为通过；保留匹配平台计划可成功关联的正向用例。

### 本轮实际验证

- `gofmt`：0，覆盖本轮修改/新增 Go 文件。
- 为绕开整个项目 Go 1.27 的编译门禁，创建了临时隔离模块，原样复制本轮新增的纯 service 类型/校验/开关及两个 Repository 和对应测试。没有伪造 service stub，没有替换 SQL 实现，没有修改原项目 go.mod/go.sum。
- 临时目录：`C:/Users/Lovewell/AppData/Local/Temp/sub2api-monitor-p1-2a8f46cf028f4078ad67b92aef8c8b8b`。其 go.mod 使用本机 Go 1.26 与原项目相同版本的已缓存依赖。原项目 go.sum 仅复制到该临时目录。
- 执行命令（退出码 0）：

```bash
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go -C <上述临时目录> test -mod=readonly -tags=unit ./internal/service ./internal/repository -count=1 -timeout=20s
```

- service：11 个顶层测试函数通过，报告 0.913s；Repository：3 个顶层测试函数通过，报告 1.762s。计数另以 `go test ... -list '^Test'` 核对。
- Repository 实际通过范围：inactive 策略创建、重复创建映射、version CAS、开启态拒绝、owner SQL 条件、非法 UUID 在 nil DB 下被提前拒绝。这里是 SQLmock，不是 PostgreSQL 实测。
- `GOPROXY=off`、`GOSUMDB=off` 和 `-mod=readonly` 明确禁止下载与依赖文件自动变更；没有安装全局工具。
- `git diff --check`：0。
- 本轮未改前端，未重复运行前端全量检查。完整项目 settings/DTO/装配编译及 PostgreSQL 238/239 迁移实测仍待具备环境后执行。

### 仍待验收

数据库测试已扩展为顺序执行 238/239 且各重复执行一次，但尚未实际运行；隔离编译通过不能证明 PostgreSQL 锁行为、升级耗时或现有数据兼容性。如果数据库已有不符合新外键的历史任务，239 将失败并要求先核查数据，不会自动删除或修复它们。

本轮没有真实模型请求、生产迁移、部署、提交或全局依赖安装。新增管理员设置 optional flag 和 deployment-owned override 回归测试，缓存 Go 1.27.0 下退出码 0。


## P2：原生 Responses 流观测接入

### 本次交付

- 按蓝图 5.3/5.4/1473 后的 P2 要求修正上一轮计算器：采样规则版本 1、方法 `visible_stream_v1`，至少 16 个可见 token、两个文本增量、200 ms；输出为整数 milli tok/s，使用 128 位乘除中间值检查溢出。
- 新增 `traffic_observation_context.go`：来源由服务端类型化 context 传递，不读取请求头或请求体。默认业务来源；无效来源拒绝。尚未覆盖所有 usage/ops 成败持久化路径，不宣称统计污染问题已经端到端解决。
- 新增 `openai_stream_observation.go`：原生 OpenAI Responses 单次尝试的观测器，只保留计数/时间，不持有正文。正文 delta 才更新首末时间；terminal usage 明确提供 reasoning_tokens 后才计算可见 token。工具、拒答、图像、音频、未知事件、失败及缺失明细不产生正式 TPS。
- `openai_gateway_response_handling.go`：同步读取及异步队列均记录上游读取时刻；事件携带时间戳入队，避免把处理/写出等待当成事件到达时刻。保留旧 firstTokenMs、usage、缓冲/重试/失败逻辑。
- `openai_gateway_forward.go`、`openai_gateway_service.go`：观测快照沿原生 Responses 转发结果返回；未适配路径为 nil。快照复制数值，不与观测器复用可修改指针。
- 请求取消、下游写失败但继续 drain 计费、未完成终态均不生成成功 TPS 样本。缺失推理明细不被假定为零；不以字符数估算 token。

### 本次验证

使用已有缓存 Go 1.27.0，GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off、GOWORK=off，原项目 go.mod 不变：

```text
go test -mod=readonly -tags=unit ./internal/service -run 'TestOpenAIStreamObservation|TestVisibleOutputObservation|TestOpenAIVisibleOutputClassification|TestOpenAIResponsesTTFT|TestOpenAINativeMetadata' -count=1 -timeout=30s
```

- 首轮退出码 0（3.273s）；补充取消、写失败、受信来源、未适配平台测试后，最终退出码 0（3.321s）。
- 实际 Handler 测试覆盖同步/异步成功与截断、正文原样输出、计费 output_tokens 保持 100 而观测 visible tokens 为 80、伪造来源请求头无效、下游失败/取消继续保持原有 drain 语义。
- 原有语义/可见 TTFT、图像首输出和元数据不解除首输出超时回归一起通过。
- 全部为本地 io.Pipe / httptest 合成流，无真实上游请求；gofmt 与 git diff --check 通过。

### 尚未完成

P2 不标记完成：UsageLog/Ent/Repository 映射、计费前观测落库、ops 成败来源、V2 幂等聚合/直方图/水位及其他协议适配仍待实现。当前快照只到 ForwardResult，尚未持久化，也不会让页面出现新指标。不能把本次描述成 P2 全量交付。

任务4此前的“完成”只反映代码测试，不代表 PostgreSQL 验收；已另外建立待办“实测 P1 PostgreSQL 迁移”明确跟踪剩余项。

## P2 恢复：用量持久化边界回归

Astra 定价提交 `2866c5a80` 已独立发布；本节监控改动仍只在本地，不在该发布中。

- 恢复时完整重读实施蓝图。已有未提交实现包含 UsageLog 观测字段、CaptureMonitorUsage、原生 Responses RecordUsage 传递、Repository 单条/批量/best-effort INSERT 与 SELECT 扫描，以及迁移 240。此前“仅到 ForwardResult”的状态已过时，但数据库实测尚未完成，不能宣称落库验收通过。
- 新增 `traffic_observation_usage_test.go`，验证有效零缓存与缺失值区分、无效缓存排除、快照不共享数值指针、未完成输出无 TPS/TTFT、未知版本不写测量值，以及观测投影不写计费 token/金额字段。
- 实际执行：缓存 Go 1.27.0，`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off go test -mod=readonly -tags=unit ./internal/repository ./internal/service -run 'Test.*UsageLog|Test.*ScanUsage|TestOpenAIStreamObservation|TestVisibleOutputObservation|TestCaptureMonitorUsage' -count=1 -timeout=60s`；退出码 0，repository 0.308s，service 1.376s。
- 明确差异：当前 RequestOrigin 三类值尚未对齐蓝图五类来源，缺用户自测与历史未知区分；需同步类型、SQL 约束、usage/ops 写入和聚合筛选，不能只改字符串。Ent schema/生成、PostgreSQL 238/239/240 实测、V2 新指标与直方图仍待完成。
- 本次无真实 API 请求、无生产操作、无依赖变更。P2 保持进行中，未开始 P3。


## P2 来源契约与错误入队链

- `traffic_observation.go` 对齐五类值：real_traffic、availability_probe、capability_probe、user_detector、legacy_unknown。保留既有 Go 常量名以缩小调用方变更；只有 real_traffic 可生成业务 TPS。
- 新增迁移 `241_monitor_request_origins.sql`，不修改 240：转换曾用的 business/capability_detector 字面值，NULL 历史保持未知，新增 ops 来源列和约束。尚未实测；其 UPDATE/锁成本须在隔离 PostgreSQL 验收，不可直接部署。
- `ops_port.go`、`ops_repo.go` 增加来源字段及单条/事务批量 INSERT 参数。`ops_error_logger.go` 在普通失败、流式错误和重试后恢复的异步入队前，从服务端 request context 复制来源。
- `openai_gateway_usage.go` 在没有可见流观测时仍保留服务端来源，例如非流式；不因此填充任何观测 token。
- 新增 `traffic_observation_context_test.go`，核对五类精确字面值、上下文传递、旧别名拒绝及非业务 TPS 排除。新增 `ops_error_logger_origin_test.go` 的 4 来源 x 3 结果路径，实际经过 middleware、入队及后台 flush；伪造请求头不改变归属。
- `ops_repo_args_test.go` 同步参数数目并验证来源参数与 SQL 占位符。首轮唯一失败为旧 38 参数断言，修正并补测试后通过。
- 实际命令：缓存 Go 1.27.0、GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off，`go test -mod=readonly -tags=unit ./internal/repository ./internal/service ./internal/handler -run 'Test.*Ops|Test.*UsageLog|TestOpenAIStreamObservation|TestVisibleOutputObservation|TestCaptureMonitorUsage|TestRequestOriginContract' -count=1 -timeout=90s`。最终退出码 0；repository 0.327s，service 2.648s，handler 0.360s。
- 仍待完成：其他协议成功用量及专用错误入口的来源覆盖、Ent 映射、隔离数据库实测、V2 usage/ops 双侧筛选、新指标与直方图。此时不宣称 M11 已通过端到端验收。
- 本轮无真实模型请求，无生产操作，无提交或部署；P2 仍在进行。


## P1/P2 隔离 PostgreSQL 验收

- 本地 Docker Desktop 可用，复用仓库 integration harness 的 PostgreSQL 18.1/Redis 8.4 临时容器。未使用 VPS 或现有业务数据库。
- 新增 `monitor_observation_migration_integration_test.go`，在回滚事务内创建独立 schema：模拟 240 旧来源及真实观测，执行 241 两次；核对旧来源转换、历史 NULL 保留、五类合法来源、旧写入兼容、有效零缓存，以及异常缓存/来源和探测 TPS 被 SQLSTATE 23514 拒绝。
- 同时运行既有 `TestMonitorFoundationMigrationPostgres`，验证 238/239 重复执行、关闭态、策略唯一约束、预算约束、计划 owner/source/policy/revision 隔离。此前 P1 PostgreSQL 未实测的阻塞现已解除。
- 实际命令：缓存 Go 1.27.0，GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off，`go test -mod=readonly -tags=integration ./internal/repository -run 'TestMonitorFoundationMigrationPostgres|TestMonitorObservationMigrationPostgres' -count=1 -v -timeout=120s`。退出码 0，总耗时 13.627s；基础迁移测试 0.34s，观测迁移测试 0.06s。
- harness 在测试前成功执行新库完整迁移链；不是只运行手写 SQL mock。测试容器已由测试框架回收，按 session label 检查无剩余容器。
- 边界：这证明隔离库 schema/约束行为，不证明生产大表锁时长、全协议落库覆盖或 V2 聚合正确。P2 仍进行中；未启用调度/引擎、未请求真实模型、未提交或部署。


## P2 Ent 映射及公共网关来源

- `ent/schema/usage_log.go` 加入全部九个来源/观测 nullable 字段，无默认值；通过 Ent v0.14.5 生成相关模型、builder、mutation、predicate、runtime 和 schema，未手改生成代码。
- CLI readonly 生成首次因 tablewriter/cobra 缺少 go.sum 项失败。改用 `handoff/sub2api/generate-monitor-ent.go` 调用同版本 entc.Generate，保持 sql/upsert、intercept、sql/execquery、sql/lock、int64 ID 配置。命令：backend 目录 `go run -mod=readonly ../handoff/sub2api/generate-monitor-ent.go`，退出码 0；GOTOOLCHAIN=local、GOPROXY=off、GOSUMDB=off、GOWORK=off，使用缓存 Go 1.27.0；go.mod/go.sum 无变更。
- `gateway_usage_billing.go` 公共 buildRecordUsageLog 从受信 context 保存来源；未适配路径不生成观测，不改变计费字段。新增 `gateway_usage_origin_test.go` 覆盖四种活动来源和 token/金额不变。
- 新增 `usage_log_monitor_ent_integration_test.go`：Repository 写入 -> Ent 读取九字段及费用，Ent 旧写入 -> Repository 读取保留 NULL。
- 定向 unit：`go test -mod=readonly -tags=unit ./internal/service ./internal/repository ./internal/handler -run 'TestBuildRecordUsageLog|Test.*UsageLog|Test.*Ops|Test.*Astra|TestCaptureMonitorUsage|TestRequestOriginContract' -count=1 -timeout=90s`，退出码 0，耗时 service 1.662s、repository 0.265s、handler 0.296s。
- 隔离 PostgreSQL：`go test -mod=readonly -tags=integration ./internal/repository -run 'TestUsageLogMonitorEntRoundTrip|TestMonitorObservationMigrationPostgres|TestMonitorFoundationMigrationPostgres' -count=1 -v -timeout=120s`，三个顶层测试通过，退出码 0，总耗时 11.492s。
- 尚未完成：上下文跨所有异步调用方的全链验证、专用错误入口覆盖、V2 双侧筛选及指标/直方图聚合。不能将本轮描述为 P2 完成。没有模型 API 请求、生产操作、提交或部署。


## P2 V2 聚合、直方图及异步归属

- 新增迁移 242：既有 metrics 分钟/固定桶增加版本化缓存观测计数；TPS 使用独立分钟和固定桶直方图表，保留 metric_version 与 bucket_index，不借用毫秒字段。
- 新增迁移 243：延迟直方图新增 visible_ttft_v1，与旧 TTFT 分离；独立记录 observation_v1_collection_start/data_through，历史回填不伪造新口径覆盖。
- channel_monitor_v2_aggregation.go / channel_monitor_v2_observations.go：沿现有 RecomputeRange 事务、固定桶、水位和保留机制，usage/ops 双侧排除三类测试来源；旧 NULL/legacy_unknown 仅保留旧指标兼容。新缓存仅聚合版本 1 的同批有效分子分母，新 TPS 仅聚合真实业务有效样本。
- channel_monitor_tps_histogram.go / _test.go：提交 236 个稳定边界、溢出桶、整数 nearest-rank 近似分位；检查计数溢出和边界稳定性，不平均分位。
- 修复重算入口提前删除粗桶而后续跳过重建的数据丢失风险；固定桶自行负责对齐删除。错误存在同分组/Key 的最终成功 usage 时不重复算业务失败，但保留上游尝试诊断；有界 90 分钟查询，不做无限历史扫描。
- openai_gateway_handler.go 的公共 usageRecordContext 原先只复制请求 ID，异步来源会丢失；现复制可信来源并保留 worker 截止时间。openai_gateway_usage_context_test.go 覆盖四来源、请求取消隔离和 worker 取消传递，两种 handler 提交路径均测试。
- 新增真实 PostgreSQL 聚合测试：100/100+0/900=10%、缺缓存排除、版本排除、测试流量成功与失败双侧排除、跨模型 TPS P50、两次重算不累加、迟到修正、粗桶保留、两次上游错误后成功不重复失败、visible TTFT 和水位。
- 首次聚合 fixture 未设置 actual_cost 导致旧成功筛选为零，补上正常付费 fixture 后通过。旧实际成功筛选仍为 actual_cost>0，零费用成功覆盖尚未解决，不对外宣称完整最终请求成功率。细节写入新增 docs/channel-monitor-metrics.md。
- 最终 integration 命令（缓存 Go 1.27.0，GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off）：`go test -mod=readonly -tags=integration ./internal/repository -run 'TestMonitor|TestUsageLogMonitorEntRoundTrip|TestChannelMonitorV2CardsPostgres' -count=1 -v -timeout=120s`；退出码 0，五个顶层测试通过，总耗时 10.506s。
- 最终定向 unit：`go test -mod=readonly -tags=unit ./internal/repository ./internal/service ./internal/handler -run 'Test.*ChannelMonitor|TestMonitor|Test.*Observation|Test.*Origin|Test.*UsageLog|TestUsageRecordContext|TestSubmitUsageRecordTask|TestOpenAISubmitUsageRecordTask' -count=1 -timeout=120s`；退出码 0，repository 0.385s、service 4.489s、handler 0.368s。
- 扩大全包：`go test -mod=readonly -tags=unit ./internal/repository ./internal/service ./internal/handler -count=1 -timeout=180s`，退出码 1；repository 6.187s、handler 52.478s 通过。service 四个 PluginPackageInstaller 在 Windows rename 报文件占用，TestShouldClearStickySession/oauth_quota_exceeded_not_cleared 断言失败，最后耗尽 180 秒整包时限。日志 handoff/monitor-unit-full.log。首次日志重定向目录错误时 Go 未运行，后已修正路径。
- 单独复跑 `-run 'TestPluginPackageInstaller|TestShouldClearStickySession|TestUsageRecordWorkerPool_AutoScaleDisabledKeepsFixedConcurrency' -timeout=60s`，退出码 1，0.625s；同五个断言失败，worker 测试不再超时。未对照干净 HEAD，不能断言全部是基线失败；不扩大范围修插件或 sticky 路由。日志 handoff/monitor-baseline-failures.log，任务 8 跟踪。
- P2 仍未全部通过：全协议/专用入口覆盖、免费成功/完整终态口径、性能查询计划和保留周期演练仍待完成。P3/P4 UI 未开始，不伪造引擎或绿色指标。无生产操作、无真实模型测试、无提交/部署。
