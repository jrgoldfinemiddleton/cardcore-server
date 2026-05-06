.PHONY: test fmt vet lint lint-extra build doc check help

test: ## Run all tests
	go test ./...

fmt: ## Format code with gofmt
	gofmt -w .

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	go tool golangci-lint run

lint-extra: ## Run golangci-lint with the extra-strict config
	go tool golangci-lint run --config .golangci-extra.yml

build: ## Compile all packages and binaries
	go build ./...

doc: ## Browse docs locally via pkgsite
	go tool pkgsite -open .

check: fmt vet lint test ## Run fmt, vet, lint, and test

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
