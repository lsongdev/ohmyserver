# OhMyServer Documentation

## User Documentation

- [Installation Guide](installation.md) - Step-by-step installation instructions
- [Configuration Guide](configuration.md) - All configuration options
- [API v1 Reference](api-v1.md) - Current modular HTTP API and module mounts
- [Legacy API Reference](api-reference.md) - Historical routes; not valid for the current app
- [Architecture](architecture.md) - System architecture overview

## Developer Documentation

- [模块化前后端重构方案](frontend-modular-refactor.md) - Preact + HTM 原生 ESM、无构建交付、/v1 模块 API 与分阶段迁移计划
- [Improvement Suggestions](improvements.md) - Project analysis and improvement recommendations
- [Refactoring Guide](refactoring-guide.md) - Concrete refactoring steps and code examples
- [产品定位与角色化平台规划](product-roadmap.md) - Router、NAS、Cloud、HomeLab、IoT 等角色规划与能力路线
- [Web Server 与 ACME、DNS 联动设计](webserver-design.md) - 极简 Web Server、证书热更新、DNS 联动与服务管理设计
- [极简 CI/CD 服务设计](cicd-design.md) - GitHub Webhook、持久化部署队列、Docker 构建与安全发布

## Features

### 🐳 Docker Management
- **Containers**: Create, start, stop, restart, and remove containers
- **Images**: Pull images from registries, build images from Dockerfiles
- **Volumes**: Create and manage persistent storage volumes
- **Networks**: Create and configure Docker networks
- **Terminal Access**: Interactive container terminal via WebSocket
- **Container Stats**: Real-time container resource usage

### 📊 System Monitoring
- Real-time CPU usage monitoring
- Memory usage tracking
- Historical statistics with configurable retention
- RESTful API for metrics integration

### 🚀 Project Deployment
- Git repository integration
- Automated Docker image building
- One-click deployment with rollback support
- Real-time deployment logs via Server-Sent Events (SSE)
- Container lifecycle management

### 🌐 Web Server & Reverse Proxy
- Domain-based routing (HTTP listeners)
- TCP port forwarding
- HTTP/HTTPS support
- Upstream configuration
- Listener status monitoring

### 🌍 DNS Management
- Embedded UDP, TCP, DoT, and DoH DNS service
- Local DNS zones and records
- Cloudflare, DNSPod, and Name.com hosted-zone connections
- Upstream forwarding and response cache
- Optional filter rules and subscriptions

