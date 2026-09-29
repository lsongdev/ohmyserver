# 模块独立运行与嵌入运行：runtime 评审记录

日期：2026-09-28 至 2026-09-29。状态：**评审问题已关闭，v1 契约已实现并固定**。规范正文见 [模块运行时契约](module-contract.md)。

## 结论

“模块不感知宿主，只暴露必要清单；独立与嵌入使用同一份核心实现”是正确边界，并已落实为以下链路：

```text
模块相对 API + manifest + module.js + pages
                    │
                    ▼
          moduleweb 共享 HTTP/browser runtime
             ┌──────────────┴──────────────┐
             ▼                             ▼
   CLI 直接运行 Handler()        壳 StripPrefix 挂载 Handler()
   页面根与 API 根都是 /          apiBase/pageBase 完全由宿主决定
```

模块没有运行模式开关，也不从 URL 猜测身份或前缀。两种模式只在宿主拥有的布局、认证、依赖注入和跨模块可用性上不同。

## 已完成的收敛

| 原问题 | 最终处理 |
| --- | --- |
| 28 份相同的 199 行 standalone runtime | 抽到版本化 `pkg/moduleweb/browser`；每个 `index.js` 只剩 UI 注入和 `mountStandalone()` |
| 28 份相同 `standalone.css` | 删除模块副本，由共享 runtime 在独立模式加载 |
| `StandaloneHandler()` 固定 `/v1/<id>` | 删除；CLI 和壳都使用同一个完整、相对的 `Handler()` |
| 独立页从 API URL 末段推断模块 ID | ID 只来自校验后的 manifest，并与 runtime/Go 注册核对 |
| manifest 可覆盖 `apiBase/pageBase` | mount、manifest、runtime 分层保存；严格字段白名单 |
| 壳按 ID 首段解析页面 | 按宿主 `pageBase` 段边界最长匹配，支持 `/`、别名和多段挂载 |
| 两套路由与页面加载语义 | 独立和壳共用 `router.js`、`view.js` 与错误边界 |
| transport 路径、认证和 envelope 行为不一致 | 四项能力共用路径解析器并固定宿主同源凭据策略；正确保留 `data:null/false/0/""` |
| 宿主按状态改写模块 404/405 | 删除状态拦截；只在模块 ServeMux 未匹配时生成路由错误 |
| Projects 使用未注入的 `moduleHref` | 统一为 `hrefTo`，目标缺失返回 `null` |
| Files、Docker、DNS 页面硬编码宿主页面前缀 | 全部改为模块内相对导航 |
| Settings API 占用模块首页 `/` | 资源 API 调整为 `/entries`，`/` 固定服务独立页面 |
| NTP 重复关闭及模块关闭错误丢失 | 去除重复关闭，组合根聚合 ACME/Cron 等关闭错误 |
| 文档描述的生命周期接口与实现不符 | 固定“HTTP 注册与生命周期分离”；组合根显式调用具体模块方法 |
| 工作区构建掩盖发布依赖 | 28 个模块依赖 `moduleweb v0.1.0`；临时 proxy 在 `GOWORK=off` 下验证打包结果 |

## 实现边界

共享 Go 层位于 `pkg/moduleweb/module.go`。它提供完整模块根和共享 runtime 资源，保留流式接口，且只处理 ServeMux 自己的路由错误。

共享浏览器层位于 `pkg/moduleweb/browser/`：

```text
paths.js        相对路径与挂载校验
transport.js    JSON/原始 fetch/HTTP URL/WebSocket URL
router.js       Hash 解析、挂载匹配、路由匹配和导航
definition.js   mount/manifest/runtime 白名单与一致性校验
view.js         API 实例、页面加载、缓存、重试和错误边界
standalone.js   独立宿主布局
standalone.css  独立宿主共享布局样式
```

OhMyServer 壳只负责模块索引、侧栏和布局，页面承载使用同一个 `createModuleView()`。模块公开的稳定边界只有 `Name()/Handler()`、Manifest v1 和 `{ id, routes, createAPI }`。

## 验证结果

- 浏览器契约与模块 API 行为测试 37/37 通过，覆盖身份与挂载分离、字段污染、路径逃逸、同源凭据、路由优先级、编码参数、null/false/zero envelope、错误保真、流响应、懒加载去重与失败重试。
- 根模块 `go test ./...`、`go build ./...` 通过。
- `check-runtime.py` 对全部 28 个模块完成隔离源码 test/build、CLI 构建与实际启动、共享 runtime 请求，以及有 smoke host 模块的无 replace 外部消费测试；28/28 通过。
- 真实 Chrome 验证独立 Settings：`api-base="."` 下加载、进入 `/new`、浏览器后退返回列表，无 page error。
- 真实 Chrome 验证嵌入壳：API 挂载 `/v1/settings`、页面挂载 `/infra/settings`，进入 `/new`、取消返回列表，无 page error。
- 模块业务 404、ServeMux 路由 404/405、静态 404、下载、SSE 在根挂载、默认子路径和别名多段挂载下通过同一 Handler 合同测试。

`moduleweb v0.1.0` 是模块发布前需要先发布的版本标签；验证器用本地临时 Go proxy 模拟该发布顺序，不执行真实发布。
