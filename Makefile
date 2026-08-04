# Ticketmaster API — common tasks.
#
# On Windows, run these from Git Bash: several recipes use POSIX tools (rm,
# printf) that cmd.exe does not provide.
#
# Configuration comes from .env at run time, so no target needs credentials.

BINARY  := app
ifeq ($(OS),Windows_NT)
BINARY  := app.exe
endif

PKG     := ./cmd/server
IMAGE   := ticketmaster-api
PORT    ?= 8080
BASE_URL ?= http://localhost:$(PORT)

.DEFAULT_GOAL := help
.PHONY: help run build test cover fmt vet check tidy docker docker-run clean test-api

help: ## Show this help
	@printf 'Targets:\n'
	@printf '  make run          Start the API on PORT=$(PORT)\n'
	@printf '  make build        Compile ./cmd/server to ./$(BINARY)\n'
	@printf '  make test         Run the test suite\n'
	@printf '  make cover        Run tests and open a coverage report\n'
	@printf '  make fmt          Format all Go sources in place\n'
	@printf '  make vet          Run go vet\n'
	@printf '  make check        fmt-check + vet + test (what CI should run)\n'
	@printf '  make tidy         Prune and sync go.mod/go.sum\n'
	@printf '  make docker       Build the container image\n'
	@printf '  make docker-run   Run the image with .env on PORT=$(PORT)\n'
	@printf '  make clean        Remove build artefacts\n'

run: ## Start the dev server
	go run $(PKG)

build: ## Compile a static binary, same flags the Dockerfile uses
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) $(PKG)

test: ## Run tests
	go test ./...

cover: ## Test with coverage, then open the HTML report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

fmt: ## Rewrite sources with gofmt
	gofmt -w .

vet: ## Static checks
	go vet ./...

# fmt-check fails rather than rewriting, so CI reports unformatted code
# instead of silently fixing it and passing.
check: vet test ## Everything CI should run
	@test -z "$$(gofmt -l .)" || { printf 'unformatted files:\n'; gofmt -l .; exit 1; }

tidy: ## Sync go.mod / go.sum
	go mod tidy

docker: ## Build the image
	docker build -t $(IMAGE) .

docker-run: ## Run the image, reading config from .env on the host
	docker run --rm -p $(PORT):$(PORT) --env-file .env $(IMAGE)

clean: ## Remove build artefacts
	rm -f $(BINARY) coverage.out

test-api: ## End-to-end check of every route against a running API
	@BASE_URL=$(BASE_URL) bash scripts/test_api.sh
