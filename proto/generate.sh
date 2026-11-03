#!/bin/sh
# Run via `make proto` (dev container provides protoc + plugins).
set -eu
cd "$(dirname "$0")"
protoc --proto_path=. \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  nodelink/v1/nodelink.proto
go mod tidy
