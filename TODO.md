# TODO

> 依据 `architecture_*.md` 与 `design_*.md`
>
> 文档目标，对当前代码实现差距进行结构化分解。优先级: P0(必须) / P1(重要) /
> P2(增强)
>
> 状态: [ ] 未做 / [X] 完成。

## 0. 当前实现基线

- [x] CLI: `service / consumer / server`
- [x] WebSocket 最小信令透传 (无鉴权 / 无注册协议语义)
- [x] 基础 WebRTC Offer/Answer + ICE 透传
- [x] DataChannel 建立后做单路 TCP 双向转发 (简易桥接, 无半关闭处理)
- [ ] Service 支持多 Consumer (InputMap) / Consumer 仅单隧道
- [ ] 语义事件(register/publish_services/request_connection/signal/reply) 缺失
- [ ] 认证 / 配置 / 服务抽象 / 安全 / 监控 未实现

---
## 1. 协议与信令层
### 1.1 Envelope & 事件
<!-- - [ ] 新增事件: `register`, `publish_services`, `request_connection`, `signal`, `reply`
- [ ] 设计统一 Envelope: `{id, type, from_peer_id, target_peer_id, payload, reply_to, version, error}`
- [ ] 当前 `rtc_offer/rtc_answer_ok/...` 事件转换为 `signal` 子类型或在 `payload` 中嵌套 -->
- [ ] 消息 ID 生成与超时跟踪 (map[id]→chan/result)
- [ ] 版本字段 (version=1) 与向后兼容策略

### 1.2 状态机
- [ ] Client: `init→ws_connected→registered→(provider:services_published)|(consumer:idle)`
- [ ] 隧道: `requested→signaling→connecting→connected|failed|closed`
- [ ] Service Offer 审核 Hook：拒绝原因枚举

### 1.3 错误与重试
- [ ] 标准错误码: AUTH_FAILED / PEER_NOT_FOUND / SERVICE_NOT_FOUND / PERMISSION_DENIED / TIMEOUT
- [ ] WebSocket 自动重连 (指数退避 / 上限 / 恢复注册)
- [ ] Offer/ICE 发送确认 & 超时重试 (幂等 ID)
---

## 2. 认证与安全

### 2.1 认证

- [ ] `POST /api/v1/auth` (伪实现: in-memory user token → JWT)
- [ ] WebSocket 升级校验 `Authorization: Bearer <jwt>`
- [ ] 服务器维护 user_id→peers 映射

### 2.2 授权

- [ ] 同一 user 命名空间隔离：只能看到本用户内 peers / services
- [ ] 服务名唯一校验 (per user)

### 2.3 传输安全

- [ ] Server 启用 TLS (自签 + 可配置 cert/key)
- [ ] 客户端支持 wss / https
- [ ] 文档化 DTLS 安全保障 (Pion 默认) + cipher whitelist 说明

---
## 3. 配置系统 (config.yaml)
### 3.1 加载
- [ ] 定义结构: auth.token / signaling_server / ice_servers / provide[] / consume[]
- [ ] CLI 增加 `--config`，合并 CLI 覆盖策略
- [ ] 地址 & 协议校验 (schema, host:port, 端口范围)

### 3.2 Provider 部分
- [ ] 发布多服务: map[service_name]target_address
- [ ] `publish_services` 消息发送 + 重发 (失败/重连)
- [ ] 服务撤销/热更新 (P2)

### 3.3 Consumer 部分
- [ ] 多条 consume 配置并行管理 (每条独立隧道生命周期)
- [ ] DataChannel open 后再监听本地端口
- [ ] 自动重连: peer 断开、ICE 失败
---

## 4. WebRTC 层增强

### 4.1 ICE/STUN/TURN

- [ ] 读取并设置 `webrtc.Configuration{ICEServers}`
- [ ] 统计 candidate 选型 (直连 / TURN) & 暴露指标
- [ ] ICE 失败回调 & 重试策略日志

### 4.2 DataChannel

- [ ] KeepAlive / ping-pong 心跳
- [ ] DataChannel 关闭触发隧道状态更新
- [ ] QoS / 有序与否配置 (后期 P2)

### 4.3 多隧道

- [ ] Consumer 多隧道 (隧道集合管理 / goroutine 协调 / 闭包风险)
- [ ] Service 端 per-tunnel 限速/统计 (P2)

---
## 5. 数据转发层 (L4 Proxy)
### 5.1 TCP
- [ ] 基本单链路转发完成
- [ ] 支持多连接 (Consumer 侧一 local 端口 → 多 backend 会话 multiplex? 目前非必须，可保持 1:1)
- [ ] 半关闭处理 (FIN → CloseWrite / EOF 传递)
- [ ] 优化缓冲区 (使用 bufpool / 增大吞吐)

### 5.2 UDP
- [ ] DataChannel 上自定义包格式: `<2B len | meta(json) | payload>` 或简化 header
- [ ] NAT 会话表 (srcAddr→timestamp) 与过期清理

