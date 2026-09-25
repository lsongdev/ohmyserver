# Legacy API Reference (Pre-v1)

> This document describes the former template-era API and is retained only as historical material. Those routes are no longer mounted. Use [API v1 Reference](api-v1.md) for the current API.

## Historical content

This document provides comprehensive documentation for the OhMyServer REST API.

## Base URL

```
http://localhost:8080
```

## Authentication

Currently, the API does not require authentication. For production deployments, it's recommended to:
- Deploy behind a reverse proxy with authentication
- Use network-level access controls
- Enable HTTPS/TLS termination

## Response Formats

### HTML Responses
Most endpoints return HTML pages rendered from templates for browser access.

### JSON Responses
API endpoints return JSON data:
```json
{
  "id": 1,
  "name": "example",
  "status": "active"
}
```

### Server-Sent Events (SSE)
Real-time endpoints use SSE for streaming logs and status updates:
```
event: log
data: Building image...

event: status
data: running
```

---

## System Monitoring

### Get Current System Stats

Returns real-time CPU and memory statistics.

**Endpoint:** `GET /monitor/current`

**Response:** `application/json`

```json
{
  "id": 123,
  "timestamp": "2024-01-15T10:30:00Z",
  "cpu_usage": 25.5,
  "memory_total": 17179869184,
  "memory_used": 8589934592,
  "memory_free": 8589934592,
  "memory_usage": 50.0
}
```

**Fields:**
| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Record ID |
| `timestamp` | string | ISO 8601 timestamp |
| `cpu_usage` | float | CPU usage percentage (0-100) |
| `memory_total` | integer | Total memory in bytes |
| `memory_used` | integer | Used memory in bytes |
| `memory_free` | integer | Free memory in bytes |
| `memory_usage` | float | Memory usage percentage (0-100) |

**Example:**
```bash
curl http://localhost:8080/monitor/current
```

---

### Get System Stats History

Returns current and historical system statistics.

**Endpoint:** `GET /monitor/stats`

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `duration` | string | `1h` | Time duration (e.g., `1h`, `30m`, `24h`) |

**Response:** `application/json`

```json
{
  "current": {
    "id": 123,
    "timestamp": "2024-01-15T10:30:00Z",
    "cpu_usage": 25.5,
    "memory_total": 17179869184,
    "memory_used": 8589934592,
    "memory_free": 8589934592,
    "memory_usage": 50.0
  },
  "history": [
    {
      "id": 122,
      "timestamp": "2024-01-15T10:29:55Z",
      "cpu_usage": 24.8,
      "memory_total": 17179869184,
      "memory_used": 8500000000,
      "memory_free": 8679869184,
      "memory_usage": 49.5
    }
  ]
}
```

**Example:**
```bash
# Get last hour of stats
curl http://localhost:8080/monitor/stats

# Get last 24 hours
curl "http://localhost:8080/monitor/stats?duration=24h"

# Get last 30 minutes
curl "http://localhost:8080/monitor/stats?duration=30m"
```

---

### Monitor Dashboard

**Endpoint:** `GET /monitor`

**Response:** `text/html`

Web-based monitoring dashboard with real-time charts.

---

## Docker Management

### List Containers

**Endpoint:** `GET /docker/containers`

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Container ID (optional, returns single container detail) |

**Response:** `text/html` (HTML page with container list)

**Example:**
```bash
# List all containers
curl http://localhost:8080/docker/containers

# Get container details
curl "http://localhost:8080/docker/containers?id=abc123"
```

---

### Create Container

**Endpoint:** `POST /docker/containers/create`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Container name |
| `image` | string | Yes | Docker image name |
| `command` | string | No | Command to run |

**Response:** `text/html`

**Example:**
```bash
curl -X POST http://localhost:8080/docker/containers/create \
  -d "name=my-container" \
  -d "image=nginx:latest" \
  -d "command=nginx -g 'daemon off;'"
```

---

### Get Container Logs

**Endpoint:** `GET /docker/containers/{id}/logs`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Container ID or name |

**Response:** `text/plain`

**Example:**
```bash
curl http://localhost:8080/docker/containers/abc123/logs
```

---

### Container Terminal

**Page:** `GET /docker/containers/{id}/terminal`

**WebSocket:** `GET /docker/containers/{id}/terminal/ws`

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Container ID |

**Response:** `WebSocket` connection for interactive terminal

**Example:**
```javascript
const ws = new WebSocket('ws://localhost:8080/docker/containers/abc123/terminal/ws');
```

---

### Update Container

