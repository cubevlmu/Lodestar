# lodestar_update_client

Flutter client for [Lodestar](https://github.com/cubevlmu/lodestar): signed
update checks, manifest verification, package download and Android APK install
handoff.

## What it does

- calls `POST /api/v1/check` with HMAC request signing
- verifies the server's Ed25519 manifest signature before trusting a download URL
- downloads the package and checks its SHA-256
- hands an APK to the system installer on Android

## Install

```yaml
dependencies:
  lodestar_update_client: ^0.2.0
```

Android needs `REQUEST_INSTALL_PACKAGES` (merged automatically) and, on Android
8+, the user has to allow "install unknown apps" for your app once:

```dart
await LodestarUpdateInstaller.canRequestPackageInstalls();
await LodestarUpdateInstaller.openInstallPermissionSettings();
```

## Use

```dart
final client = LodestarUpdateClient(
  config: LodestarUpdateConfig(
    baseUrl: 'https://lodestar.example.com',
    appId: 'demo.app',
    clientSecret: '<project client_secret>',
    ed25519PublicKey: '<project ed25519 public key>',
    arch: 'arm64',
  ),
);

final result = await client.check(version: '1.4.1', versionCode: 1004001);
if (result.hasUpdate) {
  // Verifies the manifest, downloads and checks SHA-256.
  final package = await client.downloadAndVerify(result.update);
  // Android: await LodestarUpdateInstaller.installApk(package.filePath);
}
```

The check request carries the current version, platform, arch and channel. The
server answers with `legal` (may the client keep running) and `update` (what to
install). `result.mustUpdate` is true when the client is blocked or the update is
forced.

## Migration from `gx_update_client`

The protocol changed with Lodestar:

| | gx_update_client | lodestar_update_client |
| --- | --- | --- |
| HMAC key | base64-decoded when possible | raw `client_secret` bytes |
| Manifest prefix | `gx-update-manifest-v1` | `lodestar-manifest-v1` |

Class names are `LodestarUpdateClient`, `LodestarUpdateConfig`,
`LodestarUpdateService`, `LodestarUpdateInstaller`; exceptions are
`LodestarUpdateException` / `LodestarHttpException`.

## License

MIT
