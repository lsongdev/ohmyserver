# Architecture Documentation

This document provides a comprehensive overview of the OhMyServer system architecture.

## System Overview

OhMyServer is built as a monolithic application with a modular design, following a layered architecture pattern. The system is composed of several key components that work together to provide server management capabilities.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Web Browser                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      HTTP Server (Port 8080)                     │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                  HTTP Route Handlers                    │    │
│  │  • Docker Handlers    • Project Handlers                │    │
│  │  • Domain Handlers    • Webserver Handlers              │    │
│  │  • Cron Handlers      • TCP Listener Handlers           │    │
│  │  • Monitor Handlers   • SSH Handlers                    │    │
│  │  • ACME Handlers      • ESXi Handlers                   │    │
│  │  • File Handlers                                       │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Service Layer                              │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │ DockerClient │ │  Scheduler   │ │  DeploymentExecutor      │ │
│  └──────────────┘ └──────────────┘ └──────────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │WebserverList.│ │TCP Listener  │ │  Domain Manager          │ │
│  └──────────────┘ └──────────────┘ └──────────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │ ACME Manager │ │  SSH Client  │ │  ESXi Manager          │ │
│  └──────────────┘ └──────────────┘ └──────────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐                               │
│  │  File Manager│ │ HTTP/TCP     │                               │
│  │              │ │   Proxy      │                               │
│  └──────────────┘ └──────────────┘                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Data Layer                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                  SQLite Database                          │   │
│  │  • system_stats     • projects       • deployments        │   │
│  │  • web_hosts        • dns_zones      • dns_provider_      │   │
│  │  • dns_server_      • cron_jobs        connections        │   │
│  │    records          • tcp_listeners                       │   │
│  │  • ssh_servers      • esxi_servers   • acme_accounts      │   │
│  │  • acme_orders      • acme_certs                         │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    External Integrations                         │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │  Docker API  │ │Cloudflare API│ │  DNSPod API              │ │
│  └──────────────┘ └──────────────┘ └──────────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │ Name.com API │ │  Git Repos   │ │  ACME Servers            │ │
│  └──────────────┘ └──────────────┘ └──────────────────────────┘ │
│  ┌──────────────┐ ┌──────────────┐                              │
│  │ VMware ESXi  │ │  SSH Servers │                              │
│  └──────────────┘ └──────────────┘                              │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Main Application (`main.go`)

The entry point of the application that:
- Loads configuration from `~/.ohmyserver/config.yaml`
- Initializes the Server instance
- Sets up HTTP routes and handlers
- Starts the HTTP server on port 8080

**Key Responsibilities:**
- Configuration loading
- Server initialization
- Route registration

### 2. Configuration Module (`config/`)

**File:** `config/config.go`

Handles loading and parsing of YAML configuration:

```go
type Config struct {
    Database struct {
        Driver string
        Path   string
    }
    CollectionInterval int
    Docker struct {
        RegistryMirror string
    }
}
```

**Features:**
- YAML configuration parsing
- Default value handling
- Environment-aware paths

### 3. Server Core (`server/server.go`)

The central component that orchestrates all subsystems:

```go
type Server struct {
    mu                 sync.RWMutex
    db                 *sql.DB
    docker             *DockerClient
    interval           time.Duration
    deployments        map[int64]*DeploymentExecutor
    webserverListener  *WebserverListener
    tcpListenerManager *TCPListenerManager
    cronScheduler      *cron.Scheduler
}
```

**Responsibilities:**
- Database connection management
- Docker client initialization
- Template rendering
- Subsystem coordination

### 4. Docker Client (`server/docker.go`)

A wrapper around the official Docker SDK providing:

**Container Operations:**
- `ListContainers()` - List all containers
- `CreateContainer()` - Create new container
- `StartContainer()` - Start a container
- `StopContainer()` - Stop a container
- `RemoveContainer()` - Remove a container
- `GetContainerLogs()` - Retrieve container logs

**Image Operations:**
- `ListImages()` - List available images
- `PullImage()` - Pull image from registry
- `BuildImage()` - Build image from Dockerfile
- `RemoveImage()` - Remove an image

**Volume & Network Operations:**
- `ListVolumes()` / `CreateVolume()` / `RemoveVolume()`
- `ListNetworks()` / `CreateNetwork()` / `RemoveNetwork()`

### 5. Database Layer (`server/database.go`)

Manages all database operations using SQLite:

