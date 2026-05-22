BINARY=cli-agent-gateway
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-s -w -X main.version=$(VERSION)

# Cross-compilation targets (linux/darwin: no .exe; windows: .exe)
PLATFORMS=linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64

.PHONY: build run test clean web build-all npm-pack

web:
	@echo "Web UI is embedded from internal/webui/static/index.html"

build: web
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY).exe ./cmd/server

run: build
	./bin/$(BINARY).exe -config config.yaml

build-all: web
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} CGO_ENABLED=0 \
		go build -trimpath -ldflags="$(LDFLAGS)" \
		  -o dist/$(BINARY)-$${p%/*}-$${p#*/}$$( [ "$${p%/*}" = "windows" ] && echo .exe ) ./cmd/server; \
	done
	@echo "Built under dist/"

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install:
	go install -ldflags="$(LDFLAGS)" ./cmd/server

deps:
	go mod tidy
	go mod download

npm-pack:
	cd packages/cli-agent-gateway && npm pack
