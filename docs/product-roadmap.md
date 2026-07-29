# OhMyServer 产品定位与角色化平台规划

## 1. 愿景

OhMyServer 不应只被定义为“又一个 Linux 面板”，也不应试图成为一个新的操作系统。
它更适合作为一套可嵌入、可组合、可按需启用的节点控制面：

> 用一套轻量的 Go 控制面，把一台普通 Linux 主机组织成它当前需要承担的角色。

同一台设备可以只承担一种角色，也可以同时承担多种角色：

- 在软路由或边缘主机上，负责拨号、路由、DHCP、DNS、NTP、防火墙和 VPN；
- 在 NAS 上，负责磁盘健康、存储池、共享、快照、备份和容器应用；
- 在云服务器上，负责网站、证书、DNS、部署、软件包、systemd 和防火墙；
- 在 HomeLab 中，负责容器、虚拟化、监控、远程终端和服务发现；
- 在家庭或工业网络中，负责 Zigbee、Yeelight、MQTT 和设备自动化；
- 在管理节点上，统一连接 SSH、IPMI、iDRAC、PVE、ESXi、OpenWrt 等外部设备。

这里的“角色”不是不同版本，也不是互斥模式。角色是经过验证的能力组合、默认配置和
导航视图。用户可以从角色模板开始，再自由增加或移除能力。

## 2. 产品原则

### 2.1 公共库优先

`dns-go`、`dhcp-go`、`ntp-go`、`yabber`、`yeelight-go`、`zigbee-go` 等项目首先是
独立公共库，其次才是 OhMyServer 的实现细节。

公共库应尽量满足：

- 不依赖 OhMyServer 的数据库、模板、HTTP Handler 或全局状态；
- 配置使用普通 Go struct，运行时通过显式构造函数创建；
- 生命周期至少支持 `Start`、`Stop`、`Reload` 或等价的上下文取消方式；
- 暴露状态、指标和结构化错误，但不规定如何展示；
- I/O、存储、日志、时钟等有需要时通过小接口注入；
- 可以被命令行工具、其他面板、测试程序和嵌入式应用直接引用；
- 保持协议实现和操作系统编排之间的边界。

OhMyServer 为这些库提供 adapter，把数据库模型、Web UI、权限、审计和服务生命周期
转换成公共库能够理解的配置与调用。

### 2.2 优先组合，不重复造轮子

适合做成公共库的部分包括协议、解析器、轻量服务端、客户端和跨平台逻辑。对于高度依赖
内核或成熟系统组件的领域，应优先做安全的适配层：

- 网络配置使用 netlink、NetworkManager 或 systemd-networkd 等现有能力；
- 防火墙使用 nftables；
- PPPoE 使用成熟的 PPP 实现；
- 文件共享管理 Samba、NFS、WebDAV 或 SFTP；
- RAID/存储池管理 mdadm、LVM、ZFS 或 Btrfs；
- 服务和日志管理 systemd、D-Bus 与 journald；
- 软件包管理适配 apt、dnf、pacman、apk、opkg 等后端。

面板可以统一模型和体验，但不应自行实现 RAID、文件系统、防火墙内核或包管理器。

### 2.3 所有变更都应可理解、可验证、可恢复

路由、防火墙、磁盘和远程访问配置可能让机器失联或造成数据损失。此类功能必须具备：

- 应用前预检和变更摘要；
- 明确区分只读检查、可逆变更和破坏性操作；
- 配置快照、备份与恢复；
- 失败回滚或带超时的自动回滚；
- 幂等操作；
- 审计记录；
- 对会中断网络、删除数据或降级冗余的操作进行二次确认。

“可以从网页执行命令”不是功能完成的标准。

### 2.4 默认简单，允许逐步深入

角色模板提供能工作的安全默认值。高级能力逐层展开，底层配置仍然可以查看和导出。
面板不能为了“简单”而隐藏端口冲突、权限、数据位置、证书来源和实际运行状态。

## 3. 产品模型：节点、角色、能力与资源

### 3.1 节点

节点是一个运行 OhMyServer 或 OhMyServer Agent 的主机。节点记录：

