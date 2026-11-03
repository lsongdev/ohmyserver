# 通知中心设计

状态：**第 1 阶段已实现**（事件、问题、页面、ZFS 检查、总览页），第 2、3 阶段未开始。与下文的差异见第 16 节。

## 1. 背景与目标

OhMyServer 的很多模块都有需要让人知道的事：存储池降级、任务失败、证书续期失败、主机离线。现状有三个问题：

- 总览页 `home/home.go` 只汇总数字，比如“2 个证书将过期”，没有统一的“哪里出了问题”。
- 问题只能打开对应页面才看得到。Disks 不再显示存储池状态后，存储池降级尤其容易漏掉。
- 没有通用的通知渠道，也没有地方配置。

**目标**：做一个功能简洁但够用的通知中心。

- 模块、脚本、其他主机都能用一条简单的调用发送事件。
- 集中查看历史事件和当前未解决的问题。
- 把事件按规则发到外部渠道，并做好去重、限流和重试。
- 按模块约定实现，也能独立运行。

**不做**：

| 不做的事 | 原因 |
|---|---|
| 运行日志（`slog`、journal） | 量级和用途都不同，journal 已经够用 |
| 规则表达式、模板语言 | 容易变成半个 Prometheus，维护成本高 |
| 升级和值班 | 个人和小团队场景用不上 |
| 指标和图表 | 这是 Monitor 的职责 |
| 双向机器人 | 只发不收；需要时再考虑 |
| 监控面板自身存活 | 面板挂了发不出通知，用外部心跳服务解决（见第 12 节） |

## 2. 概念

| 概念 | 含义 |
|---|---|
| **事件（Event）** | 模块明确发送的一条消息，有级别。只增不改。 |
| **问题（Issue）** | 带同一个 `Key` 的事件归成的一件持续的事，有打开和解决两种状态。 |
| **渠道（Channel）** | 一个外部投递目标，比如某个 Telegram 聊天或某个飞书群。 |
| **路由（Route）** | 规则：哪些事件发到哪个渠道。 |
| **投递（Delivery）** | 一次发送任务，有状态和重试记录。 |

事件必须由模块明确发送，所以这里记录的都是有业务含义的事，比如“任务运行失败”，不包括程序的调试输出。

## 3. 架构

```text
模块（ZFS、Disks、Cron…）──┐
                          ├─ Notify(Event) ─→ 写入事件表和发件箱（立即返回）
脚本、其他主机 ─ HTTP ─────┘                        │
                                     ┌──────────────┼───────────────┐
                                     ▼              ▼               ▼
                                 问题状态        路由匹配        后台投递
                                     │              │          （重试、限流）
                                     ▼              ▼               ▼
                          总览页 / 通知中心页面   投递记录   Webhook / Telegram / 邮件
```

- **同一条代码路径**：模块只调用 `Notifier` 接口。集成时注入通知中心的实现，独立运行时注入 Webhook 客户端，没有配置时不做任何事。
- **模块不被拖慢**：`Notify` 只写 SQLite 就返回，发送全部在后台。
- **依赖方向**：通知中心不了解其他模块。总览页通过 `home.Sources` 读取它的“当前问题”，和读取其他模块的方式相同。

## 4. 模块接口

```go
type Level string

const (
    LevelDebug    Level = "debug"
    LevelInfo     Level = "info"
    LevelWarning  Level = "warning"
    LevelCritical Level = "critical"
)

type Event struct {
    Level    Level
    Title    string            // 一行说明：“存储池 data 已降级”
    Body     string            // 可选：详情，纯文本
    Link     string            // 可选：发送方模块内的路径，如 /pool?name=data
    Key      string            // 可选：同一个问题用同一个 Key
    Resolved bool              // 可选：这个 Key 的问题已恢复
    Fields   map[string]string // 可选：结构化字段，只用于展示
}

type Notifier interface {
    // Notify 记录事件，不等待外部发送。
    Notify(ctx context.Context, e Event) error
}
```

`Source`（来源）不由事件填写。模块用的 `Notifier` 在装配时就绑定了模块 id，HTTP 接入则取令牌的名字，这样来源不能伪造。

### 调用方式

