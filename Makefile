APP_NAME := hec2logstashhttp
VERSION_FILE := VERSION
VERSION ?= $(shell test -f $(VERSION_FILE) && tr -d '[:space:]' < $(VERSION_FILE) || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Version=$(VERSION) \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Commit=$(COMMIT) \
	-X github.com/hellqvio/hec2logstashhttp/internal/version.Date=$(DATE)

.PHONY: fmt fmt-check test coverage vet lint build run tidy clean renovate renovate-summary version show-version

fmt:
	gofmt -w .

fmt-check:
	files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "Unformatted files:"; \
		echo "$$files"; \
		exit 1; \
	fi

test:
	go test ./...

coverage:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

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
	rm -f bin/$(APP_NAME)
	go clean -testcache

version:
	@CURRENT="$$(test -f $(VERSION_FILE) && tr -d '[:space:]' < $(VERSION_FILE) || echo "0.0.0")"; \
	NEW_V="$(V)"; \
	if [ -z "$$NEW_V" ]; then \
		BUMP_TYPE="$${BUMP:-patch}"; \
		major=$$(echo "$$CURRENT" | cut -d. -f1); \
		minor=$$(echo "$$CURRENT" | cut -d. -f2); \
		patch=$$(echo "$$CURRENT" | cut -d. -f3); \
		case "$$BUMP_TYPE" in \
			major) major=$$((major + 1)); minor=0; patch=0 ;; \
			minor) minor=$$((minor + 1)); patch=0 ;; \
			patch) patch=$$((patch + 1)) ;; \
			*) echo "Error: Invalid BUMP '$$BUMP_TYPE'. Allowed: major, minor, patch" >&2; exit 1 ;; \
		esac; \
		NEW_V="$$major.$$minor.$$patch"; \
	else \
		NEW_V="$${NEW_V#v}"; \
	fi; \
	if ! echo "$$NEW_V" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "Error: Version '$$NEW_V' must be valid SemVer without v-prefix (e.g. 1.2.3)" >&2; \
		exit 1; \
	fi; \
	echo "$$NEW_V" > $(VERSION_FILE); \
	echo "Version updated: $$CURRENT -> $$NEW_V"

show-version:
	@test -f $(VERSION_FILE) && tr -d '[:space:]' < $(VERSION_FILE) || echo "unknown"

renovate:
	@./bin/renovate/run.sh

renovate-summary:
	@./bin/renovate/run.sh --summary

