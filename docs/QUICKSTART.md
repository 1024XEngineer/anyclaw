# AnyClaw Quickstart

This guide covers a source checkout of AnyClaw and focuses on commands that are
available in this repository.

## Prerequisites

- Go 1.25 or newer
- Node.js 22 or newer, with Corepack enabled, when building or running the
  control UI from source
- Docker, when using the Compose-based gateway runtime

## Build the CLI

From the repository root:

```sh
go build -o anyclaw ./cmd/anyclaw
```

On Windows, use an `.exe` suffix if you want a directly runnable binary:

```powershell
go build -o anyclaw.exe ./cmd/anyclaw
```

Then verify the command list:

```sh
./anyclaw --help
```

On Windows:

```powershell
.\anyclaw.exe --help
```

## Configure a Provider

Run the first-run setup wizard:

```sh
./anyclaw onboard
```

For non-interactive environments, write defaults without prompts:

```sh
./anyclaw onboard --non-interactive
```

Check the resulting configuration:

```sh
./anyclaw doctor
./anyclaw models status
./anyclaw config validate
```

By default, commands read `anyclaw.json` from the current directory. Pass
`--config <path>` to commands that support an alternate config file.

## Environment Variables

`.env.example` is a template for values you export in your shell. It also
includes the `ANYCLAW_LLM_*` names that `docker-compose.yml` reads from a
Compose-managed `.env` file. The AnyClaw CLI does not load `.env` files by
itself.

For local CLI runs, export variables before launching AnyClaw. Common examples:

```sh
export LLM_PROVIDER=openai
export LLM_MODEL=gpt-4o
export OPENAI_API_KEY=...
```

For Docker Compose, the provided compose file reads `ANYCLAW_LLM_PROVIDER`,
`ANYCLAW_LLM_MODEL`, and `ANYCLAW_LLM_API_KEY` from the Compose environment or a
Compose-managed `.env` file.

## Build or Run the Control UI

The gateway serves the control UI. When running from a repository checkout, the
gateway can auto-build the UI if Node.js is available. You can also build it
explicitly:

```sh
corepack enable
corepack prepare pnpm@10.23.0 --activate
pnpm run ui:build
```

Useful UI scripts:

```sh
pnpm run ui:dev
pnpm run ui:test
pnpm run ui:preview
```

## Run the Gateway Locally

Start the gateway in the foreground:

```sh
./anyclaw gateway run --host 127.0.0.1 --port 18789
```

`gateway start` is accepted as an alias for the same foreground command:

```sh
./anyclaw gateway start --host 127.0.0.1 --port 18789
```

Run it as a daemon:

```sh
./anyclaw gateway daemon start
./anyclaw gateway daemon stop
```

Inspect the running gateway:

```sh
./anyclaw gateway status
./anyclaw gateway sessions
./anyclaw gateway events
```

## Run with Docker Compose

Create a `.env` file for Compose. Use the `ANYCLAW_LLM_*` names because these
are the variables read by `docker-compose.yml`:

```sh
ANYCLAW_LLM_PROVIDER=openai
ANYCLAW_LLM_MODEL=gpt-4o
ANYCLAW_LLM_API_KEY=...
```

Create the host-side runtime workspace, then start the gateway:

```sh
mkdir -p workspace/workflows
```

```sh
docker compose up --build
```

The service exposes the gateway on port `18789`. Gateway state is stored in the
`anyclaw-data` volume at `/workspace/.anyclaw`, while agent workflow files are
bind-mounted from `./workspace/workflows` to `/workspace/workflows`. This
runtime workspace is separate from the source checkout used to build the image.

## Useful Commands

```sh
./anyclaw models list
./anyclaw models set gpt-4o-mini
./anyclaw config file
./anyclaw status
./anyclaw health
./anyclaw sessions
./anyclaw skill list
./anyclaw skill catalog
```