| 场景 | 调用 |
|---|---|
| 一次性消息 | `Notify({Level: info, Title: "证书已续期"})` |
| 发现问题 | `Notify({Level: critical, Key: "pool:data:health", Title: …})`，可每轮重复调用 |
| 检查正常 | `Notify({Key: "pool:data:health", Resolved: true})`，也可每轮重复调用 |

重复调用没有副作用，所以模块不用记住“发过没有”。例如 ZFS 模块只需要：

```go
for _, p := range pools {
    key := "pool:" + p.Name + ":health"
    if p.Health == "ONLINE" {
        notifier.Notify(ctx, Event{Key: key, Resolved: true})
    } else {
        notifier.Notify(ctx, Event{Level: LevelCritical, Key: key, Title: …, Link: …})
    }
}
```

### 接口放在哪（待定）

约定要求模块之间不互相 import，而 `Event` 类型需要多个模块共用。

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 独立的小仓库 `notify-go`（推荐）** | 只有类型和 Webhook 客户端，和 `smartctl-go` 的做法一致 | 多一个仓库 |
| B. 每个模块定义自己的类型，在装配层转换 | 不增加依赖 | 每个模块重复写类型和去重逻辑 |

## 5. 问题的状态

只有带 `Key` 的事件才有问题状态。`(来源, Key)` 唯一确定一个问题。

```text
              Notify（未恢复）
    (无) ─────────────────────────→ 打开 ──┐ 重复上报：只更新次数和最后时间；
                                    │  ▲ ◄─┘ 级别升高时再通知一次
       Resolved: true，或手动关闭    │  │
                                    ▼  │ 再次上报
                                  已解决 ─┘
```

| 情况 | 记录 | 外部通知 |
|---|---|---|
| 新问题 | 新建问题，写一条事件 | 按路由发送 |
| 问题持续，内容相同 | 只更新次数和最后时间，**不写新事件** | 不发送；可配置定期提醒 |
| 级别升高 | 写一条事件 | 发送 |
| 恢复 | 标记解决，写一条事件 | 发到通知过这个问题的渠道 |
| 问题不存在时收到恢复 | 忽略，不写任何记录 | 不发送 |
| 已解决后再次出现 | 重新打开 | 发送，受抖动保护限制 |

- **抖动保护**：恢复后短时间内再次出现（默认 5 分钟），不新发通知，只把问题重新打开。恢复通知也推迟同样的时间再发，这样来回抖动时不会连续收到“出现、恢复、出现”。严重问题的首次通知不延迟。
- **手动关闭**：来源消失的问题（比如存储池已被删除）不会再收到恢复消息，所以页面上可以手动关闭。之后如果又上报，问题会重新打开。
- **重启**：问题存在 SQLite 里。模块下一轮检查会继续上报或恢复，停机期间恢复的问题也能被关闭。
- **静音**：问题可以静音一段时间，比如计划内的维护。静音期间仍然记录，只是不发到外部。

## 6. 渠道和路由

### 渠道类型（第一版）

| 类型 | 说明 |
|---|---|
| **Webhook** | 可配置 URL、方法、请求头和 JSON 模板。内置飞书群机器人、企业微信群机器人、Bark、ntfy 和通用 JSON 预设。 |
| **Telegram** | Bot token 和 chat ID，直接使用 `telegram-go`。 |
| **邮件** | SMTP。 |

```go
type Sender interface {
    Send(ctx context.Context, m Message) error
}
```

不引入 `miya-channels`：`channels` 包会连带 `miya-agents` 等共 228 个依赖包；它的微信、飞书实现依赖“收到消息后回复”，无法直接主动发送。复用它的底层 SDK（如 `telegram-go`）即可。微信个人号因为没有主动发送入口，第一版不支持。

2026-11-03 从 NAS 探测过，`api.telegram.org`、`open.feishu.cn`、`qyapi.weixin.qq.com`、`api.day.app`、`ntfy.sh` 都有响应。这只是一次探测，不代表长期稳定；实际发送也还没测过。

### 路由

一条路由：**最低级别 + 来源范围（全部或指定模块）+ 目标渠道**。匹配到的每个渠道各发一次，同一渠道不重复发送。

**默认行为**：`critical` 和 `warning` 发到所有已启用的渠道；`info` 和 `debug` 只留在通知中心。这是防止告警疲劳的关键，新增渠道后不用配置也能得到合理的结果。

## 7. 投递

