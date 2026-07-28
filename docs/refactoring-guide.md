# 代码重构指南

本文档提供具体的重构步骤和代码示例，用于改进 OhMyServer 的代码结构和质量。

---

## 1. 数据库文件拆分

### 目标

将 `server/database.go`（1860+ 行）拆分为按模块组织的小文件。

### 拆分方案

保持现有文件的模块不变，将以下内容从 `database.go` 移出：

| 模块 | 移到文件 | 包含内容 |
|------|---------|---------|
| Project/Deployment | 已有 `projects.go` | 模型 + CRUD |
| SystemStats | 已有 `monitor.go` | 模型 + CRUD |
| DNS | 已有 `domains.go` | 模型 + CRUD |
| TCPListener | 已有 `listeners.go` | 模型 + CRUD |
| CronJob | 已有 `cronjob.go` | 模型 + CRUD |

### `database.go` 保留内容

```go
// server/database.go - 仅保留通用部分

package server

import (
    "database/sql"
    "time"
    _ "github.com/mattn/go-sqlite3"
)

// parseTime 通用时间解析函数
func parseTime(timeStr string) time.Time { ... }

// ptrTime 通用时间指针函数
func ptrTime(t time.Time) *time.Time { ... }

// initSchema 仅负责调用各模块的 init 方法
func (s *Server) initSchema() error {
    if err := s.initSettingsSchema(); err != nil { ... }
    if err := s.initMonitorSchema(); err != nil { ... }
    // ... 仅调用，不重复
}
```

---

## 2. 路由注册拆分

### 目标

将 `main.go` 的 220 行路由注册分散到各模块。

### 实现模式

每个模块添加 `RegisterRoutes` 方法：

```go
// server/ipmi.go
func (s *Server) RegisterIPMIRoutes(mux *http.ServeMux, auth func(http.HandlerFunc) http.HandlerFunc) {
    mux.HandleFunc("/ipmi", auth(s.IPMIView))
    mux.HandleFunc("/ipmi/new", auth(s.IPMINewHandler))
    mux.HandleFunc("/ipmi/{id}", auth(s.IPMIDetailHandler))
    mux.HandleFunc("/ipmi/{id}/edit", auth(s.IPMIEditHandler))
    mux.HandleFunc("/ipmi/{id}/power", auth(s.IPMIPowerControlHandler))
    mux.HandleFunc("/ipmi/{id}/test", auth(s.IPMITestConnectionHandler))
}
```

### 简化后的 main.go

```go
func main() {
    config, err := config.LoadConfig()
    if err != nil { panic(err) }

    server, err := server.NewServer(config)
    if err != nil { panic(err) }
    
    router := http.NewServeMux()
    
    // 不经过认证的路由
    router.HandleFunc("/login", server.LoginView)
    router.HandleFunc("/logout", server.LogoutHandler)
    router.Handle("/assets/", http.StripPrefix("/assets/", 
        http.FileServer(http.Dir("web/assets"))))
    
    auth := server.AuthMiddleware
    
    // 各模块注册自己的路由
    server.RegisterMonitorRoutes(router, auth)
    server.RegisterDockerRoutes(router, auth)
    server.RegisterProjectRoutes(router, auth)
    server.RegisterDomainRoutes(router, auth)
    server.RegisterListenerRoutes(router, auth)
    server.RegisterSSHRoutes(router, auth)
    server.RegisterESXiRoutes(router, auth)
    server.RegisterIPMIRoutes(router, auth)
    server.RegisterIDRACRoutes(router, auth)
    server.RegisterOpenWrtRoutes(router, auth)
    server.RegisterClashRoutes(router, auth)
    server.RegisterACMERoutes(router, auth)
    server.RegisterCronRoutes(router, auth)
    server.RegisterWorkerRoutes(router, auth)
    server.RegisterLLMRoutes(router, auth)
    server.RegisterSpeedtestRoutes(router, auth)
    server.RegisterYeelightRoutes(router, auth)
    server.RegisterDiskRoutes(router, auth)
    server.RegisterFileRoutes(router, auth)
    server.RegisterSettingsRoutes(router, auth)
    server.RegisterUptimeRoutes(router, auth)
    
    // 页面路由
    router.HandleFunc("/", auth(server.IndexView))
    router.HandleFunc("/apps", auth(server.AppsView))
    router.HandleFunc("/apps/{id}", auth(server.AppDetailView))
    router.HandleFunc("/about", auth(server.AboutView))
    
    log.Fatal(http.ListenAndServe(config.Listen, router))
}
```

---

## 3. 安全增强

### 3.1 Cookie 安全属性

```go
// server/auth.go - 修改后的 SetCookie
http.SetCookie(w, &http.Cookie{
    Name:     authCookieName,
    Value:    hashKey(s.config.AccessKey),
    Path:     "/",
    MaxAge:   authCookieMaxAge,
    HttpOnly: true,
    Secure:   true,       // 仅 HTTPS
    SameSite: http.SameSiteStrictMode, // 防 CSRF
})
```

### 3.2 改进认证哈希

```go
import "golang.org/x/crypto/bcrypt"

func hashKey(key string) (string, error) {
    hashed, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
    if err != nil {
        return "", err
    }
    return string(hashed), nil
}

func verifyKey(key, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(key))
    return err == nil
}
```

