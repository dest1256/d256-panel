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

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S d256 && adduser -S -G d256 -h /app d256 \
 && mkdir -p /data && chown d256:d256 /data
WORKDIR /app
COPY --from=builder /out/d256-panel /app/d256-panel
COPY --from=builder /app/internal/web/translation /app/internal/web/translation
ENV XUI_DB_FOLDER=/data XUI_LOG_FOLDER=/data/log
USER d256
EXPOSE 2053 8444
VOLUME ["/data"]
CMD ["/app/d256-panel"]
