FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hec2logstashhttp ./cmd/hec2logstashhttp

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/hec2logstashhttp /app/hec2logstashhttp

EXPOSE 8088
ENTRYPOINT ["/app/hec2logstashhttp"]
