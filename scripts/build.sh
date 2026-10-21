#!/usr/bin/env bash
# Build Lodestar: WebUI first, then the Go binary with the UI embedded.
#
#   ./scripts/build.sh              # local build
#   VERSION=1.4.2 ./scripts/build.sh
#   GOOS=linux GOARCH=amd64 ./scripts/build.sh   # cross compile
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo dev)}"
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

OUT="${OUT:-lodestar}"
if [ "$GOOS" = "windows" ]; then
  case "$OUT" in
    *.exe) ;;
    *) OUT="$OUT.exe" ;;
  esac
fi

echo "==> WebUI"
if [ ! -d webui/node_modules ]; then
  (cd webui && npm install --no-audit --no-fund)
fi
(cd webui && npm run build)

echo "==> Go ($GOOS/$GOARCH, version $VERSION, commit $COMMIT)"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
  -trimpath \
  -ldflags "-s -w -X lodestar/internal/buildinfo.Version=$VERSION -X lodestar/internal/buildinfo.Commit=$COMMIT" \
  -o "$OUT" ./cmd/lodestar

echo "==> done: $OUT"
"$OUT" version 2>/dev/null || true