### 🔐 ACME / SSL Certificates
- ACME protocol support (Let's Encrypt compatible)
- Multiple ACME account management
- One-step certificate issuance with explicit account selection
- CSR (Certificate Signing Request) creation
- Certificate order management
- Automatic HTTP-01 and local/hosted DNS-01 challenge completion
- Certificate lifecycle management

### 🔧 SSH Management
- SSH server management (add, edit, delete)
- SSH key management
- Interactive terminal access via WebSocket
- Real-time connection stats
- Test connection functionality

### 💻 VMware ESXi Management
- ESXi host management
- Virtual machine operations
- VM lifecycle management
- Host credential management

### 📁 File Management
- Web-based file browser
- File upload and download
- Folder creation
- File operations (create, delete)

### ⏰ Cron Job Scheduler
- Standard 5-field cron expressions
- Special expressions (@yearly, @monthly, @weekly, @daily, @hourly, @minutely)
- One-time scheduled tasks
- Job execution history and logging
- Timeout control
- Enable/disable jobs dynamically
- Manual job execution (Run Now)

## Quick Start

### Prerequisites

- Go 1.25.0 or later
- Docker installed and running
- SQLite3 (embedded, no separate installation needed)

### Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/lsongdev/ohmyserver.git
   cd ohmyserver
   ```

2. **Build the application**:
   ```bash
   go build -o ohmyserver
   ```

3. **Create configuration directory**:
   ```bash
   mkdir -p ~/.ohmyserver
   ```

4. **Create configuration file** (optional, defaults will be used if not provided):
   ```bash
   cp config.yaml.example ~/.ohmyserver/config.yaml
   ```

5. **Run the server**:
   ```bash
   ./ohmyserver
   ```

6. **Access the web interface**:
   Open your browser and navigate to `http://localhost:8080`

## Project Structure

```
ohmyserver/
├── cmd/
│   ├── ohmyserver/             # Host process entry point and HTTP shell
│   └── agent/                  # Metrics collection agent
├── go.work                     # Local workspace for extracted Go modules
├── config/                     # Host configuration
├── internal/
│   ├── app/                    # Shared module mounting contracts
│   ├── host/                   # Host lifecycle, assembly, adapters, and routes
│   └── architecture/           # Architecture boundary checks
├── modules/                    # Feature modules: Go API + owned web assets
│   ├── docker/
│   ├── acme/
│   ├── dns/
│   └── ...
├── frontend/                   # Shell, shared UI, and pinned browser dependencies
│   ├── app/
│   ├── shared/
│   └── vendor/
├── pkg/                        # Reusable Go libraries and module web helpers
│   ├── cron/
│   ├── moduleweb/
│   └── terminal/
├── tests/module-smoke/          # Standalone module host checks
└── docs/                        # User and architecture documentation
```

The host entry point delegates startup to `internal/host`; HTTP routing, shell serving, and module assembly live there. Feature work belongs in `modules/<feature>/` with backend and frontend together.

## Configuration

OhMyServer uses a YAML configuration file located at `~/.ohmyserver/config.yaml`.

### Example Configuration

```yaml
database:
  driver: sqlite3
  path: ~/.ohmyserver/ohmyserver.db

collection_interval_seconds: 5

docker:
  registry_mirror: ""
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `database.driver` | string | `sqlite3` | Database driver |
| `database.path` | string | `~/.ohmyserver/ohmyserver.db` | Database file path |
| `collection_interval_seconds` | int | `5` | Metrics collection interval |
| `docker.registry_mirror` | string | `""` | Docker registry mirror URL |

For detailed configuration documentation, see [Configuration Guide](docs/configuration.md).

## API Endpoints

### System Monitoring
- `GET /monitor` - Monitoring dashboard
- `GET /monitor/stats` - Get system statistics (with history)
- `GET /monitor/current` - Get current system stats

### Docker Management
- `GET /docker/containers` - List containers
- `POST /docker/containers/create` - Create container
- `PUT /docker/containers/{id}?action=start|stop|restart` - Container actions
- `GET /docker/containers/{id}/logs` - Get container logs
- `GET /docker/containers/{id}/stats` - Get container stats
- `GET /docker/containers/{id}/terminal` - Container terminal page
- `GET /docker/containers/{id}/terminal/ws` - Container terminal WebSocket
- `GET /docker/images` - List images
- `POST /docker/images/pull` - Pull image (SSE)
- `POST /docker/images/build` - Build image
- `GET /docker/volumes` - List volumes
- `POST /docker/volumes/create` - Create volume
- `GET /docker/networks` - List networks
- `POST /docker/networks/create` - Create network

### Project Management
- `GET /projects` - List projects
- `POST /projects/new` - Create project
- `GET /projects/{id}` - Project details
- `POST /projects/{id}/settings` - Edit project
- `POST /projects/{id}/delete` - Delete project
- `GET /projects/{id}/deployments` - List deployments
- `POST /projects/{id}/deployments` - Deploy project
- `GET /projects/{id}/deployments/{deploymentId}` - Deployment details
- `GET /projects/{id}/deployments/{deploymentId}/logs` - Deployment logs (SSE)
- `POST /projects/{id}/deployments/{deploymentId}/cancel` - Cancel deployment

### DNS Management
- `GET /dns` - DNS service overview
- `GET /dns/zones` - List local DNS zones
- `POST /dns/zones/new` - Create a local DNS zone
- `GET /dns/filters` - Manage filter rules and subscriptions
- `GET /dns/settings` - Configure listeners, cache, and upstreams
- `GET /services` - View and manage network service state

### ACME / SSL Certificates
- `GET /acme/certificates` - List certificates
- `POST /acme/create-csr` - Create CSR
- `GET /acme/providers` - List ACME providers
- `POST /acme/providers/new` - Add ACME provider
- `POST /acme/providers/{id}/register` - Register ACME account
- `GET /acme/accounts` - List ACME accounts
- `GET /acme/orders` - List certificate orders
- `POST /acme/orders/new` - Create new order
- `POST /acme/orders/{id}/finalize` - Finalize order

### SSH Management
- `GET /ssh/servers` - List SSH servers
- `POST /ssh/servers/new` - Add SSH server
- `GET /ssh/servers/{id}` - SSH server details
- `POST /ssh/servers/{id}/edit` - Edit SSH server
- `GET /ssh/servers/{id}/terminal` - SSH terminal page
- `GET /ssh/servers/{id}/terminal/ws` - SSH terminal WebSocket
- `POST /ssh/servers/test` - Test SSH connection

### ESXi Management
- `GET /esxi` - List ESXi hosts
- `POST /esxi/new` - Add ESXi host
- `GET /esxi/{id}` - ESXi host details
- `POST /esxi/{id}/edit` - Edit ESXi host

### File Management
- `GET /files` - File browser
- `POST /files/new` - Create file
- `POST /files/new-folder` - Create folder
- `POST /files/upload` - Upload file
- `GET /files/download` - Download file

### Network Services, Web Server & TCP/UDP Proxy
- `GET /services` - View and manage service runtime state
- `POST /services/{id}/action` - Start, stop, or reload a service
- `GET /web` - Yabber overview
- `POST /web/listeners/new` - Create HTTP/HTTPS listener
- `POST /web/hosts/new` - Create virtual host
- `POST /web/hosts/{id}/routes/new` - Create route
- `GET /proxy` - List TCP/UDP proxies
- `POST /proxy/new` - Create a TCP/UDP proxy
- `GET /proxy/{id}` - Proxy details
- `POST /proxy/{id}/edit` - Edit a proxy
- `POST /proxy/{id}/delete` - Delete a proxy
- `POST /proxy/status` - Activate, deactivate, or restart a proxy

### Cron Jobs
- `GET /cron` - List cron jobs
- `POST /cron/new` - Create cron job
- `GET /cron/{id}` - Cron job details
- `POST /cron/{id}/edit` - Edit cron job
- `POST /cron/{id}/delete` - Delete cron job
- `POST /cron/{id}/toggle` - Enable/disable cron job
- `POST /cron/{id}/run` - Run cron job now

For the current modular API, see [API v1 Reference](api-v1.md). The old [API Reference](api-reference.md) documents routes removed during the frontend/API migration.

## Architecture

OhMyServer follows a modular architecture:

- **Web Layer**: HTTP handlers, HTML templates, and static assets (CSS/JS)
- **Service Layer**: Business logic for Docker, DNS, ACME, SSH, ESXi, cron, projects, files
- **Proxy Layer**: HTTP and TCP reverse proxy implementations
- **Data Layer**: SQLite database for persistence
- **Integration Layer**: Docker SDK, DNS provider APIs, ACME protocol, VMware vSphere SDK, SSH

See [Architecture Documentation](docs/architecture.md) for detailed information.

## Development

### Building from Source

```bash
go build -o ohmyserver
```

### Running Tests

```bash
go test ./...
```

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

For issues and feature requests, please open an issue on GitHub.