- **发件箱**：事件写入时同一事务生成投递记录，重启后继续发送，不丢失。
- **重试**：指数退避，比如 30 秒、2 分钟、10 分钟、1 小时，24 小时后放弃并标记失败。
- **限流**：每个渠道每分钟最多 20 条（待定）。超出时合并成一条摘要，比如“还有 15 条事件”，并附通知中心链接。
- **不会自己触发自己**：渠道发送失败只更新渠道状态，**绝不**产生新事件，否则会循环。
- **并发**：每个渠道一个发送协程，慢渠道不影响其他渠道。

## 8. HTTP 接口

响应格式和其他模块相同。下面都是模块内的路径，集成后挂载在 `/m/notify/api/` 下。

### 接入

```http
POST /events
Authorization: Bearer <令牌>

{"level": "warning", "title": "备份失败", "body": "…", "key": "backup:nightly"}
```

模块独立运行时的 Webhook 使用**完全相同的格式**，所以独立的 ZFS 模块只要配置通知中心的地址和令牌就能接入。发到普通 Webhook 时，公共库里的客户端按 `Key` 在内存中去重，只在状态变化时发送。

这个路径要在 manifest 的 `public` 中声明，因为脚本没有面板登录状态。模块自己校验令牌，并按令牌限速。

```bash
curl -H "Authorization: Bearer $TOKEN" -d '{"level":"info","title":"备份完成"}' \
  http://nas/m/notify/api/events
```

### 管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/events` | 分页：`level`、`source`、`q`、`before` |
| GET | `/issues` | `state=open\|resolved` |
| POST | `/issues/{id}/resolve` | 手动关闭 |
| POST | `/issues/{id}/mute` | `{until}` |
| GET, POST | `/channels` | 返回时隐藏密钥 |
| GET, PUT, DELETE | `/channels/{id}` | 更新时留空密钥表示不修改 |
| POST | `/channels/{id}/test` | 发一条测试消息，返回结果 |
| GET, POST | `/routes` | |
| PUT, DELETE | `/routes/{id}` | |
| GET | `/deliveries` | 投递记录和失败原因 |
| GET, POST, DELETE | `/tokens` | 接入令牌；创建时只显示一次 |
| GET | `/summary` | 给总览页用的计数和当前问题 |

## 9. 数据模型

模块 id 为 `notify`，所有表以 `notify_` 开头，按约定在 `New` 中迁移。

```sql
CREATE TABLE notify_events (
  id INTEGER PRIMARY KEY, created_at INTEGER NOT NULL,
  source TEXT NOT NULL, level TEXT NOT NULL,
  title TEXT NOT NULL, body TEXT, link TEXT, fields TEXT,
  issue_id INTEGER REFERENCES notify_issues(id) ON DELETE SET NULL,
  kind TEXT NOT NULL            -- message, opened, escalated, resolved
);
CREATE INDEX notify_events_time ON notify_events(created_at DESC);

CREATE TABLE notify_issues (
  id INTEGER PRIMARY KEY, source TEXT NOT NULL, key TEXT NOT NULL,
  state TEXT NOT NULL,          -- open, resolved
  level TEXT NOT NULL, title TEXT NOT NULL, body TEXT, link TEXT,
  first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL, resolved_at INTEGER,
  occurrences INTEGER NOT NULL, muted_until INTEGER,
  UNIQUE (source, key)
);

CREATE TABLE notify_channels (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL,
  config TEXT NOT NULL, enabled INTEGER NOT NULL,
  last_error TEXT, last_success_at INTEGER
);

CREATE TABLE notify_routes (
  id INTEGER PRIMARY KEY, min_level TEXT NOT NULL,
  sources TEXT,                 -- 空表示全部
  channel_id INTEGER NOT NULL REFERENCES notify_channels(id) ON DELETE CASCADE
);

CREATE TABLE notify_deliveries (
  id INTEGER PRIMARY KEY,
  event_id INTEGER NOT NULL REFERENCES notify_events(id) ON DELETE CASCADE,
  channel_id INTEGER NOT NULL REFERENCES notify_channels(id) ON DELETE CASCADE,
  state TEXT NOT NULL,          -- pending, sent, failed, dropped
  attempts INTEGER NOT NULL, next_attempt_at INTEGER, sent_at INTEGER, error TEXT
);
CREATE INDEX notify_deliveries_due ON notify_deliveries(state, next_attempt_at);

CREATE TABLE notify_tokens (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,   -- 也是事件的 source
  token_hash TEXT NOT NULL, created_at INTEGER NOT NULL, last_used_at INTEGER
);
```

