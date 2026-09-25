# pkgmgr

`pkgmgr` 是一个不依赖 OhMyServer、数据库或 Web 框架的软件包管理抽象，当前支持：

| Backend | Refresh | Install | Remove | Upgrade | Query |
| --- | --- | --- | --- | --- | --- |
| APT | `apt-get update` | `apt-get install` | `apt-get remove` | `apt-get upgrade` | `apt`、`apt-cache`、`dpkg-query` |
| DNF/DNF5 | `makecache` | `install` | `remove` | `upgrade` | `repoquery` |
| Pacman | `-Sy` | `-S` | `-R` | `-Syu` | `-Q`、`-Qu`、`-Ss`、`-Si` |
| APK | `update` | `add` | `del` | `upgrade` | `list`、`search`、`info` |
| opkg | `update` | `install` | `remove` | 显式包升级 | `list-installed`、`list-upgradable`、`info` |
| Homebrew | `update` | `install` | `uninstall` | `upgrade` | `list`、JSON v2、`search` |

## 使用

```go
manager, err := pkgmgr.Detect(pkgmgr.Options{
	// 只提升变更操作；查询仍以 OhMyServer 的普通运行用户执行。
	MutatingExecutor: pkgmgr.Sudo(nil),
})
if err != nil {
	return err
}

installed, err := manager.ListInstalled(ctx)
if err != nil {
	return err
}

result, err := manager.Install(
	ctx,
	[]string{"curl"},
	pkgmgr.ChangeOptions{AssumeYes: true},
)
```

也可以显式选择后端：

```go
manager, err := pkgmgr.New(pkgmgr.APT, pkgmgr.Options{})
```

`Command` 可以在执行前生成，用于预览和审计：

```go
command, err := manager.Command(
	pkgmgr.Remove,
	[]string{"curl"},
	pkgmgr.ChangeOptions{AssumeYes: true},
)
```

## 执行边界

- 所有包名都作为独立 argv 传入，绝不经过 shell；
- 包名不能以 `-` 开头，也不能包含空白或控制字符；
- 查询与变更可以使用不同的 `Executor`；
- 默认不自动使用 sudo；
- `PrefixExecutor` 可连接 sudo、doas 或未来的权限 Helper；
- 同一个 `Manager` 会串行执行命令，避免自身并发争用包管理器锁；
- `LC_ALL=C` 和 `LANG=C` 用于稳定解析命令输出。

## opkg

OpenWrt 不建议对整个系统直接执行裸的 `opkg upgrade`。当调用：

```go
manager.Upgrade(ctx, nil, options)
```

`pkgmgr` 会先执行 `opkg list-upgradable`，取得明确的包名集合，再执行：

```text
opkg upgrade package1 package2 ...
```

`Capabilities().SafeUpgradeAll` 仍为 `false`，控制面应在执行前显示风险提示。若调用方已经
选择了具体包，则可以直接把包名传给 `Upgrade`。

由于生成该命令前必须先查询运行中的设备，`Command(Upgrade, nil, ...)` 不会隐式执行
查询，而会返回 `ErrUnsafeOperation`。需要自动解析时应调用 `Upgrade` 或 `Apply`。

## 独立迁出边界

包内不应加入：

- OhMyServer 数据库模型；
- HTTP Handler 或 HTML 模板；
- 审计日志的具体存储；
- 特定的 root Helper 协议；
- 发行版角色或 UI 导航。

这些能力应由 OhMyServer 通过 `Executor` 和公开方法进行组合。
