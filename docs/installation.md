# Installation Guide

This guide provides detailed instructions for installing and configuring OhMyServer on your system.

## Prerequisites

### Required Software

1. **Go 1.25.0 or later**
   
   Check your Go version:
   ```bash
   go version
   ```
   
   If you need to install or upgrade Go:
   - Download from [https://go.dev/dl/](https://go.dev/dl/)
   - Or use a version manager like `gvm`:
     ```bash
     curl -sSL https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer | bash
     gvm install go1.25
     gvm use go1.25
     ```

2. **Docker**
   
   OhMyServer requires Docker to manage containers. Install Docker Desktop or Docker Engine:
   
   **macOS:**
   ```bash
   # Using Homebrew
   brew install --cask docker
   ```
   
   **Linux:**
   ```bash
   # Ubuntu/Debian
   curl -fsSL https://get.docker.com | sh
   sudo usermod -aG docker $USER
   
   # Or using package manager
   sudo apt-get update
   sudo apt-get install docker.io docker-compose
   ```
   
   **Windows:**
   - Download Docker Desktop from [https://www.docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop)

3. **Git** (for project deployments)
   ```bash
   # macOS
   xcode-select --install
   
   # Linux
   sudo apt-get install git
   
   # Windows
   # Download from https://git-scm.com/download/win
   ```

## Installation Steps

### Step 1: Clone the Repository

```bash
git clone https://github.com/lsongdev/ohmyserver.git
cd ohmyserver
```

### Step 2: Download Dependencies

```bash
go mod download
```

This will download all required Go modules as defined in `go.mod`.

### Step 3: Build the Application

```bash
go build -o ohmyserver
```

This creates an executable named `ohmyserver` in the current directory.

**For production optimization:**
```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o ohmyserver
```

### Step 4: Create Configuration Directory

```bash
mkdir -p ~/.ohmyserver
```

### Step 5: Create Configuration File (Optional)

Copy the example configuration:

```bash
cp config.yaml.example ~/.ohmyserver/config.yaml
```

Edit the configuration file if needed:

```bash
nano ~/.ohmyserver/config.yaml
```

**Default configuration:**
```yaml
database:
  driver: sqlite3
  path: ~/.ohmyserver/ohmyserver.db

collection_interval_seconds: 5

docker:
  registry_mirror: ""
```

**Note:** The configuration file is optional. If not provided, OhMyServer will use default values and create the database at `~/.ohmyserver/ohmyserver.db`.

### Step 6: Verify Docker Access

Ensure Docker is running and accessible:

```bash
docker ps
```

If you get a permission error on Linux:

```bash
sudo usermod -aG docker $USER
newgrp docker
docker ps
```

### Step 7: Run OhMyServer

**Development mode:**
```bash
./ohmyserver
```

**Production mode (as a background service):**

Using `nohup`:
```bash
nohup ./ohmyserver > ohmyserver.log 2>&1 &
```

Using `systemd` (Linux):

Create a service file `/etc/systemd/system/ohmyserver.service`:

```ini
[Unit]
Description=OhMyServer Management Platform
After=network.target docker.service

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/ohmyserver
ExecStart=/path/to/ohmyserver/ohmyserver
Restart=on-failure
RestartSec=10
Environment="PATH=/usr/bin:/usr/local/bin"
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable ohmyserver
sudo systemctl start ohmyserver
sudo systemctl status ohmyserver
```

### Step 8: Access the Web Interface

Open your browser and navigate to:

```
http://localhost:8080
```

You should see the OhMyServer dashboard.

## Verification

### Check Application Status

```bash
# Check if the process is running
ps aux | grep ohmyserver

# Check if port 8080 is listening
lsof -i :8080
# or
netstat -tlnp | grep 8080

# Check logs (if running in foreground)
# The application will output startup messages
```

### Test API Endpoints

```bash
# Test system stats endpoint
curl http://localhost:8080/monitor/current

# Test Docker connectivity
curl http://localhost:8080/docker/containers
```

### Verify Database Creation

```bash
ls -la ~/.ohmyserver/
# Should show:
# - config.yaml (if created)
# - ohmyserver.db (auto-created on first run)
```

## Platform-Specific Instructions

### macOS

**Apple Silicon (M1/M2) Considerations:**

Some Docker images may not be available for ARM64 architecture. Use multi-arch images or build locally:

```bash
# Build images for current architecture
docker build --platform linux/amd64 -t myimage .
```

**Docker Desktop Settings:**
1. Open Docker Desktop
2. Go to Settings → Resources
3. Allocate sufficient memory (recommended: 4GB+)
4. Enable "Virtualization framework"

### Linux

**Firewall Configuration:**

If you have a firewall enabled, allow port 8080:

```bash
# Ubuntu/Debian (UFW)
sudo ufw allow 8080/tcp

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# iptables
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
```

**SELinux (if enabled):**

```bash
# Check SELinux status
getenforce

# If Enforcing, set context for the binary
chcon -t bin_t /path/to/ohmyserver
```

### Windows (WSL2)

**Using WSL2:**

1. Install WSL2 and Ubuntu from Microsoft Store
2. Install Docker Desktop with WSL2 backend
3. Follow Linux installation steps inside WSL2

**Access from Windows:**

The application will be accessible at:
```
http://localhost:8080
```

### Raspberry Pi

**Requirements:**
- Raspberry Pi 4 or later (recommended)
- Raspberry Pi OS 64-bit
- At least 2GB RAM

**Installation:**

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install dependencies
sudo apt install -y golang-go docker.io git

# Add user to docker group
sudo usermod -aG docker $USER

# Follow standard installation steps above
```

**Performance Tips:**
- Use a fast SD card or boot from USB
- Limit concurrent deployments
- Increase swap space if needed:
  ```bash
  sudo dphys-swapfile swapoff
  sudo nano /etc/dphys-swapfile  # Set CONF_SWAPSIZE=2048
  sudo dphys-swapfile setup
  sudo dphys-swapfile swapon
  ```

## Docker Registry Mirror Configuration

If you're in a region with slow Docker Hub access, configure a registry mirror:

### China Users

**Option 1: Update config.yaml**

```yaml
docker:
  registry_mirror: "https://hub-mirror.c.163.com"
```

**Option 2: Configure Docker daemon**

Edit `/etc/docker/daemon.json`:

```json
{
  "registry-mirrors": [
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
```

Restart Docker:

```bash
sudo systemctl restart docker
```

**Note:** Many public mirrors have been discontinued. Consider:
- Using Docker Desktop with WSL2
- Setting up a private registry mirror
- Pulling images manually during off-peak hours

### Other Regions

**Cloud Provider Mirrors:**

- **AWS:** Use Amazon ECR proxy
- **GCP:** Use Artifact Registry cache
- **Azure:** Use ACR task with upstream

## Troubleshooting

### Application Won't Start

**Issue:** `bind: address already in use`

```bash
# Find process using port 8080
lsof -i :8080

# Kill the process
kill -9 <PID>

# Or change the port in main.go
```

**Issue:** `database is locked`

```bash
# Check for other instances
ps aux | grep ohmyserver

# Remove lock file
rm -f ~/.ohmyserver/ohmyserver.db-shm
rm -f ~/.ohmyserver/ohmyserver.db-wal
```

**Issue:** `permission denied`

```bash
# Make executable
chmod +x ohmyserver

# Fix database permissions
chmod 600 ~/.ohmyserver/ohmyserver.db
```

### Docker Connection Issues

**Issue:** `Cannot connect to the Docker daemon`

```bash
# Check if Docker is running
systemctl status docker

# Start Docker
sudo systemctl start docker

# Check socket permissions
ls -la /var/run/docker.sock

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

**Issue:** `context deadline exceeded`

```bash
# Increase Docker operation timeout
# Edit server/docker.go and increase timeout values
```

### Database Issues

**Issue:** `no such table`

```bash
# Delete and recreate database
rm ~/.ohmyserver/ohmyserver.db
./ohmyserver  # Will auto-create schema
```

**Issue:** `database disk image is malformed`

```bash
# Backup and recreate
cp ~/.ohmyserver/ohmyserver.db ~/.ohmyserver/ohmyserver.db.backup
rm ~/.ohmyserver/ohmyserver.db
./ohmyserver
```

### Build Issues

**Issue:** `package not found`

```bash
# Clear Go module cache
go clean -modcache

# Re-download dependencies
go mod download
go mod tidy
```

**Issue:** `CGO_ENABLED` errors

```bash
# Install GCC compiler
# macOS
xcode-select --install

# Linux
sudo apt install build-essential

# Build with CGO
CGO_ENABLED=1 go build -o ohmyserver
```

## Uninstallation

### Stop the Application

```bash
# If running in foreground, press Ctrl+C

# If running as service
sudo systemctl stop ohmyserver
sudo systemctl disable ohmyserver
```

### Remove Files

```bash
# Remove application
rm -rf /path/to/ohmyserver

# Remove configuration and database
rm -rf ~/.ohmyserver

# Remove systemd service (if installed)
sudo rm /etc/systemd/system/ohmyserver.service
sudo systemctl daemon-reload
```

## Next Steps

After successful installation:

1. **Configure DNS Providers** - Set up Cloudflare, DNSPod, or Name.com integration
2. **Create Your First Project** - Deploy a Git repository
3. **Set Up Monitoring** - Configure alerting and metrics collection
4. **Schedule Cron Jobs** - Automate recurring tasks
5. **Configure Reverse Proxy** - Set up domain-based routing
6. **Manage SSL Certificates** - Set up ACME accounts and issue certificates
7. **Add SSH Servers** - Configure remote server access with terminal
8. **Manage ESXi Hosts** - Add VMware ESXi hosts for VM management
9. **Use File Manager** - Browse and manage files via web interface

For more information, see:
- [Configuration Guide](configuration.md)
- [API Reference](api-reference.md)
- [Architecture Documentation](architecture.md)

## Support

If you encounter issues not covered in this guide:

1. Check the [GitHub Issues](https://github.com/lsongdev/ohmyserver/issues)
2. Review application logs for error messages
3. Verify all prerequisites are properly installed
4. Ensure Docker is running and accessible
