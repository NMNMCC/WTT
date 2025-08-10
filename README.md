# Ming

一个轻量的 WebRTC 隧道端口转发项目。内置一个轻量的 WebSocket 信令服务器，
能够协调客户端与主机（Host）之间建立 WebRTC PeerConnection，并用 DataChannel
转发 TCP 或 UDP 流量，从而实现穿越 NAT 的端到端转发。

- 组件：server（信令服务器）、host（被访问端/转发端）、client（访问端/入口端）
- 协议：WebRTC DataChannel（可靠/不可靠两种）、WebSocket 作为信令通道
- 依赖：Go、Pion WebRTC、Gorilla WebSocket、glog、pflag

## 快速开始

确保本机已安装 Go（go.mod 中为 Go 1.24）。以下命令对 fish/bash 通用。

1. 启动信令服务器（终端 1）：

```
go run . --mode server --addr :8080
```

2. 启动 Host（终端 2）：

- 将 Host 注册为 ID "myhost"；
- 将远端业务（示例为本机 127.0.0.1:25565 的 TCP 服务）暴露给客户端。

```
go run . --mode host --signal ws://127.0.0.1:8080 --id myhost --remote 127.0.0.1:25565 --protocol tcp
```

3. 启动 Client（终端 3）：

- 连接到 ID 为 "myhost" 的 Host；
- 在本地 127.0.0.1:25565 开端口，等待本机应用连接，然后通过 DataChannel 转发至
  Host 的远端服务。

```
go run . --mode client --signal ws://127.0.0.1:8080 --id myhost --local 127.0.0.1:25565 --protocol tcp
```

4. 将你的应用连接到 Client 本地端口（如 127.0.0.1:25565），流量将通过 WebRTC
   隧道发往 Host 的远端地址（示例中的 127.0.0.1:25565）。

## 安装与构建

- 直接运行：`go run . --mode ...`
- 构建二进制：

```
mkdir -p bin
go build -o bin/ming .
```

随后使用：

```
./bin/ming --mode server --addr :8080
```

## 命令行参数

顶层参数（在 `main.go` 中）：

- --mode string
  - 运行模式：server | host | client（必选）

Server 模式：

- --addr string
  - 信令服务器监听地址，默认 ":8080"

Host/Client 通用：

- --signal string
  - 信令服务器地址，默认 "ws://localhost:8080"
- --id string
  - Host 模式：注册的 ID；Client 模式：要连接的 Host ID
- --protocol string
  - 隧道协议："tcp" 或 "udp"，默认 "tcp"

Client 专有：

- --local string
  - 本地监听地址（TCP/UDP），默认 "localhost:25565"

Host 专有：

- --remote string
  - Host 侧要转发至的远端地址（TCP/UDP），默认 "localhost:25565"

glog 日志参数（通过 pflag 导入 Go 原生 flag，可使用单横杠）：

- -alsologtostderr 在终端输出日志
- -v=N 设置日志级别（如 -v=2）
- -log_dir=DIR 写入日志文件目录

示例：

```
go run . --mode server --addr :8080 -alsologtostderr -v=2
```

## 使用示例

### 1) TCP 隧道

- Server：`go run . --mode server --addr :8080`
- Host：`go run . --mode host --signal ws://127.0.0.1:8080 --id myhost --remote 127.0.0.1:25565 --protocol tcp`
- Client：`go run . --mode client --signal ws://127.0.0.1:8080 --id myhost --local 127.0.0.1:25565 --protocol tcp`

然后将客户端应用连接到 127.0.0.1:25565，即可通过隧道访问 Host 的 127.0.0.1:25565
TCP 服务。

提示：当前 Client 只接受一次本地 TCP 连接（`Accept()` 之后关闭
listener），若需并发/多连接需自行扩展。

### 2) UDP 隧道

将 protocol 改为 udp：

- Host：`--protocol udp --remote 127.0.0.1:9999`（将 DataChannel 数据转发到该
  UDP 目标）
- Client：`--protocol udp --local 127.0.0.1:9999`（在本地监听此 UDP 端口，转发到
  DataChannel）

当前实现为：

- Client 侧 UDP：首次收到本地应用数据包时，记录其来源作为回写地址，并在
  DataChannel 收到数据时写回同一地址。
- Host 侧 UDP：创建到 `--remote` 的 UDP 连接，双向转发 DataChannel 与 UDP
  套接字之间的数据。

## 原理与架构

### 整体架构与角色

- server（信令服务器）：
  - WebSocket 升级后接入。允许任意 Origin。
  - 维护一个 Host 连接表（ID -> WebSocket 连接）。
  - 当收到 client 发来的消息（含 TargetID=HostID）时，将其转发给对应 Host。
  - 不保存状态/SDP，仅做路由转发。

- host（被访问端）：
  - 以指定 ID 向信令服务器注册。
  - 收到 client 的 Offer 和 ICE 候选后，创建 PeerConnection、设置远端描述，生成
    Answer 并回传。
  - 当 DataChannel 打开后，将数据流量与本地 `--remote` 地址的 TCP/UDP
    套接字之间做双向转发。

- client（访问端）：
  - 连接信令服务器，创建 PeerConnection。
  - 主动创建 DataChannel（TCP：可靠；UDP：不可靠/无重传），发起 Offer 给目标
    Host。
  - DataChannel 打开后，监听本地 `--local`
    地址（TCP/UDP），与应用建立一次连接/会话，并双向转发。

