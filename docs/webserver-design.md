# Web Server 与 ACME、DNS 联动设计

状态：Draft  
适用范围：OhMyServer 内置 Web Server 第一阶段，以及后续拆分为独立 Go 项目时的公共 API

## 1. 背景

OhMyServer 已经具备三块相关能力：

- 前一版 `proxy/http_proxy.go` 使用 `http.ServeMux` 和 `httputil.ReverseProxy` 实现了域名、方法、路径匹配和反向代理，现已由 `yabber` 替换。
- `server/web_proxy.go` 使用 SQLite 保存代理路由，并在路由变化时重建 HTTP Server。
- ACME 模块已经把 CSR、证书链和私钥保存在 SQLite；DNS 模块已经可以通过不同 Provider 操作域名记录。

目前缺少的是三者之间稳定、清晰的边界：

1. Web Server 如何在不依赖 OhMyServer 数据库的前提下取得证书。
2. ACME 续期后如何让 HTTPS 立即使用新证书。
3. Web Route、DNS Record、Certificate 之间如何建立显式关系。
4. 配置修改后哪些内容需要 reload，哪些内容需要 restart。
5. 是否需要实现服务管理，以及它应当管理到什么范围。

## 2. 设计目标

### 2.1 目标

- 尽量直接使用 Go 标准库：`net/http`、`http.ServeMux`、`httputil.ReverseProxy`、`crypto/tls`。
- `yabber` 是纯运行时库，不导入 OhMyServer 的 `server`、SQLite、ACME 或 DNS 实现。
- HTTP 和 HTTPS 都是一等能力，支持按域名选择证书。
- 路由和证书更新尽量热加载，不因普通配置变化重启整个 OhMyServer。
- ACME、DNS、Web 通过小接口和显式绑定联动，不互相读取对方的内部表。
- 第一阶段适合内嵌在 OhMyServer 进程，成熟后可以低成本迁移到独立仓库。

### 2.2 非目标

第一阶段不实现：

- 完整替代 Caddy、Nginx 或 Envoy 的所有功能。
- 动态模块、脚本语言、复杂负载均衡和服务发现。
- 跨机器分布式配置一致性。
- 自己实现 ACME、DNS 协议或 TLS 协议。
- 自己实现 systemd、launchd 等操作系统服务管理器。

## 3. 核心结论

### 3.1 配置采用“文件 + 数据库”双层模型

不建议把现有 Settings 全部迁移到 TOML，也不建议把所有 Web Route 写入配置文件。

配置分为两类：

| 类型 | 存储 | 示例 | 生效方式 |
|---|---|---|---|
| 控制面启动配置 | YAML 文件 | 面板监听地址、数据库路径、数据目录、日志 | 进程启动时读取，修改后重启 OhMyServer |
| 受管理组件期望状态 | SQLite | Web Listener、Route、Certificate、DNS Binding、启停状态 | 运行时读取，reload 或仅重启对应组件 |

项目当前已经使用 `~/.ohmyserver/config.yaml`，没有必要仅为了 Web Server 引入 TOML，形成两种配置格式。YAML 保持为最小启动配置：

```yaml
listen: ":9000"

database:
  driver: sqlite
  path: ~/.ohmyserver/ohmyserver.db
```

站点 Listener 使用独立结构化表，而不是通用 Settings：

