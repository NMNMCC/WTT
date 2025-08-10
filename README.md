# webrtc-tunnel

一个轻量的 WebRTC 隧道端口转发工具。内置 WebSocket 信令服务器，用 WebRTC
DataChannel 转发 TCP/UDP 流量，实现穿越 NAT 的端到端转发。

- 组件：server（信令服务器）、host（被访问端/转发端）、client（访问端/入口端）
- 协议：WebRTC DataChannel（TCP 可靠／UDP 不可靠）、WebSocket 作为信令通道
- 依赖：Go 1.23+、Pion WebRTC、Gorilla WebSocket、Cobra/Viper、glog

## 快速开始

确保已安装 Go（go.mod: 1.23）。以下命令对 fish/bash 通用。

1. 启动信令服务器（终端 1）

```
go run . server --addr :8080
```

2. 启动 Host（终端 2）

- 将 Host 注册为 ID "myhost"
- 把远端业务 127.0.0.1:25565（TCP）暴露给客户端

```
go run . host \
  --signal ws://127.0.0.1:8080 \
  --id myhost \
  --remote 127.0.0.1:25565 \
  --protocol tcp
```

3. 启动 Client（终端 3）

- 连接到 ID 为 "myhost" 的 Host
- 在本地 127.0.0.1:25565 开端口，应用连接到此端口

```
go run . client \
  --signal ws://127.0.0.1:8080 \
  --id myhost \
  --local 127.0.0.1:25565 \
  --protocol tcp
```

然后将你的应用连接到 127.0.0.1:25565，流量会通过 WebRTC 隧道转发到 Host 的
127.0.0.1:25565。

提示：当前 Client 已支持“多个并发 TCP 连接”（循环 Accept + 每连接一条
DataChannel）。

## 安装与构建

- 直接运行：`go run . <subcommand> [flags]`
- 构建二进制：

```
mkdir -p bin
go build -o bin/webrtc-tunnel .
```

随后使用：

```
./bin/webrtc-tunnel server --addr :8080
```

## 配置与参数优先级

本项目支持三种配置来源（优先级从高到低）：

1. 命令行参数（flags）
2. 环境变量（前缀 WTT_，点号用下划线替换）
3. 配置文件 config.yaml（默认当前目录）

示例环境变量：

- `WTT_SIGNAL=ws://localhost:8080`
- `WTT_STUN_SERVER=stun:stun.l.google.com:19302`
- `WTT_SERVER_ADDR=:8080`
- `WTT_SERVER_TLS_CERT_FILE=/path/cert.pem`
- `WTT_SERVER_TLS_KEY_FILE=/path/key.pem`
- `WTT_SERVER_ALLOWED_ORIGINS=http://localhost:3000,https://example.com`
- `WTT_SERVER_AUTH_VALID_TOKENS=my-secret-token,another-token`

glog 日志（通过 pflag 导入标准 flag）：

- `-alsologtostderr` 在终端输出日志
- `-v=N` 设置日志级别（如 `-v=2`）
- `-log_dir=DIR` 写入日志目录

## 子命令与参数

所有子命令均支持通用参数：

- `--config string` 指定配置文件（默认 `./config.yaml`）

### server（信令服务器）

- `--addr string` 信令服务器监听地址，默认 `:8080`
- `--tls-cert-file string` WSS 证书文件路径（设置后开启 TLS）
- `--tls-key-file string` WSS 私钥文件路径
- `--allowed-origins stringSlice` 允许的 WebSocket Origin 白名单，默认 `[*]`
- `--valid-tokens stringSlice` 允许的 Bearer Token 列表（为空则不做鉴权）

示例（启用 TLS 与鉴权）：

```
./webrtc-tunnel server \
  --addr :8443 \
  --tls-cert-file /etc/ssl/cert.pem \
  --tls-key-file /etc/ssl/key.pem \
  --allowed-origins https://my-app.com \
  --valid-tokens my-secret-token
```

### host（被访问端）

- `--id string` 必填，注册到信令服务器的 Host ID
- `--remote string` 必填，要转发到的远端地址（默认 `localhost:25565`）
- `--signal string` 信令服务器地址，默认 `ws://localhost:8080`
- `--protocol string` `tcp` | `udp`，默认 `tcp`
- `--stun-server string` STUN 服务器，默认 `stun:stun.l.google.com:19302`
- `--token string` 可选，连接信令服务器时使用的 Bearer Token

### client（入口端）

- `--id string` 必填，要连接的 Host ID
- `--local string` 本地监听地址（TCP/UDP），默认 `localhost:25565`
- `--signal string` 信令服务器地址，默认 `ws://localhost:8080`
- `--protocol string` `tcp` | `udp`，默认 `tcp`
- `--stun-server string` STUN 服务器，默认 `stun:stun.l.google.com:19302`
- `--token string` 可选，连接信令服务器时使用的 Bearer Token

## 使用示例

### TCP 隧道

- Server：`webrtc-tunnel server --addr :8080`
- Host：`webrtc-tunnel host --signal ws://127.0.0.1:8080 --id myhost --remote 127.0.0.1:25565 --protocol tcp`
- Client：`webrtc-tunnel client --signal ws://127.0.0.1:8080 --id myhost --local 127.0.0.1:25565 --protocol tcp`