数据路径（示意）： 应用 -> Client 本地端口 -> DataChannel -> Host -> 远端服务

### 信令协议消息结构（`signal/`）

核心类型：

- Message
  - type: "register-host" | "offer" | "answer" | "candidate" | "error"
  - payload: 不同消息对应不同负载
  - target_id: 目标 Host/Client 的 ID
  - sender_id: 发送方 ID

负载：

- OfferPayload{ SDP webrtc.SessionDescription }
- AnswerPayload{ SDP webrtc.SessionDescription }
- CandidatePayload{ Candidate webrtc.ICECandidateInit }

信令服务器仅透传这些 JSON，Host/Client 分别处理。

### 连接建立流程（Offer/Answer/ICE）

1. Client 连接信令服务器，创建 PeerConnection 与 DataChannel。
2. Client 创建 Offer（本地描述）并通过信令发送给目标 Host。
3. Host 收到 Offer，创建 PeerConnection，设置远端描述，生成 Answer，发回
   Client。
4. 双方在 ICE 收集期间互相发送
   Candidate，直到可达的候选对建立成功（connected）。
5. DataChannel open 事件触发后开始转发真实业务流量。

### DataChannel 转发实现

- TCP：
  - Client：DataChannel.Detach() 拿到 io.ReadWriteCloser，与本地 Accept() 的 TCP
    连接做双向 io.Copy。
  - Host：同理，与 `net.Dial("tcp", remote)` 的连接做双向 io.Copy。
  - 可靠、有序，适合需要可靠传输的应用。

- UDP：
  - Client：`net.ListenPacket("udp", local)`
    读取数据，第一个包确定回写地址；DataChannel 收到的数据写回该地址。
  - Host：`net.Dial("udp", remote)`，DataChannel 与 UDP 套接字之间双向转发。
  - 不可靠、无序：通过 DataChannelInit 设置
    `Ordered=false, MaxRetransmits=0`，接近原生 UDP 的特性。

### NAT 穿透与 STUN

- 使用 Google 公共 STUN 服务器：`stun:stun.l.google.com:19302`（在 client/host
  代码中常量 `stunServer`）。
- 仅使用 STUN 进行打洞，不包含 TURN 中继能力；若双方都在对称
  NAT、或受企业防火墙限制，可能无法直连（PeerConnectionStateFailed）。
- 可扩展为引入 TURN（未实现）。

## 运行与日志

- 默认日志使用 glog。
- 常见参数：`-alsologtostderr -v=2`。
- 进程退出：
  - Client 监听 SIGINT/SIGTERM 后退出。
  - PeerConnection 失败/关闭时会清理资源。

## 故障排查

- 连接状态一直 Failed：
  - NAT 环境限制，尝试换网或自建 TURN。
  - 确保能访问 STUN（UDP 3478/随机端口）。
- Host 未收到消息：
  - 确认 Host 已注册成功（日志有 "Host registered"）。
  - Client 的 `--id` 与 Host 注册 ID 一致。
- 无法本地连接到 Client：
  - 该实现仅接受一次 TCP 连接，确认第一条连接是否占用；或换一个 `--local`
    端口重新启动 Client。
- UDP 回写丢包：
  - Client 端仅对“首个收到的本地来源地址”回写，若本地应用来源变化需重启 Client。

## 安全性与注意事项

- 加密：WebRTC 基于 DTLS/SRTP，DataChannel 传输已加密。
- 认证/授权：当前无鉴权、无 ACL；信令服务器允许任意 Origin 且任何人可注册任意
  ID。生产必须加上鉴权、ID 管理与访问控制。
- 资源限制：无速率限制/连接数限制，生产需加。
- 暴露面：信令服务器用 WebSocket 明文/或基于 TLS 的 wss，建议在生产使用 wss
  与可信来源校验。

## 已知限制与改进方向

- Client TCP 仅接受一次连接；可改为循环 Accept，或支持多路复用。
- Client UDP 仅记录第一条来源地址作为回写地址；可扩展为多五元组映射。
- 仅 STUN，无 TURN 支持。
- 信令服务器无鉴权、无心跳、无断线重连策略。
- STUN/TURN/ICE 参数不可配置（代码常量）；可加入命令行或配置文件。

## 项目结构

```
.
├─ main.go            # 程序入口，选择运行模式与解析参数
├─ server/            # WebSocket 信令服务器，仅做消息路由
│  └─ server.go
├─ host/              # Host 端：注册 ID，处理 Offer/Answer/Candidate，数据转发到 --remote
│  └─ host.go
├─ client/            # Client 端：创建 DataChannel，监听 --local，转发到 Host
│  └─ client.go
├─ signal/            # 信令层的消息类型定义
│  └─ signal.go
└─ common/            # 预留的公共类型（当前未被主流程使用）
```

## FAQ

- 可以更换 STUN/TURN 吗？
  - 目前 STUN 写死在 `client/host` 的常量里，修改后重新编译即可；TURN 尚未集成。
- 可以一台 Host 被多个 Client 连接吗？
  - 支持。Host 内部用 map 维护多个 PeerConnection；Client
    侧当前一次运行只服务一次本地连接（TCP）。
- WebSocket 服务端路径？
  - 直接挂在 `/`。若需拆路由可自行扩展。

## 许可证

未包含 LICENSE，请按需添加。