```sql
CREATE TABLE web_listeners (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    protocol TEXT NOT NULL,       -- http, https
    address TEXT NOT NULL,        -- :80, :443
    enabled INTEGER NOT NULL DEFAULT 1,
    redirect_to_https INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

首次初始化创建 `http :8081` Listener，之后完全以 `web_listeners` 为事实来源。修改 Route 或证书时执行 reload；修改 Listener 地址时只 restart Yabber，面板控制端仍然运行。

`webserver.tls.cert` 这类字段不应直接保存某个证书文件路径。一个 Web Server 通常需要为多个域名提供不同证书，证书选择应由数据库中的 Route/Certificate Binding 决定。

文件配置只回答“控制面如何启动”；数据库回答“受管理组件在哪里监听、服务哪些域名和路由”。

### 3.2 证书保存在控制面的数据库，通过接口交给运行时

第一阶段建议继续以 SQLite 作为证书事实来源：

- 现有 ACME 已经在同一事务边界内保存证书、私钥、订单状态。
- 证书续期完成后不需要额外保证数据库与文件副本一致。
- 备份和恢复关系清晰。
- Web Server 可以通过存储适配器直接构建内存中的 `tls.Certificate`。

但 `yabber` 不能知道 SQLite，也不能接收证书记录 ID。它只认识下面这样的抽象：

```go
type CertificateLoader interface {
    LoadCertificate(Certificate) (*tls.Certificate, error)
}
```

OhMyServer 在 `server` 层实现 `CertificateLoader`，按 Yabber 实际引用的证书名称从数据库加载；Yabber 校验后构建不可变 SNI 快照。独立模式使用文件 Loader，以后也可以换成 Vault 或用户自定义实现。

私钥处理要求：

- SQLite 文件至少使用 `0600` 权限，所在目录使用 `0700`。
- API、日志和模板中不得返回私钥。
- 数据库备份应按密钥材料处理。
- 后续可增加“主密钥加密私钥列”，主密钥来自环境变量或权限受限的独立文件。
- PEM 文件只作为导出或兼容模式，不作为内部联动的主通道。

### 3.3 ACME 续期后热更新证书，不重启 HTTP Server

HTTPS Listener 使用：

```go
tls.Config{
    GetCertificate: certificateIndex.GetCertificate,
}
```

证书签发或续期成功后：

1. ACME 在事务中写入新的证书版本并标记为 active。
2. 联动层触发 `CertificateChanged`。
3. Certificate Store 重新解析相关证书。
4. 使用 `atomic.Pointer` 或加锁替换只读证书快照。
5. 后续 TLS 握手立即取得新证书。

已经建立的 TLS 连接不受影响，新连接使用新证书。因此证书更新不需要 stop、start 或 restart。

### 3.4 需要轻量组件管理，不需要重新发明系统服务管理

面板内部需要一个统一的组件生命周期接口：

```go
type Service interface {
    Name() string
    Start(context.Context) error
    Stop(context.Context) error
    Reload(context.Context) error
    Status() Status
}
```

第一阶段只管理当前进程内的组件，例如：

- `webserver`
- `tcp-proxy`
- `cron`
- 将来可能加入 `dns-server`、`dhcp-server`、`ntp-server`

语义定义：

| 操作 | 含义 |
|---|---|
| start | 根据最新期望状态创建 Listener 和后台任务 |
| stop | 优雅停止组件，但保持 OhMyServer 控制面可用 |
| reload | 校验新配置，原子替换路由或证书快照 |
| restart | `stop + start`，仅监听地址等不可热更新字段变化时使用 |
| status | 返回期望状态、实际状态、监听地址、启动时间和最后错误 |

操作系统级的开机启动、进程崩溃拉起仍交给 systemd、launchd 或容器平台。面板后续可以生成这些配置或显示状态，但不应在公共 Web Server 库中实现它们。

## 4. 分层架构

```text
┌──────────────────── OhMyServer 控制面 ────────────────────┐
│ UI / HTTP Handler                                         │
│     │                                                     │
│     ▼                                                     │
│ Application / Coordinator                                 │
│  ├─ ApplyWebRoute                                         │
│  ├─ EnsureDNSRecord                                       │
│  ├─ IssueOrRenewCertificate                               │
│  └─ ReloadWebServer                                       │
│     │                  │                    │              │
│     ▼                  ▼                    ▼              │
│ RouteRepository   CertificateRepository   DNS Provider    │
│     │                  │                    Adapter        │
│     └──────────── SQLite / provider API ──────────────────│
└───────────────────────────┬────────────────────────────────┘
                            │ runtime config + small interfaces
                            ▼
