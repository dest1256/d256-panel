# d256-panel task runner. Every target runs inside the dev container (.devcontainer/Dockerfile);
# nothing is installed on the host. On a host with make: `make test`. Without make: `./dev make test`.

ifeq ($(D256_IN_DEV),1)
RUN :=
else
RUN := ./dev
endif

GO_PKGS := $$(go list ./... | grep -v /node_modules/)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

.PHONY: devimage
devimage: ## Build the dev container image
	docker build -t d256-panel-dev .devcontainer

# go:embed of internal/web/dist needs the directory even before the bundle is built.
.PHONY: dist-stub
dist-stub:
	@mkdir -p internal/web/dist && touch internal/web/dist/.gitkeep

.PHONY: proto
proto: ## Regenerate Go code from proto/*.proto
	$(RUN) sh -c 'cd proto && ./generate.sh'

.PHONY: gen
gen: ## Regenerate Zod schemas + OpenAPI from Go sources
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm run gen'

.PHONY: gen-check
gen-check: gen ## Fail if generated files are stale
	git diff --exit-code -- frontend/src/generated frontend/public/openapi.json

.PHONY: lint-go
lint-go: dist-stub ## golangci-lint + go vet + staticcheck
	$(RUN) sh -c 'golangci-lint run && go vet $(GO_PKGS) && staticcheck $(GO_PKGS)'

.PHONY: lint-fe
lint-fe: ## oxlint on frontend sources
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm run lint'

.PHONY: lint
lint: lint-go lint-fe ## All linters

.PHONY: format-check
format-check: ## oxfmt in check mode
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm run format:check'

.PHONY: typecheck
typecheck: ## tsc --noEmit
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm run typecheck'

.PHONY: vulncheck
vulncheck: dist-stub ## govulncheck
	$(RUN) govulncheck ./...

.PHONY: test-go
test-go: dist-stub ## Go tests (shuffle, no cache)
	$(RUN) sh -c 'go test -shuffle=on -count=1 $(GO_PKGS)'

.PHONY: race
race: dist-stub ## Go tests with the race detector
	$(RUN) sh -c 'go test -race -shuffle=on -count=1 -timeout 25m $(GO_PKGS)'

.PHONY: test-fe
test-fe: ## Frontend tests (vitest)
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm test'

.PHONY: test
test: test-go test-fe ## All tests

.PHONY: build-fe
build-fe: ## Build the Vite bundles into internal/web/dist
	$(RUN) sh -c 'cd frontend && npm ci --no-audit --no-fund && npm run build'

.PHONY: build
build: build-fe ## Frontend bundle, then the Go binary (./build/d256-panel)
	$(RUN) sh -c 'mkdir -p build && go build -ldflags "-w -s" -o build/d256-panel .'

.PHONY: image
image: ## Production runtime image (multi-stage Dockerfile)
	docker build -t d256-panel:local .

.PHONY: release
release: verify image ## Full gate, then the runtime image

.PHONY: verify
verify: lint format-check typecheck test build vulncheck ## Full gate (mirrors CI)
	@echo "verify: OK"
