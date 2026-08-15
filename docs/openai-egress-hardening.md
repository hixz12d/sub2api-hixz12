# OpenAI / Codex 出口加固

## 应用策略

本 fork 的部署示例采用“按账号选择出口”：

```yaml
gateway:
  openai_egress:
    mode: optional
    failover_route_policy: any
    invalidate_connections_on_proxy_change: true
```

`optional` 模式下，账号绑定了有效代理就必须经该代理；账号没有绑定代理时，resolver 会显式返回 direct 路由并允许直连。已配置但未加载、停用、过期、协议/主机/端口非法，或配置为 `direct` fallback 的代理仍会报错，不能静默改走直连。需要全局禁止直连时，可将 `mode` 改为 `proxy_required`。

代理 URL 仅支持 `http`、`https`、`socks5` 和 `socks5h`。`socks5` 会规范化为 `socks5h`，使目标域名由代理侧解析。字面 IPv6 地址必须写成 URL 的方括号形式，例如：

```text
http://${PROXY_USER}:${PROXY_PASSWORD}@[2001:db8::10]:8080
socks5h://${PROXY_USER}:${PROXY_PASSWORD}@[2001:db8::20]:1080
```

管理界面中代理主机字段填写裸 IPv6（例如 `2001:db8::10`）即可，服务端通过 `net.JoinHostPort` 生成带方括号的 URL。日志和指标不得记录上述完整 URL或凭据；路由审计只使用截断 SHA-256 `RouteKey`。

HTTP 客户端缓存按完整规范化代理配置隔离。WebSocket 池额外把 `RouteKey` 纳入握手兼容键；代理 ID、地址、端口、协议、凭据或 `updated_at` 变化后，新请求不会复用旧路由连接。

## 网络层 kill switch

网络层策略必须与应用出口策略保持一致：

- 使用 `proxy_required` 时，可以只允许指定代理地址、PostgreSQL、Redis、必要 DNS 和内部服务，其余公网 IPv4/IPv6 默认拒绝；
- 使用本 fork 默认的 `optional` 时，未绑定代理的账号允许 OpenAI/Codex 直连，因此网络层不能无条件封禁 OpenAI 目标；应按部署需要允许相应直连地址，或改用 `proxy_required`；
- Docker bridge 的转发流量通常应在 `DOCKER-USER`、容器 network namespace、TUN/WireGuard sidecar 或云防火墙处理；宿主机普通 `OUTPUT` 链不一定覆盖容器转发；
- 变更防火墙前保留独立管理连接并准备原子回滚。

下面仅为验证思路，不含真实地址，也不会自动修改宿主机：

```bash
# 容器内确认代理 IPv6 可达
curl -v --proxy 'http://${PROXY_USER}:${PROXY_PASSWORD}@[2001:db8::10]:8080' https://api.openai.com/

# 严格模式下停止代理后，OpenAI 请求应失败；同时观察宿主机是否出现直连
sudo tcpdump -ni any '(ip or ip6) and (host api.openai.com)'

# Docker 主机查看实际转发规则
sudo iptables -S DOCKER-USER
sudo ip6tables -S DOCKER-USER
```

回滚网络规则时应按部署平台恢复先前保存的规则集，而不是直接清空整个防火墙。应用配置的紧急兼容回滚可将 `mode` 改为 `optional`，但这会重新允许明确直连，必须作为有审计记录的临时措施。

## 验证清单

1. 无代理时应返回显式 direct 路由并允许请求；已配置的停用、过期或错误代理不得触发直连；
2. IPv6 HTTP/HTTPS/SOCKS5H 代理均能规范解析；
3. 断开已配置代理后 HTTP、SSE、passthrough、WS、OAuth、usage、隐私和账号测试均失败或保持未知；
4. optional 模式下无代理请求的 fake OpenAI target 直连计数符合预期；proxy_required 模式下计数必须为零；
5. 代理 A 改为代理 B 后，WS 新请求建立新连接，旧连接不再复用；
6. 同时检查 IPv4 与 IPv6 抓包，不能只验证一个地址族。