- 操作系统、架构、内核、发行版和版本；
- 网络接口、磁盘、虚拟化和容器能力；
- 可用命令、systemd、nftables、文件系统和硬件特征；
- 已启用角色与能力；
- 控制面版本、健康状态和最近心跳。

第一阶段仍以单机为主，但数据模型不应阻止未来管理多个节点。

### 3.2 能力

能力是最小的可管理功能，例如 `dns.server`、`dhcp.server`、`web.reverse_proxy`、
`storage.smart`、`share.smb` 或 `system.packages`。

每项能力应声明：

```go
type CapabilityDescriptor struct {
	ID                string
	Name              string
	SupportedPlatforms []PlatformConstraint
	Requires          []string
	Optional          []string
	Conflicts         []string
	Ports             []EndpointRequirement
	Privileges        []Privilege
	Destructive       bool
}
```

能力实现不必使用同一种技术，可以分为：

- **Embedded**：直接嵌入 Go 库，例如 DNS、DHCP、NTP、Yabber；
- **System Adapter**：调用或配置现有系统组件，例如 systemd、nftables、Samba；
- **Remote Adapter**：管理外部 API 或设备，例如 PVE、ESXi、OpenWrt、iDRAC；
- **Application**：通过 Docker/Compose 安装和管理的应用。

### 3.3 角色

角色是能力的推荐集合，而不是新的耦合层：

```yaml
id: router
name: Router
requires:
  - network.interfaces
  - network.routing
  - firewall.nftables
recommended:
  - dhcp.server
  - dns.server
  - ntp.server
  - vpn.wireguard
  - monitor.network
```

启用角色时只创建建议配置并启用相应导航，不复制模块数据。移除角色也不能直接删除用户
已经创建的 DNS Zone、共享目录或证书。

### 3.4 资源：期望状态与观测状态

所有长期运行服务都应区分：

- **Desired State**：用户希望启用的配置；
- **Observed State**：端口、进程、健康检查和依赖反映出的真实状态；
- **Conditions**：例如 `Ready`、`Degraded`、`PortConflict`、`MissingDependency`；
- **Last Applied Revision**：最后成功应用的配置版本。

现有 `service_preferences` 已经具备这一方向的雏形，后续应扩展为通用协调器，而不是让
每个页面各自保存配置后直接启动进程。

## 4. 角色规划

### 4.1 Router / Gateway

目标是让一台具有至少两个网络接口的 Linux 服务器安全地成为家庭、实验室或小型办公室
网关。

完整链路应是：

```text
WAN/PPPoE/DHCP Client
        ↓
Interface + Route + IPv4/IPv6 Forwarding
        ↓
nftables Zones / NAT / Port Forward
        ↓
LAN / VLAN / Bridge
        ↓
DHCP + DNS Filter + NTP
```

当前已有 DNS、DHCP、NTP、TCP/UDP Forward 和 Clash 管理能力，但距离“路由器”仍缺少
决定性的系统网络层：

- 网卡、地址、路由、Bridge、Bond 和 VLAN 管理；
- PPPoE 和普通 WAN DHCP Client；
- IPv4/IPv6 转发、NAT 和 nftables Zone 模型；
- IPv6 Prefix Delegation、Router Advertisement 和 DHCPv6；
- 端口转发与防火墙规则的统一模型；
- WireGuard，后续再考虑 OpenVPN、Tailscale/Headscale 等；
- DDNS、UPnP/NAT-PMP（默认关闭）；
- 每客户端流量、连接、DNS 命中和带宽观测；
- 网络变更超时回滚，防止远程操作后失联；
- DNS、DHCP 和本地域名联动，例如租约自动生成局域网记录。

Router MVP 的验收标准不应只是“能够开启 DHCP”，而应是一台新装 Linux 机器通过向导
完成 WAN、LAN、防火墙、DHCP 和 DNS 后，客户端可以安全地访问互联网。

### 4.2 NAS / File Server

目标是管理已有 Linux 存储栈并提供可靠共享，不把 OhMyServer 变成自研文件系统。