**保存时间（待定）**：`debug` 7 天，`info` 30 天，`warning` 90 天，`critical` 1 年，事件总数上限 10 万条。未解决问题的事件不清理。每小时清理一次。

## 10. 界面

按项目的交互规范：复杂配置用独立页面，确认只用于破坏性操作。

| 页面 | 路由 | 内容 |
|---|---|---|
| **通知中心** | `/` | 上方是当前问题（级别、来源、持续时间、关闭、静音），下方是事件时间线（按级别、来源、关键字筛选，筛选条件放在 URL 里） |
| **渠道** | `/channels` | 列表显示类型、启用状态、最近成功或失败 |
| **编辑渠道** | `/channels/new`、`/channels/:id` | 独立页面，含预设选择和发送测试消息 |
| **路由** | `/routes` | 列表内联编辑 |
| **投递记录** | `/deliveries` | 失败原因，可手动重试 |
| **接入令牌** | `/tokens` | 创建时显示一次，附 `curl` 示例 |

事件和问题的 `Link` 用 `ctx.link(source, path, query)` 转成可点击的跳转；来源模块没有挂载时显示纯文本。

**总览页**：有未解决问题时在顶部列出，并跳转到相应页面；没有问题时不占位置。

**侧边栏角标（待定）**：现有 manifest 和主程序都不支持。可以给主程序加一个专门的未解决数量接口，或者第一版只在总览页显示。我建议先不做。

## 11. 模块接入

### 第一批

| 模块 | Key | 级别 | 触发 |
|---|---|---|---|
| **ZFS** | `pool:<名字>:health` | critical | 状态不是 ONLINE |
| | `pool:<名字>:errors` | critical | 有已知数据错误 |
| | `pool:<名字>:capacity` | warning（≥80%）、critical（≥90%） | 容量 |
| | `pool:<名字>:scrub` | warning | scrub 超过 35 天 |
| | `dataset:<名字>:schedule` | warning | 快照计划失败 |
| **Disks** | `disk:<序列号>:health` | critical | SMART 判定故障 |
| | `array:<名字>:health` | critical | RAID 降级 |

ZFS 模块目前只在页面打开时才读取状态，所以需要新增一个定期检查，默认 30 秒。它只运行 `zpool list` 和 `zpool status`，不会造成明显负载。`zpool` 有 5 秒超时；超时本身也算一个问题（`zpool:unresponsive`），因为这通常意味着存储池被挂起。

检查逻辑放在 ZFS 模块内。第一版不接 `zed`，以后可以用它实现即时响应。

### 第二批

| 模块 | 事件 |
|---|---|
| **Cron** | 任务失败，带 `Key`，下次成功时恢复 |
| **证书** | 续期失败（`warning`）、已过期（`critical`）、续期成功（`info`） |
| **Monitor** | 主机离线（持续 5 分钟后才算）、磁盘接近满 |
| **CI** | 构建失败 |
| **Gateway、DNS** | 监听端口启动失败 |

接入后，`home/home.go` 中“哪些地方有问题”的零散判断可以换成读取当前问题；计数仍保留。

## 12. 风险和对策

| 风险 | 对策 |
|---|---|
| **告警疲劳** | 默认只外发 `warning` 以上；去重、抖动保护、合并摘要；问题持续不重复通知 |
| **事件写爆数据库** | 按来源限速；问题重复上报不写事件；保存时间和总量上限 |
| **模块被拖慢** | `Notify` 只写本地数据库；失败只记日志，绝不影响模块主流程 |
| **面板挂了发不出通知** | 通知中心解决不了。用 Cron 任务定时向 healthchecks.io 这类外部服务发心跳，在文档里说明 |
| **通知循环** | 渠道失败不产生事件 |
| **敏感信息泄露** | 见下 |

### 安全

