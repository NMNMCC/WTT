# WTT (WebRTC Transport Tool)

WTT is a Layer 4 network forwarding tool built on the WebRTC framework. It provides a stable, fast, and easy-to-use P2P tunnel to securely access services behind NAT or firewalls.

## Core Concept

WTT uses a "Service Gateway" model:
- **Provider**: Deployed inside a private network, it acts as a gateway, publishing internal services without exposing any inbound ports.
- **Consumer**: Deployed on an external network, it subscribes to a Provider's services and maps them to a local port.

This model enhances security by eliminating the need for public-facing ports on the Provider side, aligning with Zero Trust principles.

## Features

- **Secure**: End-to-end encryption via DTLS, built into WebRTC.
- **NAT Traversal**: Uses STUN/TURN for high-connectivity success rates.
- **Protocol Support**: Forwards both TCP and UDP traffic (current implementation focuses on TCP).
- **Configuration Driven**: Uses a simple `config.yaml` file.
- **Cross-Platform**: Written in Go.

## How to Run

### 1. Configuration

Edit the `config.yaml` file to set up your client.

- **`auth.token`**: A pre-shared secret to authenticate with the signaling server.
- **`signaling_server`**: The WebSocket URL of your signaling server.
- **`ice_servers`**: A list of STUN/TURN servers. A public STUN server is provided by default. For reliable connections in all network conditions, deploying your own TURN server (like `coturn`) is recommended.
- **`provide`**: A list of services you want to expose from this client.
- **`consume`**: A list of remote services you want to access from this client.

**Example `config.yaml` for a Provider:**
```yaml
auth:
  token: "my-secret-token"
signaling_server: "ws://my-wtt-server.com/ws"
ice_servers:
  - urls: "stun:stun.l.google.com:19302"
provide:
  - service_name: "my-ssh-service"
    target_address: "tcp://localhost:22"
```

**Example `config.yaml` for a Consumer:**
```yaml
auth:
  token: "my-secret-token"
signaling_server: "ws://my-wtt-server.com/ws"
ice_servers:
  - urls: "stun:stun.l.google.com:19302"
consume:
  - name: "remote-ssh"
    local_address: "tcp://127.0.0.1:2222"
    remote_peer_id: "<Provider's PeerID>" # Get this from the Provider's client logs on first run
    remote_service_name: "my-ssh-service"
```

### 2. Run the Signaling Server

**Using Docker (Recommended):**
```bash
# Build the Docker image
docker build -t wtt-server .

# Run the server
docker run -p 8080:8080 -d wtt-server
```

**Running Locally:**
```bash
# Run the server directly
go run ./cmd/wtt-server
```
The server will start on port `8080`.

### 3. Run the WTT Client

You can run the same client binary as a Provider, a Consumer, or both, based on your `config.yaml`.

**Running Locally:**
```bash
# Run the client
go run ./cmd/wtt-client
```

When a client first connects, it will print its unique `PeerID` in the logs. You will need this `PeerID` for the `remote_peer_id` field in the Consumer's configuration.

### Example Usage

1.  Set up a signaling server and run it.
2.  On a machine inside a private network (e.g., at home), configure `config.yaml` with a `provide` block to share its SSH service.
3.  Run `go run ./cmd/wtt-client` on the home machine. Note its `PeerID` from the logs.
4.  On another machine (e.g., your laptop on a public network), configure `config.yaml` with a `consume` block, using the home machine's `PeerID`. Set `local_address` to `tcp://127.0.0.1:2222`.
5.  Run `go run ./cmd/wtt-client` on the laptop.
6.  You can now SSH into your home machine by connecting to your local port: `ssh user@localhost -p 2222`. The traffic will be securely tunneled via WebRTC.