当前已有文件浏览、挂载点容量和 SMART 读取。后续按风险递增：

1. 只读盘点：磁盘、分区、文件系统、温度、SMART、阵列和存储池状态；
2. 健康能力：SMART 定时测试、Scrub 状态、容量阈值、坏盘和降级告警；
3. 共享：Samba、NFS、WebDAV、SFTP，统一到“Share”资源；
4. 身份与权限：本地用户/组、ACL、只读/读写、Guest 和网络限制；
5. 存储管理：已有 mdadm、LVM、ZFS、Btrfs 的发现、导入和常用操作；
6. 数据保护：快照、回收站、复制、restic/rclone 备份和恢复演练；
7. UPS、休眠、磁盘待机和温控。

创建阵列、格式化、扩容、替换磁盘属于高风险功能，应晚于只读状态、告警、共享和备份。
首个 NAS 版本可以只管理“已挂载目录 + 共享”，不必立即提供创建 RAID 的 UI。

Web 文件分享应与 Yabber/ACME 复用：

- 创建只读或带有效期的分享链接；
- 由 Yabber 暴露下载路由；
- 可选绑定域名和 ACME 证书；
- 支持访问策略、速率限制、并发限制、审计与撤销。

### 4.3 Cloud / Web Host

这是当前功能最接近完整闭环的角色：

- Yabber Listener、Policy、Virtual Host 与 Route；
- ACME 证书签发和热加载；
- 本地/托管 DNS；
- Docker、Compose、应用目录、项目部署和 GitHub Webhook；
- SSH、Terminal、监控、Cron 和 Uptime。

优先补齐：

- systemd Unit 的查看、启动、停止、重启、开机启用和日志；
- journald 结构化检索与 Web Server 日志页；
- apt/dnf/pacman/apk 软件包查询、安装、升级和安全更新；
- nftables/主机防火墙、安全组提示与端口暴露视图；
- 数据库实例管理与备份（可先通过应用和 adapter，不自研数据库服务）；
- 网站、证书、DNS、部署和备份组成一个可追踪的应用资源；
- 备份、恢复、迁移和灾难恢复文档；
- 通知、告警和维护窗口。

### 4.4 HomeLab / Virtualization

当前已有 Docker、Compose、PVE、ESXi、IPMI、iDRAC 和 SSH 管理。后续应统一成：

- Node → Runtime → Workload 的资源层级；
- 容器、虚拟机和物理机的状态、指标、控制与终端入口；
- 统一的应用目录，但保留 Compose 原文和数据目录；
- 服务发现和局域网域名自动登记；
- Wake-on-LAN、关机顺序、维护模式；
- 跨节点备份、迁移和简单调度；
- 基于标签的批量操作。

OhMyServer 不需要立刻成为 Kubernetes 或 PVE 的替代品。它应先做好多种运行时的轻量
统一入口，以及 DNS、证书、Web 暴露和监控之间的联动。

### 4.5 Edge / IoT Gateway

Yeelight、Zigbee 等库不应只形成一组设备详情页，而应汇入统一设备与自动化模型：

- 设备发现、配对、在线状态和能力描述；
- Room、Group、Tag；
- Zigbee Coordinator 与网络拓扑；
- MQTT Bridge；
- Scene 与 Automation；
- 指标、事件历史和离线告警；
- 局域网优先，不强依赖第三方云。

自动化建议使用“触发器 → 条件 → 动作”模型。脚本可以作为一种动作，但设备、Cron、
Webhook、证书续期、服务 Reload 等核心联动应使用类型化动作，不能只靠 shell 字符串。

### 4.6 Remote Management Node

SSH、OpenWrt、PVE、ESXi、IPMI、iDRAC 等现有模块可以组成远程管理角色：

- 统一资产、凭据引用和标签；
- 在线状态、版本、硬件摘要和告警；
- 远程 Terminal 与带审计的操作；
- 批量软件更新或命令任务；
- 带外电源控制；
- 凭据轮换和最小权限。

远端采集最终应由轻量 Agent 与无 Agent 的 SSH/API 两种模式共同支持。

## 5. 模块联动设计