- **面板密码是前提**。NAS 上的面板目前在局域网内免密开放，任何人都能看到或修改渠道配置，也能发送任意告警。**接入外部渠道之前应当先加密码。**（待定：是否强制，比如未设密码时禁止配置渠道。）
- **令牌**：只存哈希，只显示一次，可单独吊销，每个令牌对应一个来源。
- **渠道密钥**：接口绝不返回，写入日志前要脱敏。存储方式和现有 SSH 模块的凭据一样，明文存在 SQLite 中（从 `modules/ssh/store` 的读写代码看没有加密）。全局的凭据加密是另一个独立的话题。
- **Webhook 目标**：由管理员配置，可以访问内网，这是有意的（比如自建 ntfy）。因此配置权限必须受面板认证保护。
- **消息内容**：事件内容会发给第三方服务，模块不应把密码、令牌写进事件。
- **HTTP 接入**：限制请求体大小，校验字段长度，`link` 只接受模块内路径，防止伪造跳转。

## 13. 测试

- **单元测试**：状态流转（重复、升级、恢复、抖动、重开、手动关闭）、路由匹配、重试退避、限流合并、保存时间清理。
- **发送器**：用 `httptest` 模拟 Webhook 和 Telegram，覆盖超时和错误响应。
- **重启**：发送中途重启后继续投递，且不重复发送已成功的投递。
- **端到端**：独立运行模式下，把问题上报到本地测试接收端。ZFS 的故障场景只使用 `/tmp` 文件做的临时存储池，**不操作物理盘和 `data` 池**。
- **真实渠道**：测试消息只发到你指定的聊天或群。

## 14. 实施阶段

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| **1. 事件核心** | 模块骨架、`Notifier`、事件和问题、通知中心页面、总览页接入、ZFS 定期检查 | 临时池降级后，几十秒内在通知中心和总览页出现；恢复后自动解除 |
| **2. 渠道和路由** | 发件箱、重试、限流、Webhook 和 Telegram、测试发送、投递记录 | 临时池降级后收到一条通知，恢复后再收到一条，抖动时不刷屏 |
| **3. 扩展接入** | HTTP 接入和令牌、邮件、第二批模块、`home.go` 整理 | 脚本 `curl` 能发事件，各模块问题统一出现 |

每个阶段单独提交和验证。第 1 阶段不涉及外部发送，风险最低，做完可以先用一段时间，再决定默认规则是否合适。

## 15. 待决定

| # | 问题 | 建议 |
|---|---|---|
| 1 | `Event` 类型放哪：独立仓库 `notify-go`，还是各模块自己定义 | 独立的 `notify-go` |
| 2 | 模块 id 和标题 | `notify`，显示为 Notifications |
| 3 | 第一版渠道：Webhook、Telegram、邮件 | 同意；邮件放第 3 阶段 |
| 4 | 未设面板密码时是否禁止配置渠道 | 不强制，但显示醒目的警告；你的 NAS 建议在第 2 阶段前加密码 |
| 5 | 持续问题是否定期提醒 | 第一版默认不提醒，每条路由可选 |
| 6 | 保存时间和限流数值 | 用第 7、9 节的默认值，后续按实际调整 |
| 7 | 是否做侧边栏角标 | 第一版不做 |
| 8 | 面板语言：文档是中文，模块界面现在都是英文 | 事件文案也用英文，和现有界面保持一致 |

## 16. 实现时的决定

第 1 阶段实现时与上文不同或补充的地方：

- **暂不建 `notify-go` 仓库**：只有 ZFS 接入，由模块定义 `Notifier`，在 `notify/` 包里适配，和 `cron.ZFSJob` 的做法相同。第 3 阶段需要共用 Webhook 客户端时再抽出来。
- **`Fields` 和静音推迟**：`Event.Fields` 没有实际用途，静音只影响外部发送，都放到第 2 阶段再加，表也到时再迁移。
- **恢复事件延迟写入**：问题恢复时立刻标记为已解决，但 `resolved` 事件等过了抖动窗口才写入，日期仍是恢复的时间。窗口内再次出现就当没恢复过，时间线上不留痕迹。手动关闭立即写入。
- **未扫描过的池**：从建池时间算起，超过 35 天才报 scrub 过期，新建的池不会马上告警。
- **消失的池**：检查时发现之前见过的池不见了，就关闭它的问题；面板重启后不再记得，要手动关闭。
- **读不到不算正常**：`zpool status` 读取失败时，错误和 scrub 既不报问题也不报恢复，避免错误地关闭真实问题。
