# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /saveinator ./cmd/saveinator

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg curl ca-certificates python3 python3-pip unzip \
    && curl -fsSL https://deno.land/install.sh | DENO_INSTALL=/usr/local sh \
    # yt-dlp pinned: 2026.8.19 carries the upstream TikTok bot-challenge fixes
    # (yt-dlp#17452/#17460) and the YouTube 403 fixes; 2026.7.4 fails both
    # (YouTube audio downloads return "HTTP Error 403: Forbidden").
    && pip3 install --no-cache-dir --break-system-packages "yt-dlp[default,curl-cffi]==2026.8.19" \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /saveinator /app/saveinator

ENV SAVEINATOR_MODE=all
ENV USE_POLLING=false
ENV METRICS_ENABLED=true
ENV METRICS_PORT=9101

EXPOSE 8000 9101 9102

ENTRYPOINT ["/app/saveinator"]
