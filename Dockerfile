# Multi-stage Dockerfile for AnyClaw
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /anyclaw ./cmd/anyclaw

# Runtime image
FROM alpine:3.20

RUN apk add --no-cache \
    bash \
    curl \
    git \
    jq \
    python3 \
    py3-pip \
    ripgrep \
    chromium \
    chromium-chromedriver \
    && rm -rf /var/cache/apk/*

COPY --from=builder /anyclaw /usr/local/bin/anyclaw

ENV ANYCLAW_SANDBOX=1
WORKDIR /workspace

ENTRYPOINT ["anyclaw"]
CMD ["gateway", "run"]
