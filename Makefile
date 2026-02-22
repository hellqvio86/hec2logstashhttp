APP_NAME := hec2logstashhttp
VERSION_FILE := VERSION
VERSION ?= $(shell test -f $(VERSION_FILE) && tr -d '[:space:]' < $(VERSION_FILE) || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Version=$(VERSION) \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Commit=$(COMMIT) \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Date=$(DATE)

.PHONY: test vet lint build run tidy clean

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

run:
	go run -ldflags="$(LDFLAGS)" ./cmd/$(APP_NAME)

tidy:
	go mod tidy

clean:
	rm -rf bin
	go clean -testcache