┌──────────────────────── yabber ───────────────────────────┐
│ Server                                                     │
│  ├─ HTTP Listener                                          │
│  ├─ HTTPS Listener + tls.Config.GetCertificate             │
│  ├─ atomic Router snapshot                                 │
│  └─ Status / Start / Stop / Reload                         │
│                                                           │
│ Router                                                     │
│  └─ http.ServeMux                                          │
│       ├─ static file handler                               │
│       ├─ httputil.ReverseProxy                             │
│       ├─ redirect handler                                  │
│       └─ ACME HTTP-01 handler                              │
└───────────────────────────────────────────────────────────┘
```

关键原则是：公共库提供机制，OhMyServer 提供数据和策略。

## 5. `yabber` 包结构

```text
yabber/
├── config.go          # YAML/Go Struct 配置模型
├── certificate.go     # 文件及可注入的证书加载器
├── compile.go         # 校验、ServeMux、ReverseProxy 和运行时快照
├── server.go          # Listener 生命周期与原子 Apply
├── yabber_test.go
├── config.example.yaml
└── README.md
```

公共模型保持小而明确：

```go
package yabber

type Config struct {
    Version      int
    Server       ServerConfig
    Listeners    []Listener
    Certificates []Certificate
    Hosts        []VirtualHost
}

type VirtualHost struct {
    Name      string
    Hostnames []string
    Listeners []string
    TLS       *HostTLS
    HTTP      *HostHTTP
    Routes    []Route
}

type Route struct {
    Name   string
    Match  Match
    Handle Handler
}

type Handler struct {
    Proxy    *ProxyTarget
    Files    *FileTarget
    Redirect *RedirectTarget
    Static   *StaticResponse
}

type ProxyTarget struct {
    URL         *url.URL
    StripPrefix string
}
```

约束：

- 一个 `Handler` 只能设置一种目标类型，构建 Router 时校验。
- Host 统一转为小写并去除末尾的点，端口匹配策略必须明确。
- Path 使用 `ServeMux` 的原生匹配规则。
- Method、Host、Path 最终组合成 Go 1.22+ 的 ServeMux Pattern，例如 `GET example.com/api/`。
- 重复 Pattern、无效上游 URL、无效监听地址在 reload 前返回错误。
- 默认增加合理的 `ReadHeaderTimeout`，防止慢请求长期占用连接。

## 6. Router 与热加载

不建议每次路由变化都关闭 Listener。可以让固定 Handler 指向当前 Router 快照：

```go
type switchHandler struct {
    current atomic.Pointer[http.Handler]
}

