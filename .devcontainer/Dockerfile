# Dev/CI toolchain. Nothing is installed on the host: every make target runs in this image.
FROM golang:1.27

RUN apt-get update && apt-get install -y --no-install-recommends \
      make git ca-certificates protobuf-compiler unzip \
    && rm -rf /var/lib/apt/lists/*

# Node comes from the official image so the version matches .nvmrc (26).
COPY --from=node:26-bookworm /usr/local/bin/node /usr/local/bin/node
COPY --from=node:26-bookworm /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

ENV GOBIN=/usr/local/bin
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
 && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 \
 && go install honnef.co/go/tools/cmd/staticcheck@latest \
 && go install golang.org/x/vuln/cmd/govulncheck@latest \
 && go clean -cache -modcache

ENV D256_IN_DEV=1 \
    GOFLAGS=-buildvcs=false \
    GOCACHE=/cache/go-build \
    GOMODCACHE=/cache/go-mod \
    npm_config_cache=/cache/npm \
    CGO_ENABLED=1
WORKDIR /workspace
