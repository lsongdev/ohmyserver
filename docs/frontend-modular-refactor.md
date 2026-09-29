# 模块化面板重构：Preact + HTM + 原生 ESM

状态：功能代码已按 28 个独立 Go module 收拢到 `modules/<id>/`，每个模块均有 Web 静态资源、清单和独立命令；宿主入口、HTTP 路由和 Go 组合根位于 `cmd/ohmyserver/` 与 `internal/host/`。28 个模块均已在关闭 Go workspace 的情况下通过测试和构建。剩余工作是逐项完成真实服务/设备验收、外部最小宿主集成与模块发布流程验证，并继续把宿主中确属单模块的业务实现迁入模块。

### 表单与页面导航约定（2026-09-24 补充）

- 持久化资源的创建统一使用独立路由；列表右上角提供“添加/创建”入口。父子资源的创建页保留父资源上下文。
- 表单页顶部不单独占一行放返回按钮：优先把返回/取消放在标题工具栏右侧；若表单底部已有“取消”，顶部可以不重复，底部取消与提交按钮并列。
- 列表内的删除、启停、刷新、拉取等即时操作不算创建表单。编辑表单目前仍有少数以折叠区形式嵌在列表/详情中（Projects、Cron、Web Server 等），应作为后续独立编辑页工作项跟踪，不得混淆为创建表单已全部外移。
- 2026-09-24 已将 SSH 服务器、DNS 区域/记录/订阅/过滤规则、Web Server Listener/Policy/Host/Route、Files 新建/目录/上传入口拆到独立页面；并整理了这些新页面及 ACME、Cron、Projects、设置、设备等表单页的返回/取消位置。Standalone/旧模块入口仍需逐个确认与主模块路由是否重复，不能仅凭前端清单判断为死代码。

目标是让 OhMyServer 成为组装模块的外壳：服务端组合模块 handler，前端组合页面组件与路由；Docker、ACME 等模块可以单独开发、测试，并在成熟后作为独立版本发布。组合根显式注册编译进主程序的模块；模块 handler 自己托管 API、清单和 ESM/CSS；壳通过 `/v1/modules` 发现注册模块，并按模块清单加载前端入口与导航。

## 1. 技术选择与边界

- 前端使用 Preact、HTM、浏览器原生 ES modules，全部使用 `.js`，不使用 JSX、TypeScript 编译、打包器或运行时转译。
- `modules/<module>/` 是一个完整功能模块，同目录拥有可复用 Go 库、Web/API 服务、数据库、浏览器 ESM/CSS、启动命令和测试；`frontend/` 只保留应用壳、共享 UI 与固定依赖。
- 后端继续使用 Go `net/http`、`http.ServeMux`。每个模块的 handler 自行托管相对 API 路由、`/assets/` 和 `/manifest.json`；壳只给该 handler 分配根路径，例如 `/v1/docker`。
- 页面地址与 API 地址分离，例如 `#/docker/containers` 调用 `/v1/docker/containers`。模块源码放在 `modules/<module>/web/assets/`，由模块自身 handler 托管前端文件。
- 模块之间通过明确的小接口协作，不读取对方的数据库表，不导入对方的页面实现。
- 继续使用原生 CSS 和公共样式库，不引入 UI 框架。先保持功能和简洁风格，再独立优化交互。
- 无构建指前端源码直接交付浏览器；Go 编译、依赖下载校验和可选的测试工具不属于前端构建流程。
- 暂不做微前端沙箱、运行时插件市场、通用事件总线或通用工作流引擎。所有模块先编译进主程序并由 Go 组合根显式注册；模块自行提供 HTTP handler 和前端发现清单。