func (h *switchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    handler := h.current.Load()
    (*handler).ServeHTTP(w, r)
}
```

实际实现可以用一个包含 `http.Handler` 的具体 snapshot 类型，避免接口指针使用不便。Reload 流程为：

1. 从 Repository 读取全部 enabled Routes。
2. 在后台构建新的 `http.ServeMux` 和所有 `ReverseProxy`。
3. 完整校验。
4. 一次原子替换 Router snapshot。
5. 老请求继续使用旧 Router，新请求使用新 Router。

如果第 2 或第 3 步失败，旧配置继续服务，并记录 `last_error`，不会出现“先停掉旧服务，再发现新配置错误”的窗口。

只有以下变化需要重启对应 Listener：

- HTTP/HTTPS 监听地址变化。
- TLS 基础参数变化且无法安全热替换。
- 组件由 disabled 变为 enabled，或反向变化。

## 7. HTTPS 与证书选择

### 7.1 数据关系

建议新增显式绑定，而不是在 Route 中保存 PEM：

```sql
CREATE TABLE web_certificate_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname TEXT NOT NULL UNIQUE,
    certificate_id INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (certificate_id) REFERENCES certificates(id)
);
```

一个多 SAN 证书可以绑定多个 hostname；同一 hostname 同一时刻只绑定一个 active 证书。

证书匹配顺序：

1. SNI 精确匹配。
2. 单层通配符匹配，例如 `*.example.com`。
3. 可选的 default certificate。
4. 没有匹配时返回明确错误，不静默发送错误域名的证书。

证书载入时必须校验：

- 私钥与证书匹配。
- 当前时间位于证书有效期内，可允许小的时钟偏差。
- 绑定 hostname 被 SAN 覆盖；不以 Common Name 作为主要判断依据。
- 证书链 PEM 可以被完整解析。

### 7.2 证书版本

续期不建议直接覆盖唯一记录。更稳妥的长期模型是：

```text
certificate_names       逻辑证书，例如 example.com + *.example.com
certificate_versions    每次签发得到的 cert/key/not_before/not_after/status
web_certificate_bindings hostname -> certificate_name
```

第一阶段可以沿用现有 `certificates` 表，但续期时应保证：

- 新证书验证成功以后才切换 active。
- 切换失败时保留旧证书。
- 旧证书在短暂保留期后再清理，便于回滚。

## 8. ACME 联动

快速签发的产品入口属于 `/acme/certificates/new`。Web Host 不向 ACME
传递内部 ID；它只用 `hostname` 查询参数预填域名，并在签发完成后由用户
选择已有证书。ACME 签发页必须明确选择 ACME Account。DNS-01 还必须选择
一个本地 Zone 或 Provider Hosted Zone。

### 8.1 HTTP-01

Web Server 暴露一个高优先级的临时 Challenge Store：

```go
type HTTP01Store interface {
    GetHTTP01Token(host, token string) (keyAuthorization string, ok bool)
}
```

对 `/.well-known/acme-challenge/{token}` 的请求，处理顺序必须早于：

- HTTP 到 HTTPS 跳转。
- 用户配置的反向代理 Route。
- 静态文件 Route。

流程：

```text
ACME Coordinator
  -> 写入临时 token
  -> 通知 ACME Server 校验
  -> 等待 authorization valid
  -> 删除临时 token
  -> finalize
  -> 保存证书版本
  -> 刷新 Certificate Store
```

这样不需要把 challenge 文件写入某个网站目录。

### 8.2 DNS-01

DNS-01 不应让 ACME 模块直接依赖具体 DNS Provider。定义最小能力：

```go
type DNSChallengeProvider interface {
    Present(ctx context.Context, fqdn, value string) (cleanupID string, err error)
    CleanUp(ctx context.Context, cleanupID string) error
}
```

OhMyServer 用现有 DNS Provider 适配此接口。Coordinator 负责：

1. 根据申请域名找到 zone 和 provider binding。
2. 创建 `_acme-challenge` TXT 记录。
3. 使用权威 DNS 查询等待记录传播，而不是固定 sleep。
4. 通知 ACME 完成 challenge。
5. 无论成功或失败都尝试清理本次创建的 TXT 记录。
6. 清理失败进入可重试任务，并在 UI 显示。

Wildcard 证书必须使用 DNS-01。

### 8.3 自动续期

新增一个轻量定时任务扫描 active 证书：

- 默认在到期前 30 天进入 renew window。
- 使用随机抖动避免所有证书同时续期。
- 同一逻辑证书只允许一个续期任务运行。
- 失败使用指数退避，并记录下次重试时间和最后错误。
- 续期成功后自动刷新内存证书，不重启 Web Server。

## 9. DNS 与 Web Route 联动

DNS 记录和 Web Route 不应通过“域名字符串相同”隐式联动。建议增加显式关系：

```sql
CREATE TABLE web_dns_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    route_id INTEGER NOT NULL,
    provider_id INTEGER NOT NULL,
    zone_id TEXT NOT NULL,
    hostname TEXT NOT NULL,
    record_type TEXT NOT NULL,
    record_value TEXT NOT NULL,
    managed INTEGER NOT NULL DEFAULT 1,
    provider_record_id TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    last_error TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

`managed` 表示该记录是否由 OhMyServer 创建并拥有。删除 Route 时：

