# Clients

Official update clients for Lodestar, in four flavours. They all speak the same
protocol, so a mixed fleet (Flutter mobile app, Java desktop tool, C# service,
Go binary) can be served by one Lodestar instance.

| Language | Package | Distribution |
| --- | --- | --- |
| Dart / Flutter | `lodestar_update_client` | Release asset `.tar.gz` |
| Java | `lodestar-update` | Release asset `.jar` (+ sources/javadoc) |
| C# | `Lodestar.Update` | Release asset `.nupkg` |
| Go | `examples/client/go` (in this repo) | source |

Every `v*` tag publishes these files as GitHub Release assets, built by the
`Release` workflow — there is no registry to sign up for.

| Asset | Install |
| --- | --- |
| `lodestar_update_client-<v>.tar.gz` | extract into `packages/`, then `lodestar_update_client: { path: packages/lodestar_update_client }` |
| `Lodestar.Update.<v>.nupkg` | `dotnet nuget add source ./nuget --name lodestar`, then `dotnet add package Lodestar.Update` |
| `lodestar-update-<v>.jar` | `mvn install:install-file -Dfile=... -DgroupId=com.cubevlmu -DartifactId=lodestar-update -Dversion=<v> -Dpackaging=jar`, or add it as a plain jar |

## The protocol

### 1. Check

`POST /api/v1/check`, JSON body:

```json
{
  "app_id": "demo.app",
  "platform": "android",
  "arch": "arm64",
  "channel": "stable",
  "version": "1.4.1",
  "version_code": 1004001,
  "client_id": "optional-stable-anonymous-id"
}
```

Headers `X-App-Id`, `X-Timestamp` (epoch seconds), `X-Nonce` (unique per request)
and `X-Signature`, where

```
payload   = METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + NONCE + "\n" + hex(sha256(body))
signature = base64(HMAC-SHA256(client_secret, payload))
```

The key is the **raw bytes** of `client_secret`. Lodestar never base64-decodes
it, so neither should a client — secrets that happen to look like base64 used to
break exactly here.

`PATH` is the request path as sent, including any reverse-proxy prefix
(`/api/v1/check`, or `/updates/api/v1/check` when mounted under a prefix).

Response:

```json
{
  "request_id": "…",
  "server_time": 1760000000,
  "legal":  { "allowed": true, "code": "", "message": "" },
  "update": {
    "available": true, "update_type": "recommended",
    "version": "1.5.0", "version_code": 1005000,
    "platform": "android", "arch": "arm64", "channel": "stable",
    "force": false, "min_supported_code": 1000000,
    "package": { "file_name": "demo.apk", "download_url": "…", "size": 123, "sha256": "…" },
    "manifest_signature": "…"
  }
}
```

`legal.allowed = false` means the client is blocked (maintenance or below the
minimum version); `update_type` is `none` / `recommended` / `force`.

### 2. Verify the manifest

The download URL and hash must be checked before use. The server signs:

```
lodestar-manifest-v1
app_id=<app id>
version=<version>
version_code=<code>
platform=<platform>
arch=<arch>
channel=<channel>
package_url=<download url>
package_size=<size>
package_sha256=<sha256>
force=<true|false>
min_supported_code=<code>
```

with its Ed25519 key (the trailing newline matters). Clients verify it with the
project's public key before downloading.

### 3. Download

`GET` the signed `download_url` (it carries a short-lived token, so do not cache
it), stream to disk, and compare SHA-256 with `package.sha256`. A mismatch must
delete the file.

## Tests

Each client has its own test suite that pins the signing payload, the HMAC of a
known vector and the canonical manifest string:

```bash
cd flutter/lodestar_update_client && dart test
cd java && javac --release 17 -d out $(find src -name '*.java') && java -cp out com.cubevlmu.lodestar.update.SelfTest
cd csharp && dotnet test Lodestar.Update.Tests
```

There is also an end-to-end check that starts a real Lodestar, registers a
release and drives the Java and C# clients through check → verify → download:

```bash
python .tmp-test/clients-check.py   # local only, not committed
```
