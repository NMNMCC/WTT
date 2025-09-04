WTT (Minimal Prototype)

当前代码为最小可运行雏形，用于后续按照设计文档迭代：

功能涵盖：
1. 基础信令服务器 (/ws WebSocket) - 仅转发部分消息。
2. Provider / Consumer 基础启动流程与配置解析。
3. Provider 注册、发布服务骨架；Consumer 启动本地监听骨架。

待实现（下一步）：
- 完整 Envelope / 事件类型与错误回复机制。
- SDP/ICE 信令交互与 WebRTC PeerConnection 建立。
- DataChannel 上的双向 TCP/UDP 转发。
- JWT 获取与校验、用户隔离、多租户。
- TURN/STUN 配置透传。
- Provider 端对服务和连接生命周期管理；Consumer 端按需发起 request_connection。
- UI (TUI/GUI) 实现。

运行示例：
1. 启动信令服务器：
   go run . -mode=server -addr=:8080
2. 启动 Provider：
   go run . -mode=provider -config=config.yaml
3. 启动 Consumer：
   go run . -mode=consumer -config=config.yaml

示例配置文件见 config.example.yaml。

注意：当前尚未建立真实 WebRTC 连接，仅做项目结构铺垫。