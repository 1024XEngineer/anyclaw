# anyclaw Sandbox Docker Image

This directory contains the Dockerfile for building the sandbox image used by anyclaw's Shell tool.

## Building

```bash
# Build the image
docker build -t anyclaw/sandbox:latest .

# Or with a custom tag
docker build -t my-registry/anyclaw-sandbox:v1.0.0 .
```

## Included Tools

The sandbox image includes the following tools:

- **bash**: Unix shell
- **curl**: Command-line tool for transferring data with URLs
- **wget**: Command-line download utility
- **python3**: Python 3 interpreter
- **py3-pip**: Python package installer
- **nodejs**: Node.js JavaScript runtime
- **npm**: Node package manager
- **git**: Distributed version control system
- **jq**: Command-line JSON processor
- **openssh-client**: SSH client for remote connections
- **coreutils**: Core GNU utilities (ls, cat, cp, mv, etc.)
- **grep**, **sed**, **awk**: Text processing tools

## Using with anyclaw

Add the following to your `~/.anyclaw/config.json`:

```json
{
  "tools": {
    "shell": {
      "sandbox": {
        "enabled": true,
        "image": "anyclaw/sandbox:latest"
      }
    }
  }
}
```

## Security

This image is based on Alpine Linux and includes only minimal tools required for common development tasks. When used with anyclaw's sandbox configuration with `network: "none"` and `privileged: false`, it provides a secure isolation environment.

## Customization

To customize the sandbox image, modify the Dockerfile to add or remove tools as needed for your use case.
