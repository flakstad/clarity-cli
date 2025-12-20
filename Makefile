BINARY_NAME=clarity

.PHONY: build run install tidy fmt test

build:
	go build -o ./dist/$(BINARY_NAME) ./cmd/clarity

run:
	go run ./cmd/clarity

tidy:
	go mod tidy

fmt:
	gofmt -w .

test:
	go test ./...

install:
install: test
	go install ./cmd/clarity
	@echo "Installed: $$(go env GOPATH)/bin/$(BINARY_NAME)"
