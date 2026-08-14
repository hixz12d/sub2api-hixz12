# OpenAI / Codex 出口加固

## 应用策略

本 fork 的部署示例启用：

```yaml
gateway:
  openai_egress:
    mode: proxy_required
    failover_route_policy: same_route
    invalidate_connections_on_proxy_change: true
```

`proxy_required` 会在任何 OpenAI/Codex 上游拨号前校验账号代理。代理缺失、关系未加载且无法查询、停用、过期、协议/主机/端口非法，或配置为 `direct` fallback 时，请求会失败而不是改用服务器公网地址直连。通用默认仍是 `optional`，用于兼容上游部署；本 fork 的生产配置应显式保留上述严格值。

代理 URL 仅支持 `http`、`https`、`socks5` 和 `socks5h`。`socks5` 会规范化为 `socks5h`，使目标域名由代理侧解析。字面 IPv6 地址必须写成 URL 的方括号形式，例如：

```text
http://${PROXY_USER}:${PROXY_PASSWORD}@[2001:db8::10]:8080
socks5h://${PROXY_USER}:${PROXY_PASSWORD}@[2001:db8::20]:1080
```

管理界面中代理主机字段填写裸 IPv6（例如 `2001:db8::10`）即可，服务端通过 `net.JoinHostPort` 生成带方括号的 URL。日志和指标不得记录上述完整 URL或凭据；路由审计只使用截断 SHA-256 `RouteKey`。

HTTP 客户端缓存按完整规范化代理配置隔离。WebSocket 池额外把 `RouteKey` 纳入握手兼容键；代理 ID、地址、端口、协议、凭据或 `updated_at` 变化后，新请求不会复用旧路由连接。

## 网络层 kill switch

应用层检查应配合网络层默认拒绝策略，防止未来新增代码遗漏 resolver：

- 应用容器只允许访问指定代理 IPv6 地址和端口；
- 允许 PostgreSQL、Redis、DNS（仅在代理主机使用域名时）及必要内部服务；
- 其他公网 IPv4 和 IPv6 出站默认拒绝；
- Docker bridge 的转发流量通常应在 `DOCKER-USER`、容器 network namespace、TUN/WireGuard sidecar 或云防火墙处理；宿主机普通 `OUTPUT` 链不一定覆盖容器转发；
- 不要只封 IPv4，否则仍可能经 IPv6 直连；
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

1. 无代理、停用代理、过期代理和错误端口均不能触发上游请求；
2. IPv6 HTTP/HTTPS/SOCKS5H 代理均能规范解析；
3. 断开代理后 HTTP、SSE、passthrough、WS、OAuth、usage、隐私和账号测试均失败或保持未知；
4. fake OpenAI target 的非代理直连计数为零；
5. 代理 A 改为代理 B 后，WS 新请求建立新连接，旧连接不再复用；
6. 同时检查 IPv4 与 IPv6 抓包，不能只验证一个地址族。
