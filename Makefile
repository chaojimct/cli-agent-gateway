BINARY=cursor-gateway.exe
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build run test clean web

web:
	@echo "Web UI is embedded from internal/webui/static/index.html"

build: web
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/server

run: build
	./bin/$(BINARY) -config config.yaml

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" ./cmd/server

deps:
	go mod tidy
	go mod download
