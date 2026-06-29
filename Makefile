.PHONY: test fmt vet lint lint-extra race build doc check help create-labels apply-labels clean

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

race: ## Run all tests with the race detector
	go test -race ./...

build: ## Compile all packages and binaries to bin/
	@mkdir -p bin
	go build -o bin/ ./cmd/...

doc: ## Browse docs locally via pkgsite
	go tool pkgsite -open .

check: fmt vet lint test ## Run fmt, vet, lint, and test

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

create-labels: ## Provision the repository label set
	./scripts/sync-labels.sh

apply-labels: ## Compute and apply labels for PR=<n>
	@if [ -z "$(PR)" ]; then echo "usage: make apply-labels PR=<pr-number>" >&2; exit 1; fi
	./scripts/apply-labels.sh $(PR)

clean: ## Remove build output directory
	rm -rf bin/
