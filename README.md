# WebRTC TCP/UDP Tunnel

A complete WebRTC-based TCP/UDP tunneling solution with integrated signaling server, built in Go using Pion WebRTC. All functionality is packaged in a single binary for easy self-deployment.

## Features

- **WebRTC P2P Tunneling**: Establish direct peer-to-peer connections through NAT/firewalls
- **Integrated Signaling Server**: Built-in WebSocket and HTTP-based signaling server
- **TCP/UDP Support**: Forward TCP and UDP traffic through WebRTC data channels
- **Single Binary**: All components (signaling server, client, server) in one executable
- **Easy Deployment**: No external dependencies, ready for self-hosting

## Use Cases

- **Minecraft Java Edition Remote Play**: Connect to remote Minecraft servers through WebRTC tunnel
- **Game Server Access**: Access game servers behind NAT/firewalls
- **Remote Application Access**: Tunnel any TCP-based application
- **Development/Testing**: Access services in restricted network environments

## Architecture

```
[Local Client] <-> [Client Mode] <-> [WebRTC P2P] <-> [Server Mode] <-> [Remote Service]
                        |                                    |
                        +---> [Signaling Server] <-----------+
```

## Usage

### 1. Start Signaling Server

First, start the signaling server on a publicly accessible machine:

```bash
./ming -mode=signaling -signal-port=8080
```

This will start:
- WebSocket endpoint: `ws://your-server:8080/ws`
- HTTP endpoints: `http://your-server:8080/offer` and `http://your-server:8080/answer`

### 2. Start Server Mode (Near Target Service)

On the machine where your target service (e.g., Minecraft server) is running:

```bash
./ming -mode=server -remote=localhost:25565 -signal=http://your-signaling-server:8080
```

This will:
- Connect to the target service when WebRTC connection is established
- Create WebRTC offer and send to signaling server
- Forward traffic between WebRTC and target service

### 3. Start Client Mode (User's Machine)

On the user's local machine:

```bash
./ming -mode=client -listen=localhost:25566 -signal=http://your-signaling-server:8080
```

This will:
- Receive offer from signaling server
- Create local listening port (25566)
- Forward traffic between local applications and WebRTC

### 4. Connect Your Application

Now connect your application (e.g., Minecraft client) to `localhost:25566` instead of the original server.

## Command Line Options

```
-mode string
    Mode to run in: 'server', 'client', or 'signaling'

-listen string
    Address to listen on (client mode) (default "localhost:25565")

-remote string
    Remote address to connect to (server mode) (default "localhost:25565")

-signal string
    Signaling server address (default "http://localhost:8080")

-signal-port string
    Port for signaling server (signaling mode) (default "8080")
```

## Example: Minecraft Java Edition

### Setup

1. **Signaling Server** (on a VPS with public IP):
   ```bash
   ./ming -mode=signaling -signal-port=8080
   ```

2. **Server Mode** (on machine with Minecraft server):
   ```bash
   ./ming -mode=server -remote=localhost:25565 -signal=http://your-vps-ip:8080
   ```

3. **Client Mode** (on player's machine):
   ```bash
   ./ming -mode=client -listen=localhost:25566 -signal=http://your-vps-ip:8080
   ```

4. **Connect Minecraft Client** to `localhost:25566`

## Building

```bash
go build -o ming main.go
```

## Dependencies

- Go 1.24.5+
- [Pion WebRTC](https://github.com/pion/webrtc)
- [Coder WebSocket](https://github.com/coder/websocket)

## Network Requirements

- **Signaling Server**: Requires public IP or accessible endpoint
- **STUN Servers**: Uses Google's public STUN servers by default
- **Firewall**: WebRTC traffic may require UDP ports to be open

## Troubleshooting

### WebRTC Connection Fails

1. **Check STUN servers**: Ensure STUN servers are accessible
2. **Firewall/NAT**: WebRTC may require additional NAT traversal setup
3. **Network restrictions**: Some corporate networks block WebRTC traffic
4. **TURN servers**: Consider adding TURN servers for difficult network environments

### Connection Timeouts

1. **Signaling server**: Verify signaling server is accessible from both client and server
2. **Target service**: Ensure target service is running and accessible
3. **Ports**: Check that no other services are using the specified ports

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.