FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder
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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=builder /out/hec2logstashhttp /app/hec2logstashhttp

EXPOSE 8088
ENTRYPOINT ["/app/hec2logstashhttp"]
