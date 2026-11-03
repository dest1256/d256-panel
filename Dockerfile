# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
COPY internal/web/translation /src/internal/web/translation
RUN npm run build

FROM golang:1.27-alpine AS builder
WORKDIR /app
RUN apk add --no-cache build-base
COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
ENV CGO_ENABLED=1 CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
RUN go build -trimpath -ldflags "-w -s" -o /out/d256-panel .

# The panel generates REALITY/ECH/ML-KEM/ML-DSA/VLESS-encryption keys by running the Xray CLI. It is built from
# the same xray-core version as the module and is used only as a key generator, never as a proxy.
FROM golang:1.27-alpine AS xraycli
WORKDIR /src
COPY go.mod go.sum ./
COPY proto/go.mod proto/go.sum ./proto/
RUN mkdir -p /out && CGO_ENABLED=0 go build -mod=mod -trimpath -ldflags "-w -s" -o /out/xray-$(go env GOOS)-$(go env GOARCH) github.com/xtls/xray-core/main

FROM alpine:3.22 AS geo
RUN apk add --no-cache curl ca-certificates  && mkdir -p /geo && cd /geo  && curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat  && curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S d256 && adduser -S -G d256 -h /app d256 \
 && mkdir -p /data && chown d256:d256 /data
WORKDIR /app
COPY --from=builder /out/d256-panel /app/d256-panel
COPY --from=builder /app/internal/web/translation /app/internal/web/translation
# The panel validates Xray templates and browses geo lists, so it needs the same geo files as the nodes.
COPY --from=geo /geo /app/bin
COPY --from=xraycli /out/ /app/bin/
ENV XUI_DB_FOLDER=/data XUI_LOG_FOLDER=/data/log XUI_BIN_FOLDER=/app/bin
USER d256
EXPOSE 2053 8444
VOLUME ["/data"]
CMD ["/app/d256-panel"]
