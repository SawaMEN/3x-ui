# Canonical task runner. Mirrors .github/workflows/ci.yml so `make verify`
# reproduces the PR gate locally. Run `make help` for the list.

SHELL := bash
FRONTEND = frontend

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

# go:embed of internal/web/dist needs the dir to exist even when the
# frontend bundle has not been built. CI stubs it the same way.
.PHONY: dist-stub
dist-stub:
	@mkdir -p internal/web/dist && touch internal/web/dist/.gitkeep

.PHONY: gen
gen: ## Regenerate Zod schemas + OpenAPI from Go sources
	cd $(FRONTEND) && npm run gen

.PHONY: gen-check
gen-check: gen ## Fail if generated files are stale
	git diff --exit-code -- frontend/src/generated frontend/public/openapi.json

.PHONY: lint-go
lint-go: dist-stub ## golangci-lint on Go sources
	golangci-lint run

.PHONY: lint-fe
lint-fe: ## oxlint on frontend sources
	cd $(FRONTEND) && npm run lint && npm run lint:deprecated

.PHONY: lint
lint: lint-go lint-fe ## All linters

.PHONY: format-check
format-check: ## oxfmt in check mode on frontend sources
	cd $(FRONTEND) && npm run format:check

.PHONY: typecheck
typecheck: ## tsc --noEmit
	cd $(FRONTEND) && npm run typecheck

.PHONY: msw-worker-check
msw-worker-check: ## Verify the tracked worker matches the installed MSW runtime
	cmp $(FRONTEND)/public/mockServiceWorker.js $(FRONTEND)/node_modules/msw/lib/mockServiceWorker.js

.PHONY: test-go
test-go: dist-stub ## Go tests (shuffle, no cache)
	go test -p 12 -vet=off -shuffle=on -count=1 ./...

.PHONY: race
# internal/web/service runs ~10x slower under -race and overruns go test's 10m default.
race: dist-stub ## Go tests with the race detector (needs a C compiler)
	go test -race -p 12 -short -vet=off -timeout 20m ./internal/... .

.PHONY: test-fe
test-fe: ## Frontend tests (vitest)
	cd $(FRONTEND) && npm test

.PHONY: test
test: test-go test-fe ## All tests

.PHONY: vulncheck
vulncheck: dist-stub ## govulncheck
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: build-fe
build-fe: ## Build the Vite bundles into internal/web/dist
	cd $(FRONTEND) && npm run build

.PHONY: build
build: build-fe ## Build the frontend then the Go binary
	go build -p 12 ./...

.PHONY: build-storybook
build-storybook: ## Build the static Storybook (compile-checks all stories)
	cd $(FRONTEND) && npm run build-storybook

# Fail fast if code generation is stale; then run independent checks in parallel.
# Keep the bundle build after tests so go:embed never sees partially written assets.
# Override VERIFY_JOBS for larger local machines (e.g. make VERIFY_JOBS=4 verify).
VERIFY_JOBS ?= 2
.PHONY: verify
verify: ## Full local gate with bounded parallelism
	@$(MAKE) gen-check
	@$(MAKE) -j$(VERIFY_JOBS) lint format-check typecheck msw-worker-check test
	@$(MAKE) -j$(VERIFY_JOBS) build build-storybook
	@echo "verify: OK"
