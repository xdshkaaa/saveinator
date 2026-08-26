# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dash ./cmd/dash

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /dash /app/dash

ENV DASH_LISTEN=127.0.0.1
ENV DASH_PORT=9000

EXPOSE 9000

ENTRYPOINT ["/app/dash"]
