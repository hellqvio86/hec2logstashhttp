FROM golang:1.26-alpine AS builder
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w \
  -X github.com/hellqvio/hec2logstashhttp/internal/version.Version=${VERSION} \
  -X github.com/hellqvio/hec2logstashhttp/internal/version.Commit=${COMMIT} \
  -X github.com/hellqvio/hec2logstashhttp/internal/version.Date=${DATE}" \
  -o /out/hec2logstashhttp ./cmd/hec2logstashhttp

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/hec2logstashhttp /app/hec2logstashhttp

EXPOSE 8088
ENTRYPOINT ["/app/hec2logstashhttp"]