**Schema Tables:**
- `system_stats` - System metrics (CPU, memory)
- `projects` - Project definitions
- `deployments` - Deployment records
- `deployment_logs` - Deployment log entries
- `web_listeners` - Yabber HTTP/HTTPS sockets and TLS defaults
- `web_hosts` - Virtual Hosts and ACME certificate bindings
- `web_host_listeners` - Virtual Host to Listener bindings
- `web_routes` - ServeMux routes and handlers
- `tcp_listeners` - TCP proxy configs
- `dns_provider_connections` - Hosted DNS provider credentials
- `dns_zones` - Embedded DNS zones
- `dns_server_records` - Embedded DNS records
- `acme_issuances` - Background automatic certificate workflows
- `cron_jobs` - Scheduled tasks
- `cron_job_executions` - Job execution history

**Key Features:**
- Connection pooling
- Thread-safe operations with mutex
- Time parsing utilities
- Migration support

### 6. Deployment System (`server/deploy.go`)

Handles automated project deployments:

```
┌─────────────────────────────────────────────────────────┐
│              DeploymentExecutor                          │
├─────────────────────────────────────────────────────────┤
│  1. cloneRepository()                                    │
│     └─> Git clone to temp directory                      │
│  2. prepareImage()                                       │
│     └─> Pull or build Docker image                       │
│  3. deployContainer()                                    │
│     └─> Create and start container with volume mount     │
└─────────────────────────────────────────────────────────┘
```

**Deployment Flow:**
1. Update status to "running"
2. Clone Git repository (if configured)
3. Pull Docker image or build from Dockerfile
4. Remove existing container (if any)
5. Create new container with volume mount
6. Start container
7. Update status to "completed"

**Features:**
- Real-time logging via SSE
- Cancellation support
- Error handling and rollback
- Working directory preservation

### 7. Monitoring System (`server/monitor.go`)

Collects and stores system metrics:

```go
func CollectStats() (*SystemStats, error) {
    cpuUsage := cpu.Percent(time.Second, false)
    memInfo := mem.VirtualMemory()
    return &SystemStats{
        CPUUsage:    cpuUsage,
        MemoryTotal: memInfo.Total,
        MemoryUsed:  memInfo.Used,
        MemoryFree:  memInfo.Free,
        MemoryUsage: memInfo.UsedPercent,
    }
}
```

**Collection Process:**
- Runs every 5 seconds (configurable)
- Collects CPU and memory metrics
- Stores in `system_stats` table
- Provides historical data API

**API Endpoints:**
- `GET /monitor/stats` - Current + historical stats
- `GET /monitor/current` - Real-time stats only

### 8. Cron Scheduler (`cron/`)

**Components:**

**`cron/parser.go`** - Cron Expression Parser
- Supports standard 5-field cron format
- Special expressions: `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`
- Range, step, and list support

**`cron/scheduler.go`** - Job Scheduler
```go
type Scheduler struct {
    jobs       map[int64]*scheduledJob
    executor   JobExecutor
    onComplete func(job, executionID, status, output, errorMsg, exitCode)
}
```

**Features:**
- Dynamic job scheduling
- Job persistence in database
- Execution history tracking
- Timeout control
- Enable/disable jobs

### 9. Web Server Module (`server/webserver.go`, `webserver_listener.go`)

Provides reverse proxy functionality:

**Webserver Entity:**
```go
type Webserver struct {
    ID         int64
    Domain     string
    TargetPort int
    Upstream   string
    Status     string
}
```

**Listener Management:**
- Creates HTTP listeners per domain
- Routes requests to backend services
- Supports custom upstream URLs

### 10. TCP Listener Module (`server/tcp_listener.go`, `tcp_handlers.go`)

Manages TCP port forwarding:

**Features:**
- Dynamic TCP listener creation
- Port-to-container mapping
- Status monitoring
- Start/stop control

### 11. Domain Management (`domains/`)

**Structure:**
```
dnsprovider/
├── providers/
│   ├── dns_providers.go # Provider factory
│   ├── cloudflare/      # Cloudflare implementation
│   ├── dnspod/          # DNSPod implementation
│   └── namecom/         # Name.com implementation
└── types/
    └── types.go         # Shared type definitions
```

**Provider Interface:**
```go
type IDNSProvider interface {
    ListZones() ([]Zone, error)
    ListRecords(zoneID string) ([]Record, error)
    AddRecord(zoneID string, record *Record) error
    UpdateRecord(zoneID string, record *Record) error
    DeleteRecord(zoneID, recordID string) error
}
```

