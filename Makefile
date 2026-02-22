APP_NAME := hec2logstashhttp

.PHONY: test vet lint build run tidy

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

run:
	go run ./cmd/$(APP_NAME)

tidy:
	go mod tidy
