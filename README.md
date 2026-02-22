# hec2logstashhttp

[![CI](https://github.com/hellqvio/hec2logstashhttp/actions/workflows/ci.yml/badge.svg)](https://github.com/hellqvio/hec2logstashhttp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hellqvio/hec2logstashhttp)](https://github.com/hellqvio/hec2logstashhttp/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/hellqvio/hec2logstashhttp)](https://github.com/hellqvio/hec2logstashhttp/blob/main/go.mod)
[![License](https://img.shields.io/github/license/hellqvio/hec2logstashhttp)](https://github.com/hellqvio/hec2logstashhttp/blob/main/LICENSE)
[![GHCR](https://img.shields.io/badge/ghcr-hec2logstashhttp-blue?logo=docker)](https://github.com/hellqvio/hec2logstashhttp/pkgs/container/hec2logstashhttp)

`hec2logstashhttp` is a small Go service that provides a Splunk HEC-compatible HTTP endpoint for senders like Home Assistant and forwards events to Logstash HTTP input.

## Disclaimer

This project is an independent community effort and is not affiliated with, endorsed by, or sponsored by Splunk or Elastic.

## Why

Some senders expect Splunk HEC response semantics (`{"text":"Success","code":0}`), while Logstash `input-http` typically responds with plain text `ok`. This shim bridges that gap.

## Features

- Splunk-compatible endpoints:
  - `POST /services/collector`
  - `POST /services/collector/event`
- Health endpoint:
  - `GET /healthz`
- Optional token validation (`Authorization: Splunk <token>`)
- Forwards event payloads to Logstash HTTP input
- Adds standard proxy forwarding headers (`Forwarded`, `X-Forwarded-*`, `X-Real-IP`)
- Normalizes Splunk HEC envelopes into Logstash-friendly JSON (supports single and batched HEC events)
- Rejects non-HEC payloads with Splunk-compatible error response (`{"text":"Invalid data format","code":6}`)

## Configuration

Environment variables:

- `HEC_LISTEN_ADDR` (default `:8088`)
- `HEC_FORWARD_URL` (default `http://127.0.0.1:18088/services/collector/event`)
- `HEC_FORWARD_UA` (default `false`; when `true`, forwards incoming client `User-Agent`)
- `HEC_TOKEN` (default empty; if empty, auth is not enforced)
- `HEC_REQUEST_TIMEOUT` (default `5s`)
- `HEC_SHUTDOWN_TIMEOUT` (default `10s`)
- `HEC_MAX_BODY_BYTES` (default `1048576`)
- `HEC_LOG_LEVEL` (`debug`, `info`, `warn`, `error`; default `info`)

## Local Run

```bash
make run
```

## Build

```bash
make build
```

## Versioning and Releases

Release version is controlled by the root `VERSION` file (for example `0.1.0`).
Builds inject version metadata into the binary via `-ldflags`.

Build with current version:

```bash
make build
```

Set a new release version:

```bash
echo "0.2.0" > VERSION
```

Print version from the binary:

```bash
./bin/hec2logstashhttp -version
```

### CI/CD release flow

1. Update `VERSION` and merge to `main`.
2. `ci.yml` automatically creates a **draft GitHub Release** (`v<version>`) when `VERSION` changes.
3. Publish that draft release in GitHub UI.
4. On publish, `ci.yml` will:
- validate release tag matches `VERSION` (`v0.2.0` -> `0.2.0`)
- build and attach Go release artifacts for:
  - `linux/amd64`
  - `linux/arm64` (Raspberry Pi 64-bit)
  - `linux/arm/v7` (Raspberry Pi 32-bit)
- generate and attach SBOM files (source + container image)
- build and publish multi-arch Docker image to `ghcr.io/<owner>/<repo>`

## Test

```bash
make test
make vet
```

## Docker

Published image:

- `ghcr.io/hellqvio/hec2logstashhttp`

Image tags are pushed when a GitHub Release is published:

- `latest`
- `<major>` (example `1`)
- `<major>.<minor>` (example `1.2`)
- `<major>.<minor>.<patch>` (example `1.2.3`)

Supported platforms:

- `linux/amd64`
- `linux/arm64`
- `linux/arm/v7`

Pull from GHCR:

```bash
docker pull ghcr.io/hellqvio/hec2logstashhttp:latest
```

Run published image:

```bash
docker run --rm -p 8088:8088 \
  -e HEC_FORWARD_URL=http://host.docker.internal:18088/services/collector/event \
  ghcr.io/hellqvio/hec2logstashhttp:latest
```

Build locally:

```bash
docker build -t hec2logstashhttp:dev .
docker run --rm -p 8088:8088 \
  -e HEC_FORWARD_URL=http://host.docker.internal:18088/services/collector/event \
  hec2logstashhttp:dev
```

## Quick Check

```bash
curl -i -H "Content-Type: application/json" \
  -d '{"event":"hello"}' \
  http://localhost:8088/services/collector/event
```

Expected response body:

```json
{"text":"Success","code":0}
```