### 5.1 不以数据库互相读取作为长期接口

同一单体内，Web Server 可以通过仓储接口加载 ACME 证书，但不能长期依赖
`acme_certificates` 的具体表结构。模块之间应依赖稳定接口：

```go
type CertificateSource interface {
	GetCertificate(ctx context.Context, id int64) (tls.Certificate, error)
	WatchCertificates(ctx context.Context) <-chan CertificateEvent
}
```

数据库是当前实现，接口才是模块契约。这样 Yabber 独立运行时可改用 TOML/文件，
OhMyServer 中则由 SQLite adapter 提供证书。

### 5.2 事件用于通知，命令用于改变状态

建议引入进程内的类型化 Event Bus：

- `CertificateIssued`
- `CertificateRenewed`
- `DNSRecordChanged`
- `DHCPLeaseChanged`
- `ServiceStateChanged`
- `DiskHealthChanged`
- `DeploymentCompleted`
- `DeviceOffline`

事件说明“已经发生了什么”，可以有多个订阅者。需要改变系统状态的操作则通过明确命令
执行，例如 `ReloadWebServer`、`PublishDNSRecord`、`CreateBackup`，并返回结果。

事件投递至少要支持：

- 事件 ID、时间、来源和资源引用；
- 同一事件重复投递时消费者幂等；
- 失败记录和有限重试；
- 关键事件落库，普通高频指标不必落库；
- 关联 ID，便于从 ACME 续期追踪到 Yabber Reload。

### 5.3 Scripts 是逃生舱，不是内部总线

Scripts 模块值得做，但定位应是用户扩展边界：

- 手动、Cron、Webhook 或事件触发脚本；
- 参数 Schema、超时、工作目录和环境变量白名单；
- 使用受限执行身份，不默认 root；
- Secret 只以引用注入并在日志中脱敏；
- 输出、退出码、开始/结束时间和触发来源可审计；
- 并发策略：Allow、Forbid、Replace；
- 可选容器或 Sandbox 隔离。

ACME → Web Server、DHCP → DNS 这类内置联动应走 Go 接口和类型化命令。脚本适合执行
“证书续期后同步到一台旧设备”之类无法预先建模的用户逻辑。

### 5.4 Cron 是触发器，不是业务所有者

Cron 可以触发证书检查、SMART Test 或备份，但续期条件、锁、重试和状态仍归 ACME
Issuance 所有。任务调度器不应知道如何签证书，ACME 模块也不应自己实现另一套 Cron。

## 6. 与其他面板的差异

竞品不是一个单一类别。OhMyServer 实际同时接近系统管理、网站托管、NAS、路由器和
家庭云面板，因此应分别比较，不能用“功能数量”得出简单结论。

| 类别与代表 | 强项 | OhMyServer 的机会 | OhMyServer 当前劣势 |
| --- | --- | --- | --- |
| Cockpit | 深度结合 Linux、systemd、journald、存储和网络，接近系统原生管理 | 用更紧凑的 Go 控制面提供内嵌网络服务与跨模块工作流 | 系统管理深度、发行版适配、权限和成熟度不足 |
| Webmin | 模块多、覆盖传统 Unix 服务、长期积累广 | 使用现代 Go 接口和资源模型，减少直接编辑配置文件带来的耦合 | 模块数量、平台覆盖、社区积累不足 |
| 1Panel / 网站面板 | 网站、数据库、容器、应用安装和运维闭环成熟 | DNS、DHCP、NTP、IoT、远程硬件等范围更广，Yabber 可嵌入 | 网站生态、数据库管理、备份迁移和用户体验不足 |
| TrueNAS 等 NAS 系统 | 存储池、数据保护、共享、权限和硬件验证非常深入 | 在普通 Linux 上提供轻量 NAS 能力，并与 Web、DNS、容器组合 | 不应承诺同等级的数据安全、ZFS 深度和硬件验证 |
| OpenWrt / LuCI | 路由、无线、网络接口、防火墙、软件包生态完整 | 在通用 Linux/小主机上组合路由、NAS、Web Host 和 HomeLab | 缺少网络配置、IPv6、无线、回滚和大量设备适配 |
| CasaOS 类家庭云 | 上手简单、应用目录和家庭服务器体验友好 | 不局限于容器应用，可管理协议服务和底层系统资源 | 安装引导、应用生态、视觉体验和非技术用户路径不足 |