**Supported Providers:**
- **Cloudflare**: Uses `cloudflare-go` SDK
- **DNSPod**: Uses `dnspod-go` library
- **Name.com**: Uses `namedotcom/go` client

### 12. ACME / Certificate Management (`server/acme_handlers.go`, `server/acme_types.go`)

Manages SSL/TLS certificates via the ACME protocol (Let's Encrypt compatible):

**Components:**
- **ACME Accounts**: Registration and management
- **Certificate Orders**: Create, track, and finalize orders
- **CSR Generation**: Certificate Signing Request creation
- **Challenge Handling**: HTTP-01 and DNS-01 challenge completion

**Workflow:**
1. Register ACME account with provider
2. Create certificate order
3. Complete challenges (HTTP-01 or DNS-01)
4. Finalize order and download certificate
5. Store certificate in database

### 13. SSH and Terminal (`server/ssh.go`, `terminal/`)

Provides remote server access via SSH:

**Features:**
- SSH server CRUD operations
- SSH key management
- SSH PTY sessions implementing the generic terminal contract
- First-use host-key pinning and later key-change detection
- Real-time connection stats
- Test connection functionality

SSH owns profiles, authentication, host-key verification, and SSH connections.
The terminal package owns only the interactive session contract and WebSocket
bridge. Local, SSH, and Docker sessions therefore share the same transport and
lifecycle behavior.

**Terminal Architecture:**
```
Browser ◄──WebSocket──► terminal.Bridge ◄──Session──► Local / SSH / Docker
```

### 14. VMware ESXi Management (`server/esxi.go`, `server/esxi.go`)

Manages VMware ESXi hosts and virtual machines:

**Features:**
- ESXi host registration
- VM lifecycle management (power on/off, create, delete)
- Host credential management
- VM resource monitoring

**Integration:**
- Uses `vmware/govmomi` SDK
- Connects to vSphere/ESXi API

### 15. File Management (`server/files.go`)

Web-based file browser and manager:

**Features:**
- File listing and navigation
- File upload and download
- Folder creation
- File operations (create, delete)

### 16. Proxy Layer (`yabber/`, `proxy/tcp_proxy.go`)

HTTP and TCP reverse proxy implementation:

**Yabber HTTP/HTTPS Server:**
- Listener, Virtual Host, and Route configuration
- SNI certificate selection from active ACME certificates
- ServeMux routing and reverse proxy, file, response, and redirect handlers
- Atomic Route and certificate reload

**TCP Proxy:**
- Port-to-port forwarding
- Dynamic listener management
- Connection pooling

## Data Flow

### Deployment Flow

```
User Request → HTTP Handler → Create Deployment Record
                                    │
                                    ▼
                          DeploymentExecutor Created
                                    │
                                    ▼
                          Async Execution Started
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
              Clone Repo      Prepare Image   Deploy Container
                    │               │               │
                    └───────────────┼───────────────┘
                                    ▼
                          Update Status & Logs
                                    │
                                    ▼
                          SSE Stream to Client
```

### Monitoring Flow

```
┌─────────────┐
│   Timer     │ (5 second interval)
└──────┬──────┘
       │
       ▼
┌─────────────┐     ┌─────────────┐
│ Collect CPU │     │Collect Memory│
└──────┬──────┘     └──────┬──────┘
       │                   │
       └────────┬──────────┘
                │
                ▼
       ┌────────────────┐
       │ Insert to DB   │
       └────────┬────────┘
                │
                ▼
       ┌────────────────┐
       │ API Endpoint   │◄─── Client Request
       └────────────────┘
```

### DNS Management Flow

```
User Adds Domain
       │
       ▼
Select DNS Provider
       │
       ▼
┌──────────────────┐
│ Provider Factory │
└────────┬─────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
Cloudflare  DNSPod  Name.com
    │         │         │
    └────┬────┴─────────┘
         │
         ▼
  API Call to Provider
         │
         ▼
  Store in Local DB
```

## Concurrency Model

OhMyServer uses Go's concurrency primitives extensively:

### Mutex Protection
```go
type Server struct {
    mu sync.RWMutex  // Protects shared state
    db *sql.DB
    deployments map[int64]*DeploymentExecutor
}
```

### Goroutine Patterns

**Deployment Execution:**
```go
go func() {
    defer cleanup()
    executor.Execute()
}()
```

**Monitoring Collection:**
```go
go func() {
    ticker := time.NewTicker(interval)
    for {
        select {
        case <-ticker.C:
            collectOnce()
        case <-ctx.Done():
            return
        }
    }
}()
```

**WebSocket Handling:**
```go
wg.Add(1)
go func() {
    defer wg.Done()
    // Read from container
}()

wg.Add(1)
go func() {
    defer wg.Done()
    // Read from WebSocket
}()
```

## Security Considerations

### Authentication
Currently, the application does not include built-in authentication. For production use:
- Deploy behind a reverse proxy with authentication
- Use network-level access controls
- Enable HTTPS/TLS

### Docker Access
- Requires Docker daemon access (Unix socket or TCP)
- Runs with Docker API privileges
- Consider using Docker socket proxy for restricted access

### Database Security
- SQLite file stored in user's home directory
- File permissions should be restricted (600)
- Consider encryption at rest for sensitive data

## Performance Characteristics

### Database
- SQLite with connection pooling
- Indexed queries for common lookups
- Write-ahead logging (WAL) mode available

### Memory Usage
- In-memory deployment executor map
- System stats collected at fixed intervals
- Template caching in production

### Concurrency Limits
- One deployment executor per deployment
- Cron jobs executed sequentially per job
- WebSocket connections per container terminal

## Extension Points

### Adding New DNS Providers
1. Create new directory under `dnsprovider/providers/`
2. Implement `DNSProvider` interface
3. Register in provider factory (`dns_providers.go`)

### Adding New ACME Providers
1. Extend ACME provider configuration in `acme_types.go`
2. Add handler logic in `acme_handlers.go`

### Custom Deployment Steps
1. Extend `DeploymentExecutor` type
2. Add new steps in `Execute()` method
3. Update deployment schema if needed

### New Monitoring Metrics
1. Add metric collection in `CollectStats()`
2. Update `SystemStats` struct
3. Modify database schema
4. Update API response format

### Adding New Proxy Protocols
1. Extend proxy types in `proxy/` directory
2. Implement listener management
3. Add web UI for configuration

## Deployment Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Production Server                     │
│                                                          │
│  ┌────────────┐                                         │
│  │ OhMyServer │  :8080                                  │
│  │  Binary    │                                         │
│  └─────┬──────┘                                         │
│        │                                                 │
│        ├─► Unix Socket: /var/run/docker.sock            │
│        │                                                 │
│        ├─► File: ~/.ohmyserver/ohmyserver.db           │
│        │                                                 │
│        ├─► Files: ~/.ohmyserver/workspace/              │
│        │                                                 │
│        ├─► External: ACME Servers (Let's Encrypt)       │
│        │                                                 │
│        ├─► External: VMware ESXi Hosts                  │
│        │                                                 │
│        └─► External: SSH Servers                        │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │              Managed Containers                     │ │
│  │  • project-app-1  • project-api-1  • custom-1      │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │              Managed VMs (ESXi)                     │ │
│  │  • vm-web-1  • vm-db-1  • vm-app-1                │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25.0 |
| Database | SQLite3 (mattn/go-sqlite3) |
| Docker SDK | docker/docker v25.0.4 |
| WebSocket | gorilla/websocket v1.5.3 |
| Config | YAML (gopkg.in/yaml.v3) |
| System Metrics | shirou/gopsutil/v3 |
| DNS Providers | cloudflare-go, dnspod-go, namedotcom/go |
| ACME | lsongdev/acme-go |
| VMware | vmware/govmomi |
| SSH | golang.org/x/crypto/ssh |
| HTTP Server | net/http (standard library) |
| Static Assets | go:embed (standard library) |

## Future Architecture Considerations

### Potential Improvements
1. **Microservices Split**: Extract Docker management, DNS management, and monitoring into separate services
2. **Message Queue**: Add Redis/RabbitMQ for async task processing
3. **Multi-Node Support**: Distributed deployment across multiple servers
4. **Plugin System**: Extensible architecture for custom integrations
5. **Event Bus**: Internal event system for better decoupling

### Scalability Options
1. **Database**: Migrate to PostgreSQL for concurrent writes
2. **Caching**: Add Redis for frequently accessed data
3. **Load Balancing**: Multiple instances behind load balancer
4. **Horizontal Scaling**: Stateless design for easy replication
