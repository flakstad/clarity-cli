BINARY_NAME=clarity

.PHONY: build run install tidy fmt test it

ROOT_DIR := $(shell pwd)

# Go caches:
# - Default is to use a shared per-user cache dir so isolated agent dirs (worktrees/copies)
#   do not "redownload the world" on each build/test.
# - Override by setting GO_CACHE_DIR (or pre-setting GOMODCACHE/GOCACHE).
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	DEFAULT_GO_CACHE_DIR := $(HOME)/Library/Caches/clarity-cli-go
else
	DEFAULT_GO_CACHE_DIR := $(HOME)/.cache/clarity-cli-go
endif

GO_CACHE_DIR ?= $(DEFAULT_GO_CACHE_DIR)

export GOMODCACHE ?= $(GO_CACHE_DIR)/gomodcache
export GOCACHE ?= $(GO_CACHE_DIR)/gocache

prep-cache:
	@mkdir -p "$(GOMODCACHE)" "$(GOCACHE)"

build:
build: prep-cache
	go build -o ./dist/$(BINARY_NAME) ./cmd/clarity

run:
run: prep-cache
	go run ./cmd/clarity

tidy:
tidy: prep-cache
	go mod tidy

fmt:
	gofmt -w .

test:
test: prep-cache
	@# If an in-repo module cache exists under tmp/, `go test ./...` will walk it and fail.
	@# These caches are often read-only, so we rename them into an ignored dir (leading '_').
	@# If a previous ignored cache already exists, move the new one aside (to avoid leaving tmp/gomodcache behind).
	@if [ -d ./tmp/gomodcache ]; then \
		if [ -d ./tmp/_gomodcache ]; then \
			i=1; while [ -d "./tmp/_gomodcache$$i" ]; do i=$$((i+1)); done; \
			mv ./tmp/gomodcache "./tmp/_gomodcache$$i"; \
		else \
			mv ./tmp/gomodcache ./tmp/_gomodcache; \
		fi; \
	fi
	@if [ -d ./tmp/gocache ]; then \
		if [ -d ./tmp/_gocache ]; then \
			i=1; while [ -d "./tmp/_gocache$$i" ]; do i=$$((i+1)); done; \
			mv ./tmp/gocache "./tmp/_gocache$$i"; \
		else \
			mv ./tmp/gocache ./tmp/_gocache; \
		fi; \
	fi
	go test ./...

it:
it: prep-cache
	bash ./scripts/cli_integration.sh

install:
install: prep-cache test it
	go install ./cmd/clarity
	@echo "Installed: $$(go env GOPATH)/bin/$(BINARY_NAME)"
