# 模块运行时契约

状态：**v1 已固定并实现**。2026-09-29 起，独立进程和 OhMyServer 壳使用同一份 Go Handler、浏览器 runtime、模块入口、API client 和页面实现。

## 基本原则

模块只知道自己的稳定 ID、相对 API、相对页面路由和资源，不读取或推断宿主挂载。宿主拥有 HTTP 挂载、Hash 页面挂载、认证、布局以及跨模块可用性。独立运行也是一个宿主，它把同一个模块 Handler 直接挂在 `/`。

禁止通过运行模式开关、URL 末段、全局 service locator、转发头或宿主包让模块感知宿主。需要新能力时扩展明确的版本化契约；不得在独立和嵌入路径分别实现。

## 发布单元

```text
modules/<id>/
  go.mod
  <id>/                    # 可选的公共领域/协议库
  web/                     # repository、service、HTTP Handler
  web/assets/
    manifest.json          # 声明性清单
    module.js              # 浏览器可执行入口
    api.js                 # 相对 API client
    pages/                 # 两种宿主共用的页面
    index.html             # 独立宿主文档
    index.js               # 仅向共享 runtime 注入 UI primitives
  cmd/<id>/main.go         # 独立进程组合根
```

模块依赖 `github.com/lsongdev/ohmyserver/pkg/moduleweb` v0.1.0。该包拥有唯一的 HTTP 适配、transport、清单/runtime 校验、路由、页面加载、错误边界和独立布局实现。模块不得复制这些实现。

## Go HTTP 契约

宿主只需要：

```go
type HTTPModule interface {
    Name() string
    Handler() http.Handler
}
```

`Handler()` 是完整且与挂载无关的模块 HTTP 根：

| 相对路径 | 所有者与语义 |
| --- | --- |
| `/` | 模块独立页面，HTML 中 `api-base="."` |
| `/manifest.json` | 模块公开清单 |
| `/assets/*` | 模块资源 |
| `/assets/runtime/*` | `moduleweb` 保留的共享 runtime |
| 其他路径 | 模块 API |

独立 CLI 直接运行 `Handler()`。嵌入宿主用 `http.StripPrefix` 挂载同一 Handler，并把裸挂载地址重定向到带 `/` 的模块根。模块收到的请求始终是去掉前缀后的相对路径。

模块业务 handler 生成的状态、header、错误 body、下载、SSE 和 WebSocket 原样透传。`moduleweb` 只对 `http.ServeMux` 未匹配的路由生成 JSON 404/405，不按响应状态重写业务错误。宿主只处理“模块未注册”等宿主级错误。

模块构造器与 Handler 不隐式启动后台任务。schema、迁移、`Start` 和 `Close` 由 CLI 或应用组合根显式协调；HTTP 注册表不反射调用生命周期。具体模块只实现其需要的方法，且不关闭注入的共享 DB/client。

## 宿主挂载契约

宿主索引只公开：

```json
{
  "modules": [{
    "id": "clash",
    "apiBase": "/v1/clash",
    "pageBase": "/clash"
  }]
}
```

`id` 是模块身份；`apiBase` 与 `pageBase` 是宿主配置。三者分别校验且必须唯一。`apiBase` 可以是 `/` 或任意合法同源子路径；`pageBase` 可以是 `/` 或多段 Hash 路径，与 ID 无关。清单 URL 固定由 `apiBase + /manifest.json` 派生，不另设可漂移字段。

runtime 内部始终保存 `{ mount, manifest, runtime }`，禁止平铺合并。manifest 中出现 `apiBase`、`pageBase`、`routes`、`createAPI` 等非白名单字段时整份清单无效，无法覆盖宿主配置。

页面定位按 `pageBase` 段边界做最长匹配，根挂载最后兜底。宿主使用同一组函数生成链接和解析 URL，因此别名、多段路径、反向代理前缀和编码参数具有一致行为。

## Manifest v1

允许字段仅有：

| 字段 | 规则 |
| --- | --- |
| `contractVersion` | 必须为 `1` |
| `id` | 稳定模块 ID，必须与 Go 注册 ID 和 runtime ID 相同 |
| `title`、`icon`、`group` | 展示信息 |
| `entry` | 模块资源，例如 `assets/module.js` |
| `home` | 模块内绝对相对路由，默认 `/` |
| `styles` | 模块功能样式；不含共享独立布局样式 |
| `navigation` | 可见子导航，每项仅含 `title`、模块内 `path` |

