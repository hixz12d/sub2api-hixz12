# 官方 v0.2.9 融合记录

## 来源与版本

- 本地起点：`4af8d8236`，此前已融合官方 v0.2.8。
- 官方仓库：`https://github.com/Wei-Shaw/sub2api.git`。
- 官方标签：`v0.2.9`，目标提交 `4c00df2e0183e2c70b7fa8ba45914205e36aad0c`。
- 本地版本：`0.2.9-hixz12.1`。官方 v0.2.9 标签中的 `backend/cmd/server/VERSION` 仍为 `0.2.8`，因此单独更新本地版本文件。
- 相对共同祖先，官方更新涉及 116 个文件，其中 37 个与本地定制重叠。本次没有引入数据库迁移或更改依赖清单、锁文件。

## 融合处理

1. OpenAI 请求头冲突：接入官方 `OpenAI-Beta` 白名单与旧 `responses=experimental` 标记清理逻辑，保留本地最终身份统一步骤，不恢复旧的 originator 设置路径。扩充官方测试，用于验证多代理 Beta 透传同时仍使用受管理的 Codex UA、originator 和版本；该服务层回归尚未完成执行，见下方验证限制。
2. WebSocket 上下文切换：接入窗口变化时清理 `previous_response_id` 的官方修复，删除补丁中对本地已经不存在的 `currentPayloadBytes` 的赋值。保留本地重试预算、会话身份、续链恢复及 HTTP/1.1 ALPN 修复。
3. 保留已有 PerPay 支付与充值金额处理、购买入口、账号回答对比与推理强度、代理池、GPT-6 / Opus 5.5 定价、Astra 路由暂停和 Codex 客户端配置定制。
4. 接入官方其余修复，包括 Anthropic structured outputs Beta、thinking disabled、DeepSeek reasoning、工具参数及流式文本恢复、自动重置额度投影与退避、客户端断开归类、模型发现合并、分组模型通配符、CC Switch 导入和 Windows Codex 模型目录路径。

## 需要了解的行为变化

- 渠道图片输入或输出单价留空时沿用模型目录价格；显式填写 `0` 仍表示覆盖为零。尤其是图片输出，此前留空可能按免费处理，更新后应检查依赖这种旧行为的渠道配置。
- 已知额度重置时间尚未来临时，旧额度快照仍会维持暂停，避免过早恢复调度。
- 模型广场使用独立视频倍率，账号统计遵循长上下文计费开关。
- Redis Compose 命令改为参数列表形式。本次仅修改仓库内模板，没有修改 VPS 部署目录或运行中的服务。

## 验证

- 前端 `pnpm run typecheck`：通过。
- 前端 `pnpm run lint:check`：通过。
- 前端项目关键测试、受影响测试及账号对比 / PerPay 回归：30 个文件、424 项测试通过。
- Docker Compose 安全、Gateway 环境变量、运行资源、Caddy 缓存策略检查：通过。
- Docker Compose simple-mode 环境变量检查：通过。Windows 下用已安装的 Python 执行脚本内原有断言，并将空环境文件 `/dev/null` 适配为 `NUL`，覆盖四份 Compose 文件的四种配置值。
- `bash -n deploy/apple-container.sh`：通过；macOS 专用行为测试受 Windows `stat` 与权限语义限制，未完成。
- 后端 `go test -p 4 -tags=unit ./...`：应用代码编译通过，已输出通过结果的包包括 `cmd/server`、`internal/handler`、`internal/pkg/apicompat`、`internal/pkg/antigravity`、`internal/pkg/tlsfingerprint`、`internal/repository` 和 `internal/server` 等。服务层尚未输出最终结果，进程以 `3221225786`（`0xC000013A`，中断退出）结束，不能视为完整单元测试通过。
- 后端 `go test -p 2 -tags=integration ./...` 与后续服务层定向回归也被同样的退出码中断，没有完整结果。中断来源未确认；完整单元、数据库 / Redis 集成和新增服务层回归需要在稳定执行环境补验。
- 本机没有可用的 `golangci-lint`，未执行该独立检查。

本次为本地源码融合，未推送远端、未部署生产环境，也未进行真实上游请求或付款验收。
