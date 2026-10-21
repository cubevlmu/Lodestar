# syntax=docker/dockerfile:1

# ---------- WebUI ----------
FROM node:22-alpine AS webui
WORKDIR /src/webui
COPY webui/package.json webui/package-lock.json* ./
RUN npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund
COPY webui/ ./
# vite outDir is ../internal/web/dist
RUN npm run build

# ---------- binary ----------
FROM golang:1.24-alpine AS builder
ARG VERSION=dev
ARG COMMIT=docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=webui /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X lodestar/internal/buildinfo.Version=${VERSION} -X lodestar/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/lodestar ./cmd/lodestar

# ---------- runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 lodestar \
 && mkdir -p /app/data /app/storage \
 && chown -R lodestar:lodestar /app
WORKDIR /app
COPY --from=builder /out/lodestar /app/lodestar
USER lodestar

ENV LODESTAR_ADDR=:8080 \
    LODESTAR_DB_PATH=/app/data/lodestar.db \
    LODESTAR_STORAGE_DIR=/app/storage

EXPOSE 8080
VOLUME ["/app/data", "/app/storage"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/lodestar"]
CMD ["serve"]
