# 项目改进建议

本文档基于项目现状进行全面分析，列出存在的问题并提供改进建议。

---

## 1. 安全问题（高优先级）

### 1.1 认证机制薄弱

**现状**：`server/auth.go` 使用 SHA-256 简单哈希作为认证令牌，无盐值。

```go
func hashKey(key string) string {
    h := sha256.Sum256([]byte(key))
    return hex.EncodeToString(h[:])
}
```

**问题**：
- 无盐值哈希，容易受到彩虹表攻击
- Cookie 缺少 `SameSite` 属性，存在 CSRF 风险
- Cookie 未设置 `Secure` 属性（应仅在 HTTPS 下传输）
- 无会话过期/轮换机制

**建议**：
- 使用 `bcrypt` 或 `argon2` 进行密码哈希
- 添加随机盐值
- 设置 Cookie 的 `SameSite`, `Secure`, `HttpOnly` 属性
- 实现会话管理机制

### 1.2 无 CSRF 防护

**问题**：所有 POST/PUT/DELETE 请求均无 CSRF token 验证。

**建议**：实现 CSRF token 机制，至少对修改类操作进行防护。

### 1.3 无 HTTPS/TLS 内置支持

**问题**：应用仅支持 HTTP，依赖外部反向代理提供 TLS。

**建议**：
- 内置 HTTPS 支持（通过配置启用）
- 支持 Let's Encrypt 自动证书管理
- 或至少支持加载已有的证书/私钥

### 1.4 敏感数据未加密存储

**问题**：
- IPMI/iDRAC/SSH/OpenWrt 等设备的密码明文存储在 SQLite 中
- DNS 提供商的 API Token 以 JSON 明文存储

**建议**：
- 对敏感字段进行加密存储
- 使用主密钥（master key）解密
- 或至少使用加密后的格式存储

---

## 2. 代码质量问题

### 2.1 无测试代码

**问题**：项目中零测试文件（`*_test.go`）。

**影响**：
- 无法保证代码正确性
- 重构风险极高
- 回归 bug 难以发现

**建议**：
- 优先为核心模块（CRUD 操作、数据库层）添加单元测试
- 为工具函数（`parseTime`, `formatTime`）添加测试
- 逐步为 HTTP handlers 添加集成测试

### 2.2 数据库文件过大且职责混乱

**现状**：`server/database.go` 超过 1860 行，混合了多个模块的模型定义和 CRUD 操作。

**包含内容**：
- SystemStats, Project, Deployment, DeploymentLog
- DNSProvider, Domain, DNSRecord
- TCPListener
- CronJob, CronJobExecution
- 各模块的完整 CRUD 方法

**建议**：
- 按模块拆分文件（已有 `settings.go`, `ipmi.go`, `idrac.go` 等模式）
- 每个文件包含：模型定义 + Schema 初始化 + CRUD 方法 + HTTP 处理器
- `database.go` 仅保留通用工具函数和共享模型

### 2.3 Schema 初始化重复调用

**问题**：`server/database.go:117-119` 和 `255-257` 都调用了 `initIPMISchema()`，`205-207` 和 `239-242` 都调用了 `initCertificateSchema()`。

**建议**：移除重复调用。

### 2.4 main.go 路由注册过于集中

**现状**：`main.go` 有 220 行，几乎全是路由注册。

**建议**：
- 将路由注册拆分到各模块文件中
- 每个模块提供 `RegisterRoutes(mux *http.ServeMux, s *Server)` 方法
- `main.go` 仅保留入口和模块注册调用

### 2.5 缺少输入验证

**问题**：HTTP 处理器中直接读取表单值，未做任何输入验证。

**建议**：
- 实现统一的输入验证层
- 至少对长度、格式、必填字段进行验证
- 防止 XSS（模板自动转义已部分防护）

---

## 3. 架构问题

### 3.1 单文件单锁的并发瓶颈

**问题**：所有数据库操作共享同一个 `s.mu` 互斥锁（`sync.RWMutex`）。

```go
func (s *Server) GetAllTCPListeners() ([]TCPListener, error) {
    
    defer 
    // ...
}
```

**影响**：任何模块的写操作都会阻塞所有其他模块的读写。

