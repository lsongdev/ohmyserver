# Configuration Reference

This document provides a comprehensive reference for configuring OhMyServer.

## Configuration File Location

The configuration file is located at:

```
~/.ohmyserver/config.yaml
```

The configuration file is **optional**. If it doesn't exist, OhMyServer will use the following default values:

| Option | Default Value |
|--------|--------------|
| `database.driver` | `sqlite3` |
| `database.path` | `~/.ohmyserver/ohmyserver.db` |
| `collection_interval_seconds` | `5` |
| `docker.registry_mirror` | `https://docker.mirrors.ustc.edu.cn` |

The database will be automatically created at `~/.ohmyserver/ohmyserver.db` on first run.

## Complete Configuration Example

```yaml
# OhMyServer Configuration File
# Location: ~/.ohmyserver/config.yaml

# Database configuration
database:
  driver: sqlite3
  path: /home/user/.ohmyserver/ohmyserver.db

# System monitoring collection interval (in seconds)
collection_interval_seconds: 5

# Docker configuration
docker:
  # Registry mirror for faster image pulls
  # Leave empty to use official Docker Hub
  registry_mirror: ""
```

## Configuration Options

### Database Configuration

The `database` section configures the database connection.

#### `database.driver`

- **Type:** string
- **Default:** `sqlite3`
- **Required:** No
- **Options:** `sqlite3`

Currently, only SQLite3 is supported. The driver uses the `mattn/go-sqlite3` package.

**Example:**
```yaml
database:
  driver: sqlite3
```

#### `database.path`

- **Type:** string
- **Default:** `~/.ohmyserver/ohmyserver.db`
- **Required:** No

The file system path to the SQLite database file.

**Examples:**
```yaml
# Default location
database:
  path: ~/.ohmyserver/ohmyserver.db

# Custom location
database:
  path: /var/lib/ohmyserver/data.db

# Absolute path
database:
  path: /opt/ohmyserver/database/ohmyserver.db
```

**Notes:**
- The directory must exist and be writable by the OhMyServer process
- SQLite database files should have restricted permissions (600)
- The application will create the database file if it doesn't exist

### Collection Interval

#### `collection_interval_seconds`

- **Type:** integer
- **Default:** `5`
- **Required:** No
- **Minimum:** `1`

The interval (in seconds) at which system metrics (CPU, memory) are collected and stored in the database.

**Examples:**
```yaml
# Collect every 5 seconds (default)
collection_interval_seconds: 5

# Collect every 10 seconds
collection_interval_seconds: 10

# Collect every minute
collection_interval_seconds: 60
```

**Considerations:**
- **Lower values** (1-5): More granular data, higher database write frequency
- **Higher values** (30-60): Less storage overhead, coarser metrics
- **Recommended:** 5-10 seconds for most use cases

**Impact on Storage:**
At 5-second intervals:
- ~12 records per minute
- ~720 records per hour
- ~17,280 records per day

At 60-second intervals:
- ~60 records per hour
- ~1,440 records per day

### Docker Configuration

The `docker` section configures Docker-related settings.

#### `docker.registry_mirror`

- **Type:** string
- **Default:** `https://docker.mirrors.ustc.edu.cn`
- **Required:** No

URL of a Docker registry mirror for faster image pulls.

**Examples:**
```yaml
# Use default Docker Hub
docker:
  registry_mirror: ""

# Use NetEase mirror (China)
docker:
  registry_mirror: "https://hub-mirror.c.163.com"

# Use Tencent Cloud mirror (China)
docker:
  registry_mirror: "https://mirror.ccs.tencentyun.com"

# Use Aliyun mirror (China, requires registration)
docker:
  registry_mirror: "https://<your-id>.mirror.aliyuncs.com"
```

**Notes:**
- Registry mirrors are particularly useful in regions with slow Docker Hub access
- Many public mirrors have been discontinued; consider setting up a private mirror
- The mirror is used for image pulls only; pushes go directly to the registry
- If the mirror is unavailable, the application falls back to Docker Hub

## Environment Variables

Currently, OhMyServer does not support environment variable configuration. All settings must be specified in the YAML configuration file.

**Future Enhancement:** Environment variable overrides may be added in a future release.

## Path Expansion

The following path expansions are supported:

- `~` - Expands to the current user's home directory
- `$HOME` - Can be used in shell commands but not in YAML

**Examples:**
```yaml
# These are equivalent:
database:
  path: ~/.ohmyserver/ohmyserver.db

database:
  path: /home/username/.ohmyserver/ohmyserver.db
```

## Configuration Validation

OhMyServer performs basic validation on startup:

1. **Database path:** Checks if the directory is writable
2. **Collection interval:** Ensures value is positive
3. **Registry mirror:** Validates URL format (if provided)

**Invalid Configuration Example:**
```yaml
# This will use defaults:
collection_interval_seconds: -5  # Ignored, uses default of 5

# This will use defaults:
database:
  path: ""  # Ignored, uses default path
```

## Default Values Summary

| Option | Default Value |
|--------|--------------|
| `database.driver` | `sqlite3` |
| `database.path` | `~/.ohmyserver/ohmyserver.db` |
| `collection_interval_seconds` | `5` |
| `docker.registry_mirror` | `https://docker.mirrors.ustc.edu.cn` |

**Note:** The `docker.registry_mirror` default is set to a China-based mirror. Users outside China may want to set this to an empty string (`""`) to use Docker Hub directly, or configure their own regional mirror.

## Configuration Management Best Practices

### Backup Configuration

Regularly backup your configuration file:

