# Dockerfile for anyclaw sandbox image
FROM alpine:latest

# Install common tools for shell command execution
RUN apk add --no-cache \
    bash \
    curl \
    wget \
    python3 \
    py3-pip \
    nodejs \
    npm \
    git \
    jq \
    coreutils \
    grep \
    sed \
    awk \
    ca-certificates \
    openssh-client

# Set working directory
WORKDIR /workspace

# Default shell
CMD ["/bin/sh"]