- `managed = true`：提示用户是否连同 DNS Record 一起删除。
- `managed = false`：只解除绑定，不删除用户原有记录。

推荐的“创建站点”应用层流程：

1. 创建 disabled Route。
2. 根据用户选择创建或绑定 A、AAAA、CNAME 记录。
3. 等待 DNS 解析到预期地址。
4. 申请或选择证书。
5. 创建 hostname 到 certificate 的绑定。
6. 校验整个运行时配置。
7. 启用 Route 并 reload Web Server。

这是一段应用层工作流，不属于 `yabber`。各步骤应持久化状态以便重试；外部 DNS 和 ACME 操作无法与 SQLite 做真正的分布式事务，因此需要补偿动作，而不是假装它们在一个事务里。

## 10. Service Manager 设计

建议在 OhMyServer 内新增 `service.Manager`，而不是放进 `yabber`：

```go
type Manager struct {
    services map[string]Service
}

func (m *Manager) Start(ctx context.Context, name string) error
func (m *Manager) Stop(ctx context.Context, name string) error
func (m *Manager) Reload(ctx context.Context, name string) error
func (m *Manager) Restart(ctx context.Context, name string) error
func (m *Manager) Status(name string) (Status, error)
```

状态至少包括：

```go
type State string

const (
    StateStopped  State = "stopped"
    StateStarting State = "starting"
    StateRunning  State = "running"
    StateStopping State = "stopping"
    StateFailed   State = "failed"
)

type Status struct {
    Name       string
    Desired    State
    Actual     State
    Since      time.Time
    Listeners  []string
    LastError  string
    Generation uint64
}
```

`Generation` 每次成功应用配置后增加，便于 UI 判断数据库中的期望配置是否已经运行。

注意避免自我关闭：

- 面板控制端继续使用 `cfg.Listen`，例如 `:9000`。
- 被管理的 Web Server 使用独立的 `:80`、`:443`。
- 停止 `webserver` 只停止站点 Listener，不能停止面板控制端。
- 如果以后让 Web Server 代理面板域名，也仍将面板作为 `127.0.0.1:9000` 的 upstream。

## 11. 数据库与 Settings 的边界

现有 `settings` 表适合少量、动态、非结构化的功能开关，但不适合承载完整 Web Server 配置：

- 类型信息较弱。
- 多字段更新缺少整体校验。
- 不便表达 Route、Listener、Certificate 之间的关系。
- 难以进行外键约束和迁移。

建议：

- `settings` 保留给通用 UI 设置和小型 feature flags。
- Web Server 使用自己的结构化表，例如 `web_routes`、`web_listeners`、`web_certificate_bindings`、`web_dns_bindings`。
- 只有控制面进程启动所必需的字段放 YAML；站点 Listener 属于受管理组件，放专用表。
- 不增加 TOML，除非未来整个项目统一从 YAML 迁移，并有明确迁移收益。

## 12. 错误处理与可观测性

每次 apply/reload 需要记录：

- 配置 generation。
- apply 开始和结束时间。
- 成功或失败。
- 失败所在阶段：load、validate、build、bind、swap。
- 受影响的 Route/Certificate/Listener ID。

Web Server 状态页应展示：

- HTTP/HTTPS 实际监听地址。
- 当前 generation 和最后 reload 时间。
- enabled/loaded Route 数量。
- 已载入证书数量及最近到期时间。
- 最近一次错误。

日志不得包含：

- 证书私钥。
- ACME Account Key。
- DNS Provider Token。
- 完整的敏感配置对象。

## 13. 安全默认值