### 5.3 统计
- [ ] 连接时长 / 上下行字节 / 当前活动连接数
- [ ] Prometheus metrics (client & server)
---

## 6. Signaling Server 功能扩展

### 6.1 Peer 管理

- [ ] 存储结构: user_id→peer_id→{type, services, ws, last_seen}
- [ ] 心跳/定期探测 (pong 超时清理)

### 6.2 服务发现

- [ ] 处理 `publish_services` 更新索引: service_name→peer_id→metadata
- [ ] 处理 `request_connection`: 校验 service 存在 → 转发请求 → 协议协商 (决定
      Offer 发起方)（当前默认 Consumer 发起）
- [ ] reply 格式化统一化

### 6.3 WebSocket 治理

- [ ] 每连接消息速率限制 (token bucket)
- [ ] 最大并发 peer 限制 & 配置化
- [ ] 日志脱敏 (隐藏 token / jwt)

---
## 7. 观测与诊断
### 7.1 日志
- [ ] 结构化字段：trace_id / tunnel_id / peer_id / user_id
- [ ] 日志分级细化 (signaling=info, webrtc=debug)

### 7.2 指标
- [ ] server: `online_peers`, `services_total`, `request_conn_total`, `signaling_latency_ms` (hist)
- [ ] client: `tunnels_up_total`, `reconnect_total`, `ice_selected{type}`

### 7.3 Tracing (P2)
- [ ] OpenTelemetry 接入 + context 传递 (Envelope.id 注入 span)
---

## 8. 稳定性

### 8.1 重连

- [ ] WS 断线指数退避（初 500ms ×2 上限 30s）
- [ ] 隧道重建 (DataChannel closed → 重走 signaling)

### 8.2 超时

- [ ] Offer → Answer 超时 (e.g. 15s) 超时回调
- [ ] ICE Gathering 超时 & 回退日志

### 8.3 防护

- [ ] 单 Peer 最大隧道数 (配置)
- [ ] 单 IP 注册频次限制

---
## 9. CLI / UI
### 9.1 CLI
- [ ] `--config` / `--version` / `--dry-run`
- [ ] 状态查看子命令: `status` (列出当前隧道/bytes/peer latency)

### 9.2 TUI (P2)
- [ ] bubbletea: 面板(隧道列表/日志/统计)

### 9.3 GUI (P2)
- [ ] Fyne 初版窗口
---

## 10. 测试体系

### 10.1 单元

- [ ] Envelope 编解码 / 版本兼容测试
- [ ] 配置解析 + 校验测试
- [ ] ID/错误码工具函数

### 10.2 集成

- [ ] 内存启动 server + 2 client 端到端 TCP 传输校验 (hash 对比)
- [ ] 断线重连场景 (kill ws)
- [ ] TURN 回退模拟（需要外部依赖可打桩）

### 10.3 压测 (P2)

- [ ] N 并发隧道建立耗时统计
- [ ] 数据吞吐 & CPU/Mem profile

---
## 11. 构建与发布
### 11.1 构建
- [ ] Makefile / Taskfile (lint/test/build)
- [ ] 多平台交叉编译 (GOOS/GOARCH matrix)

### 11.2 版本
- [ ] 语义化版本 + git tag + `--version` 输出 (commit/date)
- [ ] 预编译二进制发布流程 (GitHub Actions)

### 11.3 交付
- [ ] Signaling Server Dockerfile
- [ ] docker-compose: server + coturn + prometheus + grafana
---

## 12. 文档对齐

- [ ] README: 快速开始 / 示例 config / 架构图链接
- [ ] 更新 architecture/design: 标注 MVP 现状与未来功能
- [ ] 增补：UDP 封装格式、错误码、状态机图、时序图 (mermaid)

---
## 13. Roadmap (P2+)
- 多租户 namespace / ACL
- 动态服务热增删 (watch config)
- 压缩/加密可插拔 (snappy/zstd 上层)
- QUIC DataChannel 优化 / 拥塞策略观察
- Web 管理控制台 (服务发现 / 隧道列表 / 手动断开)
---

## 14. 建议实施批次 (执行顺序)

1. (P0) 协议统一 + register/publish/request_connection/signal/reply
2. (P0) config.yaml 支持 + 多服务/多消费循环
3. (P0) 简单认证 (静态 token → JWT) + 鉴权
4. (P0) ICE servers 注入 + 指标框架骨架
5. (P0) 隧道抽象重构 (多隧道管理集合)
6. (P1) 错误码/超时/重连策略
7. (P1) TCP 桥接健壮性与半关闭
8. (P1) 日志结构化 + metrics + README
9. (P1) TLS + TURN 集成
10. (P2) UDP 支持 / TUI / 压测

---

## 15. P0 验收指标

- register → publish_services → request_connection → signal 完整握手成功
- 1MB 传输哈希一致
- WS 断线 10s 内自动恢复隧道
- 冷启动到隧道可用 <3s (本地直连)
- 无 panic；日志无高频重复错误
