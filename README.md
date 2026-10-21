<div align="center">
  <img src="webui/public/logo.svg" alt="Lodestar Logo" width="96" height="96" />
  <h1>Lodestar</h1>
  <p><strong>自托管的应用发布、版本分发与远程部署平台</strong></p>
  <p>一个控制台，管理版本包、生产主机、部署流程与客户端更新。</p>
  <p><strong>简体中文</strong> · <a href="README.en.md">English</a></p>
  <p>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/cubevlmu/lodestar/actions/workflows/ci.yml/badge.svg" /></a>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/release.yml"><img alt="Release" src="https://github.com/cubevlmu/lodestar/actions/workflows/release.yml/badge.svg" /></a>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/docker.yml"><img alt="Docker" src="https://github.com/cubevlmu/lodestar/actions/workflows/docker.yml/badge.svg" /></a>
    <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-blue.svg" /></a>
  </p>
</div>

## 项目简介

Lodestar 面向需要管理多个应用、多个版本及多台服务器的个人开发者和小型团队。CI 构建完成后，上传产物到控制面；生产机上的 Agent **主动拉取任务**，完成校验、备份、文件同步、重启和失败回滚。控制面同时提供 Web 管理后台及供桌面、移动客户端使用的版本检查 API。

Lodestar **不是构建系统或容器编排器**。它接收已经构建好的发布包，负责分发与部署。部署命令由用户配置，Agent 执行它们时拥有相应的主机权限。

### 主要功能

| 功能 | 说明 |
| --- | --- |
| 版本管理 | 发布包上传或登记外部下载地址；版本、平台、架构、渠道、变更日志及灰度比例 |
| 自动部署 | HTTP API 对接 GitHub Actions、GitLab CI 或其他 CI；按目标主机下发任务 |
| 生产机 Agent | 主动长轮询，不要求生产机开放入站 SSH 端口；一个 Agent 管理多个应用 |
| 安全换版 | SHA-256 校验、解压路径防护、备份、镜像同步、可选重启和健康检查、失败回滚 |
| 运行数据保护 | 通过 `preserve` 保留配置、数据库、上传文件等路径，部署及回滚不会覆盖它们 |
| 客户端更新 | HMAC-SHA256 请求认证、版本策略、维护模式、版本封禁、Ed25519 清单签名 |
| Web 控制台 | 项目、版本包、目标主机、部署配置、任务日志、回滚及审计 |
| 简单部署 | Go + Gin + GORM + SQLite；Vue 3 + PrimeVue；前端可嵌入 Go 可执行文件 |

## 工作方式

```text
GitHub Actions / CI ── 上传构建产物 ──> Lodestar Server
       Web 管理后台 ── 管理项目/部署 ──> │   ├─ SQLite（元数据、任务、审计）
         App 客户端 ── 检查更新 ───────> │   └─ Storage（版本包）
                                       │
           生产机 Agent <── 长轮询任务 ─┘
                 │
                 └─ 下载 → SHA-256 校验 → 解压 → 停服（可选）
                    → 备份 → 同步文件 → 启服（可选）
                    → 健康检查（可选）→ 失败则恢复备份
```

**核心对象：** Project（应用）、Release（发布版本）、Target（主机）、Deploy Config（主机与应用的部署配置）、Job（部署或回滚任务）。每台主机可针对不同应用设置不同安装路径、命令和保留规则。

## 快速开始

### 方式一：Docker Compose

需要 Docker 和 Docker Compose。克隆仓库后启动控制面：

```bash
git clone https://github.com/cubevlmu/lodestar.git
cd lodestar
docker compose up -d --build
docker compose logs lodestar
```

访问 `http://localhost:8080`。首次运行自动创建数据库、配置文件和管理员账户；随机初始密码会输出到日志，并保存于 `data/initial-admin.txt`。**登录后立即修改密码。**

Compose 默认将数据库/配置保存到 `./data`，发布包保存到 `./storage`。生产环境请配置 HTTPS 反向代理，并设置 `LODESTAR_PUBLIC_BASE_URL=https://你的域名`。

### 方式二：从源码构建

需要 Go 1.24+、Node.js 20+ 和 npm。先构建 WebUI，再编译 Go 程序：

```bash
git clone https://github.com/cubevlmu/lodestar.git
cd lodestar
cd webui
npm ci
npm run build
cd ..
go build -o lodestar ./cmd/lodestar
./lodestar serve
```

Linux/macOS 也可运行 `make build`。Windows 可参考 `scripts/build.ps1`。生产环境请使用构建完成且内嵌 WebUI 的程序；仓库内的 `internal/web/dist/.gitkeep` 本身不是前端构建产物。

### 接入生产机 Agent

1. 登录 WebUI，在**项目**中新建应用，取得项目的 `deploy_token`。
2. 在**目标主机**中登记生产机，复制 `target_token`。
3. 在生产机放置 Lodestar 可执行程序，创建 `data/config/system.yaml`：

```yaml
agent:
  server_url: "https://lodestar.example.com"
  target_token: "REPLACE_WITH_TARGET_TOKEN"
  name: "prod-01"
  deploy_root: "/srv/apps"
  backup_dir: "/srv/lodestar-backups"
  work_dir: "/var/lib/lodestar"
  allow_server_commands: true
```

