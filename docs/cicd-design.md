# 极简 CI/CD 设计

状态：Implemented
范围：OhMyServer 单机版

## 目标

`/projects` 提供一条固定、容易理解的 Docker 发布流程：

```text
手动点击 / GitHub push
          ↓
创建 Deployment
          ↓
检出确定的 Git commit
          ↓
使用仓库根目录 Dockerfile 构建镜像
          ↓
替换该 Project 的运行容器
          ↓
保存状态、提交信息和日志
```

保持以下约束：

- 一个 Project 同时最多运行一个 Deployment；
- 手动触发和 GitHub Webhook 复用同一个执行器；
- SQLite 是 Project、Deployment、状态和日志的事实来源；
- 内存只保存当前执行器，用于取消任务；
- 不引入 Pipeline DSL、任务队列、Redis、独立 Runner 或 GitHub API；
- 编译和测试由 Dockerfile 的 multi-stage build 表达；
- 不在宿主机直接执行仓库提供的脚本。

## 数据模型

### projects

现有字段保存名称、描述和 Git 仓库地址，新增：

| 字段 | 用途 |
|---|---|
| `webhook_secret` | GitHub Webhook HMAC-SHA256 Secret |
| `webhook_branch` | 接受 push 的分支，默认 `main` |

每个 Project 的 Webhook 地址为：

```text
POST /hooks/github/{projectID}
```

Project ID 只是路由参数，不是凭据。请求必须通过 `webhook_secret` 验签。

### deployments

`Deployment` 同时表示一次构建和一次发布。关键字段：

| 字段 | 用途 |
|---|---|
| `status` | `pending/running/completed/failed/cancelled` |
| `commit` | 实际构建的完整 Git SHA |
| `commit_msg`、`author` | 构建来源信息 |
| `docker_image` | 本次构建产生的唯一镜像标签 |
| `container_name/id` | 发布后的稳定容器名称和实例 ID |
| `triggered_by` | `Manual` 或 `GitHub push` |
| `github_delivery_id` | GitHub 投递去重键 |

状态机只有一条：

```text
pending → running → completed
                  ├→ failed
                  └→ cancelled
```

服务重启时，失去执行器的 `pending/running` 任务标记为 `failed` 并写入日志。

## GitHub Webhook

### 请求

GitHub Repository Webhook 配置：

- Payload URL：`https://<host>/hooks/github/<projectID>`
- Content type：`application/json`
- Secret：Project Settings 中生成的 Secret
- Event：只选择 `push`

接收端最多读取 2 MiB 原始请求体，并验证：

```text
X-Hub-Signature-256 = sha256=<HMAC-SHA256(rawBody, webhook_secret)>
```

使用 `hmac.Equal` 比较签名。验签前不解析 JSON。

### 事件处理

| GitHub Event | 行为 |
|---|---|
| `ping` | 验签后返回 `200`，不创建 Deployment |
| `push` | 校验分支和 commit，创建 Deployment |
| 其他事件 | 返回 `200 ignored` |

`push` 只使用以下字段：

```json
{
  "ref": "refs/heads/main",
  "after": "<40-char commit sha>",
  "deleted": false,
  "head_commit": {
    "message": "commit message",
    "author": {"name": "author"}
  }
}
```

规则：

1. 签名必须正确；
2. `ref` 必须等于 `refs/heads/<webhook_branch>`；
3. 删除分支事件不触发；
4. `after` 必须是 40 位十六进制 SHA；
5. `X-GitHub-Delivery` 必须存在并在数据库中唯一；
6. Project 已有 `pending/running` Deployment 时返回 `409`，不并行构建。

Webhook 只创建任务并启动现有异步执行器，不在 HTTP Handler 内等待构建。

## Checkout、Build、Deploy

### Checkout

- 每次 Deployment 使用独立目录：
  `workspace/projects/<projectID>/<deploymentID>`；
- 手动构建浅克隆仓库默认分支；
- Webhook 构建检出 payload 指定的 commit；
- 检出后从 Git 读取并持久化 commit、message、author；
- 完成后删除工作目录。

Project 名称不参与文件路径，避免名称冲突和路径穿越。

### Build

- 必须存在仓库根目录 `Dockerfile`；
- 镜像标签固定为：
  `ohmyserver/project-<projectID>:deployment-<deploymentID>`；
- Docker build 使用 Deployment 的取消 Context；
- Docker 流式错误必须返回执行器，不能只写日志后仍标记成功。

### Deploy

- 一个 Project 使用稳定容器名：
  `ohmyserver-project-<projectID>`；
- 镜像构建成功后才移除旧容器；
- 创建和启动新容器后保存 Container ID；
- 创建或启动失败时 Deployment 标记为 `failed`；
- 当前版本不实现蓝绿发布、健康检查和自动回滚。

## HTTP 与页面

管理路由继续使用登录认证：

```text
GET        /projects
GET/POST /projects/new
GET/DELETE /projects/{id}
GET/POST   /projects/{id}/settings
GET/POST   /projects/{id}/deployments
GET        /projects/{id}/deployments/{deploymentId}
POST       /projects/{id}/deployments/{deploymentId}/cancel
```

Webhook 路由不使用登录认证，只使用 HMAC：

```text
POST /hooks/github/{id}
```

Deployment 页面通过 SSE 读取数据库日志和状态。所有详情、日志、取消请求都必须
校验 Deployment 属于 URL 中的 Project。

## 明确不实现

- YAML Pipeline、Stage、DAG 或矩阵构建；
- 多机 Runner 和并行构建；
- GitHub App、OAuth、Check Run 和状态回写；
- GitLab/Gitea Webhook；
- 构建缓存和 Artifact 服务；
- 自动回滚、蓝绿或金丝雀发布；
- 任意宿主机部署脚本。

这些能力只有在固定 Docker 流程无法满足实际需求时再增加。

## 参考

- [GitHub：Validating webhook deliveries](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)
- [GitHub：Best practices for using webhooks](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)