**Endpoint:** `PUT /docker/containers`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Container ID |
| `action` | string | Yes | Action: `start`, `stop`, `restart` |

**Response:** `200 OK` on success

**Example:**
```bash
# Start container
curl -X PUT "http://localhost:8080/docker/containers?id=abc123&action=start"

# Stop container
curl -X PUT "http://localhost:8080/docker/containers?id=abc123&action=stop"

# Restart container
curl -X PUT "http://localhost:8080/docker/containers?id=abc123&action=restart"
```

---

### Delete Container

**Endpoint:** `DELETE /docker/containers`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Container ID |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X DELETE "http://localhost:8080/docker/containers?id=abc123"
```

---

### List Images

**Endpoint:** `GET /docker/images`

**Response:** `text/html` (HTML page with image list)

**Example:**
```bash
curl http://localhost:8080/docker/images
```

---

### Pull Image

**Endpoint:** `POST /docker/images/pull`

**Query/Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Image name (e.g., `nginx:latest`) |

**Response:** Server-Sent Events (SSE) stream

**SSE Events:**
| Event | Data | Description |
|-------|------|-------------|
| `log` | string | Progress log message |
| `error` | string | Error message |
| `done` | string | Completion message |

**Example:**
```bash
# Using curl to see SSE
curl "http://localhost:8080/docker/images/pull?name=nginx:latest"
```

**JavaScript Example:**
```javascript
const evtSource = new EventSource('/docker/images/pull?name=nginx:latest');

evtSource.onmessage = (event) => {
  console.log('Event:', event.type);
  console.log('Data:', event.data);
};
```

---

### Build Image

**Endpoint:** `POST /docker/images/build`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Image name |
| `dockerfile` | string | Yes | Dockerfile content |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X POST http://localhost:8080/docker/images/build \
  -d "name=myapp:latest" \
  -d "dockerfile=FROM nginx\nCOPY . /usr/share/nginx/html"
```

---

### Delete Image

**Endpoint:** `DELETE /docker/images`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Image ID |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X DELETE "http://localhost:8080/docker/images?id=sha256:abc123"
```

---

### List Volumes

**Endpoint:** `GET /docker/volumes`

**Response:** `text/html` (HTML page with volume list)

**Example:**
```bash
curl http://localhost:8080/docker/volumes
```

---

### Create Volume

**Endpoint:** `POST /docker/volumes/create`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Volume name |
| `driver` | string | No | Volume driver (default: `local`) |

**Response:** `text/html`

**Example:**
```bash
curl -X POST http://localhost:8080/docker/volumes/create \
  -d "name=my-volume" \
  -d "driver=local"
```

---

### Delete Volume

**Endpoint:** `DELETE /docker/volumes`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Volume name |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X DELETE "http://localhost:8080/docker/volumes?id=my-volume"
```

---

### List Networks

**Endpoint:** `GET /docker/networks`

**Response:** `text/html` (HTML page with network list)

**Example:**
```bash
curl http://localhost:8080/docker/networks
```

---

### Create Network

**Endpoint:** `POST /docker/networks/create`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Network name |
| `driver` | string | No | Network driver (default: `bridge`) |
| `subnet` | string | No | Subnet CIDR |
| `gateway` | string | No | Gateway IP |
| `iprange` | string | No | IP range CIDR |

**Response:** `text/html`

**Example:**
```bash
curl -X POST http://localhost:8080/docker/networks/create \
  -d "name=my-network" \
  -d "driver=bridge" \
  -d "subnet=172.20.0.0/16" \
  -d "gateway=172.20.0.1"
```

---

### Delete Network

**Endpoint:** `DELETE /docker/networks`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | Yes | Network ID or name |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X DELETE "http://localhost:8080/docker/networks?id=my-network"
```

---

## Project Management

### List Projects

**Endpoint:** `GET /projects`

**Response:** `text/html` (HTML page with project list)

**Example:**
```bash
curl http://localhost:8080/projects
```

---

### Create Project

**Endpoint:** `POST /projects/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Project name (must be unique) |
| `description` | string | No | Project description |
| `repo` | string | No | Git repository URL |

**Response:** Redirect to `/projects`

**Example:**
```bash
curl -X POST http://localhost:8080/projects/new \
  -d "name=my-project" \
  -d "description=My awesome project" \
  -d "repo=https://github.com/user/repo.git"
```

---

### Get Project Details