```bash
# Create backup
cp ~/.ohmyserver/config.yaml ~/.ohmyserver/config.yaml.backup.$(date +%Y%m%d)

# Restore from backup
cp ~/.ohmyserver/config.yaml.backup.20240101 ~/.ohmyserver/config.yaml
```

### Version Control

Store configuration in version control (excluding sensitive data):

```bash
# Add to git
git add config.yaml.example

# Never commit actual config with credentials
echo "config.yaml" >> .gitignore
```

### Multiple Environments

Maintain separate configurations for different environments:

```bash
# Development
cp config.dev.yaml ~/.ohmyserver/config.yaml

# Production
cp config.prod.yaml ~/.ohmyserver/config.yaml
```

**Example config.dev.yaml:**
```yaml
database:
  path: ~/.ohmyserver/dev.db

collection_interval_seconds: 10

docker:
  registry_mirror: ""
```

**Example config.prod.yaml:**
```yaml
database:
  path: /var/lib/ohmyserver/prod.db

collection_interval_seconds: 5

docker:
  registry_mirror: "https://mirror.example.com"
```

### Permissions

Set appropriate file permissions:

```bash
# Configuration file (readable only by owner)
chmod 600 ~/.ohmyserver/config.yaml

# Database file (readable and writable only by owner)
chmod 600 ~/.ohmyserver/ohmyserver.db

# Directory (accessible only by owner)
chmod 700 ~/.ohmyserver
```

## Troubleshooting Configuration Issues

### Configuration Not Loaded

**Symptom:** Application uses default values despite config file existing

**Solutions:**
1. Verify file location: `ls -la ~/.ohmyserver/config.yaml`
2. Check file permissions: `chmod 644 ~/.ohmyserver/config.yaml`
3. Validate YAML syntax: `python3 -c "import yaml; yaml.safe_load(open('~/.ohmyserver/config.yaml'))"`
4. Check application logs for parsing errors

### Invalid YAML Syntax

**Symptom:** Application fails to start with YAML parsing error

**Common Issues:**
```yaml
# Wrong: Missing space after colon
database:
  driver:sqlite3  # Should be: driver: sqlite3

# Wrong: Inconsistent indentation
database:
  driver: sqlite3
    path: ~/.ohmyserver/db.db  # Should be aligned with driver

# Wrong: Using tabs instead of spaces
database:
	driver: sqlite3  # Should use spaces, not tabs
```

### Database Path Issues

**Symptom:** "permission denied" or "no such file or directory"

**Solutions:**
```bash
# Create directory if it doesn't exist
mkdir -p ~/.ohmyserver

# Set correct permissions
chmod 700 ~/.ohmyserver

# Verify ownership
chown $USER:$USER ~/.ohmyserver
```

### Registry Mirror Not Working

**Symptom:** Image pulls fail or fall back to Docker Hub

**Solutions:**
1. Test mirror URL manually:
   ```bash
   curl -I https://hub-mirror.c.163.com
   ```
2. Configure Docker daemon mirror as well:
   ```json
   // /etc/docker/daemon.json
   {
     "registry-mirrors": ["https://hub-mirror.c.163.com"]
   }
   ```
3. Restart Docker:
   ```bash
   sudo systemctl restart docker
   ```

## Migration Guide

### From Previous Versions

**Version 0.1.x to Current:**

No breaking changes. Existing configuration files are compatible.

**Future Versions:**

Watch the [CHANGELOG](../CHANGELOG.md) for configuration changes in future releases.

## Advanced Configuration

### Custom SQLite Settings

For advanced users, SQLite PRAGMA statements can be configured by modifying the database connection code in `server/database.go`:

```go
// Enable WAL mode for better concurrency
db.Exec("PRAGMA journal_mode=WAL")

// Set synchronous mode
db.Exec("PRAGMA synchronous=NORMAL")

// Enable foreign keys
db.Exec("PRAGMA foreign_keys=ON")
```

### Memory-Mapped I/O

For improved performance on systems with sufficient RAM:

```go
// In server/database.go, after opening database
db.Exec("PRAGMA mmap_size=268435456")  // 256MB
```

### Connection Pooling

SQLite connection pooling is handled automatically by the `database/sql` package. For high-concurrency scenarios, you can adjust pool settings:

```go
db.SetMaxOpenConns(1)  // SQLite only supports one writer
db.SetMaxIdleConns(2)
db.SetConnMaxLifetime(time.Hour)
```

## Security Considerations

### Sensitive Data

**Do not store sensitive data in the configuration file:**

```yaml
# ❌ Bad: Storing credentials in config
docker:
  username: admin
  password: secret123

# ✅ Good: Use Docker credential helpers or environment variables
```

For DNS provider credentials, use the web interface to store them securely in the database.

### File Permissions

Always restrict access to configuration and database files:

```bash
# Configuration file
chmod 600 ~/.ohmyserver/config.yaml

# Database file
chmod 600 ~/.ohmyserver/ohmyserver.db

# Directory
chmod 700 ~/.ohmyserver
```

### Network Exposure

The application binds to all interfaces (`:8080`) by default. For local-only access:

**Modify `main.go`:**
```go
// Change from:
log.Fatal(http.ListenAndServe(":8080", router))

// To:
log.Fatal(http.ListenAndServe("127.0.0.1:8080", router))
```

## Related Documentation

- [Installation Guide](installation.md) - Step-by-step installation instructions
- [Architecture Documentation](architecture.md) - System architecture overview
- [API Reference](api-reference.md) - REST API documentation

## Support

For configuration-related issues:

1. Check application logs for error messages
2. Verify YAML syntax
3. Ensure file permissions are correct
4. Review this documentation for guidance
5. Open an issue on GitHub if problems persist