Client 现已支持多个并发本地 TCP 连接：每个新连接都会创建一个独立 DataChannel
并与 Host 的远端 TCP 建立双向转发。

### UDP 隧道

将 `--protocol` 改为 `udp`：

- Host：`--protocol udp --remote 127.0.0.1:9999`
- Client：`--protocol udp --local 127.0.0.1:9999`

行为说明：

- Client 侧 UDP：首次收到本地应用的数据包时记录来源地址作为回写地址；之后 WebRTC
  收到的数据写回该地址。
- Host 侧 UDP：创建到 `--remote` 的 UDP 连接，DataChannel 与该 UDP
  套接字间双向转发。
- DataChannel 参数：`Ordered=false, MaxRetransmits=0`，尽量贴近原生
  UDP（不可靠/无序）。

## 配置文件示例（config.yaml）

```
# Common
signal: "ws://localhost:8080"
protocol: "tcp"

stun:
  server: "stun:stun.l.google.com:19302"

# Signaling server
server:
  addr: ":8080"
  allowed-origins: ["*"]
  tls:
    cert-file: ""
    key-file: ""
  auth:
    valid-tokens: []  # e.g. ["my-secret-token"]

# Host
host:
  id: "myhost"
  remote: "localhost:25565"

# Client
client:
  id: "myhost"
  local: "localhost:25565"

# Optional: default bearer token to send from host/client when dialing the server
token: ""
```

## 原理与架构

- server（信令服务器）
  - WebSocket 接入（可选 TLS/WSS），可配置 Origin 白名单与 Bearer Token 鉴权。
  - 维护 Host 连接表（ID -> WebSocket 连接），转发 client 的信令到对应 Host。
  - 不保存 SDP/候选，仅透传消息。
- host（被访问端）
  - 使用指定 ID 注册；收到 Offer/ICE 后建立 PeerConnection，生成 Answer 返回。
  - DataChannel 打开后，与 `--remote`（TCP/UDP）间进行双向转发。
- client（入口端）
  - 连接信令服务器，创建 PeerConnection。
  - TCP：按需为每个本地连接创建 DataChannel；UDP：单通道不可靠传输。

数据路径：应用 -> Client 本地端口 -> DataChannel -> Host -> 远端服务

### 信令消息（`signal/`）

- Message
  - type: "register-host" | "offer" | "answer" | "candidate" | "error"
  - payload: 对应的 SDP / ICE / 文本错误
  - target_id: 目标 ID（Host 或 Client）
  - sender_id: 发送方 ID

## NAT 穿透与 STUN/TURN

- 默认使用 Google 公共 STUN：`stun:stun.l.google.com:19302`
- 当前仅 STUN，不包含 TURN；在对称
  NAT/受限网络下可能无法直连（PeerConnectionStateFailed）。
- 可自行扩展为引入 TURN。

## 运行与日志

- 日志：glog（支持 `-alsologtostderr`、`-v=2` 等）
- 进程退出：
  - Host/Client 监听 SIGINT/SIGTERM 优雅退出。
  - 连接失败/关闭会清理资源并按指数退避重连（最大 30s）。

## 故障排查

- 连接状态一直 Failed
  - NAT 环境限制，尝试更换网络或使用 TURN。
  - 确保能访问 STUN（UDP 3478/随机端口）。
- Host 未收到消息
  - 确认 Host 已成功注册（日志有 "Host registered"）。
  - Client 的 `--id` 与 Host 注册 ID 一致。
  - 若开启鉴权，确认 Client/Host 传入了正确的 `--token`，服务器也配置了对应
    `--valid-tokens`。
- 无法本地连接到 Client
  - 确认本地监听地址未被占用。
  - 查看日志中是否存在 DataChannel 打开/关闭信息。
- UDP 回写丢包
  - Client 端仅对“首个收到的本地来源地址”回写，来源改变需重启 Client。

## 安全性注意

- 传输加密：WebRTC 基于 DTLS/SRTP，DataChannel 已加密。
- 鉴权/授权：可在服务器端启用 Bearer Token 白名单；生产环境建议开启 TLS/WSS
  并限制 Origin。
- 资源限制：未内置速率/连接数限制，生产需自行加固。

## 项目结构

```
.
├─ main.go            # 入口：初始化 pflag/glog，执行 Cobra 根命令
├─ cmd/               # CLI 子命令（server/host/client）
│  ├─ root.go
│  ├─ server.go
│  ├─ host.go
│  └─ client.go
├─ server/            # WebSocket 信令服务器
│  └─ server.go
├─ host/              # Host 端数据转发实现
│  └─ host.go
├─ client/            # Client 端数据转发实现
│  └─ client.go
├─ signal/            # 信令消息定义
│  └─ signal.go
├─ common/            # 预留的公共类型
│  └─ *.go
└─ config.yaml.example
```

## FAQ

- 可以更换 STUN/TURN 吗？
  - STUN 可通过 `--stun-server` 或配置文件修改；TURN 尚未集成。
- 一台 Host 能否被多个 Client 连接？
  - 支持。Host 内部用 map 维护多个 PeerConnection；Client 侧 TCP 可并发多连接。
- WebSocket 路径？
  - 默认挂在 `/`。如需拆路由可自行扩展。
