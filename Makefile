# Development tasks for the mail bridge.
#
# Run `make` with no target for the list.

BINARY     := bridge
BUILD_DIR  := bin
CMD        := ./cmd/bridge

# Where the Mail API serves its OpenAPI document, used by `make types`.
# Override for another environment: make types MAIL_API_URL=https://...
MAIL_API_URL ?= http://localhost:3100
OPENAPI_URL  := $(MAIL_API_URL)/docs-json
OPENAPI_FILE := internal/api/openapi.json

# Pinned so regenerating types gives the same output on every machine.
OAPI_CODEGEN := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0

.DEFAULT_GOAL := help

## help: list the available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## run: start the bridge
run:
	go run $(CMD)

## build: compile the bridge into bin/
build:
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD)

## test: run every test with the race detector
test:
	go test ./... -race

## check: what CI should agree with before a PR
check: fmt vet test

## fmt: format the source
fmt:
	gofmt -w .

## vet: report suspicious constructs
vet:
	go vet ./...

## tidy: sync go.mod and go.sum with the imports
tidy:
	go mod tidy

## spec: download the OpenAPI document from the running API
spec:
	curl -sSf $(OPENAPI_URL) -o $(OPENAPI_FILE)
	@echo "saved $(OPENAPI_FILE) from $(OPENAPI_URL)"

## types: regenerate the API types from the saved OpenAPI document
##        Only types are generated: the HTTP client, its retries and its typed
##        errors are hand-written and must not be overwritten.
types:
	go run $(OAPI_CODEGEN) -generate types -package api -o internal/api/types.gen.go $(OPENAPI_FILE)

## clean: remove build output
clean:
	rm -rf $(BUILD_DIR)

.PHONY: help run build test check fmt vet tidy spec types clean