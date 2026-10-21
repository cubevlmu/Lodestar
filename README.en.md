<div align="center">
  <img src="webui/public/logo.svg" alt="Lodestar Logo" width="96" height="96" />
  <h1>Lodestar</h1>
  <p><strong>Self-hosted application release management, update delivery, and remote deployment</strong></p>
  <p>Manage packages, production hosts, deployment jobs, and application updates from one control plane.</p>
  <p><a href="README.md">简体中文</a> · <strong>English</strong></p>
  <p>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/cubevlmu/lodestar/actions/workflows/ci.yml/badge.svg" /></a>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/release.yml"><img alt="Release" src="https://github.com/cubevlmu/lodestar/actions/workflows/release.yml/badge.svg" /></a>
    <a href="https://github.com/cubevlmu/lodestar/actions/workflows/docker.yml"><img alt="Docker" src="https://github.com/cubevlmu/lodestar/actions/workflows/docker.yml/badge.svg" /></a>
    <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-blue.svg" /></a>
  </p>
</div>

## Overview

Lodestar is a self-hosted release and deployment platform for individual developers and small teams operating multiple applications or servers. CI uploads build artifacts to the control plane. Agents running on production hosts **poll for jobs** and handle verification, backups, file synchronization, optional service restarts, and rollback on failure. The same server provides a Web dashboard and a signed application update API.

Lodestar **is not a build system or container orchestrator**. It deploys artifacts you have already built. Service management commands are configured by the operator and run with the agent's host privileges.

### Features

| Feature | Description |
| --- | --- |
| Release registry | Uploaded or externally hosted packages with versions, platforms, architectures, channels, changelogs, and staged rollout percentages |
| CI integration | HTTP deployment API for GitHub Actions, GitLab CI, or any pipeline capable of running curl |
| Pull-based agents | Agents poll outbound; no inbound SSH access required; multiple apps per production host |
| Deployment safeguards | SHA-256 verification, safe extraction, backups, mirrored synchronization, optional restarts and health checks, failure rollback |
| Persistent data | A `preserve` list protects configurations, databases, uploads, and other runtime paths |
| App update API | HMAC-SHA256 request authentication, update policies, maintenance mode, blocked versions, and Ed25519 signed manifests |
| Administration UI | Projects, releases, targets, per-app deployment configurations, jobs, logs, retries, rollback, and audit events |
| Lightweight operation | Go + Gin + GORM + SQLite, Vue 3 + PrimeVue, and an embeddable WebUI |

## Architecture

```text
GitHub Actions / CI ── upload release ──> Lodestar Server
       Web dashboard ── management ────> │  ├─ SQLite (metadata, jobs, audit)
          App clients ── update checks ─> │  └─ Storage (release packages)
                                         │
           Production Agent <── poll ────┘
                   │
                   └─ download → SHA-256 verify → extract
                      → optional stop → backup → mirror files
                      → optional start / restart → health check
                      → restore backup if deployment fails
```

**Core concepts:** Project (application), Release (versioned artifact), Target (host), Deploy Config (application-specific settings for a host), and Job (deployment or rollback). One agent can host multiple applications with separate paths, commands, and preservation rules.

## Quick start

### Option 1: Docker Compose

Requires Docker and Docker Compose.

```bash
git clone https://github.com/cubevlmu/lodestar.git
cd lodestar
docker compose up -d --build
docker compose logs lodestar
```

Open `http://localhost:8080`. The server initializes its database, configuration, and administrator on first launch. The generated password is printed in the logs and stored in `data/initial-admin.txt`. **Change it immediately after signing in.**

Compose stores persistent server data under `./data` and release artifacts under `./storage`. For production, put the service behind HTTPS and set `LODESTAR_PUBLIC_BASE_URL=https://your-domain.example`.

### Option 2: Build from source

Requires Go 1.24+, Node.js 20+, and npm. Build the WebUI before the Go executable:

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

On Linux/macOS, `make build` performs both build steps. On Windows, see `scripts/build.ps1`. The checked-in `internal/web/dist/.gitkeep` is not a compiled WebUI. Build the frontend first for a functional embedded dashboard.

### Connect a production agent

1. Sign in and create a **Project**. Copy its `deploy_token` for CI.
2. Register a **Target** in the dashboard. Copy its `target_token`.
3. Copy the Lodestar executable to the production host and create `data/config/system.yaml`:

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

4. In **Deploy Configs**, configure the application installation path, optional stop/start/restart commands, health check, preserved paths, and backup retention. Leaving commands blank performs a file-only deployment without restarting the service.

The agent makes outbound requests to the server, so no direct inbound connection to the production host is needed. Run it under an account with appropriate access to deployment and backup directories.

Alternatively, run the optional Compose agent profile after setting `LODESTAR_AGENT_TARGET_TOKEN`:

```bash
docker compose --profile agent up -d --build
```