**建议**：
- 按模块使用独立的锁
- 或使用 SQLite 的 WAL 模式（`PRAGMA journal_mode=WAL`）支持并发读写
- 评估是否真的需要应用层锁（SQLite 本身已有并发控制）

### 3.2 缺少健康检查端点

**问题**：无 `/health` 或 `/ready` 端点，无法监控应用状态。

**建议**：添加标准健康检查端点，返回数据库连接状态、Docker 连接状态等。

### 3.3 无日志系统

**现状**：仅使用标准库 `log.Printf`。

**问题**：
- 无日志级别控制
- 无结构化日志
- 无法追踪请求链路
- 错误信息缺乏上下文

**建议**：
- 引入 `slog`（Go 1.21+ 标准库）或 `zap`/`zerolog`
- 添加请求日志中间件
- 支持日志级别配置

### 3.4 无错误处理中间件

**问题**：每个处理器都需要手动处理错误和返回响应。

**建议**：实现统一的错误处理中间件。

---

## 4. 基础设施缺失

### 4.1 无 Docker 容器化支持

**问题**：项目无 `Dockerfile`，无法直接容器化部署。

**建议**：
- 添加多阶段构建的 `Dockerfile`
- 提供 `docker-compose.yml` 一键部署

### 4.2 无 CI/CD

**问题**：无 GitHub Actions 或其他 CI 配置。

**建议**：
- 添加 CI 工作流（lint + build + test）
- 配置自动构建和发布

### 4.3 无 Makefile

**问题**：构建、运行、清理等操作无统一命令。

**建议**：创建 `Makefile` 提供标准命令：
```makefile
make build      # 构建
make run        # 运行
make test       # 测试
make lint       # 代码检查
make clean      # 清理
```

---

## 5. 文档问题

### 5.1 文档与实际代码不一致

**问题**：
- README 和文档说端口是 8080，实际代码默认是 9000（`config/config.go:31`）
- `architecture.md` 引用了不存在的文件（`webserver_listener.go`, `tcp_handlers.go`, `acme_handlers.go`, `acme_types.go`）
- 架构文档中的 `Server` struct 定义与实际不符

**建议**：更新所有文档以匹配实际代码。

### 5.2 缺少版本/变更日志

**问题**：无 `CHANGELOG.md`，无版本号管理。

**建议**：
- 添加 `CHANGELOG.md`
- 使用语义化版本（SemVer）

### 5.3 缺少 API 端点的完整文档

**问题**：`docs/api-reference.md` 仅覆盖部分端点，缺少大量新模块（IPMI、iDRAC、OpenWrt、Clash、Yeelight、LLM、Workers 等）的 API 文档。

**建议**：补全所有模块的 API 文档。

---

## 6. 功能改进建议

### 6.1 缺少分页

**问题**：所有列表查询无分页，数据量大时性能差。

**建议**：为列表接口添加分页支持。

### 6.2 缺少速率限制

**问题**：API 无任何速率限制，可能被滥用。

**建议**：添加中间件级别的速率限制。

### 6.3 缺少事件/通知系统

**问题**：部署失败、Cron 任务异常、监控告警等无法通知用户。

**建议**：
- 添加 Webhook 通知支持
- 支持邮件/钉钉/企业微信通知

### 6.4 数据清理机制

**问题**：`system_stats` 表无限增长，无自动清理机制。

**建议**：
- 添加数据保留策略配置
- 自动清理过期数据

---

## 7. 优先级排序

| 优先级 | 类别 | 具体项目 |
|--------|------|---------|
| P0 | 安全 | 敏感数据加密存储、CSRF 防护、Cookie 安全属性 |
| P0 | 质量 | 添加核心模块单元测试 |
| P1 | 架构 | 数据库文件拆分、独立锁、健康检查端点 |
| P1 | 安全 | HTTPS 支持、认证机制增强 |
| P1 | 基础设施 | Dockerfile、CI/CD |
| P2 | 质量 | 输入验证、日志系统、错误处理中间件 |
| P2 | 架构 | 路由注册拆分 |
| P2 | 文档 | 更新文档一致性、补全 API 文档 |
| P3 | 功能 | 分页、速率限制、通知系统、数据清理 |
| P3 | 运维 | Makefile、CHANGELOG、版本管理 |