### 6.1 可能形成的核心优势

1. **公共库可以脱离面板复用**：功能不是只存在于页面后的 shell 命令，而是可测试、可嵌入的 Go 能力。

2. **同一控制面覆盖多种角色**：DNS、DHCP、NTP、Web、ACME、设备和系统管理可以组合，而不是要求用户更换整套系统。

3. **跨模块工作流可以成为一等能力**：例如 ACME → Yabber、DHCP Lease → DNS、Share → Web Route → Certificate、
   Deploy → Health Check → DNS Cutover。

4. **单二进制和较低资源占用**：嵌入式网络服务适合小主机、边缘节点和临时环境。

5. **协议层可控且便于观测**：自有公共库能够提供一致的指标、错误和生命周期接口。

### 6.2 必须正视的劣势

1. **范围极宽，容易每个领域都只做到演示级**：路由、NAS 和云托管都有很深的安全与可靠性要求。

2. **系统差异远大于纯 Go 服务差异**：发行版、init、包管理、网络栈、文件系统、权限和硬件会显著增加测试矩阵。

3. **控制面拥有高权限，安全风险集中**：文件、Terminal、Docker、网络和磁盘操作一旦越权，影响是整台主机。

4. **生态和文档尚不足**：成熟面板的价值不仅在代码，还在模板、硬件验证、迁移工具、用户社区和故障经验。

5. **单进程内嵌服务会扩大故障域**：DNS、DHCP、Web 与控制面需要隔离故障、限制资源，并能分别重启和诊断。

因此不应以“替代所有面板”为短期目标，而应优先做好几条其他产品难以同时完成的闭环。

## 7. 值得做但目前缺失的基础能力

以下能力比继续增加孤立模块更重要。

### P0：可信控制面

- 多用户、RBAC、可选 OIDC/SSO 和 MFA；
- CSRF、Secure Cookie、登录限速和会话管理；
- Secret Store：数据库敏感字段加密、主密钥管理、脱敏；
- 完整审计日志：谁在何时对哪个资源做了什么，结果如何；
- 配置导出、控制面数据库备份与恢复；
- 高风险操作的 Plan/Apply、快照、回滚和确认；
- 文件系统访问边界，避免默认暴露整台主机；
- 权限代理：控制面尽量非 root，只把必要操作交给受限 Helper；
- 安全更新机制和可验证发布产物。

### P1：角色化平台骨架

- Capability Registry 与平台探测；
- Role Manifest、安装向导和按角色导航；
- 通用 Service Coordinator 与 Reconciler；
- 任务系统：进度、日志、取消、超时、重试和互斥；
- 类型化 Event Bus、Automation 与受限 Scripts；
- 统一结构化日志、请求 ID、指标、健康检查和诊断包；
- 通知渠道：Webhook、Email，以及可插拔的消息服务；
- 配置修订历史和差异；
- 单独重启关键运行时或未来拆分为子进程的能力。

### P2：三条可交付角色闭环

**Cloud/Web Host**

- systemd、journald、packages、firewall；
- Web Server 日志、证书续期告警；
- 应用级备份/恢复；
- 网站、部署、DNS、证书的统一资源视图。

**Router**

- Interface/VLAN/Bridge、WAN/PPPoE；
- nftables Zone/NAT/Forward；
- IPv6、WireGuard；
- 网络安全回滚；
- DHCP/DNS/客户端可观测性。

**NAS**

- SMART 定时测试与告警；
- Share + User/Group/ACL；
- Samba、NFS、WebDAV；
- 已有存储池只读发现；
- Snapshot/Backup/Restore。

### P3：多节点与设备自动化

- 轻量 Agent、节点注册和证书身份；
- 批量任务、标签和维护窗口；
- 多节点指标、日志和事件；
- Zigbee、MQTT、Scene、Automation；
- 服务发现与跨节点 DNS；
- 备份复制和迁移辅助。

