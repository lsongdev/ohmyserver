# AGENTS.md - OhMyServer 开发规范

## 模块化应用规范（新功能默认）

- 完整功能模块放在 `modules/<feature>/`，并按职责分层：`<feature>/` 是公共 Go 库（例如 `clash/client.go` 封装外部 API），`web/` 提供模块 API、业务服务和 `http.Handler`，`web/assets/` 存放清单及浏览器 ESM/CSS，`cmd/<feature>/` 提供独立启动命令（例如 `clash web serve`）。模块的数据库 schema、repository、迁移和测试也由该模块拥有。独立命令与壳应用复用同一个 `web` 实现；壳只依赖模块的 `web` 包，不依赖 `cmd` 或模块内部实现。
- 模块 `web/` 通过 `go:embed` 自行托管相对 API 路由、`/assets/` 和 `/manifest.json`，不硬编码宿主挂载前缀。前端使用 Preact + HTM + 浏览器原生 ESM，不使用 JSX、打包器或运行时转译。所有模块先编译进主程序并由 Go 组合根显式注册；浏览器壳从模块清单发现 JS 入口、样式、侧边栏分组及可选子导航。宿主决定模块 HTTP 根路径、API 前缀和 Hash 页面前缀，并通过模块启动函数注入已作用域化的 transport；模块 JS 只使用相对 API 路径和相对页面路由，不读取或拼接宿主前缀，且应支持相对根路由 `/`。`frontend/` 只保留壳层、共享 UI 和 vendor。
- 模块独立 UI 的 `web/assets/index.js` 直接从 `https://lsong.org/scripts/react/index.js` 导入 `html`、`render` 和 hooks；通过 HTML 的 `api-base` 配置独立服务挂载前缀。不要把 Preact/HTM/shared runtime 复制进每个模块。给壳加载的 `module.js` 仍由宿主注入 transport，模块 API 客户端保持相对路径。
- 模块独立页面直接引用公共组件样式表 `https://lsong.org/stylesheets/index.css`；模块 CSS 只补充功能特有的布局和样式，不重复定义公共按钮、输入框、表格等组件。
- 模块之间不查询对方数据表、不导入对方仓储。跨模块协作用消费者定义的窄接口，并由应用装配层注入实现；共享 SQLite 连接池不代表共享表所有权。
- 数据库 schema、迁移和 repository 由拥有该数据的 `modules/<feature>` 管理。应用层只负责模块组装、跨模块 adapter 与生命周期协调。
- 模块以未来独立仓库和版本发布为目标：Go 代码、`web/`、契约、迁移和测试一起交付；模块不得依赖宿主 `internal/`、`server/`、组合根或其他功能模块的实现。跨模块能力和共享库须通过明确的版本化公共契约依赖，不能靠同仓相对路径才能构建。
- API 使用 JSON envelope、明确 HTTP 方法与状态码；密钥/密码不得进入 DTO；写入请求限制 body、校验 Content-Type 与未知字段。文件下载、SSE、WebSocket 等流式/二进制响应保留各自协议语义。
- 页面通过注入的 `api`、`params`、`search`、`navigate` 工作；副作用在组件生命周期内清理，列表/筛选状态需要可分享时写入 hash URL。
- 所有持久化资源的创建表单必须在独立页面/路由中；列表页右上角 toolbar 提供创建入口，不内嵌、折叠或 inline-create。子资源也采用父资源上下文下的独立创建路由。安装、拉取、部署、检查等操作表单不属于资源创建；编辑表单可另行规划。
- 表单页的返回操作不得在标题上方单独占一行：优先放在标题 toolbar 右侧；表单底部的“取消”应与提交/保存按钮并列，已有底部取消时避免顶部重复返回。详情页返回入口也应合并到标题 toolbar 右侧。
- 每个 `<input>` 必须包含 `class="input"`，每个 `<button>` 必须包含 `class="button"`；使用变体时保留基础类。删除操作通过 `<a method="delete">` 语义触发 DELETE，不使用隐藏 `_method` 表单。
- 使用原生 CSS 和现有公共组件样式，保持简洁，不重复定义 `.input`、`.button` 等通用类，不给表单添加不必要的 max-width。
- 每次修改同步验证：根模块执行 `go test ./...`、`go build ./...`；有独立 `go.mod` 的模块还要在模块目录以 `GOWORK=off` 执行 `go test ./...`、`go build ./...` 并验证独立命令；对 `frontend/` 和 `modules/*/web/assets/` 下 JavaScript 执行 `node --check`，以及 `git diff --check`；涉及流式功能时补模块级 HTTP 契约测试。