**Container caveat:** A containerized agent can only modify files mounted into it. Managing other containers or host services requires explicit permissions and suitable commands.

### Publish from CI

Copy the project's `deploy_token` into your CI secret store, then upload an already-built package:

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

Accepted package formats include `.zip`, `.tar`, `.tar.gz`, and standalone binaries. Set `wait_seconds=0` to enqueue the deployment without waiting, or set it to a positive value (up to 600 seconds) to receive a job result. A registered release does **not** guarantee successful deployment, especially if no eligible targets or deployment configurations exist.

See the [GitHub Actions integration example](examples/github-actions.yml).

## Deployment, backups, and rollback

The agent downloads and verifies the package before extraction. It runs an optional stop command, creates a backup, then **mirrors** the extracted package into the install directory. Mirroring updates changed files **and removes files absent from the new release**. An optional start/restart command and health check follow; failures trigger an attempt to restore the previous backup.

Use the `preserve` list for paths relative to the install directory:

```yaml
preserve:
  - appsettings.json
  - data
  - uploads
  - logs
```

Existing preserved files and directories are not overwritten or removed during deploys, are excluded from version backups, and retain their current values after rollback. **Preserve runtime databases, user uploads, secrets, and settings, or store them outside the install directory.** Only packaged application files should be fully managed by mirroring.

The dashboard provides job logs, retries, and rollback. Rollback restores an available host backup; it is not an arbitrary historical-version selector. Configure a useful health check to detect failures after the process starts.

## Client update checks

App clients call `POST /api/v1/check` with `X-App-Id`, `X-Timestamp`, `X-Nonce`, and `X-Signature`. The request body is included in its HMAC-SHA256 signature. Responses can provide a matching release, update policy, expiring signed download URL, and Ed25519-signed update manifest. Policies can enforce minimum supported versions, maintenance windows, or version blocks.

### Official clients

Four implementations live under `clients/`, all speaking the same protocol:

| Language | Package | Registry |
| --- | --- | --- |
| Dart / Flutter | `lodestar_update_client` | pub.dev |
| Java | `com.cubevlmu:lodestar-update` | GitHub Packages (Maven) |
| C# | `Lodestar.Update` | NuGet |
| Go | — (source) | [examples/client/go](examples/client/go/main.go) |

Each one signs the request, verifies the Ed25519 manifest and checks the
package SHA-256 after download; the signing rules are documented in
[clients/README.md](clients/README.md). Clients should verify signatures rather
than trusting a download URL on its own.

```dart
final client = LodestarUpdateClient(config: LodestarUpdateConfig(
  baseUrl: 'https://lodestar.example.com',
  appId: 'demo.app',
  clientSecret: '<client_secret>',
  ed25519PublicKey: '<public key>',
));
final result = await client.check(version: '1.4.1', versionCode: 1004001);
```

```java
var response = client.check("1.4.1", 1_004_001, null);
if (response.hasUpdate()) {
    var pkg = client.downloadAndVerify(response.update(), Path.of("updates"));
}
```

```csharp
var response = await client.CheckAsync("1.4.1", 1_004_001);
if (response.HasUpdate)
{
    var package = await client.DownloadAndVerifyAsync(response.Update, "updates");
}
```

Pushing a `v*` tag runs `.github/workflows/publish-clients.yml`, which tests and
publishes all three packages; channels without credentials are skipped with a
warning.

## Configuration and API

Default configuration: `data/config/system.yaml`. Override via `-config` or `LODESTAR_CONFIG`.

| Environment variable | Purpose |
| --- | --- |
| `LODESTAR_ADDR` | Listen address, defaults to `:8080` |
| `LODESTAR_DB_PATH` | SQLite database path |
| `LODESTAR_STORAGE_DIR` | Release artifact storage |
| `LODESTAR_PUBLIC_BASE_URL` | Public-facing base URL, especially important behind reverse proxies |
| `LODESTAR_MAX_UPLOAD_MB` | Maximum upload size |
| `LODESTAR_ADMIN_USERNAME` | Initial admin username |
| `LODESTAR_ADMIN_PASSWORD` | Optional initial admin password |

For additional settings, see [server configuration](examples/system.example.yaml) and [agent configuration](examples/agent.example.yaml).

| API | Purpose | Authentication |
| --- | --- | --- |
| `POST /api/v1/auth/login` | Dashboard login | Username and password |
| `/api/v1/projects`, `/api/v1/releases` | Project and release management | Bearer session token |
| `/api/v1/targets`, `/api/v1/deploy-configs` | Production targets and deploy configs | Bearer session token |
| `/api/v1/jobs` | Deployment jobs and logs | Bearer session token |
| `POST /api/v1/deploy` | CI upload and deployment | `X-Deploy-Token` |
| `GET /api/v1/deploy/jobs/{id}` | CI deployment status | `X-Deploy-Token` |
| `POST /api/v1/check` | Signed client update check | HMAC request signature |
| `/api/v1/agent