### 3.3 CSRF 防护

```go
// server/csrf.go
package server

import (
    "crypto/rand"
    "encoding/base64"
    "net/http"
)

const csrfCookieName = "csrf_token"
const csrfFormFieldName = "_csrf"

func GenerateCSRFToken() (string, error) {
    b := make([]byte, 32)
    _, err := rand.Read(b)
    if err != nil { return "", err }
    return base64.URLEncoding.EncodeToString(b), nil
}

func CSRFMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 仅对修改类请求验证
        if r.Method == http.MethodGet || r.Method == http.MethodHead {
            next(w, r)
            return
        }
        
        token := r.FormValue(csrfFormFieldName)
        cookie, err := r.Cookie(csrfCookieName)
        if err != nil || token != cookie.Value {
            http.Error(w, "CSRF token mismatch", http.StatusForbidden)
            return
        }
        next(w, r)
    }
}
```

---

## 4. 并发优化

### 4.1 启用 SQLite WAL 模式

```go
// server/server.go - NewServer 中打开数据库后
db, err := sql.Open("sqlite3", cfg.Database.Path+"?_journal_mode=WAL")

// 或初始化后执行
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
db.Exec("PRAGMA foreign_keys=ON")
db.Exec("PRAGMA busy_timeout=5000")
```

### 4.2 模块级锁

```go
type Server struct {
    mu         sync.RWMutex  // 全局锁（保留用于跨模块操作）
    
    // 模块级锁
    settingsMu  sync.RWMutex
    monitorMu   sync.RWMutex
    ipmiMu      sync.RWMutex
    idracMu     sync.RWMutex
    dockerMu    sync.RWMutex
    projectMu   sync.RWMutex
    cronMu      sync.RWMutex
    sshMu       sync.RWMutex
    esxiMu      sync.RWMutex
    // ... 其他模块
}
```

---

## 5. 日志系统

### 使用标准库 slog（Go 1.21+）

```go
// server/logger.go
package server

import (
    "log/slog"
    "os"
)

var Logger *slog.Logger

func InitLogger(level string) {
    var l slog.Level
    switch level {
    case "debug": l = slog.LevelDebug
    case "info": l = slog.LevelInfo
    case "warn": l = slog.LevelWarn
    case "error": l = slog.LevelError
    default: l = slog.LevelInfo
    }
    
    opts := &slog.HandlerOptions{
        Level: l,
        AddSource: true,
    }
    Logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
}

// 使用示例
Logger.Info("server started", "port", config.Listen)
Logger.Error("database error", "error", err, "query", sql)
Logger.Debug("request processed", "method", r.Method, "path", r.URL.Path)
```

### 请求日志中间件

```go
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next(w, r)
        Logger.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "duration", time.Since(start),
        )
    }
}
```

---

## 6. Docker 容器化

### Dockerfile

```dockerfile
# 构建阶段
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o ohmyserver

# 运行阶段
FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite
WORKDIR /app
COPY --from=builder /app/ohmyserver .
COPY --from=builder /app/web ./web
COPY config.yaml.example ./config.yaml.example

EXPOSE 9000
VOLUME ["/data"]

ENV OMS_CONFIG_PATH=/data/config.yaml
ENV OMS_DB_PATH=/data/ohmyserver.db

ENTRYPOINT ["./ohmyserver"]
```

### docker-compose.yml

```yaml
version: "3.8"
services:
  ohmyserver:
    build: .
    container_name: ohmyserver
    ports:
      - "9000:9000"
    volumes:
      - ./data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    restart: unless-stopped
    environment:
      - TZ=Asia/Shanghai
```

---

## 7. Makefile

```makefile
.PHONY: build run test lint clean docker-build docker-run

BINARY=ohmyserver
GO=go
GOFLAGS=-ldflags="-s -w"

build:
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -o $(BINARY)

run: build
	./$(BINARY)

test:
	$(GO) test -v -cover ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)
	go clean -cache

docker-build:
	docker build -t ohmyserver:latest .

docker-run:
	docker-compose up -d
```

---

## 8. 健康检查端点

```go
// server/health.go
package server

import (
    "encoding/json"
    "net/http"
)

type HealthStatus struct {
    Status   string            `json:"status"`
    Checks   map[string]string `json:"checks"`
    Uptime   string            `json:"uptime"`
    Version  string            `json:"version"`
}

var startTime = time.Now()

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
    status := HealthStatus{
        Status:  "ok",
        Checks:  make(map[string]string),
        Uptime:  time.Since(startTime).String(),
        Version: "dev",
    }
    
    // 检查数据库
    if err := s.db.Ping(); err != nil {
        status.Checks["database"] = "error: " + err.Error()
        status.Status = "degraded"
    } else {
        status.Checks["database"] = "ok"
    }
    
    // 检查 Docker
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if _, err := s.docker.client.Ping(ctx); err != nil {
        status.Checks["docker"] = "error: " + err.Error()
        status.Status = "degraded"
    } else {
        status.Checks["docker"] = "ok"
    }
    
    w.Header().Set("Content-Type", "application/json")
    statusCode := http.StatusOK
    if status.Status != "ok" {
        statusCode = http.StatusServiceUnavailable
    }
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(status)
}
```
