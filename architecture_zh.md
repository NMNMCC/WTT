# WTT 架构概述

WTT 架构的核心是 **Peer Client (对等客户端)** 和 **Signaling Server
(信令服务器)**。其通信模型分为两个层面：

1. **信令平面 (Signaling Plane)**: 客户端与信令服务器之间通过 **WebSocket**
   建立的持久连接。此通道_不传输任何业务数据_，仅用于身份认证、对等端发现、服务发布以及在建立连接时交换元数据（SDP
   和 ICE Candidate）。
2. **数据平面 (Data Plane)**: 客户端之间通过 **WebRTC DataChannel** 建立的直接
   P2P 连接。所有终端用户的业务流量（TCP/UDP
   数据）都在此通道上进行端到端加密传输。

一个 WTT 客户端实例可以同时扮演 **Provider (服务提供方)** 和 **Consumer
(服务使用方)**
的角色。客户端内部会为每一个独立的隧道（无论是提供服务还是消费服务）维护一个独立的
`RTCPeerConnection` 实例，确保各个隧道的隔离性和独立管理。

## 信令通道设计 (客户端 ↔︎ 服务器)

### 认证流程

客户端不能直接使用长期的用户 Token 来建立 WebSocket
连接。为了安全和灵活性，采用两步认证：

1. **获取会话令牌 (JWT)**: 客户端启动后，首先通过一个标准的 **HTTPS RESTful**
   接口 (`POST /api/v1/auth`)，使用其长期有效的用户 Token
   向信令服务器换取一个**有时效性**的会话令牌 (JWT)。
2. **WebSocket 升级请求**: 客户端在发起 WebSocket 连接请求时，将上一步获得的 JWT
   包含在 HTTP Header 中，格式为
   `Authorization: Bearer {{jwt.token}}`。服务器在协议升级前对此 JWT 进行验证。

### 消息封装 (Envelope)

为简化解析和路由，所有在 WebSocket 上交换的消息都被统一封装在一个 `Envelope`
结构中。

```json
{
  "id": "unique-message-id-c1", // 消息唯一 ID，用于追踪和回复
  "type": "event-type",         // 事件类型
  "target_peer_id": "peer-id-b",  // 目标 PeerID，仅在转发消息时需要
  "payload": { ... }            // 事件的具体负载
}
```

- **`id`**: 由消息发起方生成的唯一标识符 (UUID or
  nanoid)。服务器在回复时会包含此 ID，方便客户端将响应与请求对应起来。
- **`type`**: 核心事件类型，决定了 `payload` 的结构和服务器的行为。
- **`target_peer_id`**: 当一个客户端希望向另一个客户端发送消息时（如 SDP 或
  ICE），需要指定此字段。
- **`payload`**: 消息的具体内容。

### 主要事件 (Events)

| `type`               | 发起方 | 目标   | `payload` 示例                                        | 描述                                                           |
| :------------------- | :----- | :----- | :---------------------------------------------------- | :------------------------------------------------------------- |
| `register`           | Client | Server | `{}`                                                  | 客户端在 WebSocket 连接成功后，请求注册并获取自己的 `PeerID`。 |
| `publish_services`   | Client | Server | `{"services": ["db", "api"]}`                         | **Provider** 向服务器声明其可提供的服务列表。                  |
| `request_connection` | Client | Server | `{"target_peer_id": "B", "service_name": "db"}`       | **Consumer** 请求连接到 Peer B 提供的 `db` 服务。              |
| `signal`             | Client | Peer   | `{"to_peer_id": "B", "data": {"type": "offer", ...}}` | 封装 SDP 或 ICE Candidate，请求服务器转发给另一个 Peer。       |
| **`reply`**          | Server | Client | `{"success": true, "reply_to": "c1", "data": ...}`    | 服务器对客户端请求的**通用回复**。`success` 字段标识成功与否。 |

## WebRTC 连接流程详解 (客户端 ↔︎ 客户端)

以下是建立一个 WebRTC 数据通道的**完整、精确**的步骤，包含了关键的 **ICE
Candidate** 交换过程。

1. **发起方 (Peer A / Consumer) 准备 Offer**:

   1. Peer A 创建 `RTCPeerConnection` 对象。
   2. Peer A 调用 `createDataChannel()` 在连接上预创建数据通道。
   3. Peer A 调用 `createOffer()` 创建 SDP `offer`。
   4. Peer A 调用 `setLocalDescription(offer)`。**此调用会触发 ICE Agent
      开始收集 Candidate**。
   5. Peer A 监听 `onicecandidate` 事件。每当收集到一个新的 ICE
      Candidate，就通过信令服务器的 `signal` 事件将其发送给 Peer B。
   6. Peer A 将 `offer` 和 WTT 元数据（例如
      `{"service_name": "db"}`）包装后，通过信令服务器的 `signal` 事件发送给
      Peer B。

2. **接收方 (Peer B / Provider) 处理 Offer 并准备 Answer**:

   1. Peer B 收到来自 Peer A 的 `offer` 和元数据。
   2. **连接决策**: Peer B 检查元数据中的 `service_name` 是否在自己配置的
      `provide` 列表中。如果不在，则通过信令服务器回复拒绝消息；如果在，则继续。
   3. Peer B 创建自己的 `RTCPeerConnection` 对象。
   4. Peer B 调用 `setRemoteDescription(offer)`，将 Peer A 的 `offer`
      设置为远端描述。
   5. Peer B 调用 `createAnswer()` 创建 SDP `answer`。
   6. Peer B 调用 `setLocalDescription(answer)`。**此调用同样会触发 ICE Agent
      开始收集 Candidate**。
   7. Peer B 监听 `onicecandidate` 事件，并将收集到的每个 ICE Candidate
      实时通过信令服务器发送给 Peer A。
   8. Peer B 将 `answer` 通过信令服务器的 `signal` 事件发送回 Peer A。
   9. Peer B 监听 `ondatachannel` 事件，等待 Peer A 创建的数据通道的“抵达”。

3. **连接建立与完成**:

   1. Peer A 收到 Peer B 的 `answer`，并调用 `setRemoteDescription(answer)`。
   2. 在此期间，双方不断接收并调用 `addIceCandidate()` 添加对方发送过来的 ICE
      Candidate。
   3. ICE Agent 使用双方交换的 Candidate 尝试建立连接。当找到一条可用的 P2P
      路径后，`RTCPeerConnection` 的连接状态变为 `Connected`。
   4. 此时，Peer B 的 `ondatachannel` 事件被触发，获取到数据通道的实例。
   5. 双方的 `DataChannel` 状态变为 `open`，双向通信正式建立。
