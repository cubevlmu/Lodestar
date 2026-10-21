## 0.2.0

- Renamed to `lodestar_update_client` (was `gx_update_client`).
- Signing updated for Lodestar: HMAC uses the raw `client_secret` bytes, manifest
  canonical string is `lodestar-manifest-v1`.
- Added pub.dev metadata (repository, issue tracker, topics).

## 0.1.0

- First release: signed update checks, manifest verification, package download
  and Android APK install handoff.