`entry/styles` 必须位于本模块 `assets/`，拒绝绝对 URL、query、fragment、反斜杠、百分号和目录穿越。`home/navigation` 必须能匹配 runtime route。未知字段直接拒绝，避免悄悄形成第二份契约。

## 浏览器 runtime v1

`module.js` 默认导出仅允许三个字段：

```js
export default {
  id: 'clash',
  createAPI,
  routes: [
    { path: '/', load: () => import('./pages/servers.js') },
    { path: '/:id', load: () => import('./pages/detail.js') },
  ],
};
```

路由只允许 `path/load/props`。等价参数模板、非法参数名和重复参数在加载前拒绝。匹配时逐段比较，越靠左的静态段优先。`route.props` 不得覆盖下列宿主能力：

```text
api, params, search, navigate, href, hrefTo
```

页面只通过这些能力工作：

- `params` 为一次解码后的参数；编码斜杠仍属于一个参数段。
- `search` 为当前 URL 的 `URLSearchParams`。
- `navigate(path)`、`href(path)` 只接受以单个 `/` 开头的模块内路径。
- `hrefTo(moduleID, path)` 是唯一跨模块能力；目标不可用时返回 `null`。独立宿主也始终提供该函数。

页面组件以“模块实例 + route template”为身份。同一路由内 params/query 变化只更新 props；切换路由或模块才卸载。页面负责清理自己的请求、timer、EventSource、reader 和 WebSocket。共享 runtime 缓存成功的懒加载，失败后允许重试，并提供加载与渲染错误边界。

## Transport v1

`createAPI(transport)` 必须同步、无副作用，只返回模块 API client。transport 固定提供：

| 能力 | 语义 |
| --- | --- |
| `request(path, options)` | JSON envelope；204 → `null`；按 `data` 字段存在性解包，保留 `null/false/0/""`；错误保留 `status/code/message` |
| `fetchImpl(path, options)` | 原始 `Response`，供上传、下载和流使用，不解包、不缓冲 |
| `apiURL(path)` | 同源 HTTP URL，供下载与 EventSource 使用 |
| `websocketURL(path)` | 同一路径规则，协议转换为 `ws/wss` |

四项能力使用同一解析器，只接受以单个 `/` 开头的模块内路径，可带 query；拒绝 scheme、协议相对 URL、fragment、反斜杠、空白和目录穿越。动态参数必须 `encodeURIComponent`。宿主固定使用 `credentials: same-origin`；调用方传入的业务 header 和 abort signal 被保留，JSON transport 管理自己的 `Accept` 与 `Content-Type`。

## UI 依赖

模块页面统一从 `@module/ui` 和 `preact/hooks` 导入。独立页 import map 将二者指向 `https://lsong.org/scripts/react/index.js`；OhMyServer 壳将二者指向同一个本地 Preact 实例。共享 runtime 本身不打包 Preact，由宿主注入 `html/render/Component/hooks`。

`index.js` 只能完成这一注入并调用 `mountStandalone()`，不得包含 manifest 获取、transport、路由或业务页面实现。共享 `standalone.css` 由 runtime 加载，模块 manifest 只列功能 CSS。

## 验收门槛

任何契约修改必须同时通过：

1. `node --test frontend/app/*.test.js pkg/moduleweb/browser/*.test.js`。
2. 根模块 `go test ./...`、`go build ./...`。
3. 每个独立模块在 `GOWORK=off` 下 test/build。
4. 28 个独立 CLI 用临时 DB 与空闲端口实际启动，读取 `/`、manifest、模块 entry 和共享 runtime，并正常关闭。
5. 无 local replace 的临时 Go proxy 外部宿主测试。
6. 独立浏览器和嵌入壳浏览器至少覆盖加载、模块内导航、后退/返回和控制台错误。
7. 所有前端与模块 JS 通过 `node --check`，并通过 `git diff --check`。

批量验证入口为 `tests/module-smoke/check-runtime.py`。具体实现与问题关闭记录见 [runtime 评审](module-runtime-review.md)。