Preact 官方支持直接在浏览器运行，并推荐 HTM 作为无需编译的 JSX 替代。采用独立的 `preact`、`preact/hooks` 和 `htm` ESM，通过 import map 指向同一套固定版本依赖，避免 standalone HTM 内嵌另一份 Preact。参考：[Preact Getting Started](https://preactjs.com/guide/v10/getting-started/)、[HTM](https://github.com/developit/htm)。具体依赖版本在基础阶段锁定，不使用浮动 `latest`。

## 2. 现状与需要拆开的依赖

原始现状中，`main.go` 逐条注册各模块页面和操作；`server/<feature>.go` 混合模型、业务和 HTTP；`server/server.go` 的 `Server` 持有数据库、Docker、ACME、DNS、yabber、调度器和任务状态；`web/embed.go` 打包 HTML 和静态资源，`Server.Render` 渲染模板。此基线已迁移：当前入口位于 `cmd/ohmyserver/`，组合根与宿主实现位于 `internal/host/`，旧根级 `server/` 与 `web/` 已移除。

现有 JSON 路径并不统一，例如 `/monitor/hosts/api/stats`；Docker/项目部署/SSH/终端还存在 SSE、WebSocket 和文件响应。当前认证失败会跳转 `/login`，直接用于 fetch 会得到 HTML，必须提供独立 API 认证行为。

因此不能只把 HTML 翻译成组件。每个模块的迁移应同时完成数据契约、业务入口、HTTP 路由、前端页面、资源清理与测试。

## 3. 目标目录

目录按“宿主装配层 / 可发布功能模块 / 公共契约与基础设施 / 迁移区”划分。功能模块目录是完整的发布单元；宿主只保留启动、配置、模块注册与跨模块装配。模块的启用状态和挂载关系由 Go 组合根驱动，模块 `manifest.json` 只描述模块自身前端入口和它选择公开的导航，不作为模块注册表或启停配置。

```text
cmd/
  ohmyserver/
    main.go                     # 调用宿主启动入口
internal/                       # 仅宿主私有；任何可发布模块不得依赖
  app/
    modules.go                  # HTTPModule 与 API 模块挂载
  architecture/                # 模块边界和发布布局检查
  host/
    boot.go                     # 配置、共享资源与进程生命周期
    registry.go                 # 显式注册模块工厂
    routes.go                   # 壳路由、认证与模块 handler 挂载
    *_api.go                    # 跨模块 adapter 和宿主专属能力
go.work                         # 单仓开发时组合宿主与模块 Go modules
pkg/                            # 可独立版本化的公共 Go 库
  moduleweb/                    # 模块清单与 ESM 静态资源托管
  terminal/                     # 多模块共享的终端传输基础库
modules/
  <id>/
    go.mod                      # 每个功能模块独立构建
    README.md                   # 能力、依赖与独立运行方法
    <client>/                   # 可选：面向 Go 使用者的公共库
    web/                        # HTTP handler、service、repository 与资源嵌入
      assets/
        manifest.json           # 模块前端入口与可选子导航
        index.html              # 独立运行入口，使用公共 CSS
        index.js                # 独立页面启动代码，使用共享 Preact ESM
        module.js               # 壳加载的模块 ESM 入口
        api.js                  # 使用宿主注入的相对路径传输层
        pages/                  # 模块页面
    cmd/<id>/main.go             # 独立启动命令，例如 <id> web serve
frontend/
  embed.go                      # 只嵌入壳、共享 UI 与 vendor
  app/                          # 壳启动、模块发现、导航和路由
  shared/                       # 公共 UI、HTTP transport 与样式
  vendor/                       # 固定版本浏览器依赖
 tests/
  module-smoke/                 # 仓库外最小宿主与发布包集成验证
docs/
  frontend-modular-refactor.md  # 模块契约、宿主装配和迁移状态
```

每个功能模块保持 `web/`、`web/assets/`、`cmd/<id>/` 这几个职责边界；需要公开 Go API 的模块另设领域/客户端包，例如 `clash/clash/client.go`。`web/` 组合该模块客户端、repository 和业务能力，提供 API 与模块自己的静态文件；`cmd/<id>` 提供独立运行入口。壳应用只依赖模块的 `web` 包并注入能力，不调用模块私有实现。`web/assets/index.html` 与 `index.js` 提供模块自己的独立页面；`index.js` 从 `https://lsong.org/scripts/react/index.js` 导入 Preact/HTM，HTML 加载 `https://lsong.org/stylesheets/index.css`，不复制通用运行时或组件样式。清单的 `entry` 指向 `module.js` 这样的壳加载入口，模块页面资源仍由模块 handler 托管。独立页面的 API 根由 standalone handler 提供，模块业务 API client 只使用相对路径。最终不保留根级 `server/`、`web/` 或按语言拆开的功能代码。宿主专属代码不能放进可发布模块；模块共用的底层能力放在有独立 module 边界的公共 `pkg/` 中。

每个模块都有自己的 `go.mod`，单仓用 `go.work` 组合开发；28 个模块当前均已在关闭 Go workspace 时通过 `go test ./...` 与 `go build ./...`。宿主注册仍由 Go 组合根显式驱动，不增加 JSON 重复声明模块是否注册或启用。独立 `go.mod` 与自托管 ESM 是构建边界，不足以证明模块已能独立发布：发布验收还需在仓库外的最小宿主中只依赖发布版本，验证 API、清单/静态资源、页面运行和数据初始化。Docker、Cron、ACME、DNS、IPMI、iDRAC、Settings、Widgets、Uptime 与 Terminal 的最小宿主现从临时 Go module proxy 获取按发布 zip 格式打包的模块版本，不含宿主 `replace`，并验证 API、manifest 与嵌入静态资源；Terminal 测试同时从 proxy 获取 `pkg/terminal` 版本，验证其依赖图。该流程验证发布包消费路径，但还不代表公开版本已发布；其他模块的发布包集成验收仍需补齐。

Go 的 `//go:embed` 不能嵌入包目录外的文件。因此目标结构中 `web/` 包嵌入同目录下的 `assets/`，并由 `ServeHTTP` 提供 `/assets/` 文件及 `/manifest.json` 清单；`frontend/embed.go` 只嵌入壳资源。宿主 Go 注册代码分配 HTTP 根和 Hash 页面根，浏览器壳从模块清单获得顶级菜单分组、JS 入口及可选子导航；未注册模块的 handler、导航和 JS 均不挂载或加载。

目录合并不要求把所有代码塞进单一 Go 包：模块内部可继续分包，但不跨到另一个功能模块的数据层。宿主共享的 `frontend/shared`、`frontend/vendor` 与宿主私有的 `internal/host` 由壳提供，避免在每个模块复制 Preact 或基础组件；模块可依赖的 Go 公共库必须放在可独立引用的 `pkg/` 下，不能让模块依赖宿主的 `internal/`。当前 28 个模块的前端已归入各自 `modules/<id>/web`，壳通过服务端公开模块索引和模块清单动态发现并加载模块入口。

### 独立发布的边界

目录合并只是准备条件。独立发布的目标是：把 `modules/<id>/` 移到单独仓库，增加独立 `go.mod` 和版本标签后，宿主只通过版本化 Go 依赖取得该模块；模块的 Go 代码、嵌入式 ESM/CSS、API 契约、迁移、测试和许可证一起交付。无构建前端仍由模块包嵌入，不需要宿主复制该模块的源码或重新打前端包。第一阶段仍采用编译期登记，升级模块版本要重新构建宿主；动态安装和运行时卸载不在本方案内。

为此，模块的公开边界是强类型构造器和构造后返回的 `http.Handler`；该 handler 自行托管相对 API、静态文件和发现清单，确有后台任务时再实现可选生命周期接口。模块 JS 只导出启动入口和相对页面路由。宿主决定模块 HTTP 根路径、页面前缀、侧边栏分组和 JS 加载时机，并提供版本化的共享 UI/HTTP 契约。模块可以依赖明确版本的公共包，但不能 import 宿主的 `internal/`、`server/`、组合根或另一功能模块；浏览器代码也不能用 `../..` 读取宿主源码目录。Cron 调度由 `modules/cron` 提供，终端传输归入 `pkg/terminal`；进一步独立发布时，这类模块共享库仍需作为单独版本化依赖发布。

发布验收要求在仓库外的临时目录创建最小宿主，仅依赖该模块发布版本和其声明的公共依赖：能编译并运行模块 API，挂载嵌入的前端资源，加载 Hash 页面，执行模块测试，并验证迁移与升级。发布时记录宿主兼容范围、API/共享 UI 契约版本及资源清单；不以 `replace` 指回 OhMyServer 工作树，也不要求复制 `frontend/<id>`。在达到这项验收前，模块仍是单仓内聚模块，文档不称它已经能独立发布。

## 4. 服务端模块挂载

每个模块提供强类型的 `New(具体依赖, 模块选项)`，返回 `http.Handler`；同一个 handler 在相对 API 路径、`/assets/`、`/manifest.json` 下分别处理业务 API、前端文件和公开清单。宿主在编译期维护 ID 到构造工厂的映射，从共享数据库等宿主资源中挑选模块专用依赖；构造器不得接收整个 `*server.Server` 或全局 service locator。后台启动与关闭采用可选接口，完整装配契约见 [模块装配与前端清单契约](module-contract.md)。

示意 API（规划代码，不是现有实现）：

```go
// 构造时创建一次 mux，Module.ServeHTTP 委托给它。
func (m *Module) routes() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /containers", m.listContainers)
    mux.HandleFunc("GET /containers/{id}", m.getContainer)
    mux.HandleFunc("POST /containers/{id}/start", m.startContainer)
    mux.HandleFunc("GET /manifest.json", m.serveManifest)
    mux.Handle("/assets/", http.StripPrefix("/assets/", m.assetsHandler()))
    return mux
}

// 壳只设置模块根挂载；模块不硬编码 /v1/docker。
root.Handle("/v1/docker/", http.StripPrefix("/v1/docker", dockerModule))
```

示例中的模块根 `/v1/docker` 由宿主 Go 注册代码分配，API 地址是 `/v1/docker/containers`，模块清单地址是 `/v1/docker/manifest.json`。模块只知道自己的内部相对协议路径；不依赖 HTTP 重定向纠正写操作路径。外壳对重复模块 ID、重复挂载、非法前缀在启动时直接报错。API 的 404/405 必须为 JSON，静态文件缺失保留普通 404，不能回落到 SPA，也不能把已开始的 SSE/文件响应改写成 JSON；中间件需保留 Flush/Hijack 等能力。

生命周期不强制塞进每个模块接口：纯 HTTP 模块只提供 `ServeHTTP`，有后台循环的模块显式 `Start(ctx)` / `Close(ctx)`。外壳按依赖顺序初始化并启动，失败回收已经创建的资源，关机按逆序停止并限定超时。能力不可用（例如 NAS 没有 Docker）应在模块状态中报告，不应让整个面板启动失败；必需基础设施失败则启动失败。

所有模块共用当前应用配置的同一个 SQLite 数据库和连接池。每个模块只拥有自己的表、repository 和版本化迁移；启动外壳统一串行调用各模块迁移。模块代码只能查询或修改本模块表，禁止跨模块 SQL、跨表 join 或绕过对方 API 直接读写数据。跨模块需要关联信息时，通过应用层注入的窄接口查询，不能依赖数据库布局。禁止把 DB 层抽成泛型 CRUD 框架；模块独立时再替换存储实现。

迁移期可让旧 `Server` 暂时实现某个模块所需的小接口，以复用业务；标记为过渡适配，模块迁移验收前移除该模块对旧聚合对象的依赖。

## 5. 前端模块挂载与前缀

宿主 Go 代码注册模块并分配 HTTP 根与 Hash 页面根。模块自己在 `/manifest.json` 提供名称、图标、菜单分组、前端入口、样式与可选子导航，并在 `/assets/` 托管 JS/CSS；完整结构见 [模块装配与前端清单契约](module-contract.md)。模块 JS 只实现业务页面、相对路由和相对 API 调用，不声明宿主前缀。省略子导航时，壳只显示 `#/docker` 这样的顶级入口。

```json
{
  "contractVersion": 1,
  "id": "docker",
  "title": "Docker",
  "icon": "◧",
  "group": "容器与部署",
  "entry": "assets/module.js",
  "styles": ["assets/style.css"],
  "navigation": [{ "title": "容器", "path": "/containers" }]
}
```

这是模块自己的 `/manifest.json` 响应；`entry/styles` 都是相对模块 HTTP 根的 URL 路径（通过 `/assets/` 提供），不能包含宿主挂载前缀。清单中的导航仅表示需要展示的入口，不限定模块 JS 实际支持的全部路由；`home` 指向模块入口的首页路由。

```js
// modules/docker/web/api.js：只使用模块内相对路径
export function createAPI({ request }) {
  return {
    listContainers: ({ signal } = {}) =>
      request('/containers', { signal }),
    startContainer: (id) =>
      request(`/containers/${encodeURIComponent(id)}/start`, {
        method: 'POST',
      }),
  };
}

// modules/docker/web/index.js：壳加载的模块入口
import { createAPI } from './api.js';
export default {
  id: 'docker',
  createAPI,
  routes: [
    { path: '/containers', load: () => import('./pages/containers.js') },
    { path: '/containers/:id', load: () => import('./pages/container.js') },
  ],
};
```

服务端壳从 Go 注册表创建并挂载模块的 handler，向浏览器提供只含已注册模块的公开索引。浏览器壳向每个索引中的同源模块根请求 `/manifest.json`，核对 ID 和契约版本，用清单绘制顶级菜单及可选子导航；当前模块第一次打开时，懒加载清单中的 JS 入口并读取其默认导出的模块对象，再把相对路由挂到宿主分配的页面根下。模块 JS import 时不请求数据、不建立连接，运行时行为在模块和页面生命周期内发生。

壳创建按模块作用域的 `transport`：`request('/containers')` 使用宿主 `mount` 拼接 URL，并统一处理认证、CSRF、JSON 错误和取消。SSE、WebSocket、下载及图片也通过壳提供的相对路径 URL/连接能力取得地址；模块 JS 不自行拼全局前缀。壳还提供相对导航与跨模块链接能力，页面只使用注入的接口。`mount` 与 `page` 可在不同宿主下配置，包括子路径部署；模块源码保持不变。

`frontend/app/modules.js` 从已验证的模块清单取得同源入口 URL，再用原生 `import()` 加载；该 URL 由模块自己的 handler 提供。页面继续相对于模块 `module.js` 懒加载。`frontend/shared` 使用 import map 别名 `@panel/`，模块内部文件用相对 import，跨到共享 UI 时用 `@panel/shared/...`。模块不得 import `app/` 或其他模块的前端文件。独立运行 Docker 时只需模块自身目录和一个实现同一装配/注入契约的最小宿主。同源部署为默认值。

壳从模块清单加载已注册模块的 CSS，`frontend/index.html` 只引用壳层与 vendor 样式。模块样式与页面一起迁移，不能在壳的 HTML 中长期维护模块 CSS 清单。清单 ID 必须与 Go 注册 ID 匹配，入口/样式必须在模块自己的挂载根内；清单请求或入口加载失败时只影响该模块。

Hash 路由由外壳唯一管理：URL 形如 `/#/docker/containers`，监听 `hashchange` 并提供 `navigate()`。查询参数位于 hash 内，例如 `#/docker/containers?state=running`，用 URLSearchParams 解析；刷新和浏览器前进/后退由浏览器原生 hash 历史支持，不要求服务端为每个页面路径配置 SPA fallback。静态段优先于参数段，按路径段匹配避免 `/docker-other` 命中 `/docker`，未知路径显示 404。拦截仅限无修饰键的站内导航，保留下载、外链、新标签页和 `method="delete"` 的语义。

页面导出默认组件，接收 `{ api, params, search, navigate, href, hrefTo }`。其中 `navigate`/`href` 只处理本模块路由，`hrefTo(moduleID, path)` 仅用于显式跨模块链接。`useEffect` 中的请求使用 AbortController，卸载关闭轮询/SSE/WebSocket/终端实例；快速切页时旧响应不得覆盖新页面。列表筛选、分页进入 URL；表单和短期加载状态留在组件，应用 Context 只放会话、模块挂载信息和少量全局 UI 状态。

壳层处理模块加载中、模块加载失败、未知路由和 Error Boundary。侧边栏分组、模块名称/图标与可选子导航来自模块清单；未注册模块不显示，单模块清单或入口失败不影响其他模块并显示该模块错误态。切页更新标题与主区域焦点，移动端收起菜单；复用小窗口/全视口布局，内容区必须可独立滚动。

## 6. HTTP 数据契约

| 场景 | 约定 |
| --- | --- |
| 查询单项 | `GET /v1/<module>/<resource>/{id}` → `200 {"data": {...}}` |
| 列表 | `GET ...?limit=50&offset=0` → `{"data": [], "meta": {"limit":50,"offset":0,"has_more":false}}` |
| 创建 | `POST` JSON → `201` + data + 使用实际 mount 生成的 Location |
| 修改 | `PATCH` JSON，字段缺省保留；确需完整替换才使用 PUT |
| 删除 | `DELETE` → `204`，客户端不能继续解析 JSON |
| 动作 | `POST /.../{id}/start`、`/renew`、`/test`；GET 不产生操作副作用 |
| 长任务 | `202 {"data":{"id":"...","state":"queued"}}` + Location 指向任务资源 |
| 错误 | `{"error":{"code":"validation_failed","message":"...","fields":{"name":"..."},"request_id":"..."}}` |

JSON 字段 snake_case，时间 RFC3339 UTC，标识符作为不透明字符串，空列表用 `[]`，可空值明确为 `null`。分页默认 50、上限 200，确定性排序；`total` 仅在易于准确取得时返回。已有小型无分页资源可声明例外，但不能默默截断数据。

按语义使用 400（无效输入）、401（未认证）、403（无权限/CSRF）、404、409（状态冲突）、413、422（字段校验）、503（能力不可用）。校验请求 Content-Type、限制 body、拒绝未知写入字段；DTO 白名单序列化，密码/密钥不直接返回。密码修改使用“缺省保留、明确提供才替换”，不靠空字符串猜测操作意图。

客户端 `request` 负责 JSON、credentials、CSRF、错误归一化和请求取消。只对适合重试的读取做有限重试；创建、安装、重启、续签不自动重试。模块 `openapi.yaml` 与 handler 一起更新；壳层聚合文档，前端不依赖代码生成才能运行。

## 7. 会话、文件和流式任务

登录认证不作为本轮前端/API 重构的前置条件，迁移接口阶段先按当前部署行为运行。后续需要认证时，再单独设计 `/v1/auth/`：`POST /login`、`POST /logout`、`GET /session`，以及会话轮换、CSRF、Origin 和终端握手校验；在安全实现完成前，不在前端保存密钥。

Cookie 认证的写请求使用 CSRF token 与可信 Origin 校验；登录同样校验来源，不能只依赖 SameSite。默认不开放跨域，跨域凭据仅允许明确配置的来源。WebSocket 握手校验会话和 Origin；不把长期密钥放 URL、localStorage 或日志。

- 长任务：POST 创建，再 GET 状态/SSE；GET SSE 不启动安装或部署。任务归属功能模块，如 `/v1/docker/tasks/{id}`，统一状态 `queued/running/succeeded/failed/cancelled`，记录结果、错误、时间与可否取消。重启后任务必须恢复或明确标记中断，不能永久停在 running。
- SSE：`/tasks/{id}/events` 使用 `text/event-stream`，事件含序号、类型和时间；支持 Last-Event-ID/已过期游标提示、心跳与有界缓冲。客户端断开只取消订阅，不隐式取消后台任务。新建取消操作用 POST，明确是否真正可取消。
- 终端：保留现有终端协议与平台适配，统一挂 `/v1/terminal/` 的本机、SSH、Docker 会话资源；调用方通过窄 `Session` 能力交付后端会话，不让终端模块查 SSH/Docker 表。WS 升级失败先返回 HTTP 错误，升级后用协议错误/关闭码。页面退出清理连接；重连不能重复创建远端进程。
- 上传下载：multipart/二进制响应，不强行套 JSON envelope；保留文件名、大小限制、路径边界、Content-Disposition 与必要的 Range。错误仍按尚未开始响应时的 API 契约返回。
- webhook、ACME HTTP-01、DNS DoH、agent/telegraf 入口属于外部协议路由，不改成 SPA，也不强套用户登录会话；各自使用签名、挑战或 agent 身份校验。

## 8. 模块边界与联动

| 模块 ID / 目录 | 页面默认前缀 | API 前缀 | 主要职责 |
| --- | --- | --- | --- |
| system | `/about` | `/v1/system` | 版本、更新、模块状态 |
| dashboard | `/` | `/v1/dashboard` | 首页只读聚合 |
| auth | `/login` | `/v1/auth` | 登录与会话 |
| docker | `/docker` | `/v1/docker` | containers/images/volumes/networks/compose/apps |
| acme | `/acme` | `/v1/acme` | accounts/certificates/orders/issuances |
| dns、dhcp、ntp | 各自 `/<id>` | 各自 `/v1/<id>` | 对应网络服务 |
| webserver | `/web` | `/v1/webserver` | listeners/policies/hosts/routes；内部使用 yabber |
| monitor | `/monitor` | `/v1/monitor` | 主机监控与历史 |
| uptime | `/uptime` | `/v1/uptime` | 可用性监测目标与检查历史 |
| forward | `/forward` | `/v1/forward` | TCP/UDP 转发监听器 |
| ssh、terminal | 各自 `/<id>` | 各自 `/v1/<id>` | 连接管理、终端会话 |
| projects、cron | 各自 `/<id>` | 各自 `/v1/<id>` | 部署、调度 |
| files、disks、packages | 各自 `/<id>` | 各自 `/v1/<id>` | 文件、磁盘、软件包 |
| settings、widgets | 各自 `/<id>` | 各自 `/v1/<id>` | 全局配置、首页组件 |
| clash、openwrt、yeelight | 各自 `/<id>` | 各自 `/v1/<id>` | 对应功能 |
| ipmi、idrac、esxi、pve、speedtest | 各自 `/<id>` | 各自 `/v1/<id>` | 对应设备与工具 |

ACME/DNS/webserver 是首先明确的联动边界：webserver 消费证书读取接口，外壳用 ACME 适配器实现；ACME 消费 DNS challenge 接口，外壳注入 DNS 适配器。续签完成通过明确回调触发 webserver 证书刷新，不让 webserver 直接查询 `acme/certificates` 表。网页证书选择器只拿摘要和 ID，私钥不进入浏览器。

前端跨模块链接通过外壳 `href(moduleID, relativePath)` 生成；确需跨模块数据用显式注入的只读能力或组合 API。主页聚合由 dashboard/application 层完成。模块缺失时展示能力不可用，不让单个模块 import 失败导致全站白屏。

## 9. 无构建交付、缓存和本地开发

生产默认将固定版本 ESM、样式、字体和必要终端依赖放入 vendor 并嵌入 Go 二进制，保证路由器/NAS 无公网也能打开。下载既有 ESM 分发文件而非自行编译，核对所有传递依赖、相对路径、bare specifier、许可证与校验和。公共 CSS 也锁定快照，避免上游删改布局让已发布版本失效。

`frontend/embed.go` 只嵌入壳层、共享 UI 和 vendor；各 `modules/<id>/assets.go` 各自嵌入本模块 `web/`。测试、node_modules、临时文件不嵌入。开发模式分别从指定的 frontend 壳目录和模块 `web/` 目录读取，保存后刷新即可；生产读取各自的 embed.FS。测试可使用 Node/浏览器工具，但 `go build` 和运行二进制均不要求 npm install。

首期所有 ESM/CSS 返回 `Cache-Control: no-cache` + 基于内容的 ETag；index/session/module 配置不缓存或必须重新验证，避免单给入口 `?v=` 却缓存住相对 import。后续若需要 immutable 缓存，应对整个资源图使用 `/frontend/<release>/...` 路径，包括 import map 和 CSS 的相对资源；不是只改入口版本。避免 Service Worker 增加更新复杂度。

服务端仅为已挂载的页面前缀且接受 HTML 的 GET/HEAD 提供 SPA index；静态缺失返回 404，API 缺失返回 JSON 404，外部协议路由维持各自语义。页面深链接刷新必须可用。`frontend/index.html` 的静态 CSS、import map 和入口脚本相对于 `<base href>`，默认根路径；`base_path: /panel` 会将 UI、静态资源、模块 API、认证及外部回调统一挂到此前缀。模块 API 根由宿主 Go 注册代码提供给浏览器壳，不由模块 JS 自行推断；页面内部采用 Hash 路由。

## 10. 分阶段实施与完成标准

### 创建与编辑页面约定

- 所有会创建持久化资源的表单都必须有独立页面路由；列表页只在右上角提供“新建/添加”入口，不内嵌创建表单、折叠创建表单或 inline-create 行。
- 子资源创建也使用独立页面，并保留父资源上下文，例如 `#/webserver/hosts/{hostID}/routes/new`、`#/dns/zones/{zoneID}/records/new`；提交成功后返回父资源详情或资源列表。
- 创建页提供明确的返回/取消导航，校验和错误信息在创建页显示。创建成功后可展示一次性结果（如 Webhook secret），再返回列表/详情。
- 编辑是独立的更新流程，不要求为本约定一并重构；但不得将“新建/编辑”合并为列表页内联表单。若编辑当前也 inline，可单独排期拆页。
- 文件管理的文件/目录创建及上传使用带当前目录上下文的独立页面；纯动作（拉取镜像、执行部署、安装应用、运行检查）不是资源创建表单，不受此约定约束。

验收时检查所有列表型模块的创建入口与创建页路由；对详情页中的子资源新增入口也做同样检查。创建按钮统一放在列表页 toolbar 右侧，并保留 `class="button"` 基础类；所有输入继续使用 `class="input"`。

每阶段独立提交；可以删掉已迁移模块的旧 API/表单操作，不维护双份永久兼容包装。迁移期只保留尚未迁移模块的旧页面；这是交付策略，不是新架构的依赖。

### 阶段 0：冻结契约和功能清单

- 逐项登记 main.go 路由、模板、页面动作、流式接口、权限、数据库表和外部入口，含隐藏导航模块。
- 定稿本文前缀、DTO、模块注册与会话契约；标记每条旧路由的替换和验收项。
- 更新 AGENTS.md：新前端按模块注册，新后端按模块 Handler 挂载；旧模板流程只适用于未迁移部分。保留 input/button 基础类、简洁 CSS 和 DELETE 要求。

完成：每个现有功能都能追踪到目标模块；不是以页面截图相似作为唯一验收。

### 实施进度（2026-09-24）

- [x] `AGENTS.md` 已将模块化 API/ESM 开发规范设为新功能默认，并将旧模板流程限制为尚未迁移模块；保留 input/button、DELETE 与 CSS 规范。`frontend/app/form-classes.test.js` 遍历模块 ESM/HTML，检查所有 input/button 都包含基础样式类。
- [x] 前端 Preact + HTM 原生 ESM 壳、固定版本本地 vendor、Hash 路由、模块分组导航与前端模块注册表。
- [x] 本地真实浏览器加载根页面时验证 import map、Preact 壳、模块懒加载、Docker Hash 页面及深链刷新；发现并修复相对静态资源迁移时 import map 目标缺少 `./` 导致浏览器将 `preact` 映射为 null。该静态 smoke 的 `/v1` 请求由文件服务器返回 404，因此不视为 API 页面 E2E。
- [x] 静态 CSS、import map 与入口脚本改为 `<base href>` 相对路径，新增根部署和 `/panel/` URL 解析测试；完整的反向代理/子路径运行部署仍待集成验证。
- [x] Hash 路由静态段优先于参数段；路径先分段后解码，避免静态路由被 `:id` 抢占及编码斜杠改变段边界，并由路由单测覆盖。
- [x] 真实浏览器 smoke test 已验证无构建 ESM/Preact 壳、Hash 路由切换及 390px 手机视口下内容可见、菜单选路后自动收起。共享 `moduleweb` runtime 的路由与视图测试覆盖不同模块和页面不会复用旧组件。Dashboard 另通过 mock API 验证四类系统状态和组件数据能渲染，桌面与 390px 视口均无横向溢出。移动端 shell 使用独立网格行分隔导航与主内容，避免导航挤占或遮挡内容。这些浏览器测试仍不替代带隔离数据库的应用 API 联调和各设备实机验证。
- [x] 新增当前 `/v1` 模块 API 参考，并将原 `api-reference.md` 标注为迁移前历史文档，避免旧路径被误认为仍受支持。
- [x] 新增真实 `NewServer` + 隔离临时 SQLite 的组合层集成测试，验证 SPA 嵌入资源、Widgets API 与 Dashboard 实时 overview API 在同一应用路由器中工作；测试同时确认 `Server.Close` 可取消并等待 Monitor 模块采集/清理后台循环，消除原来无生命周期的 `context.Background()` goroutine。宿主 schema 初始化、旧数据迁移、服务启动和关闭顺序现显式列在 `internal/host/lifecycle.go`，不再由多个文件的包级 `init()` 隐式注册；关闭按声明顺序逆向协调并汇总错误。主 HTTP 服务响应 SIGINT/SIGTERM 后先执行有 10 秒上限的 `http.Server.Shutdown`，再调用幂等的 `Server.Close` 清理模块资源。loopback 测试验证 HTTP 取消后不再接受连接，架构测试禁止恢复包级生命周期注册。
- [x] Dashboard 通过 `/v1/dashboard/overview` 由应用层组合已启用 Widgets 与实时本机指标，Dashboard 模块只消费自己的 DTO，不直接依赖 Widgets 或 Monitor 仓储；首页提供 CPU、内存、磁盘、运行时间和负载摘要及显式刷新。HTTP handler 与资源清单位于 `modules/dashboard/web/`，壳加载 `module.js` 与 standalone `index.html`/`index.js` 均位于 `web/assets/`；新增 `LocalService` 由窄 Widget/System source 组合数据，`cmd/dashboard web serve` 独立采集本机指标并提供概览。
- [~] 挂载 `/v1/<module>` 的 Go HTTP 模块注册器；Docker 容器列表/详情/生命周期/创建/最近日志、镜像列表/删除/拉取/三种来源构建进度、存储卷和网络的列表/创建/删除已有 API 及页面。旧 Docker handler 与页面路由已移除；Docker API 契约测试覆盖容器创建输入校验/归一化、Compose 与应用动作白名单、镜像拉取 SSE 进度及终态。隔离 Docker HTTP daemon 测试通过真实 SDK 与 `/v1/docker` 覆盖容器列表/inspect/logs/start/create/delete、镜像列表/pull/Dockerfile build/delete、卷和网络列表/create/delete；另有同类隔离 SDK 测试验证 stats daemon JSON 解码与 CPU/内存/网络指标换算。build 测试检查 Dockerfile、目录与 Git 仓库 context 都正确打包且不包含 `.git`；真实浏览器已用本机 daemon 从目录和本地 Git 仓库构建 `FROM scratch` 镜像，并通过镜像列表删除临时镜像。SDK 将默认库镜像拉取规范化成 `fromImage=alpine&tag=latest`。Docker SDK client 建立与容器/镜像/卷/网络协议操作现归 `modules/docker`，壳层委托这些操作；浏览器模块资源已集中到 `modules/docker/web/assets`。`modules/docker.LocalService` 现统一承载 SDK、Compose、stats 与应用商店能力，壳 API 适配器与独立命令复用该服务；`cmd/docker web serve`、manifest 和独立 UI 已提供，handler/assets 有模块测试。独立命令对真实 Docker daemon 的启动与完整操作流程仍待实机验证。ACME 证书摘要另有 `/v1/acme/certificates` API。
- [x] 静态 ESM 返回 `Cache-Control: no-cache`；未知 API 模块返回 JSON 404。
- [x] 创建表单独立页面约定已写入本规范与 `AGENTS.md`；设置、Widgets、Uptime、Forward、ACME provider、DHCP static lease、DNS zone/provider/filter/subscription/record、Cron job、Project、SSH server、Web Server 子资源和 Files 的创建入口均已迁移。SSH 新建/编辑及 DNS provider record 编辑路由也有回归检查；这次还将 DNS provider 区域详情里的行内记录编辑表单拆为独立页面。DNS settings 属于原位配置编辑，不属于资源创建表单。
- [x] 浏览器实载验证 `#/cron/new`、`#/settings/new`、`#/dns/zones/new` 的懒加载路由、表单控件和页面结构；这次只检查桌面 DOM 可访问性/渲染，移动布局和创建提交成功流仍需专项验证。
- [x] 旧功能 HTML 不再编入服务端二进制：`web.Files` 仅嵌入认证/错误所需的根模板，以及旧壳实际使用的 `main.css`、`stats.css`、`utils.js`；未使用的旧页面 CSS/JS 不再进入二进制，测试同时锁定模板和资源清单。
- [x] 模块内未知 endpoint 与不支持的 method 由挂载层统一转为 JSON 404/405；成功 SSE 响应和 WebSocket hijack 能力透传。composition root 暴露稳定排序的模块 ID，应用集成测试自动比较后端 API modules 与嵌入式前端 ESM manifest，防止增删模块时前后端清单漂移。
- [x] `internal/architecture` 递归检查模块源码：从 SQL schema 声明自动发现表归属，验证查询/写入/JOIN/外键、ALTER/DROP 迁移和直接 PRAGMA 表检查只触及本模块表；静态分析也拒绝拼接或 `fmt.Sprintf` 构造动态 SQL 表标识符；禁止模块实现 import 其他 feature module、`server` 或 `internal/app` composition root，也禁止在生产模块中创建脱离调用链的 `context.Background/TODO`；每个模块 `Service` 方法首参数必须为 `context.Context`，跨模块组合只能在应用层完成。新增检查确保旧 `server/` 聚合层不直接访问任何已归属模块表，只有逐文件列明的退役 schema 迁移例外可触碰旧数据。
- [~] Docker Compose 的列表/详情/创建与 up/down/restart/pull 已迁入 `/v1/docker/compose`，独立 Hash 页面已接入；容器 CPU/内存/网络实时 stats 已迁入 SSE API 并接入详情页。应用商店 `app_templates` schema、清单调和查询已迁入 Docker 仓储，`/v1/docker/apps` 已提供列表/分类/同步/详情/图标/安装任务与动作 API，Preact 页面已接入；镜像 Dockerfile/目录/Git 构建也有可取消 API SSE 和 Hash 表单。旧 Docker 管理路由已移除，未使用的旧页面模板及主要页面 handler 已清理；容器列表/inspect/logs/lifecycle/create/delete、镜像列表/pull/Dockerfile-build/delete、volume/network CRUD 有隔离 SDK 集成覆盖。此前真实 Git 构建因未设置 clone 后的 context 目录必然失败，现已修复并由本地 Git+mock daemon 测试和本机浏览器+Docker daemon 双重验证。Compose 增加 V1 JSONL/V2 JSON 数组解码、请求 context 取消、独立 CLI/`docker compose` 插件回退和隔离 CLI 生命周期测试；浏览器真实访问 Compose 列表 API（200）并检查多来源表单。模块 LocalService 与 standalone UI 现已接入同一 Compose/stats/应用商店服务；实际创建/停止容器的 Compose 行为、stats SSE 和应用商店安装仍待 daemon 行为验证。
- [x] Speedtest 历史结果 schema/CRUD 已迁入 `modules/speedtest.Repository`；`/v1/speedtest` 提供历史、测速服务器、删除与实时运行 API，Hash 页面已接入测速选择、进度流与历史管理。模块 API 测试覆盖 server type 校验、删除 ID 校验、运行委派、请求 context 透传及服务错误 JSON。客户端断开后停止写 SSE、启动后续测速阶段和持久化结果；上游测速库的单阶段网络调用不接受 context，暂时不能保证立即中断。
- [x] ACME `certificates`、`acme_providers`、`acme_accounts` 及订单/签发记录的 schema/CRUD 由 `modules/acme/web.Repository` 持有；模块根 Go 包只定义领域类型、ACME 引擎和仓储能力接口，不实现 SQLite 操作。ACME API 与独立命令使用 Web 包仓储，Web Server 通过 `webCertificateSource` 窄能力消费摘要/按需 TLS 材料，不查 ACME 表。`Repository.InitializeModule` 统一初始化 ACME 所有内部表；SQLite 测试验证全量且可重复初始化。
- [~] `/v1/acme/providers`、`/v1/acme/accounts` 列表及服务商新增/删除、账户注册/删除 API 与前端操作已接入；账户私钥字段不序列化，注册时服务端获取 Directory 并验证服务条款确认。CSR 创建、证书详情、ZIP 下载也已接入 `/v1/acme` 与 Preact；详情 JSON 不含私钥，旧 ACME handler 源码已清理，端到端回归仍待完成。
- [x] 系统信息与版本检查/更新已迁到 `/v1/system` 和独立 Preact 页面；旧 `/about` 模板/API 路由已移除，更新任务通过 POST API 排队。`modules/system/web/assets` 现在同时托管壳清单资源与独立页面，并提供 `cmd/system web serve`；宿主更新能力仍通过 service 接口注入，独立命令只提供系统信息，不假装具备宿主升级能力。
- [x] Packages 的包管理器实现已归 `modules/packages/pkgmgr`，catalog/变更逻辑位于模块 `service`，Web API、manifest 与静态资源位于 `web/`、`web/assets/`；独立 `cmd/packages` 提供 `packages web serve`。模块文档说明系统包管理器需要 root 或无交互的 sudo/doas 策略。测试覆盖管理器解析/API 合同和 standalone 路由；独立模块 `GOWORK=off` 测试、构建、vet 通过，并以 Homebrew 在本机启动命令验证 HTML、manifest、模块入口、standalone UI 和 catalog API。
- [x] Disks 的本机分区用量、块设备枚举和 SMART client/service 已迁入 `modules/disks`；API handler 与嵌入资源位于 `web/`，新增独立 Preact 页面和 `cmd/disks web serve`。shell 组合根直接装配模块 service，不再依赖 `server.Server` 的磁盘采集实现。真实硬件 SMART 检查依赖宿主的 `smartctl` 工具。
- [x] NTP 的 settings repository、UDP runtime 与校验均归模块；API handler、资产和 standalone 管理页在 `web/`，并新增 `cmd/ntp web serve`。壳应用继续注入生命周期 service adapter，以参与宿主统一监听冲突与服务策略协调。
- [~] Files 的本地文件系统能力已独立为 `modules/files`，`/v1/files` 提供目录浏览、隐藏文件、下载、创建文件/目录、上传和删除；Hash 页面已接入。适配器新增 `NewLocalFilesystemAt(root)` 能力构造器，以 `os.Root` 执行受根目录约束的 I/O，所有路径解析及软链接目标均限制在授权根内，删除根内软链接只删除链接本身；本地操作及上传流会响应调用 context 取消。API 契约测试覆盖根目录越界、软链接逃逸、根删除保护、取消请求、上传/列表、Range 下载、Content-Disposition、错误映射及超限请求。浏览器在唯一 `/tmp` 测试目录实测目录/文件创建、上传、下载内容一致、文件删除与空目录删除，并确认删除后列表中消失且无 console error；390px/320px 下表格仅在局部容器滚动、整页无溢出。面板组合根当前明确授予 `/`，部署级授权根配置仍待设计。
- [~] SSH 服务器配置已有独立 `/v1/ssh/servers` CRUD 与连接测试 API 和 Preact 列表/新增/编辑页面；响应仅暴露凭据是否存在，不返回密码/私钥，更新可省略未变凭据，主机或端口变化会清除 TOFU Host Key。模块测试覆盖写入校验、默认端口、凭据不回传、连接错误映射及请求 context 透传。`ssh_servers` schema/CRUD/Host Key pinning 已归 `modules/ssh/web.Repository` 所有；CRUD 与测试/统计服务均使用请求 context，取消请求可传递至仓储。旧 SSH 配置 HTML 路由与模板已移除。Stats SSE (`/servers/{id}/stats`) 与 Prometheus 文本指标 (`/servers/{id}/metrics`) 已迁至 SSH 模块 API，主机详情 Hash 页已接入实时指标与 metrics 导出链接。SSH `web.LocalModule` 统一创建 repository、service 与 handler，宿主与 `cmd/ssh web serve` 共用该组装入口；SSH transport client、context-aware command execution 与 PTY session 位于 `modules/ssh/client.go`。宿主适配器只保留 Monitor 持续采集和终端连接装配。
- [~] Terminal 的协议/平台能力位于 `pkg/terminal`，HTTP、repository（如有）与资源托管位于 `modules/terminal/web`；宿主注入本机、SSH、Docker 会话 opener，模块提供 `/v1/terminal/local/ws`、`/ssh/{id}/ws`、`/docker/{id}/ws` 和模块清单/JS。独立 `cmd/terminal web serve` 当前提供本机 shell 与嵌入式 UI，SSH/Docker 会话依赖宿主适配器。旧 Terminal HTML/WebSocket 页面路由已移除。自动测试覆盖 websocket Origin、类型/尺寸委派、打开失败帧、Bridge 生命周期及独立 UI/清单/JS 资源；默认仅允许同 scheme+host，反向代理例外须由宿主显式注入 Origin policy。浏览器端 xterm.js/FitAddon 仍使用固定版本 CDN；SSH/Docker 独立适配与 Windows/Linux 原生运行验证待完成。
- [~] Projects 的项目/部署 schema、repository、项目 CRUD、branch 与 webhook secret 校验、部署管理器、GitHub webhook 端点及 git/build/deploy 执行器均已归 `modules/projects/web`；宿主仅注入 Docker runtime。`/v1/projects` 提供项目基础管理、运行状态、部署历史/详情/日志、触发/取消部署、Webhook Secret 轮换、GitHub webhook 与支持 Last-Event-ID 的部署 SSE，Hash 页面已接入，旧项目模板、HTML handler 和 `/projects/...` 页面路由已移除。新增 standalone UI 和 `cmd/projects web serve`，用 Docker CLI 提供本机独立 runtime；独立 HTTP 测试覆盖首页、manifest、shell/standalone JS、CSS 与项目 API。真实 Docker/Git 部署 smoke test、仓库外无 `replace` 的发布宿主验收仍待完成。
- [x] Cron 的 schema、任务/运行记录 CRUD、命令执行和调度均由 `modules/cron/web` 管理；`/v1/cron/jobs` 提供 CRUD、启停、立即运行及执行记录，Hash 页面已接入。模块 `LocalModule` 统一组合仓储、引擎、handler 与启动/关闭；宿主和 `cmd/cron` 都复用它，应用层仅注入数据库/runner 并协调生命周期。旧 `/cron` HTML handler/template 已删除。
- [x] ACME `orders`、`authorizations`、`challenges`、`issuances` schema/仓储操作已迁入 `modules/acme.Repository`；其相互关联只在 ACME 模块内部维护。启动适配器在 schema 初始化后显式调用 `RecoverInterruptedIssuances`，schema 创建本身可重复执行且不会触发恢复副作用。
- [~] ACME HTTP-01/DNS-01 签发由 `/v1/acme/issuances` API 创建并查询；证书详情现可选择有效账户和挑战方式发起续签，API 复用模块的并发续签保护，续签失败保留现有 active 证书，成功后才替换证书材料。成功持久化后由宿主注入的更新回调重新应用启用中的 Web Server 配置；模块不依赖 Web Server。ACME 仓储只管理本模块数据，Server 作为应用适配器组合 DNS challenge 能力并运行后台签发器；HTTP-01 handler 与协议测试位于 `modules/acme/web`。续签调度、挑战器完全抽离及真实 CA/DNS 端到端回归仍待后续。
- [x] Settings schema/CRUD 已迁入独立 `modules/settings`，并挂载 `/v1/settings/` API；Hash 页面可读取、新增、更新、删除设置，模块测试覆盖类型校验及 CRUD，旧页面 handler、模板及兼容路由已清理。
- [x] DHCP settings/static leases/dynamic leases schema 与仓储已迁入 `modules/dhcp`；旧表 `dhcp_reservations` 由 Server 应用层一次性读取、交给模块仓储导入并删除，DHCP 模块不访问遗留表。`/v1/dhcp` 提供配置读取/更新、静态/动态租约读取与删除、静态租约创建、运行状态及 start/stop/reload API，Preact 页面已接入这些操作。`web.LocalModule` 统一创建 repository、本地 runtime service 和 handler；宿主可注入共享策略适配器，独立命令使用模块默认 service。
- [~] DHCP 页面现覆盖配置/状态、动态租约、静态租约增删改；静态编辑使用 PUT API。配置和静态/动态租约变更已把请求 context 传入模块服务与仓储，DHCP 配置取消测试确认未写入；服务重载前会检查 context。旧 `/dhcp...` HTML 路由、DHCP 页面 handler 与模板已清理；API 测试覆盖 context 透传/取消、配置读取/更新、静态租约 CRUD、服务动作和错误响应。DHCP 协议 packet handler、租约池恢复与 UDP runtime 已迁入模块；HTTP API、SQLite repository、运行时和独立 Preact 页面现位于 `modules/dhcp/web`，模块根包仅保留领域 DTO 与校验函数。`cmd/dhcp web serve` 独立命令使用 web 包装配。壳层只负责服务偏好、冲突检测和进程生命周期装配。独立命令实测可提供页面、清单与 settings API；真实 DHCP 网络环境验证仍待补。
- [x] NTP 设置 schema/仓储已迁入 `modules/ntp`，`/v1/ntp` 暴露配置、运行状态与服务 start/stop/reload；Preact 页面已接入，配置仍复用原有合法性、监听冲突检查和失败回滚逻辑。
- [~] NTP 配置、状态和 start/stop/reload 已由模块 API 与 Hash 页面承接；配置更新 API 将请求 context 传至仓储读写，取消请求时不应用设置。`modules/ntp/web.LocalModule` 统一构造 SQLite repository、NTP LocalService 与 handler，宿主和 `cmd/ntp` 复用同一装配入口；宿主仍通过策略回调协调共享服务偏好及端口冲突，并负责服务启停。旧 `/ntp...` HTML 路由、handler 和模板已清理；API、策略回滚与 LocalModule 资源测试覆盖配置取消、动作、策略失败和 handler 装配。
- [~] DNS 根包提供领域 DTO；provider context-aware 契约、Cloudflare/DNSPod/Name.com 实现和工厂均位于 `modules/dns/providers`，不再通过本地 replace 引入模块外契约；ACME DNS-01 使用 ACME 自身的窄 challenge 能力，由壳层组合 DNS provider；HTTP handler 内直接通过 `modules/dns/web` 解析凭据、访问 provider API 并处理记录增删改，壳层不再实现这些 provider API 用例。HTTP handler、静态资源（`web/assets/`）与 SQLite repository 位于 `modules/dns/web`，DNS 配置组装、订阅刷新、UDP/TCP/DoT/DoH listener、状态与关闭由 `modules/dns/web.LocalService` 组合 `modules/dns/runtime` 实现。宿主现只通过 `RunPolicy` 注入全局启停偏好；服务 API、配置 reload 和定时刷新均复用同一模块 service。独立 `LocalService`、单页入口和 `cmd/dns web serve` 已实现；`InitializeModule` 单入口初始化，SQLite 测试验证全表集与重复初始化。旧 `dns_providers` 表只由 Server 应用层读取并导入模块仓储，DNS 模块不访问遗留表。模块托管与挑战记录能力已具备，宿主启动/错误状态协调、provider 真实账号端到端验证及浏览器页面自动化仍待完成。
- [x] `dns_settings` 与默认端口迁移版本记录也已迁入 DNS 仓储；`/v1/dns/settings` 支持读取/更新，`/v1/dns/status` 与 `/v1/dns/action` 提供状态及 start/stop/reload，配置更新保留 DNS 上游/监听校验、冲突检查与回滚；前端已接入配置操作。
- [x] DNS 本地 zones/records schema 与 CRUD 已迁入 DNS 仓储；`/v1/dns/zones`、`/v1/dns/zones/{id}/records` 和 record mutation API 已挂载，Preact 页面可创建 zone 与解析记录、查看及删除记录；记录写入由 DNS 业务适配器校验语法并 reload，API 测试覆盖区域/记录创建与查询及无效 ID。
- [x] DNS subscriptions/rules 的 schema、CRUD 与刷新状态写入已归 DNS 仓储所有；`/v1/dns/subscriptions` 和 `/v1/dns/rules` 支持管理订阅及自定义规则，Preact 过滤页已接入；下载/解析/reload 仍由过渡业务适配器完成。
- [x] ACME DNS-01 执行流程通过 `modules/acme.DNSChallenge` 窄接口使用 DNS 能力；provider 区域枚举、目标编码和 TXT 记录呈现/清理现由 `modules/dns/web.LocalService` 提供，应用层 adapter 只做两个模块 DTO 的映射及 ACME 相对记录名计算。ACME 在签发时重新解析已选择的目标，目标失效时停止签发。挑战目标与记录操作传递调用 context，取消后的清理使用有超时的脱离取消上下文；底层 provider 网络接口尚不能保证立即中断。
- [x] ACME 发证 API 与签发服务由组合根直接注册 `modules/acme.Engine`；证书 DTO 与证书属性解析使用模块定义，移除了 `server/` 中重复的证书模型及未被应用使用的 ACME service 转发方法。宿主仅保留 ACME↔DNS challenge adapter、ACME↔Web Server HTTP-01/证书能力适配，以及旧数据库调用仍依赖的组装入口。
- [x] System 的 HTTP API、清单/资源托管和独立 handler 已归 `modules/system/web`；模块根包拥有运行时系统信息组装与 context-aware `Service`，版本/配置元数据及更新调度由宿主注入，standalone 命令与组合根复用同一服务实现。
- [~] OpenWrt 的 `openwrt_servers` schema/CRUD、HTTP API 与 vendor client service 已归入 `modules/openwrt/web`；根包保留公开 DTO。HTTP handler 自托管 `web/assets/` 下的 shell manifest、ESM 与 standalone 页面；`/v1/openwrt/routers` 提供路由器 CRUD、连接测试、概览/网络/DHCP/软件包/防火墙/服务读取与重启/服务重启动作，独立 Hash 页面已接入。模块测试覆盖默认 HTTPS 端口、口令不回传/更新保留、section 路由及非法动作拦截；`cmd/openwrt web serve` 可独立启动并实测页面、清单、JS 与 API。旧 OpenWrt handler、模板及兼容路由已清理；真实路由器环境验证待补。
- [~] `system_stats` schema、兼容迁移、统计写入/历史查询/主机汇总/过期清理已归 `modules/monitor/web.Repository` 所有；`/v1/monitor` 提供主机列表、单机历史（长区间按时间桶采样）和本机即时指标。模块测试覆盖主机聚合、历史时长限制、即时采集和 collector 错误。主机列表/详情 Hash 页面已接入原先指标及 CPU/内存/磁盘/网络历史曲线，旧监控页面与查询 handler 已移除。Telegraf `/monitor/telegraf` 外部上报端点和 `/v1/monitor/telegraf` 模块 API 均委托 monitor web repository 存储；旧解析子包中的重复 standalone HTTP listener 已清理，Telegraf JSON 解析仍作为协议辅助包保留；SSH 主机采集通过宿主注入的窄 adapter 提供；周期采集、retention 清理及 worker start/close 生命周期现由 Monitor 模块管理，流式/真实多主机验证待补。 Monitor API/repository、Telegraf HTTP 接收器和静态资产现位于 `modules/monitor/web`；本机采集、sysinfo 与 Telegraf JSON 解析均归 `modules/monitor`，并新增 `cmd/monitor web serve` standalone 入口。
- [x] Uptime 的监测执行与领域 DTO 位于公共 Go 包；`monitors`/`monitor_checks` schema、repository、HTTP/API 和嵌入资产均位于 `modules/uptime/web`。监测器使用窄 `CheckRepository` 写入检查结果；壳和 `cmd/uptime web serve` 都由 web repository 装配。独立 UI 使用 Preact + HTM，manifest 指向壳加载的 `module.js`。测试覆盖 API 与 standalone 静态路由。
- [x] TCP/UDP 转发引擎已并入 `modules/forward/proxy`，`tcp_listeners` repository、HTTP handler 与资产分别归 `web/`；standalone LocalService 管理真实 TCP/UDP listener，新增独立页面与 `cmd/forward web serve`。壳应用仍装配自身 service adapter，用于统一端口冲突检查及宿主生命周期策略；引擎 listener 上下文由调用方传入。
- [x] Settings 的实体/存储契约位于模块 Go 库，SQLite repository 与 API handler 分别位于 `web/repository.go` 和 `web/server.go`；Web 自托管 shell module ESM 与 standalone Preact 页面，`cmd/settings web serve` 可独立启动，宿主通过窄 repository getter 注入共享数据库。
- [x] Widgets 的实体与 repository 契约位于模块 Go 库，schema/CRUD 实现在 `modules/widgets/web`；API handler 与独立托管资产归 Web 层，壳入口加载 `module.js`，独立 `index.html`/`index.js` 使用共享 Preact 与 CSS。`cmd/widgets web serve` 可独立启动；首页读取启用组件仍经应用层窄适配器接入 Dashboard。
- [~] IPMI `ipmi_servers` schema/CRUD 已归 `modules/ipmi.Repository` 所有，并挂载 `/v1/ipmi/servers` 配置 CRUD、连接测试、inventory/FRU 与电源控制 API；独立 Preact 页面已覆盖列表、新建/编辑、详情和电源操作，密码只写不读。旧页面兼容路由、模板和 handler 已移除；新增 API 测试验证写入凭据不回传、更新省略密码和电源动作校验。IPMI 协议 client、凭据处理、inventory/FRU 转换和电源操作现由模块 `client.go` 与 `web/service.go` 提供；`web.LocalModule` 统一创建 repository/service/handler，宿主与 `cmd/ipmi web serve` 共用。真实 BMC 验证待补。
- [x] iDRAC 的 Redfish client、SQLite repository、凭据 DTO/API、inventory 与 power service 均已迁入模块；Web API/资源在 `web/`，新增 standalone Preact UI 和 `cmd/idrac web serve`。`web.LocalModule` 统一创建 repository/service/handler，宿主与独立命令共用该入口。测试覆盖凭据只写不读、更新时密码可省略和 standalone 路由；真实 iDRAC 验证待补。
- [~] Clash `clash_servers` schema/CRUD 已迁入 `modules/clash.Repository`；`/v1/clash/servers` 提供配置 CRUD、连接测试、代理组/规则/连接快照、运行配置和切换/延迟/关闭连接动作，流量与连接实时数据通过模块 SSE 输出（含心跳、页面退出取消），运行配置密钥不会返回。模块测试覆盖 secret 写入/省略更新、配置更新和动作委派及必填输入/ID 校验。Hash 页面已覆盖实例管理、代理控制、规则、连接和配置，旧页面 handler、模板及兼容路由均已移除；Clash 实例端到端验证待补。
- [x] ESXi 的 govmomi 客户端、数据契约、SQLite repository、连接/Inventory service 和 HTTP handler 均已归模块；`web.LocalModule` 统一构建 repository/service/handler，宿主与独立命令共享装配入口。`web/` 自托管 shell ESM 与 standalone 页面，`cmd/esxi web serve` 可独立启动。壳应用仅注入共享数据库，API 保持凭据只写不读。API、存储和 standalone 资源路由有模块测试；真实 ESXi 环境验证待补。
- [~] PVE 已迁移为完整模块：`modules/pve/pve` 封装 Proxmox API client，`modules/pve/web` 提供 profile repository、业务 service、相对 API 路由和静态 UI，`modules/pve/cmd/pve` 提供独立 `pve web serve` 入口；应用层只保留 schema 生命周期及模块挂载。独立 UI 使用单份 `index.html`、Preact CDN 与模块 JS 清单。旧页面 handler/模板与根级 `pve/` 包已移除；模块测试覆盖独立页面/清单/API 与更新时保留密码。Proxmox 实机验证待补。
- [x] Yeelight 的设备实体/服务契约位于模块 Go 库，SQLite repository、Yeelight client service、HTTP API 和资产均在 `web/`；新增 standalone Preact 页面与 `cmd/yeelight web serve`。宿主仅提供共享数据库，设备连接、发现、状态和控制由模块执行。测试覆盖输入校验、RGB 校验及 standalone 路由；真实灯具验证待补。
- [x] Web Server 新建 schema 不再对 ACME `certificates` 声明外键；启动迁移会重建旧 listener/host 表并保留数据、移除旧外键，证书 ID 作为模块间不透明引用。
- [x] `/v1/webserver/listeners`、`/hosts`、`/routes`、`/policies` 已支持 CRUD API 并在写入后重新应用 yabber 配置；HTTPS listener/host 可从只含 ID 与域名的 `/certificates` 摘要中选择 ACME 有效证书。模块 API 测试覆盖 Listener 校验/CRUD 委派、嵌套路由 host ID 绑定和无效路由输入；前端 API client 测试覆盖 API 前缀、全部资源 CRUD 路径/方法及宿主覆盖 apiBase。前端已提供 Listener/Host/Policy 与五种 Route handler 的创建、编辑、删除；旧 `/web/...` 管理路由已移除，旧 Web Server HTML handler/template 已清理，`/web/metrics` 继续服务外部采集。应用层完成历史外键脱钩，Web Server 列表仓储在单 DB 连接池场景下不再嵌套持有 Rows 查询。隔离 NewServer + SQLite 集成测试覆盖 listener/policy/host/route 关联 CRUD、ACME 有效证书摘要，以及通过实际启用的 loopback HTTPS listener 将 ACME 存储证书装载到 yabber TLS loader；listener/host/route CRUD 测试资源保持禁用以避免额外监听。浏览器在隔离服务中完成 Listener、Policy、Virtual Host、Route 的新增/修改/删除及 Listener/Policy 关联；Route 固定响应编辑后通过 UI 刷新确认已保存。390px 和 320px 下三页主体均位于 58px 顶栏下方、页面无横向溢出、console 无错误。
- [~] Web Server 的领域类型与宿主注入契约留在模块根包；HTTP handler、清单/静态资源、数据库 repository、LocalService、yabber 配置生成和代理生命周期分别归入 `modules/webserver/web/` 与 `web/repository/`，浏览器资源统一在 `web/assets/`。新增单页独立管理入口和 `cmd/webserver web serve`；独立命令可管理并启动 HTTP listener，HTTPS 证书需宿主注入 `CertificateSource`。宿主通过 LocalService 的策略回调检查共享服务偏好与端口冲突，并向 Runtime 提供证书材料和 ACME HTTP-01 token 能力。旧 ACME 证书外键迁移与退役 `workers` 数据桥接仍在 Server 应用层执行；Web Server repository 不检查 `certificates`/`workers` 表。
- [ ] 其他功能模块、数据库边界、实时链路及旧模板清理尚未完成。本轮全仓盘点发现 `web/llm/*.html` 是无可达路由/API/模块实现的遗留模板（`server/llm.go` 为空包声明、LLM 表也无 schema 初始化），已移除这些死模板；因此没有将不存在的 LLM 服务功能伪记为已迁移。

- [x] Docker 应用目录同步/缓存加载、清单解析、安装状态与安装任务的实现和测试已从 `server/` 收敛到 `modules/docker`；宿主只装配模块服务、注入应用目录，并在同步失败时调用模块的本地缓存加载能力。Compose、镜像构建和 stats 的模块行为测试现位于 Docker 模块；Server 中重复的 Docker SDK/Compose 实现已清理，项目部署与终端会话通过 Docker 模块能力适配。
- [x] Docker HTTP handler、manifest/static handler 与相关 API 测试已从模块根包迁入 `modules/docker/web`；领域 service interface/DTO 留在模块根包，独立命令与组合根都从 `web` 包构造 HTTP handler。组合根直接注入 `modules/docker.LocalService`；移除 `Server` 上仅为满足 Docker HTTP 接口而存在的整组转发方法。Server 仍保留给部署、终端及应用目录启动编排使用的模块 service 访问入口。 Docker 应用目录 SQLite repository 及其契约测试也已迁入 `modules/docker/web`；根包仅通过 `AppCatalogStore` 接口消费该能力。
- [x] DHCP 静态租约校验与变更由 DHCP 模块 `LocalService` 执行，运行时对象也由该 service 持有；Server 只负责共享服务偏好、端口冲突、启动/关闭编排，并在 API 装配时提供应用层能力。服务器侧重复的 MAC/IP 校验已移除。
- [x] NTP 设置校验/规范化与运行时均由模块 `LocalService` 持有；宿主生命周期和共享端口策略通过模块的 Reload/Stop 能力管理它，不再直接创建或保存 NTP runtime。

### 阶段 1：壳与第一个只读页面

- 引入 frontend、固定 vendor/import map、开发 FS 与 embed；实现 Hash 路由、分组导航和公共请求。
- 将模块自己的 ESM/CSS 收进 `modules/<id>/web`，由模块 handler 自行托管静态资源和清单；组合根只登记模块工厂与挂载，现有模块逐批迁移。
- 实现 system 模块状态与只读 about 页，先不迁移升级操作；验证模块关闭、加载错误、未知路由。
- 建立桌面/手机共用布局、hash 深链接、静态缓存、JSON 错误和 WS/SSE middleware 基线。

完成：完全无需前端构建即可运行，离线访问可用；320/390/768/1280px 下内容可见，菜单可展开收起，键盘导航可用。认证登录不属于本轮重构前置条件。

### 阶段 2：Docker 纵向试点

- 先做容器列表、详情、start/stop/restart/delete，再迁移创建、images、volumes、networks、compose、apps。
- 补齐日志、stats、终端、pull/build/install 任务；复用底层 Docker 客户端，隔离 API DTO。
- 用单独的最小宿主只注册 Docker 模块，验证可更换 HTTP 与 Hash 页面根路径，且前端仍从模块自己的清单发现入口与导航。

完成：Docker 全部旧功能验收后，删去对应 Go 模板和旧操作路由；API 可以通过 httptest 单独测试。

### 阶段 3：ACME + DNS + Web Server

- 依次迁移 DNS challenge 能力、ACME 申请/续签任务、webserver 配置与状态。
- 明确小接口及外壳适配器，完成“DNS 验证 → 签发 → 选择证书 → HTTPS 热更新”验收。
- DHCP/NTP/forward 的 API/页面与启停流程迁移完成；Uptime、Forward、IPMI、iDRAC 旧页面 handlers 已清理，剩余工作集中在模块级契约/生命周期测试与最终整体验收。

完成：模块间无跨表读取，移除某个可选模块有可理解的行为，证书与配置不因页面迁移丢失。

### 阶段 4：其余功能与实时链路

- monitor、SSH stats/metrics、projects/cron、packages/files/disks、设备管理与工具模块依表逐个迁移；uptime 已迁为独立模块，SSH 配置与 Terminal 已有独立实现。
- 最后收口 dashboard 聚合、settings 页面及系统升级操作，特别验证进程重启、断连和更新后的重新登录。
- 每批做 API + 页面 + 旧代码清理，不累积半迁移模块。

### 阶段 5：收口与独立性验收

- 删除剩余 Go HTML 渲染、旧 web 模板及 `_method` 兼容处理；外壳只保留装配、公共 HTTP 能力、静态资源和生命周期。
- 检查每个模块不依赖旧 Server；公共库不包含面板路由、SQL 表名和页面概念。
- Docker 与 ACME 各完成一次从单个 `modules/<id>` 目录启动的独立最小宿主运行；共享 UI 通过稳定 ESM 别名引入，版本由宿主控制。
- 全部 28 个功能模块已有独立 Go module、`web/`、`web/assets/` 与 `cmd/<id>` 入口，源码不依赖宿主包；架构测试约束模块不得导入其他模块或宿主层。28 个模块及共享 Go modules 均在关闭 workspace 下通过测试和构建。`tests/module-smoke/run-release-smoke.sh` 将 Docker、Cron、ACME、DNS、IPMI、iDRAC、Settings、Widgets、Uptime 与 Terminal 模块打为临时 Go module proxy 发布包，在仓库外临时目录运行独立宿主，不使用宿主 `replace`，覆盖 `/manifest.json`、`/assets/index.js` 和模块 API；Terminal smoke 同时打包并解析 `pkg/terminal` 依赖版本。ACME、Cron 独立命令已用临时 SQLite 实际启动，并验证首页、模块清单和 JS 静态资源。实际公开版本发布与版本升级兼容验收尚未完成。
- 更新 API reference、开发文档、部署说明；按直接替换策略移除旧管理页面路径，不保留旧写接口或书签重定向。

## 11. 浏览器验收记录与清单

### 11.1 当前浏览器冒烟结果

2026-09-24 在隔离临时 SQLite 的真实 Go 服务上，使用 Chromium 直接加载生产同款嵌入式 ESM/CSS，对 28 个模块分别在 1280px 桌面、390px 与 320px 手机视口打开首屏，共 84 次页面加载；另枚举并加载了 89 条已声明的非流式子路由（包含新增/编辑/详情页与伪造 ID 的错误态）。首屏检查模块标题/路由正确、主体区域可见且非空、页面无横向溢出，并收集 `pageerror`、console error 和 `/v1/` 5xx：84 次均无前端异常、5xx 或横向溢出；所有移动视口的主体均位于 58px 顶部导航以下并可滚动。89 条子路由无 JS/console 异常或横向溢出；对不存在资源的详情页也检查错误状态。Docker Compose 页面另经浏览器确认列表 API 返回 200、Compose/YAML、目录、Git 三种表单切换正常，390px 下无溢出；本机终端页面经 WebSocket 实际执行 `echo` 并收到输出，浏览器 console 无错。本轮没有启动 Compose 容器或执行其他设备控制。

验收曾发现 Projects/Cron、Docker 镜像页中未自闭合的 `<input>` 破坏 HTM 节点树并触发 Preact `insertBefore` 错误，已修正；静态扫描同时修正 Docker 资源和 OpenWrt 表单的同类问题。错误态子路由发现 Compose、ACME、Clash、SSH、Yeelight、iDRAC、ESXi、PVE 页面会在 API 返回错误后仍显示加载占位，现已修正并通过伪造 ID 的浏览器复测。PVE 列表 API 暴露 schema 初始化钩子遗漏，已补齐。Projects 页面额外在隔离数据库中实际执行创建及删除；Docker 页面移动端截图确认导航下方内容可见，展开导航后仍可访问；Docker 目录/Git 镜像构建、列表展示和清理已在浏览器中使用本机 daemon 实际完成。随后使用临时 scratch 镜像和 `network_mode: none` 服务，经面板实际创建 Compose 项目、查看运行容器、停止并确认列表中消失，最后清理测试镜像及临时目录。真实停止流程发现 `down` 后详情页仍重读已被移除的项目并显示 404；现改为成功停止后返回 Compose 列表，并在重编译嵌入式应用后复测通过。此类冒烟不代表所有设备动作、流式链路、外部凭据及硬件协议均已做浏览器端到端验收；这类模块的真实集成仍按各自进度项跟踪。

### 11.2 每个模块的验收清单

- 列表/详情/创建/修改/删除及原有特殊操作完整；加载、空数据、失败、验证错误均有 UI。
- HTTP 方法与状态正确，认证/CSRF、资源权限、敏感数据、上传路径和容量限制有针对性测试。
- 页面切换、重复点击和网络中断不重复执行危险操作；输入内容作为文本渲染，不插入未清理 HTML。
- 保留 `<input class="input">`、`<button class="button">`；删除由共享 DeleteLink 渲染 `<a method="delete">`，确认后客户端发送 DELETE，不能由通用导航误处理。
- 确认链接/浏览器后退、查询参数、深链接刷新、可访问标签、焦点以及桌面/手机布局。
- 轮询、订阅、终端、上传和后台任务分别遵守取消/清理语义，慢客户端不会无限积累内存。
- 既有数据库可直接使用，迁移可重复执行；每次涉及 schema 都写明回滚条件，优先保持旧二进制可读的增量 schema。
- Go 业务测试与 API 契约测试通过；代表性浏览器测试覆盖真实 ESM、CSS、流式连接与资源缓存，而不是仅用 Go 测试替代 UI 验证。

## 12. 迁移期路由所有权

不要一开始将所有路径兜底到 SPA。外壳维护唯一挂载表：已迁移模块的页面前缀交给 SPA，未迁移模块保持旧 handler；跨两种页面使用完整导航。一个模块切换时同时更新导航与前缀所有权，并删除同路径旧 handler，避免 ServeMux 中更具体的旧路由继续抢占新页面。

回滚按模块恢复到上一个已验证版本，模块开关只控制挂载，不删除数据、不自动撤销 schema。移除模板前记录关键行为测试；跨模块联动批次应作为整体验收。最终形态不保留“两套前端长期运行”。

- [x] Speedtest 的结果仓储、Ookla/LibreSpeed 客户端调用、测速 SSE 和 Preact 页面现由模块提供；Web API、资源、独立 `cmd/speedtest web serve` 均归 `modules/speedtest`。壳应用仅注入共享数据库，模块自带 SQLite/测速依赖。真实测速网络端到端验证待补。
