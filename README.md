<a id="readme-top"></a>

[![Go][go-shield]][go-url]
[![Gin][gin-shield]][gin-url]
[![GORM][gorm-shield]][gorm-url]
[![SQLite][sqlite-shield]][sqlite-url]
[![Vue][vue-shield]][vue-url]
[![PrimeVue][primevue-shield]][primevue-url]

[![CI](https://github.com/cubevlmu/gx_updater/actions/workflows/ci.yml/badge.svg)](https://github.com/cubevlmu/gx_updater/actions/workflows/ci.yml)
[![Release](https://github.com/cubevlmu/gx_updater/actions/workflows/release.yml/badge.svg)](https://github.com/cubevlmu/gx_updater/actions/workflows/release.yml)
[![Docker](https://github.com/cubevlmu/gx_updater/actions/workflows/docker.yml/badge.svg)](https://github.com/cubevlmu/gx_updater/actions/workflows/docker.yml)

<br />
<div align="center">
  <h3 align="center">Lodestar</h3>

  <p align="center">
    发布与部署控制台 —— 管版本包、推任务、在生产机上安全换版。
    <br />
    Go + Gin + GORM + SQLite，前端 PrimeVue 5，单文件可执行程序，内嵌 WebUI。
  </p>
</div>

## 目录

- [这是什么](#这是什么)
- [为什么叫 Lodestar](#为什么叫-lodestar)
- [核心概念](#核心概念)
- [架构](#架构)
- [快速开始](#快速开始)
- [配置参考](#配置参考)
- [WebUI](#webui)
- [部署语义](#部署语义)
- [API 参考](#api-参考)
- [CI 集成](#ci-集成)
- [客户端自动更新](#客户端自动更新)
- [Docker 部署](#docker-部署)
- [开发与构建](#开发与构建)
- [测试](#测试)
- [目录结构](#目录结构)
- [安全说明](#安全说明)

## 这是什么

Lodestar 是一个**自托管的小型运维辅助工具**，把"构建完成 → 上线"这段流程收进一个可执行文件里。它做三件事：

1. **版本包登记与分发** —— 上传构建产物（zip / tar / tar.gz / 单文件二进制），记录版本、平台、架构、通道、灰度比例，供客户端调用 `/api/v1/check` 检查更新。
2. **远程部署** —— CI 或人把包 `POST` 给服务端，服务端把它下发到生产主机，主机上的 Agent 下载、校验、解压、备份、替换、重启，失败自动回滚。
3. **可观测** —— WebUI 看每次部署的日志、耗时、结果，支持重试、回滚、取消，所有写操作进审计日志。

和"只返回下载地址的更新服务器"的区别：Lodestar 不是只告诉你新版本在哪，它**真的会把新版本装到机器上**。所以不需要再为每次发布写一堆 `ssh + scp + systemctl restart` 的脚本。

一句话概括：

```
GitHub Actions 构建完  ──POST /api/v1/deploy──▶  Lodestar 服务端  ◀──长轮询──  lodestar agent（生产机）
                                                      │
                                                      └─ 下载 / 校验 / 备份 / 换版 / 重启 / 健康检查 / 回滚
```

### 特性

- **双模式单二进制**：`lodestar serve` 是控制面，`lodestar agent` 是主机侧执行器，同一个文件。
- **内嵌 WebUI**：`//go:embed` 打包，`./lodestar serve` 一个命令就有完整控制台，不需要 nginx、不需要 node。
- **拉模式部署**：生产机主动连服务端（长轮询 20s），不需要对外开放 SSH，也不需要服务端能访问生产内网。
- **强校验**：包体 SHA-256 校验、zip-slip 防护、解压体积上限、客户端 HMAC 签名 + nonce 防重放、清单 Ed25519 签名。
- **安全换版**：停服 → 时间戳备份 → 镜像同步（保留 `data/`、`config/` 等运行时目录）→ 启服 → 健康检查；任一步失败自动回滚。
- **保留路径**：`preserve: [data, config]` 这类目录在升级时不会被包内文件覆盖，只有主机上不存在时才用包里的默认值。
- **多项目多主机**：一个实例可以管多个 app、多个平台、多个通道、多台生产机。
- **纯 Go SQLite**：`glebarez/sqlite`（modernc 后端），无 CGO，交叉编译直接出静态二进制。

## 为什么叫 Lodestar

Lodestar 是航海里的**北极星**：位置几乎不动，所有测量都以它为基准。运维里需要的正是这么一颗星——一个稳定的参照点，告诉你"线上现在到底是哪个版本、上次为什么回滚"。

名字也刻意去掉了 `update`：它已经不只是个更新服务器了。

## 核心概念

| 概念 | 说明 |
| --- | --- |
| **项目 Project** | 一个被发布的应用。持有 `app_id`、`client_secret`（客户端签名）、`deploy_token`（CI 推送凭据）、Ed25519 密钥对（清单签名）。 |
| **版本包 Release** | 某个项目在某平台/架构/通道上的一个构建产物。含版本号、版本码、大小、SHA-256、变更说明、灰度比例、启用状态。 |
| **目标主机 Target** | 一台生产服务器（跑一个 Agent）。只记录机器本身：`target_token`、平台、默认部署根目录、备份目录。**不绑定项目，也不带任何命令**。 |
| **部署配置 TargetApp** | 「主机 × 项目」这一层，也就是「部署配置」页面里的每一行：装在哪（`deploy_dir`）、停/启/重启命令、健康检查、白名单、保留备份数、当前版本。首次部署自动创建，也可提前手工建。**命令全部可选**，留空就只替换文件。 |
| **部署任务 Job** | 一次"把某个版本装到某台主机的某个目录"的动作。任务创建时会把 `deploy_dir` / `backup_dir` **快照**下来，之后改配置也不影响历史记录。状态 `pending → running → success / failed / cancelled`，模式分 `deploy` 和 `rollback`。 |
| **部署日志 DeployLog** | Agent 逐条上报的执行日志，按 `job_id` 归属，WebUI 抽屉里看。 |
| **更新策略 Policy** | 按项目/平台/通道设定最低支持版本、维护模式、提示文案。 |
| **版本封禁 BlockedVersion** | 精确封禁某个版本码，客户端检查时被拒绝。 |

## 架构

```
                    ┌──────────────────────────────────────────┐
   CI / 人 ────────▶│  POST /api/v1/deploy      (deploy_token)  │
                    │                                          │
   WebUI ──────────▶│  /api/v1/*                (session token) │
                    │                                          │
   App 客户端 ─────▶│  POST /api/v1/check       (HMAC 签名)     │
                    └───────────────┬──────────────────────────┘
                                    │
                            ┌───────▼────────┐
                            │   SQLite (WAL) │  项目 / 版本 / 主机 / 任务 / 日志 / 审计
                            │   storage/     │  上传的包体
                            └───────┬────────┘
                                    │  GET /api/v1/agent/jobs/next?wait=20s   (target_token)
                    ┌───────────────▼──────────────────────────┐
                    │  lodestar agent（生产主机）               │
                    │  download → sha256 → extract → stop      │
                    │  → backup → sync → start → health        │
                    │  → 失败则 rollback                        │
                    └──────────────────────────────────────────┘
```

Agent 是**拉**的模式：服务端不主动连生产机，生产机开机就连上来挂着长轮询。这样生产机不用开任何入站端口，服务端也不用知道生产机的地址——NAT 后面、内网里都能用。

## 快速开始

### 1. 编译

```bash
git clone <this-repo> lodestar && cd lodestar
cd webui && npm install && npm run build && cd ..
go build -o lodestar ./cmd/lodestar
```

或者用 Makefile 一步到位（见[开发与构建](#开发与构建)）。

### 2. 启动服务端

```bash
./lodestar serve
```

首次启动会：

- 创建 `data/lodestar.db`（SQLite，WAL 模式）
- 生成 `data/config/system.yaml`（含随机 `download_token_secret`）
- 创建管理员账号，随机密码打印在终端，同时写入 `data/initial-admin.txt`
- 在 `http://127.0.0.1:8080` 起 WebUI

```
  ██╗      ██████╗ ██████╗ ███████╗███████╗████████╗ █████╗ ██████╗
  ...
  lodestar 0.1.0  ·  release control plane
  WebUI   http://127.0.0.1:8080
  Data    data/lodestar.db  ·  Storage storage

  Initial administrator created
    username: admin
    password: 7f3c9a...
```

登录后第一件事：去设置页改密码。

### 3. 建项目、登记主机

1. **项目** → 新建，填 `app_id`（如 `demo-app`）和名称。系统自动生成 `client_secret`、`deploy_token` 和 Ed25519 密钥对，点进项目详情可以复制。
2. **目标主机** → 登记这台机器（名称、平台、**部署根目录**如 `/srv`、备份目录、默认停/启/重启命令）。**不需要选项目**——一台机器可以服务多个应用。保存后会生成一段可直接粘贴的 Agent 配置。

### 4. 在生产机上跑 Agent

把 `lodestar` 可执行文件拷到生产机（`/opt/lodestar/lodestar`），写配置：

```yaml
# /opt/lodestar/data/config/system.yaml
agent:
  server_url: https://lodestar.example.com
  target_token: "<在 WebUI 里复制>"
  name: "prod-01"
  # 这台机器允许安装应用的根；每个应用装到 <root>/<app_id>，
  # 具体路径在 WebUI 部署时指定，逐个项目可以不同。
  deploy_root: /srv
  backup_dir: /srv/lodestar-backups
  work_dir: /var/lib/lodestar
  # 停启命令、健康检查、白名单在 WebUI「部署配置」里按项目设置
```

```bash
./lodestar agent
```

WebUI 里这台主机的状态会变成「在线」（15s 一次心跳）。

### 5. 发布

推一个包上去：

```bash
curl -X POST https://lodestar.example.com/api/v1/deploy \
  -H "X-Deploy-Token: $DEPLOY_TOKEN" \
  -F "version=1.4.2" \
  -F "platform=linux" -F "arch=amd64" \
  -F "changelog=修复登录超时" \
  -F "wait_seconds=120" \
  -F "package=@dist/demo-app-1.4.2.tar.gz"
```

`wait_seconds` 让请求等到部署结束再返回（最多 600s），方便 CI 里直接判定红绿。不想等就去掉，返回 `queued`，之后用 `GET /api/v1/deploy/jobs/{id}` 查。

## 配置参考

配置文件默认 `data/config/system.yaml`，可用 `-config` 指定。所有键都能用环境变量覆盖（`LODESTAR_` 前缀），环境变量优先级更高。

### 服务端

| YAML | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `addr` | `LODESTAR_ADDR` | `:8080` | 监听地址，也可用 `-addr` 覆盖 |
| `db_path` | `LODESTAR_DB_PATH` | `data/lodestar.db` | SQLite 路径 |
| `storage_dir` | `LODESTAR_STORAGE_DIR` | `storage` | 包体存放目录 |
| `data_dir` | `LODESTAR_DATA_DIR` | `data` | 数据根目录 |
| `public_base_url` | `LODESTAR_PUBLIC_BASE_URL` | 空 | 对外地址，空则按请求推断。**生产环境建议显式设置**，否则反代后生成的下载地址可能不对 |
| `download_token_secret` | `LODESTAR_DOWNLOAD_TOKEN_SECRET` | 首次运行随机生成 | 下载链接签名密钥 |
| `client_clock_skew_seconds` | `LODESTAR_CLIENT_CLOCK_SKEW_SECONDS` | `300` | 客户端签名时间窗 |
| `download_token_ttl_seconds` | `LODESTAR_DOWNLOAD_TOKEN_TTL_SECONDS` | `900` | 下载链接有效期 |
| `session_ttl_hours` | `LODESTAR_SESSION_TTL_HOURS` | `168` | 登录会话有效期（7 天） |
| `max_upload_mb` | `LODESTAR_MAX_UPLOAD_MB` | `2048` | 单包上限 |
| `admin_username` | `LODESTAR_ADMIN_USERNAME` | `admin` | 初始管理员用户名 |
| `admin_password` | `LODESTAR_ADMIN_PASSWORD` | 随机 | 初始管理员密码，只在账号不存在时生效 |
| `sql_debug` | `LODESTAR_SQL_DEBUG` | `false` | 打印每条 SQL。默认关闭，只记录真正的数据库错误（`record not found` 这类正常未命中不会刷屏） |

### Agent（生产机侧）

| YAML | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `agent.server_url` | `LODESTAR_AGENT_SERVER_URL` | `http://127.0.0.1:8080` | 控制面地址 |
| `agent.target_token` | `LODESTAR_AGENT_TARGET_TOKEN` | — | 主机令牌，WebUI 生成 |
| `agent.name` | `LODESTAR_AGENT_NAME` | `production` | 主机名（日志和心跳里用） |
| `agent.deploy_root` | `LODESTAR_AGENT_DEPLOY_ROOT` | — | **必填（或填 `deploy_dir`）**。这台机器允许安装应用的根目录，应用默认装到 `<root>/<app_id>` |
| `agent.deploy_dir` | `LODESTAR_AGENT_DEPLOY_DIR` | 空 | 兼容单应用老配置：任务没带路径时用它，同时兼作部署根 |
| `agent.backup_dir` | `LODESTAR_AGENT_BACKUP_DIR` | 空 | 默认备份目录，留空则用 `<deploy_dir>.backups` |
| `agent.work_dir` | `LODESTAR_AGENT_WORK_DIR` | `data/agent` | 下载与解压临时目录 |
| `agent.poll_interval_seconds` | `LODESTAR_AGENT_POLL_INTERVAL_SECONDS` | `5` | 长轮询间隔 |
| `agent.allow_server_commands` | `LODESTAR_AGENT_ALLOW_SERVER_COMMANDS` | `true` | 是否执行控制面下发的命令。设 `false` 则停启命令、健康检查一概不执行（只替换文件），**多租户或半可信环境建议设 false** |
| `agent.insecure_skip_verify` | `LODESTAR_AGENT_INSECURE_SKIP_VERIFY` | `false` | 跳过 TLS 校验（自签证书调试用） |

> 停/启/重启命令、健康检查、白名单、备份保留数**只存在于「部署配置」里**（按主机 × 应用），Agent 本地不再有这些配置项，也就不会有"我明明没填却执行了某个命令"的情况。字段留空 = 什么都不执行。

## WebUI

内嵌在二进制里，访问 `/` 即得。左侧导航按"工作台 / 发布 / 生产 / 系统"分组：

| 页面 | 路径 | 作用 |
| --- | --- | --- |
| **总览** | `/app/overview` | 项目数、主机在线率、今日部署、7 天成功率、14 天部署趋势、最近部署 |
| **项目** | `/app/projects` | 增删改项目，复制 `client_secret` / `deploy_token` / Ed25519 密钥，一键轮换 |
| **版本包** | `/app/releases` | 上传包、改版本信息、调灰度、启停用、手动触发部署 |
| **更新策略** | `/app/policies` | 最低版本、维护模式、版本封禁 |
| **目标主机** | `/app/targets` | 登记主机、生成 Agent 配置、轮换令牌；一行显示这台机器上的所有应用及各自版本；「部署」时从该主机的部署配置里选一个 |
| **部署配置** | `/app/configs` | 按「主机 × 应用」配置部署路径、备份目录、停/启/重启命令、健康检查、白名单（文件或目录）与备份保留数 |
| **部署记录** | `/app/deployments` | 任务列表、实时日志抽屉、重试、回滚、取消 |
| **设置** | `/app/settings` | 改密码、对外地址、服务信息、审计日志、Agent 部署指引 |

支持浅色/深色主题（右上角切换，记住偏好），窄屏自动折叠侧栏。

## 部署语义

一次 `deploy` 任务在 Agent 上的执行顺序（`internal/deploy/pipeline.go`）：

1. **下载** —— 带 `target_token` 拉包到 `work_dir`，边下边算 SHA-256。
2. **校验** —— 与 Release 记录的 `sha256`、`size` 比对，不一致直接失败，线上文件一个字节都不动。
3. **解压** —— 支持 `.zip`、`.tar`、`.tar.gz`、以及"裸二进制"。单层顶层目录会被自动展开（`dist/app-1.4.2/bin/app` → `bin/app`）；`__MACOSX`、`.DS_Store` 被丢弃；路径穿越（zip-slip）被拒绝；解压总体积有上限。
4. **停服** —— 执行 `stop_command`（如果配置了，且目标目录已存在；全新安装时跳过，避免 `chdir` 到不存在的目录）。
5. **备份** —— 把当前 `deploy_dir` 完整复制到 `backup_dir/app-<version>-<时间戳>`，超过 `backup_keep` 的旧备份自动清理。**全新安装（目录为空）不产生备份。**
6. **同步** —— 把解压结果镜像到 `deploy_dir`：
   - 包里有、主机没有 → 新建
   - 包里改了、主机也有 → 覆盖
   - **主机有、包里没有 → 删除**（保证版本干净，不留上一版的残留文件）
   - `preserve` 列表里的路径：主机上已存在则**跳过**，不存在则用包里的默认值（首次安装能带上默认配置，升级不会覆盖运行时数据）
7. **启服** —— 执行 `restart_command`（首次安装且没有备份时用 `start_command`）。
8. **健康检查** —— 配了 `health_check_url` 就轮询，非 5xx 算健康；超时算失败。
9. **失败回滚** —— 上面任一步失败，自动把备份恢复回去并重新启服；恢复不回来的破损目录被移到 `<deploy_dir>.failed-<时间戳>` 留证。

`rollback` 任务不下载包，直接取该应用在该主机上最新的备份恢复，然后重启。所以"回滚"永远回到**上一次成功的版本**，不是任意历史版本。

### 部署入口

WebUI 里发起部署只有一条路径：**先有部署配置，再选配置**。

- 版本包页点「部署」→ 列出该项目在所有主机上的部署配置（主机 + 路径 + 当前版本），勾选后执行
- 目标主机页点「部署」→ 先选这台主机上的某个应用配置，再选版本

路径、重启命令、白名单都来自配置本身，部署弹窗里不再出现路径输入框；要改就回「部署配置」页面改。CI 推送仍可带 `deploy_dir` 覆盖单次部署的路径。

### 部署目录怎么定

一台 Agent 只配置一个 `deploy_root`（这台机器允许安装应用的根），具体路径在**每次部署时**决定，所以同一台机器上的不同项目可以各装各的：

1. 部署请求里带了 `deploy_dir` → 用它
2. 否则用这个「主机 × 项目」上一次记录的 `deploy_dir`
3. 都没有 → `<deploy_root>/<app_id>`

`deploy_root` 只提供默认值，**不限制**最终路径：填 `/opt/legacy`、`D://apps//demo` 都能装。唯一拒绝的是文件系统根目录本身（`/`、`C://`）——把版本镜像到根目录会把整块盘的其他东西删掉。

备份目录同理：请求里的 `backup_dir` → 主机默认 `backup_dir` → `<deploy_dir>.backups`。

> ⚠️ 第 6 步的"删除多余文件"意味着**不要把运行时产生的文件放在 `deploy_dir` 里**，除非它被列进白名单。日志、上传、缓存、证书这类目录请务必写进白名单。

### 白名单（文件或目录，备份与部署都不动）

「部署配置」里的白名单（`preserve`）是一组**相对部署目录**的路径，**文件或目录都行**：

```yaml
preserve: ["appsettings.json", "data", "logs/app.log", "certs"]
```

对三件事同时生效：

| 环节 | 行为 |
| --- | --- |
| 部署同步 | 主机上已存在则跳过，不覆盖、不删除；主机上没有则用包里的默认值（首次安装能带上默认配置） |
| 备份 | **不复制**。运行时数据不跟着版本走，备份体积也小 |
| 回滚 / 失败恢复 | 恢复备份后再把**当前**的白名单内容搬回来，所以回滚不会把数据库或配置退回旧状态 |

也就是说：白名单里的东西，更新不覆盖、备份不打包、回滚不覆盖。配置文件（`appsettings.json`）、数据库、上传目录、证书、日志都该放这里。匹配按 `/` 归一化，`logs/app.log` 这种嵌套写法在 Windows 上同样有效。

## API 参考

### 认证方式

| 场景 | 方式 |
| --- | --- |
| WebUI / 管理 API | `Authorization: Bearer <session_token>`（`POST /api/v1/auth/login` 获取） |
| CI 推送部署 | `X-Deploy-Token: <project.deploy_token>` |
| 生产机 Agent | `X-Target-Token: <target.target_token>` |
| App 客户端检查更新 | `X-App-Id` + `X-Timestamp` + `X-Nonce` + `X-Signature`（HMAC-SHA256） |

### 管理 API（需登录）

```
POST   /api/v1/auth/login              {username, password} → {token, user}
POST   /api/v1/auth/logout
GET    /api/v1/auth/me
POST   /api/v1/auth/password           {current_password, new_password}

GET    /api/v1/overview

GET    /api/v1/projects
POST   /api/v1/projects                {app_id, name, description}
GET    /api/v1/projects/{id}
PATCH  /api/v1/projects/{id}
DELETE /api/v1/projects/{id}
POST   /api/v1/projects/{id}/rotate    {kind: client_secret|deploy_token|ed25519}

GET    /api/v1/releases?project_id=&platform=&channel=&q=&page=
POST   /api/v1/releases                multipart：package + 元信息
POST   /api/v1/releases/json           外部托管包（只登记 URL + sha256）
PATCH  /api/v1/releases/{id}
DELETE /api/v1/releases/{id}
POST   /api/v1/releases/{id}/deploy    {targets: [{target_id, deploy_dir}], message}

GET    /api/v1/targets
POST   /api/v1/targets
PATCH  /api/v1/targets/{id}
DELETE /api/v1/targets/{id}
POST   /api/v1/targets/{id}/rotate-token
POST   /api/v1/targets/{id}/deploy     {release_id, deploy_dir, backup_dir}

GET    /api/v1/deploy-configs?target_id=&project_id=
PATCH  /api/v1/targets/{id}/apps/{project_id}    部署配置（路径/命令/白名单）
DELETE /api/v1/targets/{id}/apps/{project_id}    只删记录，不动机器上的文件

GET    /api/v1/jobs?project_id=&target_id=&status=&page=
GET    /api/v1/jobs/{id}
GET    /api/v1/jobs/{id}/logs?after_seq=
POST   /api/v1/jobs/{id}/cancel
POST   /api/v1/jobs/{id}/retry
POST   /api/v1/jobs/{id}/rollback

GET    /api/v1/policies
PUT    /api/v1/policies
DELETE /api/v1/policies/{id}
GET    /api/v1/blocked-versions
POST   /api/v1/blocked-versions
PATCH  /api/v1/blocked-versions/{id}
DELETE /api/v1/blocked-versions/{id}

GET    /api/v1/settings
PUT    /api/v1/settings
GET    /api/v1/audit-logs?page=
```

写操作需要管理员角色，只读接口普通用户也能访问。

### CI 部署 API（`X-Deploy-Token`）

**`POST /api/v1/deploy`** —— `multipart/form-data`

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `package` | ✅ | 包文件（zip / tar / tar.gz / 二进制） |
| `version` | ✅ | 版本号，如 `1.4.2`，版本码按 `major*1000000 + minor*1000 + patch` 自动算出 |
| `platform` | | `linux` / `windows` / `darwin` / `android` / `all`，默认 `all` |
| `arch` | | `amd64` / `arm64` / `universal` / `all`，默认 `all` |
| `channel` | | 默认 `stable` |
| `title` | | 标题 |
| `changelog` | | 变更说明 |
| `min_supported_code` | | 最低兼容版本码 |
| `force` | | `true` 则客户端强制更新 |
| `rollout_percent` | | 灰度比例 0-100，默认 100 |
| `deploy` | | `false` 只登记不部署 |
| `target_ids` | | 逗号分隔，留空 = 所有匹配的主机 |
| `deploy_dir` | | 安装路径，作用于本次选中的主机。留空则用该「主机 × 项目」已记录的路径，再回退到 `<deploy_root>/<app_id>` |
| `wait_seconds` | | 等待部署完成秒数（≤600），0 = 立即返回 |

响应：

```json
{
  "release": { "id": 12, "version": "1.4.2", "version_code": 1004002 },
  "jobs": [{ "id": 34, "status": "running", "target_name": "prod-01" }],
  "status": "success",
  "message": ""
}
```

`status` 语义：

- `queued` —— 任务已入队，没等结果（`wait_seconds=0`）
- `success` / `partial` / `failed` —— 等到了结果：全部成功 / 部分成功 / 全部失败
- `busy` —— 匹配到的主机都已有排队或运行中的任务，本次跳过（防并发踩踏）
- `no_targets` —— 包登记成功，但没有主机匹配这个平台/通道
- `registered` —— 显式传了 `deploy=false`

**`POST /api/v1/deploy/json`** —— 包体在别处（S3、Release 附件），只登记元信息 + `external_url` / `sha256` / `size`。

**`GET /api/v1/deploy/jobs/{id}`** —— 查单个任务状态。

### Agent 协议（`X-Target-Token`）

```
POST /api/v1/agent/heartbeat                {name, platform, arch, current_version, ...}
GET  /api/v1/agent/jobs/next?wait=20s       长轮询，无任务返回 204
POST /api/v1/agent/jobs/{id}/logs           {logs: [{level, message, created_at}]} 批量上报
POST /api/v1/agent/jobs/{id}/finish         {status, version, backup_path, error, duration_ms}
GET  /api/v1/agent/packages/{id}/download   下载包体
```

任务认领用乐观锁（`UPDATE ... WHERE status='pending' AND claimed_by IS NULL`），多台 Agent 并发拉取不会重复执行。

## CI 集成

### GitHub Actions

```yaml
name: release

on:
  push:
    tags: ['v*']

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: Build
        run: |
          VERSION=${GITHUB_REF_NAME#v}
          mkdir -p dist
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -trimpath -ldflags "-s -w" -o dist/demo-app ./cmd/demo-app
          tar czf dist/demo-app-linux-amd64.tar.gz -C dist demo-app

      - name: Deploy to production
        env:
          LODESTAR_URL: ${{ secrets.LODESTAR_URL }}
          LODESTAR_TOKEN: ${{ secrets.LODESTAR_DEPLOY_TOKEN }}
        run: |
          VERSION=${GITHUB_REF_NAME#v}
          curl --fail-with-body -sS -X POST "$LODESTAR_URL/api/v1/deploy" \
            -H "X-Deploy-Token: $LODESTAR_TOKEN" \
            -F "version=$VERSION" \
            -F "platform=linux" \
            -F "arch=amd64" \
            -F "channel=stable" \
            -F "changelog=$(git log -1 --pretty=%s)" \
            -F "wait_seconds=180" \
            -F "package=@dist/demo-app-linux-amd64.tar.gz" \
            -o deploy.json

          cat deploy.json
          STATUS=$(jq -r .status deploy.json)
          case "$STATUS" in
            success|partial) echo "deploy ok: $STATUS" ;;
            *) echo "::error::deployment did not succeed ($STATUS)"; exit 1 ;;
          esac
```

`LODESTAR_DEPLOY_TOKEN` 从 WebUI 项目详情里复制，存到仓库 Secrets。

### 其他 CI

本质就是一次带 header 的 multipart POST，任何能跑 curl 的流水线都能用：

```yaml
# GitLab CI
deploy:
  script:
    - |
      curl -f -X POST "$LODESTAR_URL/api/v1/deploy" \
        -H "X-Deploy-Token: $LODESTAR_DEPLOY_TOKEN" \
        -F "version=$CI_COMMIT_TAG" \
        -F "platform=linux" -F "arch=amd64" \
        -F "package=@build/app.tar.gz"
```

### systemd 托管 Agent

```ini
# /etc/systemd/system/lodestar-agent.service
[Unit]
Description=Lodestar Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/lodestar
ExecStart=/opt/lodestar/lodestar agent
Restart=always
RestartSec=5
# 换版期间可能被自己的 restart_command 打断，别让 systemd 放弃重启
StartLimitIntervalSec=0

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload && systemctl enable --now lodestar-agent
journalctl -u lodestar-agent -f
```

> 自举场景（被部署的应用就是 Agent 本身）不要在 `restart_command` 里直接 restart 自己，用延迟重启包装，例如 `systemd-run --on-active=3 systemctl restart lodestar-agent`。

## 客户端自动更新

App 端检查更新（HMAC 签名）：

```bash
TIMESTAMP=$(date +%s)
NONCE=$(openssl rand -hex 16)
BODY='{"app_id":"demo-app","platform":"windows","arch":"amd64","channel":"stable","version":"1.4.1","version_code":1004001}'

# 待签字符串（换行分隔，最后一行是 body 的 SHA-256 十六进制小写）：
#   POST
#   /api/v1/check
#   <timestamp>
#   <nonce>
#   <sha256(body)>
# 签名 = base64(HMAC-SHA256(client_secret 的 UTF-8 字节, 待签字符串))
BODY_HASH=$(printf '%s' "$BODY" | openssl dgst -sha256 -hex | awk '{print $2}')
SIG=$(printf 'POST\n/api/v1/check\n%s\n%s\n%s' "$TIMESTAMP" "$NONCE" "$BODY_HASH" \
  | openssl dgst -sha256 -hmac "$CLIENT_SECRET" -binary | base64 -w0)

curl -X POST https://lodestar.example.com/api/v1/check \
  -H "Content-Type: application/json" \
  -H "X-App-Id: demo-app" \
  -H "X-Timestamp: $TIMESTAMP" \
  -H "X-Nonce: $NONCE" \
  -H "X-Signature: $SIG" \
  -d "$BODY"
```

响应：

```json
{
  "request_id": "6f0c...",
  "server_time": 1770000000,
  "legal": { "allowed": true, "code": "ok", "message": "ok" },
  "update": {
    "available": true,
    "update_type": "recommended",
    "version": "1.4.2",
    "version_code": 1004002,
    "platform": "windows",
    "arch": "amd64",
    "channel": "stable",
    "title": "1.4.2",
    "changelog": "修复登录超时",
    "min_supported_code": 0,
    "force": false,
    "package": {
      "id": 9,
      "file_name": "demo-app-1.4.2.zip",
      "download_url": "https://lodestar.example.com/api/v1/packages/9/download?token=...",
      "size": 18234112,
      "sha256": "9c1f..."
    },
    "manifest_signature": "<Ed25519 over canonical manifest>",
    "created_at": "2026-01-01T00:00:00Z"
  }
}
```

字段语义：

- `legal.code` —— `ok` / `maintenance`（维护中）/ `version_too_old`（低于最低支持版本）/ `version_blocked`（版本被封禁）。`allowed=false` 时客户端应提示 `message` 并禁止继续使用。
- `update.update_type` —— `recommended`（建议更新）/ `force`（强制）/ `none`（已是最新或没有可用版本）。
- `update.force` —— 综合了 Release 的 `force`、`legal` 和 `min_supported_code` 的结果，客户端直接看这个字段就行。
- `download_url` —— 带签名，`download_token_ttl_seconds` 内有效，下载时不需要再带凭据。
- `manifest_signature` —— 用项目 Ed25519 公钥验签，确认清单没被中间人改过。

可直接抄的客户端示例在 [`examples/client/`](examples/client)：

```bash
# Go：签名 + 请求 + 清单验签
go run ./examples/client/go -url https://lodestar.example.com -app demo-app \
  -secret "$CLIENT_SECRET" -pub "$ED25519_PUBLIC_KEY" \
  -platform windows -arch amd64 -version 1.4.1 -code 1004001

# Dart / Flutter：签名 + 请求
dart pub add crypto && dart run examples/client/dart/check_update.dart \
  --url=https://lodestar.example.com --app=demo-app --secret=$CLIENT_SECRET \
  --platform=android --arch=arm64 --version=1.4.1 --code=1004001
```

## Docker 部署

Lodestar 拆成**两个独立镜像**，职责完全不同：

| 镜像 | Dockerfile | 作用 | 端口 |
| --- | --- | --- | --- |
| `lodestar`（本体） | `Dockerfile` | 控制面 + 内嵌 WebUI，管版本包、任务、审计 | `8080` |
| `lodestar-agent` | `Dockerfile.agent` | 生产机侧执行器，下载包、解压、备份、换版、重启 | 无 |

Agent 镜像不装 Node、不构建前端（它不提供 UI），体积明显更小；构建时只补一个空的 `dist` 目录让 `//go:embed` 通过编译。

### 只跑本体

```bash
docker compose up -d
```

`docker-compose.yml` 已配好数据卷（`./data`、`./storage`）和健康检查。首次启动看日志拿初始密码：

```bash
docker compose logs lodestar | grep -A3 "Initial administrator"
```

### 本体 + Agent

Agent 放在 `agent` profile 下，默认不启动。先在 WebUI 的「目标主机 → 接入配置」里复制 `target_token`，然后：

```bash
export LODESTAR_AGENT_TARGET_TOKEN=<从 WebUI 复制>
export APP_ROOT=/srv/apps             # 应用根目录，每个项目装到它下面
docker compose --profile agent up -d
```

关键点：

- `APP_ROOT` 挂到容器的 `/srv`（即 `deploy_root`），每个项目默认装到 `/srv/<app_id>`；部署时可以在 WebUI 里改成别的子目录，但不能跑出 `/srv`
- 重启命令在 WebUI「部署配置」里设置（容器里没有 yaml 可改），例如 `docker compose restart demo-app`
- 容器内访问同网络的控制面用 `http://lodestar:8080`
- Agent 以 root 运行：它的职责就是替换挂载目录里的文件并执行重启脚本，降权后经常连自己的卷都写不进去。需要隔离时用 `user:` 指定

### 直接用镜像

```bash
docker pull ghcr.io/cubevlmu/gx_updater:v0.1.0
docker pull ghcr.io/cubevlmu/gx_updater-agent:v0.1.0

docker run -d --name lodestar -p 8080:8080 \
  -v lodestar-data:/app/data -v lodestar-storage:/app/storage \
  ghcr.io/cubevlmu/gx_updater:v0.1.0

docker run -d --name lodestar-agent \
  -e LODESTAR_AGENT_SERVER_URL=https://lodestar.example.com \
  -e LODESTAR_AGENT_TARGET_TOKEN=<token> \
  -e LODESTAR_AGENT_RESTART_COMMAND="supervisorctl restart demo" \
  -v /srv/apps:/srv -v lodestar-backups:/backups \
  ghcr.io/cubevlmu/gx_updater-agent:v0.1.0
```

反代到 Nginx / Caddy 时注意：

- `public_base_url` 设成对外地址，否则生成的下载链接可能是内网地址
- `max_upload_mb` 和反代的 `client_max_body_size` 保持一致
- Agent 长轮询一次占 20s，反代的 `read_timeout` 要大于 30s
- 上传大包时别开请求体缓冲，直接流式转发

## 开发与构建

### 环境

- Go 1.24+
- Node.js 20+（只在构建 WebUI 时需要）

### 命令

```bash
make dev      # 前端 dev server（:5173，/api 代理到 :8080）
make build    # 构建 WebUI + 编译二进制（产物 ./lodestar）
make run      # 构建并启动服务端
make test     # go vet + go test ./...
make clean    # 清掉二进制和前端产物
```

手动分步：

```bash
cd webui && npm install && npm run dev     # 前端热更新
cd webui && npm run build                  # 产物输出到 ../internal/web/dist
go build -o lodestar ./cmd/lodestar        # go:embed 把 dist 打进去
```

前端产物必须先生成，否则 `go build` 会因为缺 `dist` 目录失败——`internal/web/dist/.gitkeep` 保证空目录存在，此时访问 `/` 会得到 404 提示页而不是 panic。

### 其他命令

```bash
./lodestar genkeys        # 生成一组 client_secret / ed25519 密钥 / deploy_token
./lodestar hash <file>    # 算文件的 SHA-256 和大小
./lodestar version        # 版本信息
```

### CI / CD

本项目配置了 GitHub Actions，推送 `v*` 标签时自动触发：

#### 自动发布 Release

`.github/workflows/release.yml` 会并行构建三个平台的二进制文件：
- `lodestar-vX.Y.Z-linux-amd64`
- `lodestar-vX.Y.Z-linux-arm64`
- `lodestar-vX.Y.Z-windows-amd64.exe`

并自动创建 GitHub Release，附带构建好的二进制文件。

手动触发：
```bash
git tag v0.1.0
git push origin v0.1.0
```

或在 GitHub Actions 页面手动运行 `Release` workflow。

#### 自动构建 Docker 镜像

`.github/workflows/docker.yml` 用矩阵并行构建**两个镜像**（本体 + Agent），推送到 GHCR：
- 推送 `v*` 标签 → 镜像标签为版本号
- 推送 `main`/`master` 分支 → 镜像标签为 `latest`

| 镜像 | Dockerfile | 平台 |
| --- | --- | --- |
| `ghcr.io/cubevlmu/gx_updater` | `Dockerfile` | amd64 / arm64 |
| `ghcr.io/cubevlmu/gx_updater-agent` | `Dockerfile.agent` | amd64 / arm64 |

```bash
docker pull ghcr.io/cubevlmu/gx_updater:v0.1.0
docker pull ghcr.io/cubevlmu/gx_updater-agent:v0.1.0
```

#### 持续集成

`.github/workflows/ci.yml` 在 push 到 `main`/`master` 和 PR 时运行：

- `gofmt -l` 检查格式，`go vet`，`go test ./... -race`
- WebUI 的 `vue-tsc` 类型检查与 `npm run build`
- 两个 Dockerfile 各做一次不推送的构建（防止镜像坏掉才发现）

### 交叉编译

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o lodestar-linux-amd64 ./cmd/lodestar
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o lodestar-linux-arm64 ./cmd/lodestar
```

无 CGO 依赖，产物是静态链接的单文件，丢到任何 Linux 上都能跑。

## 测试

#### 单元测试

```bash
go test ./...
```

覆盖了几个容易出事的点：

- **解压** —— zip-slip 防护、`__MACOSX` 丢弃、单层目录展开、体积上限、裸二进制
- **同步** —— `preserve` 在"全新安装"和"升级"两种情况下截然不同的语义、删除陈旧文件
- **部署与回滚** —— 启动命令失败自动回滚、无备份时回滚报错、备份/恢复往返
- **命令执行** —— 输出流式回传、非零退出报错、工作目录、健康检查（非 5xx 视为存活、可被 ctx 取消）
- **签名** —— HMAC 各字段篡改、Ed25519 清单验签、下载令牌绑定与过期
- **多项目部署** —— 路径归一化（拒绝 `..`）、`deploy_dir` 的三级回退、根目录之外的绝对路径放行、拒绝把版本装到文件系统根、`app_id` 作为路径段的清洗
- **白名单** —— 文件与目录两种写法、更新不覆盖已有文件、首次安装用包内默认值、备份跳过、回滚保留当前内容、嵌套路径在 Windows 下的匹配
- **配置** —— 只靠 `LODESTAR_*` 环境变量也能配好 Agent（容器场景）、`sql_debug` 开关
- **日志** —— 默认不打印 SQL、`record not found` 不刷屏、开启调试后语句可见

#### 端到端测试（Python）

项目根目录下 `.tmp-test/e2e.py` 是一个完整流程的自动化验证脚本，不需要额外依赖（只用 Python 标准库）。它会：

1. 启动服务端（`./lodestar.exe serve -config .tmp-test/system.yaml`）
2. 登录 → 创建项目 → 登记目标主机 → 启动 Agent
3. 模拟 CI 推送两个版本（v1.0.0 全量、v1.1.0 增量）
4. 验证 Agent 下载、解压、备份、替换、重启、日志上报
5. 在同一台 Agent 上再装第二个项目到自定义目录，验证两者互不影响
6. 测试回滚（旧版恢复，运行数据保留）
7. 测试签名更新检查（HMAC-SHA256 + 重放攻击防护）
8. 测试 WebUI 嵌入、SPA 回退、未认证拒绝、审计日志

运行前确保端口 `18080` 空闲：

```bash
cd /e/Projects/frame               # 项目根目录
./lodestar.exe serve -config .tmp-test/system.yaml &
python .tmp-test/e2e.py
```

输出 `ALL CHECKS PASSED` 即全部 33 项断言通过。

#### 前端类型检查与构建

```bash
cd webui
npm run typecheck   # vue-tsc --noEmit
npm run build       # 产物输出到 ../internal/web/dist
```

`typecheck` 通过后才能提交；`build` 产物会被 `go:embed` 打包进二进制。

#### 手动验收清单（每次发版前跑一遍）

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 首次启动 `./lodestar serve` | 终端打印初始管理员密码，目录自动初始化 |
| 2 | 浏览器打开 `http://localhost:8080` | 看到登录页，Logo 正常，无控制台报错 |
| 3 | 登录后进入"总览" | 统计卡片、趋势图、近期任务列表正常显示 |
| 4 | "项目" → 新建项目 | 自动生成 `app_id`、`client_secret`、`deploy_token`、Ed25519 密钥对 |
| 5 | "目标主机" → 登记主机 | 表单校验通过，弹出接入配置（Agent YAML） |
| 6 | 复制 Agent 配置，启动 `./lodestar agent -config agent.yaml` | Agent 心跳上线，状态显示"在线" |
| 7 | "发布" → 上传 zip 包 | 包被保存，SHA-256 与大小正确显示 |
| 8 | 对该版本点击"部署到主机" | 任务进入队列，Agent 执行，日志实时回传 |
| 9 | 任务完成后查看"部署记录" | 状态"成功"，耗时、版本、主机正确，可点"查看日志" |
| 10 | 对同一主机再次部署更高版本 | 旧版被备份（带时间戳），`data/` / `config/` 被保留 |
| 11 | 在"目标主机"点击"回滚" | 旧版恢复，当前版本回退，运行数据仍在 |
| 12 | 用错误的密码连续登录 5 次 | 第 6 次被拒绝（rate limit），响应头带 `Retry-After` |
| 13 | 未带 Token 访问 `/api/v1/deploy` | 返回 `401 Unauthorized` |
| 14 | 手机浏览器（或 DevTools 375px）打开 | 登录页无左侧描述、导航显示 Logo、表格无水平滚动、操作列显示 "…" 菜单 |

#### 移动端截图自动化

需要 Node.js + `playwright-core`：

```bash
cd .tmp-test
npm install playwright-core
# Chromium 路径在脚本里硬编码，按实际环境修改 shots.mjs / ui-check.mjs
node shots.mjs      # 桌面端各页面截图
node ui-check.mjs   # 桌面 + 移动端（390×844）截图
```

截图存放在 `shots/` 目录，用于视觉回归对比。

## 目录结构

```
.
├── cmd/lodestar/           # 入口：serve / agent / genkeys / hash / version
├── internal/
│   ├── agent/              # 生产机侧：心跳、长轮询、下载校验、执行、日志上报
│   ├── api/                # HTTP 层：路由 + 各组 handler + 中间件
│   ├── config/             # 配置加载（YAML + 环境变量）
│   ├── db/                 # SQLite 连接与 AutoMigrate
│   ├── deploy/             # 部署引擎：解压、执行命令、备份/同步/回滚、健康检查
│   ├── models/             # GORM 模型
│   ├── security/           # HMAC、Ed25519、令牌、bcrypt
│   ├── versioning/         # 版本号解析与比较
│   ├── web/                # //go:embed dist + SPA fallback
│   │   └── dist/           # 前端构建产物（npm run build 生成）
│   └── buildinfo/          # 版本信息
├── webui/                  # Vue 3 + PrimeVue 5 源码
│   └── src/
│       ├── views/          # 七个页面
│       ├── components/     # 布局、导航、状态标签、日志查看器
│       ├── services/api.ts # 后端接口封装
│       └── router.ts
├── examples/               # 配置示例
│   └── client/             # 客户端检查更新的 Go / Dart 示例
├── scripts/                # build.sh / build.ps1
├── .github/workflows/      # ci.yml / release.yml / docker.yml
├── Dockerfile              # 本体镜像（内嵌 WebUI）
├── Dockerfile.agent        # Agent 镜像（无 WebUI）
├── docker-compose.yml
└── Makefile
```

## 安全说明

- **密码**：bcrypt 存储；登录接口按来源 IP 限速，连续失败短暂封禁。
- **会话**：随机 32 字节令牌，数据库只存 SHA-256 哈希，库泄露也拿不到可用令牌。
- **客户端签名**：HMAC-SHA256 覆盖 method / path / timestamp / nonce / body，nonce 单次有效，时间窗默认 300s。
- **清单签名**：每个项目一对 Ed25519 密钥，`/api/v1/check` 返回的清单带签名，客户端用公钥验签。
- **下载链接**：HMAC 签名 + 过期时间，不暴露包路径。
- **部署包**：SHA-256 强校验、解压路径穿越防护、解压体积上限。
- **命令执行**：Agent 默认接受服务端下发的命令。**如果 Lodestar 是多团队共用的，请把 `allow_server_commands` 设为 `false`**，这样只有主机本地配置里的命令会被执行，控制面被攻破也无法在主机上执行任意命令。
- **审计**：登录、凭据轮换、项目/主机增删改、部署、回滚、设置变更全部落审计日志。

### 生产检查清单

- [ ] 前面挂 TLS 反代，`public_base_url` 设成 `https://...`
- [ ] 改掉初始管理员密码
- [ ] `data/`、`storage/` 用独立磁盘并定期备份（`lodestar.db` + 包体）
- [ ] Agent 的 `allow_server_commands: false`（除非你完全信任控制面）
- [ ] `preserve` 里包含所有运行时目录（日志、上传、缓存、证书）
- [ ] `restart_command` 用 `systemctl restart` 而不是 `start`，保证幂等
- [ ] 配 `health_check_url`，否则失败回滚只能靠命令退出码判断

## License

MIT，见 [LICENSE](LICENSE)。WebUI 使用 PrimeVue，遵循其社区版许可（license key 见 `webui/.env.local`）。

<p align="right">(<a href="#readme-top">回到顶部</a>)</p>

<!-- MARKDOWN LINKS -->
[go-shield]: https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white
[go-url]: https://go.dev
[gin-shield]: https://img.shields.io/badge/Gin-1.11-008ECF?style=for-the-badge&logo=gin&logoColor=white
[gin-url]: https://gin-gonic.com
[gorm-shield]: https://img.shields.io/badge/GORM-1.30-2D7FF9?style=for-the-badge
[gorm-url]: https://gorm.io
[sqlite-shield]: https://img.shields.io/badge/SQLite-pure--Go-003B57?style=for-the-badge&logo=sqlite&logoColor=white
[sqlite-url]: https://github.com/glebarez/sqlite
[vue-shield]: https://img.shields.io/badge/Vue-3-4FC08D?style=for-the-badge&logo=vuedotjs&logoColor=white
[vue-url]: https://vuejs.org
[primevue-shield]: https://img.shields.io/badge/PrimeVue-5-41B883?style=for-the-badge
[primevue-url]: https://primevue.org
