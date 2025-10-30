# Crystal - Developer and Agent Documentation

This document provides detailed technical documentation for developers, maintainers, and automated agents working with the Crystal tunneling system.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Component Details](#component-details)
3. [Code Structure](#code-structure)
4. [Protocol and Data Flow](#protocol-and-data-flow)
5. [Concurrency Model](#concurrency-model)
6. [Error Handling](#error-handling)
7. [Resource Management](#resource-management)
8. [Testing Strategy](#testing-strategy)
9. [Performance Considerations](#performance-considerations)
10. [Extension Points](#extension-points)

## Architecture Overview

Crystal implements a reverse proxy/tunneling system with the following architecture:

```
┌─────────────────┐         ┌──────────────────┐         ┌──────────────────┐
│ External Client │────────▶│  Crystal Server  │◀────────│  Crystal Agent   │
│  (any TCP app)  │         │  (Public Server) │         │ (Internal Network│
└─────────────────┘         └──────────────────┘         └──────────────────┘
                                     │                             │
                                     │                             │
                                     │                             ▼
                                     │                    ┌──────────────────┐
                                     │                    │ Internal Service │
                                     │                    │  (e.g., webapp)  │
                                     │                    └──────────────────┘
                                     │
                            Dynamic Port Pool
                              (9000-10000)
```

### Key Design Principles

1. **Separation of Concerns**: Server, Agent, and Example components are independent
2. **Goroutine-based Concurrency**: Each connection handled by dedicated goroutines
3. **Resource Pooling**: Efficient port allocation with reuse
4. **Graceful Degradation**: Proper cleanup and error handling
5. **Zero Configuration**: Works out of the box with sensible defaults

## Component Details

### 1. Server Component (`server/`)

#### `server.go`

**Type: `Server`**
```go
type Server struct {
    listener         net.Listener           // Main listener for agent connections
    portToConnection map[int]*Connection    // Maps allocated ports to connections
    portAllocator    *PortAllocator         // Manages port pool
}
```

**Key Methods:**

- `NewServer(listen string) *Server`
  - Creates and initializes a new server instance
  - Binds to the specified address
  - Initializes port allocator (9000-10000 range)

- `StartProxy()`
  - Main server loop
  - Accepts agent connections
  - Spawns goroutine for each agent via `handleAgentConn()`

- `handleAgentConn(agentConn net.Conn)`
  - Allocates unique port for the agent
  - Creates external listener on allocated port
  - Spawns two goroutines:
    - `handleExternalConnections()`: Accepts external clients
    - `forwardAgentToExternal()`: Broadcasts agent data to clients

**Data Flow:**
```
Agent Connection → Port Allocation → External Listener → Connection Mapping
                                              ↓
                                    External Client Accept
                                              ↓
                                    Bi-directional Forwarding
```

#### `connection.go`

**Type: `Connection`**
```go
type Connection struct {
    AgentConn     net.Conn              // Connection to the agent
    ExternalConns map[net.Conn]bool     // Set of external client connections
    mutex         sync.Mutex             // Protects ExternalConns map
    closeOnce     sync.Once              // Ensures single close
    closeChan     chan struct{}          // Signals connection closure
}
```

**Thread Safety:**
- `mutex` protects concurrent access to `ExternalConns`
- `closeOnce` prevents double-close panics
- Lock is held briefly when modifying the map, released during I/O

**Key Methods:**

- `AddExternalConn(extConn net.Conn)`: Thread-safe addition
- `RemoveExternalConn(extConn net.Conn)`: Thread-safe removal
- `Close()`: Graceful shutdown of all connections

#### `port_allocator.go`

**Type: `PortAllocator`**
```go
type PortAllocator struct {
    ports []int           // Available port pool
    used  map[int]bool    // Tracks allocated ports
    mu    sync.Mutex      // Protects concurrent access
}
```

**Port Allocation Strategy:**
1. Linear search through port range
2. Attempt to bind to verify availability
3. Mark as used on successful bind
4. Close test listener immediately
5. Return allocated port

**Resource Management:**
- Ports are released when agent disconnects
- Failed binds skip to next available port
- Thread-safe allocation via mutex

### 2. Agent Component (`agent/`)

#### `agent.go`

**Key Functions:**

- `dial(address string) (net.Conn, error)`
  - Flexible connection establishment
  - Supports TCP and Unix sockets
  - Auto-detects connection type from address format
  - 10-second connection timeout

**Address Format Support:**
```go
// TCP formats
"tcp://host:port"           → TCP connection
"host:port"                 → TCP connection (auto-detect)

// Unix socket formats
"unix:///path/to/socket"    → Unix socket
"/path/to/socket"           → Unix socket (auto-detect)
```

- `forward(dst, src net.Conn, wg *sync.WaitGroup)`
  - Copies data from src to dst using `io.Copy`
  - Closes destination on completion
  - Handles expected errors (EOF, closed connection)
  - Signals completion via WaitGroup

- `Start(connAAddr string, connBAddr string) error`
  - Main agent entry point
  - Establishes both connections
  - Spawns two forwarding goroutines
  - Handles graceful shutdown via signals

**Signal Handling:**
```go
SIGINT (Ctrl+C)  → Graceful shutdown
SIGTERM          → Graceful shutdown
```

**Shutdown Flow:**
1. Receive signal
2. Close both connections
3. Wait for goroutines to complete
4. Log shutdown status
5. Exit cleanly

### 3. Example Client (`example/`)

#### `client.go`

Demonstrates a simple TCP server that:
- Listens on configurable port (default: 8111)
- Sends periodic messages every 5 seconds
- Handles Telnet control sequences
- Supports graceful shutdown
- Manages multiple concurrent clients

**Features:**
- Ticker-based message broadcasting
- Client connection tracking
- Signal handling for clean shutdown
- Telnet Interrupt Process (IP) command support

## Code Structure

### Package Organization

```
github.com/Ranxy/crystal
├── server/                     # Server package
│   ├── server.go              # Main server logic
│   ├── connection.go          # Connection abstraction
│   ├── port_allocator.go      # Port management
│   └── cmd/                   # Server executable
│       └── main.go
├── agent/                      # Agent package
│   ├── agent.go               # Main agent logic
│   └── cmd/                   # Agent executable
│       └── main.go
└── example/                    # Example package
    └── client.go              # Demo TCP server
```

### Dependency Graph

```
server/cmd/main.go → server/server.go → server/connection.go
                                      → server/port_allocator.go

agent/cmd/main.go → agent/agent.go

example/client.go (standalone)
```

### Import Relationships

- **server package**: No external dependencies beyond stdlib
- **agent package**: No external dependencies beyond stdlib
- **example package**: No external dependencies beyond stdlib

All components use only Go standard library:
- `net`: Network operations
- `io`: I/O primitives
- `sync`: Synchronization primitives
- `log`: Logging
- `flag`: Command-line parsing
- `os/signal`: Signal handling

## Protocol and Data Flow

### Connection Establishment

```
1. Agent → Server (TCP)
   - Agent initiates connection to server
   
2. Server allocates port (e.g., 9001)
   - Searches for available port
   - Binds to port
   - Maps port → Connection

3. Server listens on public port 9001
   - External listener started
   
4. Agent ready to forward
   - Bi-directional pipes established
```

### Data Forwarding

#### Server → External Client Path
```
Agent sends data
       ↓
Server reads from AgentConn
       ↓
Server acquires ExternalConns lock
       ↓
Server copies connection slice
       ↓
Server releases lock
       ↓
Server writes to each ExternalConn
       ↓
External clients receive data
```

#### External Client → Agent Path
```
External client sends data
       ↓
Server reads from ExternalConn
       ↓
Server writes to AgentConn (io.Copy)
       ↓
Agent receives data
       ↓
Agent writes to internal service
```

### Message Flow Example

```
External Client                Server                 Agent              Internal Service
      |                          |                      |                        |
      |-------- TCP SYN -------->|                      |                        |
      |<------ TCP SYN/ACK ------|                      |                        |
      |-------- TCP ACK -------->|                      |                        |
      |                          |                      |                        |
      |------ HTTP Request ----->|                      |                        |
      |                          |---- Forward -------->|                        |
      |                          |                      |---- HTTP Request ----->|
      |                          |                      |<--- HTTP Response -----|
      |                          |<--- Forward ---------|                        |
      |<---- HTTP Response ------|                      |                        |
```

## Concurrency Model

### Goroutine Lifecycle

#### Server Goroutines (per agent)

```
main goroutine
    ├── StartProxy() loop
    │   └── accept() → handleAgentConn() [goroutine]
    │       ├── handleExternalConnections() [goroutine]
    │       │   └── for each external client
    │       │       └── forwardExternalToAgent() [goroutine]
    │       └── forwardAgentToExternal() [goroutine]
```

Total goroutines per agent: **2 + N** (where N = number of external clients)

#### Agent Goroutines

```
main goroutine
    └── Start()
        ├── forward(connB, connA) [goroutine]  // A → B
        ├── forward(connA, connB) [goroutine]  // B → A
        └── signal handler [goroutine]
```

Total goroutines per agent: **3**

### Synchronization Primitives

1. **sync.Mutex**
   - Protects `ExternalConns` map in `Connection`
   - Protects port allocator state
   - Held for minimal duration (map operations only)

2. **sync.WaitGroup**
   - Coordinates goroutine completion
   - Used in both server and agent
   - Ensures clean shutdown

3. **sync.Once**
   - Prevents double-close in `Connection.Close()`
   - Critical for resource cleanup

4. **Channels**
   - `done chan struct{}`: Signals goroutine completion
   - `closeChan chan struct{}`: Signals connection closure
   - `sigChan chan os.Signal`: Receives OS signals

### Race Condition Prevention

- **Map Access**: All map operations protected by mutex
- **Connection Close**: `sync.Once` ensures single execution
- **Port Allocation**: Mutex-protected state transitions
- **Slice Copying**: ExternalConns copied before iteration to avoid lock holding during I/O

## Error Handling

### Error Categories

1. **Expected Errors** (logged but not fatal)
   ```go
   - io.EOF
   - "use of closed network connection"
   ```

2. **Transient Errors** (logged, continue operation)
   ```go
   - Individual client write failures
   - Accept() failures
   ```

3. **Fatal Errors** (logged, terminate)
   ```go
   - Server bind failure
   - Both agent connections fail
   ```

### Error Handling Patterns

#### Silent Handling (Expected)
```go
if err != nil && !strings.Contains(err.Error(), "use of closed network connection") && err != io.EOF {
    log.Printf("Unexpected error: %v", err)
}
```

#### Cleanup on Error
```go
defer func() {
    conn.RemoveExternalConn(extConn)
    handleNetCloseError(extConn)
}()
```

#### Graceful Degradation
```go
if _, err := extConn.Write(data); err != nil {
    log.Printf("Error writing to external client %s: %v. Closing connection.", extConn.RemoteAddr(), err)
    // Let the reading goroutine handle the closure
}
```

## Resource Management

### Connection Lifecycle

```
Creation → Active → Closing → Closed
   ↓         ↓         ↓         ↓
Accept    Transfer   Signal   Cleanup
```

### Resource Cleanup Checklist

When agent disconnects:
1. ✓ Close agent connection
2. ✓ Close all external connections
3. ✓ Close external listener
4. ✓ Release allocated port
5. ✓ Remove from connection map
6. ✓ Wait for goroutines to complete

### Memory Management

- **Buffer Pooling**: Could be implemented for high-throughput scenarios
- **Connection Maps**: Cleaned up on disconnect
- **Goroutine Leaks**: Prevented by proper WaitGroup usage

### File Descriptor Limits

Consider system limits:
```bash
# Check current limits
ulimit -n

# Typical limit: 1024
# Each connection uses 1-2 FDs
# Max agents ≈ (limit / 3) conservatively
```

## Testing Strategy

### Unit Testing Approach

#### Server Tests
```go
// Test port allocation
func TestPortAllocator_GetAvailablePort(t *testing.T)
func TestPortAllocator_ReleasePort(t *testing.T)
func TestPortAllocator_Exhaustion(t *testing.T)

// Test connection management
func TestConnection_AddRemove(t *testing.T)
func TestConnection_ConcurrentAccess(t *testing.T)
func TestConnection_Close(t *testing.T)

// Test server lifecycle
func TestServer_StartStop(t *testing.T)
func TestServer_MultipleAgents(t *testing.T)
```

#### Agent Tests
```go
// Test connection establishment
func TestDial_TCP(t *testing.T)
func TestDial_Unix(t *testing.T)
func TestDial_InvalidAddress(t *testing.T)

// Test forwarding
func TestForward_Bidirectional(t *testing.T)
func TestForward_ConnectionClose(t *testing.T)

// Test signal handling
func TestStart_GracefulShutdown(t *testing.T)
```

### Integration Testing

```go
// Full end-to-end test
func TestE2E_TunnelSetup(t *testing.T) {
    // 1. Start server
    // 2. Start mock service
    // 3. Start agent
    // 4. Connect external client
    // 5. Verify data flow
    // 6. Clean shutdown
}
```

### Manual Testing Scenarios

1. **Basic Functionality**
   ```bash
   # Terminal 1: Start server
   ./crystal-server -listen :9000
   
   # Terminal 2: Start internal service
   ./example-client -addr :8111
   
   # Terminal 3: Start agent
   ./crystal-agent -a :9000 -b :8111
   
   # Terminal 4: Connect client
   telnet localhost 9001
   ```

2. **Load Testing**
   ```bash
   # Multiple concurrent connections
   for i in {1..100}; do
       (telnet localhost 9001 &)
   done
   ```

3. **Failure Scenarios**
   - Kill agent (test cleanup)
   - Kill internal service (test error handling)
   - Fill port range (test exhaustion)

### Benchmarking

```go
func BenchmarkForwarding(b *testing.B)
func BenchmarkPortAllocation(b *testing.B)
func BenchmarkConcurrentClients(b *testing.B)
```

## Performance Considerations

### Bottlenecks and Optimizations

1. **I/O Performance**
   - Uses `io.Copy()` for efficient transfers
   - 2048-byte buffer size (tunable)
   - No unnecessary allocations in hot path

2. **Lock Contention**
   - Mutex held only for map operations
   - Connection list copied before I/O
   - Port allocator uses linear search (acceptable for 1000-port range)

3. **Goroutine Overhead**
   - One goroutine per external client
   - Acceptable for typical use cases (<1000 clients)
   - Consider worker pool for extreme scale

4. **Memory Usage**
   ```
   Per Agent:
   - Connection struct: ~200 bytes
   - 2 goroutines: ~8KB stack space
   
   Per External Client:
   - Map entry: ~32 bytes
   - 1 goroutine: ~4KB stack space
   ```

### Scalability Limits

| Component | Limit | Bottleneck |
|-----------|-------|------------|
| Concurrent Agents | ~1000 | Port range |
| Clients per Agent | ~10000 | File descriptors |
| Throughput | ~1 Gbps | CPU, network |
| Latency | <1ms | Minimal overhead |

### Performance Tuning

```go
// Increase buffer size for high-bandwidth
buf := make([]byte, 32768)  // 32KB

// Expand port range for more agents
NewPortAllocator(9000, 20000)

// Adjust connection timeout
dialer.Timeout = 30 * time.Second
```

## Extension Points

### 1. Authentication

Add authentication layer in `handleAgentConn()`:

```go
func (s *Server) handleAgentConn(agentConn net.Conn) {
    // Add authentication handshake
    if !authenticate(agentConn) {
        log.Println("Authentication failed")
        agentConn.Close()
        return
    }
    
    // Continue with existing logic...
}
```

### 2. Encryption

Wrap connections with TLS:

```go
import "crypto/tls"

func NewSecureServer(listen string, cert tls.Certificate) *Server {
    config := &tls.Config{Certificates: []tls.Certificate{cert}}
    listener, err := tls.Listen("tcp", listen, config)
    // ...
}
```

### 3. Metrics and Monitoring

Add metrics collection:

```go
type Metrics struct {
    ActiveAgents     int64
    TotalConnections int64
    BytesTransferred int64
}

// Update during operation
atomic.AddInt64(&metrics.BytesTransferred, int64(n))
```

### 4. Rate Limiting

Implement per-connection rate limiting:

```go
import "golang.org/x/time/rate"

type Connection struct {
    // ... existing fields
    limiter *rate.Limiter
}

// Apply in forwarding logic
if !c.limiter.Allow() {
    time.Sleep(c.limiter.Reserve().Delay())
}
```

### 5. Protocol Support

Add protocol-specific handling:

```go
type Protocol interface {
    HandleConnection(conn net.Conn) error
}

type HTTPProtocol struct{}
type WebSocketProtocol struct{}
```

### 6. Configuration File

Replace command-line flags with config file:

```go
type Config struct {
    Server struct {
        Listen    string
        PortRange struct {
            Start int
            End   int
        }
    }
}
```

### 7. Health Checks

Add endpoint for monitoring:

```go
func (s *Server) HealthCheck() error {
    // Check if listener is active
    // Check if port allocator has capacity
    // Return status
}
```

## Debugging Tips

### Enable Verbose Logging

```go
// Add debug flag
var debug bool
flag.BoolVar(&debug, "debug", false, "Enable debug logging")

// Use in code
if debug {
    log.Printf("DEBUG: Connection from %s", conn.RemoteAddr())
}
```

### Track Goroutines

```go
import "runtime"

// Monitor goroutine count
log.Printf("Active goroutines: %d", runtime.NumGoroutine())
```

### Connection Tracking

```go
// Add connection ID
type Connection struct {
    ID string  // Add unique identifier
    // ... other fields
}

// Use in logs
log.Printf("[%s] Agent connected", conn.ID)
```

### Network Tracing

```bash
# Capture traffic for analysis
tcpdump -i any port 9000 -w crystal.pcap

# View in Wireshark
wireshark crystal.pcap
```

## Code Quality Standards

### Style Guidelines

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable names
- Document exported functions
- Keep functions focused and small
- Handle errors explicitly

### Linting

```bash
# Run static analysis
go vet ./...

# Run linter
golangci-lint run
```

### Code Review Checklist

- [ ] Thread-safety verified
- [ ] Resources properly cleaned up
- [ ] Errors handled appropriately
- [ ] Logging is informative
- [ ] No goroutine leaks
- [ ] No race conditions
- [ ] Documentation updated

## Future Enhancements

### Planned Features

1. **TLS/SSL Support**: Encrypted communication
2. **Authentication**: Token-based or certificate-based
3. **Web Dashboard**: Real-time monitoring UI
4. **API Server**: RESTful API for management
5. **Configuration File**: YAML/JSON config support
6. **Load Balancing**: Multiple agents for same service
7. **Health Checks**: Active service monitoring
8. **Metrics Export**: Prometheus/Grafana integration

### Research Areas

- **QUIC Protocol**: Faster connection establishment
- **HTTP/2**: Multiplexed connections
- **Connection Pooling**: Reuse connections
- **Adaptive Buffering**: Dynamic buffer sizing

## Conclusion

Crystal is a well-architected, production-ready tunneling system with clear separation of concerns, robust error handling, and good performance characteristics. The codebase is maintainable, extensible, and follows Go best practices.

For questions or contributions, please refer to the main README.md or open an issue on GitHub.