## 8. 建议交付顺序

### Milestone 1：先让控制面值得信任

完成 P0，并把所有现有危险操作纳入审计和权限边界。没有这一层，不应开放公网管理，
也不应继续增加格式化磁盘、防火墙和包升级等能力。

### Milestone 2：建立平台骨架

实现能力探测、角色 Manifest、通用任务、事件、通知、日志和服务协调。把现有 DNS、
DHCP、NTP、Web、Forward 作为第一批能力迁入，而不是另起一套抽象。

### Milestone 3：打通 Cloud/Web Host

该角色离完成最近，优先形成：

```text
Deploy → Yabber Route → DNS Record → ACME Certificate
       → Health Check → Access/Error Logs → Backup
```

这条链路能验证事件、任务、权限、Secret 和审计等平台能力。

### Milestone 4：分别完成 Router MVP 与 NAS MVP

两个角色分开验收：

- Router MVP 以“安全完成 WAN 到 LAN 上网并可自动回滚”为准；
- NAS MVP 以“已有磁盘可观测、目录可安全共享、数据可备份恢复”为准。

先支持明确的 Linux 发行版与后端组合，例如 Debian/Ubuntu + systemd + nftables。
不要一开始宣称所有发行版、所有 RAID 和所有网络管理器都受支持。

### Milestone 5：多节点与 IoT

当单节点的资源、任务、事件和权限模型稳定后，再引入 Agent 和设备自动化，避免把单体
中尚未稳定的内部接口直接变成远程协议。

## 9. 近期可执行清单

1. 创建 `CapabilityDescriptor` 和平台探测器，只读展示当前主机能启用哪些能力；
2. 把现有五种托管服务注册到统一 Capability/Service 接口；
3. 为操作引入统一 Task 记录，不再让长任务只存在于 HTTP 请求生命周期；
4. 增加结构化 Audit Event，并先覆盖登录、服务启停、证书、部署和删除操作；
5. 增加 Secret Store，迁移 SSH、DNS Provider、IPMI/iDRAC 等凭据；
6. 增加 Web Server Logs 页面和系统 Logs 页面；
7. 实现 systemd + journald 只读管理，再逐步开放变更；
8. 建立 Router 和 NAS 的平台检测页面，先报告缺失依赖，不立即修改系统；
9. 选择 Debian/Ubuntu 作为第一组端到端验证平台；
10. 为三条关键工作流编写集成测试：
    - ACME renew → Yabber certificate reload；
    - DHCP lease → local DNS record；
    - Share publish → Yabber route + policy + certificate。

## 10. 成功标准

项目是否成功，不以菜单数量或支持多少命令衡量，而以以下结果衡量：

- 用户能看懂一项能力是否可用、为什么不可用；
- 配置写入后，真实运行状态能被持续验证；
- 跨模块操作有清晰的资源关系、任务进度和审计链路；
- 失败不会静默发生，危险变更能够恢复；
- 公共库仍可被其他项目独立引用；
- 新角色主要由组合已有能力产生，而不是复制一套实现；
- 每个正式支持的角色都有可复现的安装、升级、备份和故障恢复测试。

## 11. 参考产品边界

以下资料用于理解不同类型面板的产品边界，不表示 OhMyServer 需要复制它们的全部功能：

- [Cockpit Applications](https://cockpit-project.org/applications)
- [Cockpit Feature Internals](https://cockpit-project.org/guide/latest/features)
- [Webmin Introduction](https://webmin.com/docs/intro/)
- [Webmin Creating Modules](https://webmin.com/docs/development/creating-modules/)
- [1Panel](https://1panel.cn/)
- [TrueNAS Storage Pools](https://www.truenas.com/docs/scale/storage/pools/)
- [TrueNAS UI Configuration and Shares](https://www.truenas.com/docs/scale/24.04/gettingstarted/configure/uiconfigurationscale/)
- [OpenWrt User Guide](https://openwrt.org/docs/guide-user/start)
- [OpenWrt Firewall Configuration](https://openwrt.org/docs/guide-user/firewall/start)