## 旧模板页面流程（仅限尚未迁移模块）

以下规范只用于仍挂载在旧 HTML router 的页面。模块一旦迁入 `modules/<feature>/web` 并由自身 handler 托管 API 与前端，新增和修改必须遵循上面的模块化规范，并删除已退役的旧 handler、模板和路由。

### 创建旧模板页面流程

### 1. 数据库层

**Schema 定义** (`server/<feature>.go`)
- 每个功能模块独立文件，命名如 `settings.go`、`ipmi.go`、`idrac.go`
- 包含 `init<Feature>Schema()` 方法，在 `server/database.go` 的 `initSchema()` 中调用

```go
func (s *Server) init<Feature>Schema() error {
    query := `
    CREATE TABLE IF NOT EXISTS <table_name> (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        ...
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `
    _, err := s.db.Exec(query)
    return err
}
```

**模型定义**
- 使用 Go struct，字段导出 JSON tag
- 密码等敏感字段使用 `json:"-"` 防止序列化
- 时间字段使用 `time.Time`

**CRUD 方法**
- `Create<Feature>()` - 创建记录
- `GetAll<Feature>s()` - 获取所有（列表页用）
- `Get<Feature>ByID()` - 按 ID 获取（详情页用）
- `Update<Feature>()` - 更新记录
- `Delete<Feature>()` - 删除记录

### 2. HTTP 处理器层

**标准路由模式**

| 路径 | 方法 | 用途 |
|------|------|------|
| `/<feature>` | GET | 列表页 |
| `/<feature>/new` | GET/POST | 新建表单/创建 |
| `/<feature>/{id}` | GET | 详情页 |
| `/<feature>/{id}/edit` | GET/POST | 编辑表单/更新 |
| `/<feature>/{id}/power` | POST | 电源控制（如适用） |
| `/<feature>/{id}/test` | POST | 连接测试（如适用） |

**Handler 实现规范**

```go
// 列表页
func (s *Server) <Feature>ListView(w http.ResponseWriter, r *http.Request) {
    items, err := s.GetAll<Feature>s()
    if err != nil {
        log.Printf("Error: %v", err)
        s.Error(w, err)
        return
    }
    s.Render(w, "<feature>/index", H{"items": items})
}

// 新建页 (GET/POST)
func (s *Server) <Feature>NewHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        s.Render(w, "<feature>/new", H{})
        return
    }
    // POST 处理...
    http.Redirect(w, r, "/<feature>", http.StatusSeeOther)
}

// 详情页 (GET/PUT/DELETE)
func (s *Server) <Feature>DetailHandler(w http.ResponseWriter, r *http.Request) {
    // 解析 ID
    idStr := r.PathValue("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    // 获取数据
    item, err := s.Get<Feature>ByID(id)
    // 方法判断 (GET/PUT/DELETE)
    method := r.Method
    if r.Method == http.MethodPost {
        overrideMethod := r.FormValue("_method")
        if overrideMethod != "" { method = overrideMethod }
    }
    switch method {
    case http.MethodGet:
        s.Render(w, "<feature>/detail", H{"item": item})
    case http.MethodPut:
        // 更新逻辑
    case http.MethodDelete:
        // 删除逻辑
    }
}
```

### 3. 路由注册

在 `main.go` 中注册路由：

```go
router.HandleFunc("/<feature>", server.<Feature>ListView)
router.HandleFunc("/<feature>/new", server.<Feature>NewHandler)
router.HandleFunc("/<feature>/{id}", server.<Feature>DetailHandler)
router.HandleFunc("/<feature>/{id}/edit", server.<Feature>EditHandler)
```

**侧边导航菜单**
- 在 `web/layout.html` 的 `<nav>` 中添加模块入口
- 每个新模块创建后必须添加侧边栏链接
- 命名规范：小写，如 `<li><a href="/<feature>"><feature></a></li>`

### 4. 模板文件结构

**目录**: `web/<feature>/`

| 文件 | 用途 |
|------|------|
| `index.html` | 列表页 |
| `new.html` | 新建/编辑共用表单（编辑时传 `.server`） |
| `detail.html` | 详情页 |

**模板基础**
- 使用 `{{define "page"}}` 定义内容块
- 数据通过 `s.Render(w, "<feature>/view", H{...})` 传递
- 日期格式化用 `{{formatDate .CreatedAt}}`

### 5. 表单规范

**新建/编辑表单** (`new.html`)
- GET 时为空表单，不显示 `<no value>`
- 使用 `{{if .server}}...{{else if .fieldname}}{{.fieldname}}{{end}}` 处理值
- 编辑模式通过 `.server` 判断区分新建/编辑
- 密码字段编辑时留空表示不修改
- 所有 `<input>` 必须包含基础样式类 `class="input"`
- 所有 `<button>` 必须包含基础样式类 `class="button"`
- 使用变体样式时必须保留基础类，例如 `class="button button-primary"`、`class="button button-danger"`

```html
<input class="input" value="{{if .server}}{{.server.Name}}{{else if .name}}{{.name}}{{end}}">
<button class="button button-primary" type="submit">Save</button>
```

**删除操作**
- 所有删除按钮必须使用 `<a method="delete" href="...">delete</a>` 模式
- **禁止使用** `<form>` + `<input type="hidden" name="_method" value="DELETE">` 的方式
- **禁止使用** `<form method="POST">` 提交删除操作
- 需要确认时添加 `onclick="return confirm('...')"`
- 后端 handler 必须支持 `http.MethodDelete` 直接处理，不依赖 `_method` 参数

```html
<!-- 正确 -->
<a method="delete" href="/feature/{{.ID}}" class="button button-danger">Delete</a>
<a method="delete" href="/feature/{{.ID}}" class="button button-danger" onclick="return confirm('Are you sure?')">Delete</a>

<!-- 错误 -->
<form method="POST" action="/feature/{{.ID}}">
    <input type="hidden" name="_method" value="DELETE">
    <button type="submit">Delete</button>
</form>
```

### 6. 样式规范

- 使用原生 CSS，无外部框架
- **公共 CSS 已包含**：`.input`、`.form-group`、`.form-actions`、`.button`、`.button-primary`、`.button-danger`、`.section`、`.table` 等（见 `web/assets/css/components/`）
- **不要在页面中重复定义已有样式**，如 `.input`、`.form-group`、`.button`、`.form-actions` 等
- **不要限制表单 max-width**，表单自然填充可用空间
- 页面风格朴素简洁，避免过度装饰
- 卡片/列表使用 `border: 1px solid #e1e4e8` + 圆角即可，不需要阴影
- 仅添加组件特有的样式（如特定布局结构），用最小化的 `<style>` 块
- 像 `new.html` 编辑页面通常**不需要任何额外 CSS**

### 7. 外部设备集成（IPMI/iDRAC）

**连接管理**
- 结构体中保存 `client` 字段
- 详情页 GET 时连接设备，获取实时数据
- 使用 `defer server.Disconnect(ctx)` 确保断开

**电源控制**
- 独立 `/power` 路由，仅接受 POST
- 支持多种操作：on/off/graceful/restart/pushbutton

### 8. 常见模式

**设置类页面**（类似 Settings）
- 列表页支持 inline 编辑保存
- 使用 AJAX 避免页面刷新

**设备管理类**（类似 IPMI/iDRAC）
- 支持连接测试
- 详情页显示设备实时状态
- 电源控制按钮

**列表 + 详情类**（通用）
- 列表页显示摘要信息
- 详情页显示完整数据和操作

### 9. 代码组织

```
server/
  ├── <feature>.go          # 模型 + CRUD + 处理器
  └── database.go           # initSchema() 调用处

web/<feature>/
  ├── index.html            # 列表页
  ├── new.html              # 新建/编辑表单
  └── detail.html           # 详情页

main.go                     # 路由注册
```

### 10. 注意事项

1. **不要显示 `<no value>`** - 使用 `{{if .var}}{{.var}}{{end}}` 保护
2. **密码字段** - 编辑时留空保留原密码
3. **错误处理** - 所有 DB 操作都要检查错误
4. **并发安全** - DB 操作用 `/` 保护
5. **时间处理** - 使用 `parseTime()` 解析 DB 时间字符串
6. **资源释放** - 设备连接必须 `defer Disconnect()`
