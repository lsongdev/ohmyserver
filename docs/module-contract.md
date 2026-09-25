# 模块装配与前端清单契约

状态：HTTP 装配与浏览器清单契约已落地；模块内部“公共 Go 库 / Web 服务 / CLI”分层仍在推进，独立仓库发布需完成阶段 5 验收。

模块自行托管相对 API、前端文件和公开清单。宿主只用 Go 代码注册已编译模块及其依赖；浏览器壳读取模块清单后加载入口 JS 和绘制菜单。没有独立的模块挂载配置 JSON。

## 模块内部边界

模块的发布单元需要同时支持 Go 调用者、Web 管理界面和独立启动。以 Clash 为例，目标目录是：

```text
clash/
  go.mod
  clash/client.go          # 可被其他 Go 项目直接依赖的 Clash API 客户端
  web/server.go            # Web/API 服务，调用 ../clash 客户端
  web/assets.go            # 嵌入并托管 web/assets
  web/assets/manifest.json  # 壳应用发现入口与菜单
  web/assets/index.html     # 独立 Web UI 页面，引用公共 CSS
  web/assets/index.js       # 独立页面启动，使用共享 Preact ESM
  web/assets/module.js      # 壳应用发现的模块入口
  web/assets/pages/         # 模块页面
  cmd/clash/main.go         # clash web serve
```

其他模块沿用相同职责：模块根是独立 Go module；公共领域库放 `<id>/` 包；HTTP/API 与静态托管放 `web/`；浏览器代码、清单和样式放 `web/assets/`；独立可执行程序放 `cmd/<id>/`。`index.html` 是模块独立页面并引用 `https://lsong.org/stylesheets/index.css`；standalone `index.js` 只是一个很薄的宿主，加载与 OhMyServer 相同的 `module.js`、`pages/*` 和 `api.js`。Standalone 可以通过本地 adapter 映射到 `https://lsong.org/scripts/react/index.js`，但不得重新实现模块业务页面。manifest 指向 `module.js` 作为壳可导入的模块对象。`web` 调用公共库并组合本模块 repository/服务接口，不能让 `cmd` 成为壳应用的依赖。壳只导入模块的 `web` 包并调用其构造器；独立命令与壳应用复用同一个 Web 服务实现。

`<id>/client.go` 封装外部服务协议，避免 `web`、壳或消费者各自直接依赖供应商客户端。若模块对外提供的是领域库而非外部 API 客户端，则在同一个公共 Go 包中提供领域 API。独立命令负责独立配置、数据库/存储初始化和 HTTP 监听；只有在 `GOWORK=off` 下通过 `go test ./...`、`go build ./...` 并实际启动服务后，才算具备独立运行条件。当前模块中的扁平包可作为迁移过渡，不能作为目标结构完成的证据。

## Go 注册

Go 不能通过 interface 约束包级 `New`，因此宿主的组合根显式 import 模块包，调用模块自己的强类型构造器并注册返回值。模块统一满足 `Name()` 与 `Handler()`；组合根从宿主资源中挑选模块需要的数据库连接池、设备客户端和窄跨模块接口，传给构造器。模块包不导入 `server` 或全局 service locator。

```go
registry := app.NewModules()
module := docker.New(dockerService)
if err := registry.Register(module); err != nil { return err }
```

`Register` 按 `Name()` 将 ID `docker` 的 handler 挂到 `/v1/docker`，并给浏览器壳分配 Hash 页面根 `/docker`。handler 接收去掉 `/v1/docker` 前缀后的请求，自己处理 `/containers` 等 API 路由、`/assets/...` 前端文件和 `GET /manifest.json` 清单。由此保留现有 `/v1/docker/containers` API；浏览器顶级入口由 manifest 的 `home` 决定。宿主无须读取模块文件系统，也无需知道 JS 文件名。