```bash
./lodestar agent
```

4. 回到 WebUI **部署配置**，为「主机 × 应用」配置安装目录、停启/重启命令、健康检查、`preserve` 路径和备份保留数。命令留空代表只更新文件，不自动重启。

Agent 默认主动轮询控制面，不要求控制面可以直接连接生产机。Agent 的运行账户必须对部署目录和备份目录拥有必要权限。

也可使用 Compose 的可选 Agent profile，先设置 `LODESTAR_AGENT_TARGET_TOKEN`，再运行：

```bash
docker compose --profile agent up -d --build
```

**注意：** Agent 容器只能直接操作挂载到容器内的文件系统；如果要控制其他容器或宿主机服务，必须单独配置所需权限与可执行的管理命令。

### 从 CI 上传并部署

在 WebUI 项目详情复制 `deploy_token`，放入 CI 的 Secret。上传已经构建完成的包：

```bash
curl --fail-with-body -sS -X POST "https://lodestar.example.com/api/v1/deploy" \
  -H "X-Deploy-Token: $LODESTAR_DEPLOY_TOKEN" \
  -F "version=1.4.2" \
  -F "platform=linux" \
  -F "arch=amd64" \
  -F "channel=stable" \
  -F "wait_seconds=180" \
  -F "package=@dist/app-linux-amd64.tar.gz"
```

支持 `.zip`、`.tar`、`.tar.gz` 及单文件可执行程序。`wait_seconds=0` 时任务入队后立即返回；设置非零值可在请求中等待任务结果（上限 600 秒）。没有匹配主机或部署配置时，发布包仍可登记，但不代表已部署成功。

完整的 [GitHub Actions CI 集成示例](examples/github-actions.yml)。

## 部署、备份与回滚

Agent 先下载、校验 SHA-256，再解压构建包。按部署配置执行可选停服命令，创建备份，然后**镜像同步**目标目录：覆盖变更文件，也会删除发布包中不存在的旧文件。之后执行可选启动/重启命令及健康检查，失败时尝试恢复先前的备份。

`preserve` 是相对部署目录的文件或目录白名单，例如：

```yaml
preserve:
  - appsettings.json
  - data
  - uploads
  - logs
```

这些路径在部署中不会覆盖或删除；备份时不会复制；回滚时保留当前内容。**务必将运行时数据库、上传文件、配置、证书等加入白名单，或放到部署目录以外。** 普通应用文件才应该由发布包全量管理。

WebUI 可查看任务执行日志、重试及回滚。回滚依赖本机保留的备份，并非选择任意历史版本重新安装。强烈建议配置有效的健康检查，以便发现应用启动后的故障。

## 客户端版本更新

客户端通过 `POST /api/v1/check` 查询版本，签名头包括 `X-App-Id`、`X-Timestamp`、`X-Nonce` 和 `X-Signature`。请求体参与 HMAC-SHA256 签名；返回信息包含可用版本、更新类型、签名下载链接和 Ed25519 清单签名。更新策略可设置最低支持版本、维护状态和封禁版本。

已有可运行的 [Go 客户端示例](examples/client/go/main.go) 和 [Dart / Flutter 客户端示例](examples/client/dart/check_update.dart)。客户端必须校验签名，不能仅凭返回的下载 URL 信任包体。

## 配置与接口

配置文件默认路径为 `data/config/system.yaml`，也可以通过 `-config` 或 `LODESTAR_CONFIG` 指定。常用设置：

| 环境变量 | 作用 |
| --- | --- |
| `LODESTAR_ADDR` | 服务端监听地址，默认 `:8080` |
| `LODESTAR_DB_PATH` | SQLite 数据库路径 |
| `LODESTAR_STORAGE_DIR` | 发布包存储路径 |
| `LODESTAR_PUBLIC_BASE_URL` | 对外访问地址，反向代理环境建议明确指定 |
| `LODESTAR_MAX_UPLOAD_MB` | 单次上传大小上限 |
| `LODESTAR_ADMIN_USERNAME` | 初始管理员用户名 |
| `LODESTAR_ADMIN_PASSWORD` | 可指定首次创建管理员时的密码 |

其他选项见 [服务端配置示例](examples/system.example.yaml) 和 [Agent 配置示例](examples/agent.example.yaml)。

| API | 作用 | 认证 |
| --- | --- | --- |
| `POST /api/v1/auth/login` | 管理后台登录 | 账号密码 |
| `/api/v1/projects`、`/api/v1/releases` | 项目及版本管理 | Bearer 会话令牌 |
| `/api/v1/targets`、`/api/v1/deploy-configs` | 目标主机及部署配置 | Bearer 会话令牌 |
| `/api/v1/jobs` | 部署任务与日志 | Bearer 会话令牌 |
| `POST /api/v1/deploy` | CI 上传、登记并触发部署 | `X-Deploy-Token` |
| `GET /api/v1/deploy/jobs/{id}` | 查询 CI 部署任务 | `X-Deploy-Token` |
| `POST /api/v1/check` | 客户端检查更新 | HMAC 请求签名 |
| `/api/v1/agent
