.DEFAULT_GOAL := all

.PHONY: .pre-commit
.pre-commit: ## Check that pre-commit is installed
	@pre-commit -V || echo 'Please install pre-commit: https://pre-commit.com/'

.PHONY: install
install: .pre-commit ## Install pre-commit hooks for local development
	pre-commit install --install-hooks

.PHONY: format
format: ## Format the Go code
	gofmt -w .

.PHONY: lint
lint: ## Check Go formatting and run go vet
	@test -z "$$(gofmt -l .)" || (echo 'gofmt needs to be run, see: make format' && gofmt -l . && exit 1)
	go vet ./...

.PHONY: build
build: ## Build the Go package
	go build ./...

.PHONY: test
test: ## Run the Go tests
	go test ./...

.PHONY: all
all: format lint build test ## Format, lint, build, and test

.PHONY: help
help: ## Show this help (usage: make help)
	@echo "Usage: make [recipe]"
	@echo "Recipes:"
	@awk '/^[a-zA-Z0-9_-]+:.*?##/ { \
		helpMessage = match($$0, /## (.*)/); \
		if (helpMessage) { \
			recipe = $$1; \
			sub(/:/, "", recipe); \
			printf "  \033[36m%-20s\033[0m %s\n", recipe, substr($$0, RSTART + 3, RLENGTH); \
		} \
	}' $(MAKEFILE_LIST)
