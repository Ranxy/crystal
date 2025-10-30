# Crystal

Crystal is a lightweight, high-performance TCP tunneling and port forwarding system written in Go. It enables secure exposure of internal services through a public server by creating encrypted tunnels.

## Features

- 🚀 **Dynamic Port Allocation**: Automatically allocates ports from a configurable range (9000-10000)
- 🔄 **Bi-directional Forwarding**: Full-duplex data transfer between clients and internal services
- 🔌 **Multiple Connection Types**: Supports both TCP and Unix socket connections
- 🎯 **Concurrent Connections**: Handle multiple agents and external clients simultaneously
- 🛡️ **Graceful Shutdown**: Clean connection handling with proper resource cleanup
- ⚡ **High Performance**: Efficient buffered I/O using Go's `io.Copy`

## Architecture

Crystal consists of three main components:

### 1. Server
The server acts as a public-facing gateway that:
- Listens for incoming agent connections
- Allocates unique ports for each agent
- Accepts external client connections
- Forwards traffic between external clients and agents

### 2. Agent
The agent runs on the internal network and:
- Connects to the Crystal server
- Bridges connections to internal services
- Forwards data bi-directionally
- Supports graceful shutdown via signals

### 3. Example Client (Demo)
A simple TCP server for testing that:
- Sends periodic "Hello World" messages
- Demonstrates the tunneling capabilities
- Useful for validation and testing

## How It Works

```
External Client → Server (Public Port) → Agent → Internal Service
                      ↓                     ↓
                  Port 9000            Port 8111
```

1. Agent connects to the server and gets assigned a public port (e.g., 9000)
2. External clients connect to the server's public port
3. Server forwards traffic between external clients and the agent
4. Agent forwards traffic to the internal service and vice versa

## Installation

### Prerequisites
- Go 1.23.3 or higher

### Build from Source

```bash
# Clone the repository
git clone https://github.com/Ranxy/crystal.git
cd crystal

# Build server
go build -o crystal-server ./server/cmd

# Build agent
go build -o crystal-agent ./agent/cmd

# Build example client (optional)
go build -o example-client ./example
```

## Usage

### 1. Start the Server

```bash
# Start server on default port (9000)
./crystal-server

# Start server on custom port
./crystal-server -listen :8080
```

Options:
- `-listen`: Address for the server to listen on (default: `:9000`)

### 2. Start the Internal Service

For testing, you can use the example client:

```bash
# Start example service on port 8111
./example-client -addr :8111
```

### 3. Start the Agent

Connect the agent to both the server and your internal service:

```bash
# TCP connections
./crystal-agent -a tcp://server.example.com:9000 -b tcp://localhost:8111

# Unix socket example
./crystal-agent -a unix:///tmp/server.sock -b tcp://localhost:8080

# Mixed connection types
./crystal-agent -a tcp://server.example.com:9000 -b unix:///var/run/myapp.sock
```

Options:
- `-a`: Address for connection A (server connection)
  - TCP format: `tcp://host:port` or `host:port`
  - Unix socket: `unix:///path/to/socket` or `/path/to/socket`
- `-b`: Address for connection B (internal service)
  - Same formats as `-a`

### 4. Connect External Clients

Once the agent is connected, external clients can connect to the allocated port:

```bash
# The server will log something like:
# "Agent connected from 192.168.1.100:54321, forwarding public port 9001"

# Connect from external client
telnet server.example.com 9001
```

## Examples

### Example 1: Expose Local Web Server

```bash
# 1. Start your web server locally
python3 -m http.server 8000

# 2. Start Crystal server (on public server)
./crystal-server -listen :9000

# 3. Start Crystal agent (on local machine)
./crystal-agent -a tcp://your-public-server.com:9000 -b tcp://localhost:8000

# 4. Access your local web server via public server
# The server will assign a port (e.g., 9001) that you can use
curl http://your-public-server.com:9001
```

### Example 2: Database Tunnel

```bash
# Expose local database through the tunnel
./crystal-agent -a tcp://tunnel-server.com:9000 -b tcp://localhost:5432

# Connect to database through tunnel
psql -h tunnel-server.com -p 9001 -U username database
```

### Example 3: Unix Socket Forwarding

```bash
# Forward Unix socket to TCP
./crystal-agent -a tcp://server.com:9000 -b unix:///var/run/app.sock
```

## Configuration

### Server Port Range
The server allocates ports from 9000 to 10000 by default. To modify this range, edit `server/server.go`:

```go
portAllocator: NewPortAllocator(9000, 10000),
```

### Connection Timeout
The agent uses a 10-second connection timeout. To modify, edit `agent/agent.go`:

```go
dialer.Timeout = 10 * time.Second
```

### Buffer Size
Data transfer uses a 2048-byte buffer. To adjust, modify `server/server.go`:

```go
buf := make([]byte, 2048)
```

## Graceful Shutdown

Both server and agent support graceful shutdown:

```bash
# Send SIGINT (Ctrl+C) or SIGTERM
kill -TERM <pid>
```

The agent will:
- Close all active connections
- Clean up resources
- Log shutdown status

## Logging

Crystal provides detailed logging for debugging and monitoring:

```
Server logs:
- Agent connections and disconnections
- Port allocation and release
- External client connections
- Forwarding errors

Agent logs:
- Connection establishment
- Forwarding errors
- Graceful shutdown events
```

## Security Considerations

⚠️ **Important Security Notes:**

1. **No Built-in Encryption**: Traffic is not encrypted by default. Use with SSH tunneling or VPN for secure communications.
2. **No Authentication**: No authentication mechanism is implemented. Deploy behind a firewall or add authentication layer.
3. **Port Exposure**: Allocated ports are publicly accessible. Ensure proper firewall rules.
4. **Resource Limits**: Consider implementing rate limiting for production use.

## Development

### Project Structure

```
crystal/
├── agent/              # Agent component
│   ├── agent.go       # Main agent logic
│   └── cmd/           # Agent command-line tool
│       └── main.go
├── server/            # Server component
│   ├── server.go      # Main server logic
│   ├── connection.go  # Connection management
│   ├── port_allocator.go  # Port allocation
│   └── cmd/           # Server command-line tool
│       └── main.go
├── example/           # Example client
│   └── client.go      # Demo TCP server
├── go.mod             # Go module definition
└── README.md          # This file
```

### Building

```bash
# Build all components
go build ./...

# Run tests (if available)
go test ./...

# Format code
go fmt ./...

# Vet code
go vet ./...
```

## Troubleshooting

### "No available port" Error
- The port range (9000-10000) is exhausted
- Increase the port range or close unused connections

### "Connection refused" Error
- Ensure the server is running and accessible
- Check firewall rules
- Verify the correct address and port

### "Failed to connect to connB" Error
- Ensure your internal service is running
- Verify the address format is correct
- Check Unix socket permissions if using Unix sockets

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues.

## License

This project is open source. Please check the repository for license information.

## Author

Created by [Ranxy](https://github.com/Ranxy)

## Acknowledgments

Built with Go's powerful networking capabilities and concurrent programming features.
