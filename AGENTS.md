# AGENTS.md - OhMyServer 开发规范

## 创建新页面的完整流程

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