**Endpoint:** `GET /projects/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |

**Response:** `text/html` (HTML page with project details)

**Example:**
```bash
curl http://localhost:8080/projects/1
```

---

### Edit Project

**Endpoint:** `GET /projects/{id}/settings` (form)  
**Endpoint:** `POST /projects/{id}/settings` (submit)

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Project name |
| `description` | string | No | Project description |
| `repo` | string | No | Git repository URL |

**Response:** Redirect to `/projects/{id}`

**Example:**
```bash
# View edit form
curl http://localhost:8080/projects/1/settings

# Submit changes
curl -X POST http://localhost:8080/projects/1/settings \
  -d "name=updated-name" \
  -d "description=Updated description" \
  -d "repo=https://github.com/user/new-repo.git"
```

---

### Delete Project

**Endpoint:** `POST /projects/{id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |

**Response:** Redirect to `/projects`

**Example:**
```bash
curl -X POST http://localhost:8080/projects/1/delete
```

---

### View Deployments

**Endpoint:** `GET /projects/{id}/deployments`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |

**Response:** `text/html` (HTML page with deployment list)

**Example:**
```bash
curl http://localhost:8080/projects/1/deployments
```

---

### Create Deployment

**Endpoint:** `POST /projects/{id}/deployments`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `docker_image` | string | No | Docker image to pull |
| `dockerfile` | string | No | Dockerfile content |
| `container_name` | string | No | Container name |
| `command` | string | No | Command to run in container |

**Note:** Either `docker_image` or `dockerfile` must be provided.

**Response:** Redirect to deployment detail page

**Example:**
```bash
curl -X POST http://localhost:8080/projects/1/deployments \
  -d "docker_image=nginx:latest" \
  -d "container_name=my-app" \
  -d "command=python3 -m http.server 8080"
```

---

### Get Deployment Details

**Endpoint:** `GET /projects/{id}/deployments/{deploymentId}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |
| `deploymentId` | integer | Deployment ID |

**Response:** `text/html` (HTML page with deployment details and logs)

**Example:**
```bash
curl http://localhost:8080/projects/1/deployments/5
```

---

### Deployment Logs (SSE)

**Endpoint:** `GET /projects/{id}/deployments/{deploymentId}/logs`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |
| `deploymentId` | integer | Deployment ID |

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `lastLogId` | integer | `0` | Last received log ID |

**Response:** Server-Sent Events (SSE) stream

**SSE Events:**
| Event | Data | Description |
|-------|------|-------------|
| `connected` | JSON | `{"lastLogId": N}` |
| `status` | string | Deployment status (`pending`, `running`, `completed`, `failed`, `cancelled`) |
| `log` | string | `ID:Message` format |
| `complete` | string | Final status |
| `error` | string | Error message |

**Example:**
```bash
curl "http://localhost:8080/projects/1/deployments/5/logs?lastLogId=0"
```

**JavaScript Example:**
```javascript
const evtSource = new EventSource('/projects/1/deployments/5/logs?lastLogId=0');

evtSource.addEventListener('connected', (e) => {
  const data = JSON.parse(e.data);
  console.log('Connected, last log ID:', data.lastLogId);
});

evtSource.addEventListener('status', (e) => {
  console.log('Status:', e.data);
});

evtSource.addEventListener('log', (e) => {
  const [id, message] = e.data.split(':');
  console.log(`Log ${id}:`, message);
});

evtSource.addEventListener('complete', (e) => {
  console.log('Deployment completed:', e.data);
  evtSource.close();
});
```

---

### Cancel Deployment

**Endpoint:** `POST /projects/{id}/deployments/{deploymentId}/cancel`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID |
| `deploymentId` | integer | Deployment ID |

**Response:** `200 OK` on success

**Example:**
```bash
curl -X POST http://localhost:8080/projects/1/deployments/5/cancel
```

---

## Web Server Management

The Web Server UI manages Yabber listeners, virtual hosts, and routes. All
endpoints return HTML and require authentication.

### Overview

**Endpoint:** `GET /web`

Shows runtime status, configured and bound Listener addresses, Virtual Hosts,
certificates, and Route counts.

---

### Create Listener

**Endpoint:** `GET|POST /web/listeners/new`

**Form Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Unique Listener name |
| `address` | string | Yes | Listen address, such as `:80` |
| `protocol` | string | Yes | `http` or `https` |
| `min_tls_version` | string | HTTPS only | `TLS1.2` or `TLS1.3` |
| `default_certificate_id` | integer | No | Active ACME certificate |
| `enabled` | checkbox | No | Desired Listener state |

---

### Edit or Delete Listener

**Endpoint:** `GET|POST /web/listeners/{id}`

Use `_method=PUT` to update and `_method=DELETE` to delete. Listener topology
changes restart Yabber; Host and Route traffic remains separate from the panel
control Listener.

---

### Create Virtual Host

**Endpoint:** `GET|POST /web/hosts/new`

Virtual Host fields include `name`, newline/comma-separated `hostnames`,
`listener_ids`, optional `certificate_id`, `force_https`, and `enabled`.

### Edit or Delete Virtual Host

**Endpoint:** `GET|POST /web/hosts/{id}`

Use `_method=PUT` to update and `_method=DELETE` to delete. Deleting a Virtual
Host also deletes its Routes.

### Create Route

**Endpoint:** `GET|POST /web/hosts/{id}/routes/new`

Supported handlers are `reverse_proxy`, `file_server`, `static_response`, and
`redirect`. Route matching supports optional HTTP method and a ServeMux path
pattern.

### Edit or Delete Route

**Endpoint:** `GET|POST /web/routes/{id}`

Use `_method=PUT` to update and `_method=DELETE` to delete. Successful changes
are atomically applied to Yabber.

---

## TCP/UDP Proxy Management

### List Proxies

**Endpoint:** `GET /proxy`

**Response:** `text/html` (HTML page with TCP/UDP proxy list)

**Example:**
```bash
curl http://localhost:8080/proxy
```

---

### Create Proxy

**Endpoint:** `POST /proxy/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Friendly proxy name |
| `protocol` | string | Yes | `tcp` or `udp` |
| `listen_port` | integer | Yes | Listening port |
| `forward_port` | integer | Yes | Upstream port |
| `forward_host` | string | No | Upstream host (default: `127.0.0.1`) |

**Response:** Redirect to proxy list

**Example:**
```bash
curl -X POST http://localhost:8080/proxy/new \
  -d "name=Application" \
  -d "protocol=tcp" \
  -d "listen_port=8081" \
  -d "forward_port=8080" \
  -d "forward_host=127.0.0.1"
```

---

### Get Proxy Details

**Endpoint:** `GET /proxy/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Proxy ID |

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/proxy/1
```

---

### Edit Proxy

**Endpoint:** `POST /proxy/{id}/edit`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Proxy ID |

**Response:** Redirect to proxy details

**Example:**
```bash
curl -X POST http://localhost:8080/proxy/1/edit \
  -d "name=Application" \
  -d "protocol=tcp" \
  -d "listen_port=9090" \
  -d "forward_host=127.0.0.1" \
  -d "forward_port=3000"
```

---

### Delete Proxy

**Endpoint:** `POST /proxy/{id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Proxy ID |

**Response:** Redirect to proxy list

**Example:**
```bash
curl -X POST http://localhost:8080/proxy/1/delete
```

---

### Change Proxy Status

**Endpoint:** `POST /proxy/status`

Query parameters are `id` and `action`. Supported actions are `activate`,
`deactivate`, and `restart`.

**Response:** Empty `200 OK` response

**Example:**
```bash
curl -X POST "http://localhost:8080/proxy/status?id=1&action=restart"
```

---

## DNS Management

The embedded DNS server is configured under `/dns`. It owns local zones and
records, upstream resolvers, cache settings, rules, and subscriptions. See
[DNS Server](dns-server.md) for the current routes and data model.

The `/domains` namespace described in older releases has been removed and now
permanently redirects to `/dns`. Provider-backed hosted zones are available
under `/dns/providers`; the historical routes below remain unavailable.

### DNS Provider Connections

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/dns/providers` | GET | List hosted DNS provider connections |
| `/dns/providers/new` | GET/POST | Create a Cloudflare, DNSPod, or Name.com connection |
| `/dns/providers/{id}` | GET/DELETE | List hosted zones or delete the connection |
| `/dns/providers/{id}/zones/{zone}` | GET/POST | List, create, update, or delete hosted records |

### Historical provider routes (removed)

**Endpoint:** `GET /domains`

**Response:** `text/html` (HTML page with domain list)

**Example:**
```bash
curl http://localhost:8080/domains
```

---

### Add Domain

**Endpoint:** `POST /domains/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider_id` | integer | Yes | DNS provider ID |
| `domain` | string | Yes | Domain name |

**Response:** Redirect to domain list

**Example:**
```bash
curl -X POST http://localhost:8080/domains/new \
  -d "provider_id=1" \
  -d "domain=example.com"
```

---

### Get Domain Details

**Endpoint:** `GET /domains/{providerId}/{zoneId}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `providerId` | integer | DNS provider ID |
| `zoneId` | string | Zone ID from provider |

**Response:** `text/html` (HTML page with domain details and records)

**Example:**
```bash
curl http://localhost:8080/domains/1/zone123
```

---

### Delete Domain

**Endpoint:** `POST /domains/{providerId}/{zoneId}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `providerId` | integer | DNS provider ID |
| `zoneId` | string | Zone ID from provider |

**Response:** Redirect to domain list

**Example:**
```bash
curl -X POST http://localhost:8080/domains/1/zone123/delete
```

---

### Add DNS Record

**Endpoint:** `POST /domains/{providerId}/{zoneId}/records/add`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `providerId` | integer | DNS provider ID |
| `zoneId` | string | Zone ID from provider |

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `type` | string | Yes | Record type (A, AAAA, CNAME, MX, TXT, etc.) |
| `name` | string | Yes | Record name |
| `value` | string | Yes | Record value |
| `ttl` | integer | No | TTL in seconds (default: 3600) |
| `comment` | string | No | Comment |

**Response:** Redirect to domain details

**Example:**
```bash
curl -X POST http://localhost:8080/domains/1/zone123/records/add \
  -d "type=A" \
  -d "name=www" \
  -d "value=192.168.1.1" \
  -d "ttl=3600"
```

---

### Delete DNS Record

**Endpoint:** `POST /domains/{providerId}/{zoneId}/records/{record_id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `providerId` | integer | DNS provider ID |
| `zoneId` | string | Zone ID from provider |
| `record_id` | string | Record ID |

**Response:** Redirect to domain details

**Example:**
```bash
curl -X POST http://localhost:8080/domains/1/zone123/records/rec_abc123/delete
```

---

### List DNS Providers

**Endpoint:** `GET /domains/providers`

**Response:** `text/html` (HTML page with provider list)

**Example:**
```bash
curl http://localhost:8080/domains/providers
```

---

### Add DNS Provider

**Endpoint:** `POST /domains/providers/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Provider name |
| `type` | string | Yes | Provider type (`cloudflare`, `dnspod`, `namecom`) |
| `config` | string | Yes | JSON-encoded credentials |

**Response:** Redirect to provider list

**Example:**
```bash
# Cloudflare
curl -X POST http://localhost:8080/domains/providers/new \
  -d "name=My Cloudflare" \
  -d "type=cloudflare" \
  -d 'config={"api_key":"your-api-key","email":"user@example.com"}'

# DNSPod
curl -X POST http://localhost:8080/domains/providers/new \
  -d "name=My DNSPod" \
  -d "type=dnspod" \
  -d 'config={"api_token":"your-api-token"}'
```

---

### Delete DNS Provider

**Endpoint:** `POST /domains/providers/{id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Provider ID |

**Response:** Redirect to provider list

**Example:**
```bash
curl -X POST http://localhost:8080/domains/providers/1/delete
```

---

### Test DNS Provider

**Endpoint:** `GET /domains/providers/test`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `type` | string | Yes | Provider type |
| `config` | string | Yes | JSON-encoded credentials |

**Response:** `application/json`

```json
{
  "success": true,
  "message": "Connection successful"
}
```

**Example:**
```bash
curl "http://localhost:8080/domains/providers/test?type=cloudflare&config={...}"
```

---

### Sync Domains from Provider

**Endpoint:** `GET /domains/providers/sync`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider_id` | integer | Yes | Provider ID |

**Response:** Redirect to domain list

**Example:**
```bash
curl "http://localhost:8080/domains/providers/sync?provider_id=1"
```

---

### View Certificates

**Endpoint:** `GET /acme/certificates`

**Response:** `text/html` (HTML page with SSL certificates)

**Example:**
```bash
curl http://localhost:8080/acme/certificates
```

---

## Cron Job Management

### List Cron Jobs

**Endpoint:** `GET /cron`

**Response:** `text/html` (HTML page with cron job list)

**Example:**
```bash
curl http://localhost:8080/cron
```

---

### Create Cron Job

**Endpoint:** `POST /cron/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Job name |
| `type` | string | Yes | Job type (`cron`, `once`) |
| `schedule` | string | Yes | Cron expression or RFC3339 timestamp |
| `command` | string | Yes | Command to execute |
| `working_dir` | string | No | Working directory |
| `timeout` | integer | No | Timeout in seconds |

**Response:** Redirect to cron job list

**Example:**
```bash
# Recurring job
curl -X POST http://localhost:8080/cron/new \
  -d "name=Daily Backup" \
  -d "type=cron" \
  -d "schedule=0 2 * * *" \
  -d "command=/usr/local/bin/backup.sh" \
  -d "timeout=3600"

# One-time job
curl -X POST http://localhost:8080/cron/new \
  -d "name=Scheduled Task" \
  -d "type=once" \
  -d "schedule=2024-01-15T10:00:00Z" \
  -d "command=/usr/local/bin/task.sh"
```

---

### Get Cron Job Details

**Endpoint:** `GET /cron/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Job ID |

**Response:** `text/html` (HTML page with job details and execution history)

**Example:**
```bash
curl http://localhost:8080/cron/1
```

---

### Edit Cron Job

**Endpoint:** `POST /cron/{id}/edit`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Job ID |

**Response:** Redirect to cron job list

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/edit \
  -d "name=Updated Job Name" \
  -d "schedule=0 3 * * *" \
  -d "command=/new/command.sh"
```

---

### Delete Cron Job

**Endpoint:** `POST /cron/{id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Job ID |

**Response:** Redirect to cron job list

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/delete
```

---

### Toggle Cron Job

**Endpoint:** `POST /cron/{id}/toggle`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Job ID |

**Response:** Redirect to cron job list

**Example:**
```bash
# Enable or disable a job
curl -X POST http://localhost:8080/cron/1/toggle
```

---

### Run Cron Job Now

**Endpoint:** `POST /cron/{id}/run`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Job ID |

**Response:** Redirect to cron job details

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/run
```

---

## SSH Management

### View SSH Keys

**Endpoint:** `GET /ssh/keys`

**Response:** `text/html` (HTML page with SSH keys)

**Example:**
```bash
curl http://localhost:8080/ssh/keys
```

---

## ACME / SSL Certificates

### Create and Issue a Certificate

**Endpoint:** `GET/POST /acme/certificates/new`

The GET form accepts an optional `hostname` query parameter. POST requires:

| Field | Required | Description |
| --- | --- | --- |
| `domains` | Yes | One DNS name per line |
| `account_id` | Yes | Existing valid ACME account |
| `challenge_type` | Yes | `http-01` or `dns-01` |
| `dns_target` | DNS-01 only | A local zone or provider-hosted zone reference |

The request creates an ECDSA P-256 key and CSR, then redirects to the
background issuance status page:

```text
GET /acme/issuances/{id}
```

HTTP-01 responses are served by the Web Server before redirects and user
routes. DNS-01 records are created in the explicitly selected local or hosted
zone and cleaned up after validation. A successful issuance is stored in the
existing `certificates` table; it is not automatically bound to a Web Host.

### List Certificates

**Endpoint:** `GET /acme/certificates`

**Response:** `text/html` (HTML page with certificate list)

**Example:**
```bash
curl http://localhost:8080/acme/certificates
```

---

### View Certificate Details

**Endpoint:** `GET /acme/certificates/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Certificate ID |

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/acme/certificates/1
```

---

### Create CSR

**Endpoint:** `POST /acme/create-csr`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `common_name` | string | Yes | Common name (domain) |
| `organization` | string | No | Organization name |
| `country` | string | No | Country code |

**Response:** `text/html`

**Example:**
```bash
curl -X POST http://localhost:8080/acme/create-csr \
  -d "common_name=example.com" \
  -d "organization=My Company" \
  -d "country=US"
```

---

### List ACME Providers

**Endpoint:** `GET /acme/providers`

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/acme/providers
```

---

### Add ACME Provider

**Endpoint:** `POST /acme/providers/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Provider name |
| `directory_url` | string | Yes | ACME directory URL |

**Response:** Redirect to provider list

**Example:**
```bash
curl -X POST http://localhost:8080/acme/providers/new \
  -d "name=Let's Encrypt" \
  -d "directory_url=https://acme-v02.api.letsencrypt.org/directory"
```

---

### Register ACME Account

**Endpoint:** `POST /acme/providers/{id}/register`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Provider ID |

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `email` | string | Yes | Account email |
| `private_key_pem` | string | Yes | Private key (PEM format) |

**Response:** Redirect to provider details

**Example:**
```bash
curl -X POST http://localhost:8080/acme/providers/1/register \
  -d "email=admin@example.com" \
  -d "private_key_pem=-----BEGIN RSA PRIVATE KEY-----..."
```

---

### List ACME Accounts

**Endpoint:** `GET /acme/accounts`

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/acme/accounts
```

---

### List ACME Orders

**Endpoint:** `GET /acme/orders`

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/acme/orders
```

---

### Create ACME Order

**Endpoint:** `POST /acme/orders/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `account_id` | integer | Yes | ACME account ID |
| `domains` | string | Yes | Comma-separated domains |
| `cert_type` | string | Yes | Certificate type |

**Response:** Redirect to order details

**Example:**
```bash
curl -X POST http://localhost:8080/acme/orders/new \
  -d "account_id=1" \
  -d "domains=example.com,www.example.com" \
  -d "cert_type=rsa"
```

---

### Finalize ACME Order

**Endpoint:** `POST /acme/orders/{id}/finalize`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Order ID |

**Response:** Redirect to order details

**Example:**
```bash
curl -X POST http://localhost:8080/acme/orders/1/finalize
```

---

### Complete ACME Challenge

**Endpoint:** `POST /acme/challenges/complete`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `order_id` | integer | Yes | Order ID |
| `challenge_type` | string | Yes | Challenge type |

**Response:** `application/json`

**Example:**
```bash
curl -X POST http://localhost:8080/acme/challenges/complete \
  -d "order_id=1" \
  -d "challenge_type=http-01"
```

---

## ESXi Management

### List ESXi Servers

**Endpoint:** `GET /esxi`

**Response:** `text/html` (HTML page with ESXi server list)

**Example:**
```bash
curl http://localhost:8080/esxi
```

---

### Add ESXi Server

**Endpoint:** `POST /esxi/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Server name |
| `host` | string | Yes | ESXi host address |
| `username` | string | Yes | Username |
| `password` | string | Yes | Password |

**Response:** Redirect to ESXi list

**Example:**
```bash
curl -X POST http://localhost:8080/esxi/new \
  -d "name=esxi-01" \
  -d "host=192.168.1.100" \
  -d "username=root" \
  -d "password=secret"
```

---

### Get ESXi Server Details

**Endpoint:** `GET /esxi/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | ESXi server ID |

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/esxi/1
```

---

### Edit ESXi Server

**Endpoint:** `POST /esxi/{id}/edit`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | ESXi server ID |

**Response:** Redirect to ESXi server details

**Example:**
```bash
curl -X POST http://localhost:8080/esxi/1/edit \
  -d "name=esxi-01-updated" \
  -d "host=192.168.1.100"
```

---

## File Management

### File Browser

**Endpoint:** `GET /files`

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Current directory path |

**Response:** `text/html` (HTML page with file browser)

**Example:**
```bash
curl http://localhost:8080/files
curl "http://localhost:8080/files?path=/workspace/project"
```

---

### Create File

**Endpoint:** `POST /files/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | File path |
| `content` | string | No | File content |

**Response:** Redirect to file browser

**Example:**
```bash
curl -X POST http://localhost:8080/files/new \
  -d "path=/workspace/test.txt" \
  -d "content=Hello World"
```

---

### Create Folder

**Endpoint:** `POST /files/new-folder`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | Folder path |

**Response:** Redirect to file browser

**Example:**
```bash
curl -X POST http://localhost:8080/files/new-folder \
  -d "path=/workspace/new-folder"
```

---

### Upload File

**Endpoint:** `POST /files/upload`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `file` | file | Yes | File to upload |
| `path` | string | Yes | Destination directory |

**Response:** Redirect to file browser

**Example:**
```bash
curl -X POST http://localhost:8080/files/upload \
  -F "file=@/path/to/local/file.txt" \
  -F "path=/workspace"
```

---

### Download File

**Endpoint:** `GET /files/download`

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | Yes | File path |

**Response:** File download

**Example:**
```bash
curl "http://localhost:8080/files/download?path=/workspace/test.txt"
```

---

## Cron Jobs

### List Cron Jobs

**Endpoint:** `GET /cron`

**Response:** `text/html` (HTML page with cron job list)

**Example:**
```bash
curl http://localhost:8080/cron
```

---

### Create Cron Job

**Endpoint:** `POST /cron/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Job name |
| `type` | string | Yes | Job type (`cron`, `once`) |
| `schedule` | string | Yes | Cron expression or time |
| `command` | string | Yes | Command to execute |
| `working_dir` | string | No | Working directory |
| `timeout` | integer | No | Timeout in seconds |

**Response:** Redirect to cron list

**Example:**
```bash
curl -X POST http://localhost:8080/cron/new \
  -d "name=backup" \
  -d "type=cron" \
  -d "schedule=0 2 * * *" \
  -d "command=/usr/local/bin/backup.sh" \
  -d "working_dir=/opt/backups" \
  -d "timeout=3600"
```

---

### Get Cron Job Details

**Endpoint:** `GET /cron/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Cron job ID |

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/cron/1
```

---

### Edit Cron Job

**Endpoint:** `POST /cron/{id}/edit`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Cron job ID |

**Response:** Redirect to cron job details

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/edit \
  -d "name=updated-backup" \
  -d "schedule=0 3 * * *"
```

---

### Delete Cron Job

**Endpoint:** `POST /cron/{id}/delete`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Cron job ID |

**Response:** Redirect to cron list

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/delete
```

---

### Toggle Cron Job

**Endpoint:** `POST /cron/{id}/toggle`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Cron job ID |

**Response:** Redirect to cron job details

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/toggle
```

---

### Run Cron Job Now

**Endpoint:** `POST /cron/{id}/run`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Cron job ID |

**Response:** Redirect to cron job details

**Example:**
```bash
curl -X POST http://localhost:8080/cron/1/run
```

---

## SSH Management

### List SSH Servers

**Endpoint:** `GET /ssh/servers`

**Response:** `text/html` (HTML page with SSH server list)

**Example:**
```bash
curl http://localhost:8080/ssh/servers
```

---

### Add SSH Server

**Endpoint:** `POST /ssh/servers/new`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Server name |
| `host` | string | Yes | SSH host address |
| `port` | integer | No | SSH port (default: 22) |
| `username` | string | Yes | SSH username |
| `password` | string | No | Password or private key |

**Response:** Redirect to SSH list

**Example:**
```bash
curl -X POST http://localhost:8080/ssh/servers/new \
  -d "name=my-server" \
  -d "host=192.168.1.50" \
  -d "port=22" \
  -d "username=admin" \
  -d "password=secret"
```

---

### Get SSH Server Details

**Endpoint:** `GET /ssh/servers/{id}`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | SSH server ID |

**Response:** `text/html`

**Example:**
```bash
curl http://localhost:8080/ssh/servers/1
```

---

### Edit SSH Server

**Endpoint:** `POST /ssh/servers/{id}/edit`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | SSH server ID |

**Response:** Redirect to SSH server details

**Example:**
```bash
curl -X POST http://localhost:8080/ssh/servers/1/edit \
  -d "name=updated-server" \
  -d "host=192.168.1.50"
```

---

### SSH Terminal

**Page:** `GET /ssh/servers/{id}/terminal`

**WebSocket:** `GET /ssh/servers/{id}/terminal/ws`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | SSH server ID |

**Response:** `WebSocket` connection for interactive terminal

**Example:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ssh/servers/1/terminal/ws');
```

---

### Test SSH Connection

**Endpoint:** `POST /ssh/servers/test`

**Form Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `host` | string | Yes | SSH host address |
| `port` | integer | No | SSH port (default: 22) |
| `username` | string | Yes | SSH username |
| `password` | string | No | Password or private key |

**Response:** `application/json`

**Example:**
```bash
curl -X POST http://localhost:8080/ssh/servers/test \
  -d "host=192.168.1.50" \
  -d "port=22" \
  -d "username=admin" \
  -d "password=secret"
```

---

### Get SSH Stats

**Endpoint:** `GET /ssh/servers/{id}/stats`

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | SSH server ID |

**Response:** `application/json`

**Example:**
```bash
curl http://localhost:8080/ssh/servers/1/stats
```

---

## Settings

### View Settings

**Endpoint:** `GET /settings`

**Response:** `text/html` (HTML page with application settings)

**Example:**
```bash
curl http://localhost:8080/settings
```

---

## Error Handling

### HTTP Status Codes

| Code | Description |
|------|-------------|
| `200 OK` | Request successful |
| `302 Found` | Redirect (after form submission) |
| `400 Bad Request` | Invalid request parameters |
| `404 Not Found` | Resource not found |
| `500 Internal Server Error` | Server error |

### Error Response Format

**HTML Errors:**
Rendered using `error.html` template with error message.

**JSON Errors:**
```json
{
  "error": "Error message description"
}
```

**SSE Errors:**
```
event: error
data: Error message description
```

---

## Rate Limiting

Currently, the API does not implement rate limiting. For production use, consider:
- Implementing rate limiting in a reverse proxy (nginx, traefik)
- Adding middleware to the application
- Using API gateway solutions

---

## CORS

The application sets `Access-Control-Allow-Origin: *` for SSE endpoints to allow cross-origin requests.

For other endpoints, CORS headers are not set by default. Configure in your reverse proxy if needed.

---

## Related Documentation

- [Architecture Documentation](architecture.md) - System architecture overview
- [Configuration Reference](configuration.md) - Configuration options
- [Installation Guide](installation.md) - Installation instructions
