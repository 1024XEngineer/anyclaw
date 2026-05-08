# Multi-stage Dockerfile for AnyClaw
FROM node:22-alpine AS ui-builder

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.23.0 --activate

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY ui/package.json ./ui/package.json
RUN pnpm install --frozen-lockfile

COPY scripts ./scripts
COPY ui ./ui
COPY extensions ./extensions
RUN pnpm run ui:build

FROM golang:1.25-alpine AS builder

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
COPY --from=ui-builder /app/dist/control-ui /opt/anyclaw/control-ui

ENV ANYCLAW_SANDBOX=1
ENV ANYCLAW_CONTROL_UI_ROOT=/opt/anyclaw/control-ui
WORKDIR /workspace

ENTRYPOINT ["anyclaw"]
CMD ["gateway", "run", "--host", "0.0.0.0", "--port", "18789"]