构造器返回 `http.Handler` 已足够覆盖普通 HTTP 模块。需要后台任务或连接回收时，返回的具体 handler 可额外实现 `Start(context.Context) error`、`Close(context.Context) error`，由宿主协调生命周期。数据库连接池可共享，schema、迁移和 repository 仍归模块所有。独立发布时宿主只需更新版本化 Go 依赖及这一处注册代码；启用/禁用可通过是否注册或宿主已有的能力配置决定。

## 模块公开清单

模块在自己的 `GET /manifest.json` 提供 JSON，示例见 [module-manifest.example.json](module-manifest.example.json)。清单字段：

| 字段 | 用途 |
| --- | --- |
| `contractVersion` | 壳与模块前端的契约版本，当前为 `1` |
| `id` | 用于和宿主 Go 注册 ID 核对 |
| `title`、`icon`、`group` | 模块声明的顶级导航展示信息 |
| `entry` | 相对模块 HTTP 根的 JS 入口，例如 `assets/index.js` |
| `home` | 可选的模块首页相对路由，默认 `/` |
| `styles` | 相对模块 HTTP 根的 CSS 文件列表，可为空 |
| `navigation` | 可选的模块内子菜单，每项含 `title` 与模块内相对 `path` |

模块知道自己有哪些页面适合出现在菜单，因此 `navigation` 由模块声明。它只列可见入口，不要求列出所有可访问路由；实际页面路由由模块入口默认导出的 `routes` 提供。省略 `navigation` 或设为空数组时，壳只显示顶级链接 `#/<id>`，由 `home` 指定首屏。清单不包含宿主 API 前缀、静态前缀或页面前缀。

模块用 `//go:embed` 打包 JS/CSS，并在自己的 `ServeHTTP` 内提供 `/assets/`。宿主不接收 `fs.FS`；文件托管、MIME、缓存和静态 404 由模块负责。清单的 `entry/styles` 必须是模块根内的相对文件路径，不允许起始 `/`、`..`、URL scheme、查询或 Hash。清单和前端文件与 Go 包一起版本化。

## 浏览器加载

1. 宿主从已注册的 Go 模块生成公开模块索引 `GET /v1/modules`，只含已注册模块的 `id`、`apiBase`、`manifestURL` 和 Hash 页面根路径。这里的索引由代码注册表生成，不是另一份需要维护的配置。
2. 浏览器壳从索引中取得已挂载模块，并向每个模块请求 `GET <模块根>/manifest.json`，校验 ID 和契约版本。壳用模块清单绘制顶级菜单、分组及可选子菜单；单个清单失败会为该模块显示错误态，其他模块继续加载。
3. 进入模块时，壳按已校验的相对 `entry` 构造同源 URL，调用原生 `import(url)` 并读取默认导出的模块对象。对象只提供 `id`、相对页面 `routes` 和 `createAPI(transport)`；`title`、`icon`、`group`、`home`、`navigation`、`styles`、`entry` 只允许出现在 `manifest.json`。宿主传入已绑定模块 API 根的 `request`、`fetchImpl`、`apiURL` 和 `websocketURL`，模块代码不接触 API 挂载前缀。页面收到的 `navigate()` / `href()` 同样只接受模块根内的绝对相对路径（例如 `/leases`），不得写 `/dhcp/leases` 或 `#/dhcp/leases` 这样的宿主页面前缀。跨模块链接使用宿主注入的 `moduleHref(moduleID, path)`，例如 `moduleHref("docker", "/containers/abc")`；模块只声明目标模块 ID 与目标模块内路径，不自行拼接 Hash 或页面挂载前缀。

链路是 `Go 注册 → 模块 http.Handler → /v1/modules → 模块 manifest.json → 浏览器 import()`。模块 JS 不读取宿主全局配置，也不自行向壳注册。宿主只加载已注册模块的同源清单和入口，不执行任意外部 JS URL。壳将挂载前缀封装在 `transport` 闭包中，通过模块的 `createAPI(transport)` 启动点注入请求、流式 fetch、资源 URL 和 WebSocket URL 能力；模块 API client 只提供模块内相对路径。