- 上游 URL 默认仅允许 `http` 和 `https`。
- 是否允许代理到 loopback、私网和 link-local 地址应由 OhMyServer 策略层决定，防止 SSRF。
- Reverse Proxy 使用 `ProxyRequest.SetURL` 和 `SetXForwarded`。
- 明确设置 `ErrorHandler`、`Transport` 超时和最大空闲连接数。
- HTTPS 最低版本默认 TLS 1.2，优先采用 Go 的安全默认 cipher suites。
- Listener 启动前检查端口冲突；绑定 `:80`/`:443` 时给出权限提示。
- 文件站点先 `filepath.Clean`，并依赖 `http.FileServer` 的路径约束，禁止任意路径拼接。
- HTTP 管理接口与站点流量分开监听。

## 14. 推荐实施顺序

### Phase 1：抽出纯 Web Server 运行时

- 使用 `yabber` 替换 `proxy/http_proxy.go`。
- 保留 `ServeMux` 和 `ReverseProxy`。
- 支持独立 HTTP/HTTPS Listener。
- 实现 Route snapshot reload、生命周期和状态。
- 为 host/path/method 匹配、reload 回滚、优雅停止编写测试。

完成标准：不依赖 SQLite 的单元测试可以启动 HTTP/HTTPS Server，并热替换 Route。

### Phase 2：接入现有数据库和证书

- 实现 Route Repository adapter。
- 增加 certificate binding。
- 将现有证书解析为内存 snapshot。
- ACME finalize 成功后刷新证书 snapshot。

完成标准：续期或替换证书后，新 TLS 握手使用新证书，Listener 不重启。

### Phase 3：自动化 ACME Challenge

- 实现内存/数据库 HTTP-01 Token Store。
- 为现有 DNS Providers 增加 DNS-01 adapter。
- 增加传播检测、清理和续期调度。

完成标准：可以从 Route hostname 发起签发，并在成功后自动启用 HTTPS。

### Phase 4：DNS/Web 显式联动

- 增加 `web_dns_bindings` 和创建站点工作流。
- 记录资源所有权、状态和补偿任务。
- UI 展示 Route、DNS、Certificate 的关联状态。

完成标准：用户能看出每个域名当前缺少 DNS、证书还是 Web Route，并可安全重试。

### Phase 5：轻量服务管理 UI

- 增加 Manager、status API 和 start/stop/reload/restart 操作。
- 与 systemd/launchd 的进程级管理保持边界。

完成标准：停止站点 Web Server 不影响面板，失败 reload 不影响旧流量。

## 15. 第一阶段建议 API 草案

```go
cfg, err := yabber.Load("yabber.yaml")
if err != nil {
    return err
}

ws, err := yabber.New(cfg)
if err != nil {
    return err
}
if err := ws.Start(); err != nil {
    return err
}
defer ws.Close()
```

OhMyServer 嵌入时可以注入数据库证书加载器：

```go
cfg, err := repository.BuildYabberConfig(ctx)
if err != nil {
    return err
}
ws, err := yabber.New(
    cfg,
    yabber.WithCertificateLoader(databaseCertificateLoader),
)
if err != nil {
    return err
}
```

签发完成后重新构建 Config 并调用 `Apply`；Yabber 只重新加载被启用 HTTPS Host 引用的证书，然后原子替换证书和 Route 快照。独立模式使用 `FileCertificateLoader`，嵌入模式的 Loader 可以从内存或数据库读取，Yabber 本身不需要知道 `Certificate.ID` 或 SQLite Schema。

## 16. 最终决策摘要

1. 继续使用现有 YAML 作为控制面最小启动配置，不单独引入 TOML。
2. Route、Listener、Certificate Binding 和 DNS Binding 使用结构化数据库表。
3. ACME 证书以数据库为事实来源，Yabber 只依赖 `CertificateLoader`。
4. HTTPS 使用 `tls.Config.GetCertificate` 和内存快照，证书续期不重启服务。
5. Route 使用构建后原子替换，普通路由修改不重启 Listener。
6. DNS、ACME、Web 的联动放在 OhMyServer 应用层 Coordinator，不进入公共 Web Server 包。
7. 实现进程内轻量 Service Manager；进程级守护继续交给操作系统。
8. 优先完成可测试的极简核心，再逐步增加自动签发和联动 UI。
