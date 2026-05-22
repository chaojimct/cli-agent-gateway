# CLI Agent Gateway

[![Release](https://img.shields.io/github/v/release/chaojimct/cli-agent-gateway?label=release)](https://github.com/chaojimct/cli-agent-gateway/releases)
[![CI](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/ci.yml)
[![Build](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/build.yml/badge.svg)](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md)

Route **OpenAI Chat**, **Anthropic Messages**, **OpenAI Responses**, and **Gemini** APIs to CLI coding agents — Cursor, Claude Code, Kimi, and others — via **[ACP](https://github.com/agentclientprotocol/agent-client-protocol)** (Agent Client Protocol, JSON-RPC over stdio).

## Documentation

| Doc | Description |
|-----|-------------|
| [**Usage Guide**](docs/guide.md) · [中文](docs/guide.zh-CN.md) | Install, configure, client integration, troubleshooting |
| [**CHANGELOG**](CHANGELOG.md) | Release history |
| [config.local.yaml.example](config.local.yaml.example) | Local config template |

## Features

- **Multi-protocol frontends** — `/v1/chat/completions`, `/v1/messages`, `/v1/responses`, Gemini `generateContent` / `streamGenerateContent`
- **Multi-agent ACP backend** — One long-lived ACP subprocess per agent profile; model IDs use `agent/model` (e.g. `cursor/composer-2.5-fast`, `claude/sonnet`)
- **Auto-discovery** — Probes installed agents at startup; optional `agents.profiles` overrides
- **Tool loop translation** — ACP `session/update`, `session/request_permission`, and `cursor/*` extensions → unified IR → SSE
- **Session continuity** — Explicit `X-Conversation-Id` / `metadata.conversation_id`, or auto-derived thread keys
- **Client tools** — OpenCode-style clients: ACP permission → OpenAI `tool_calls`
- **Web UI** — Live trace viewer, diff/compare, export, runtime config, graceful restart
- **Observability** — Prometheus (`GET /metrics`), detailed status (`GET /status`)

## Quick Start

### Option A — Download release (recommended)

1. Get the latest build from **[Releases](https://github.com/chaojimct/cli-agent-gateway/releases)** (currently **v0.1.1**)
2. Extract the archive for your OS/ARCH
3. Set `CURSOR_API_KEY` (or log in via `cursor-agent` once)
4. Run:

```bash
./cli-agent-gateway
# Windows: cli-agent-gateway.exe
# Config: ./config.yaml if present, else ~/.config/cli-agent-gateway/ (auto-init)
```

5. Test: `curl http://127.0.0.1:8080/healthz` · Web UI: http://127.0.0.1:8080/

See the [**Usage Guide**](docs/guide.md) for full setup, multi-agent config, and client examples.

### Option B — npm (Node 18+)

```bash
npm install -g cli-agent-gateway
cli-agent-gateway init
cli-agent-gateway
```

Or `npx cli-agent-gateway`. Postinstall downloads the platform binary from GitHub Releases (npm version must match a published release). See [packages/cli-agent-gateway/README.md](packages/cli-agent-gateway/README.md).

### Option C — Build from source

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway
cp config.local.yaml.example config.local.yaml   # optional
make build && make run
```

Requires Go 1.23+ and a local ACP agent ([cursor-agent](https://cursor.com)).

## Minimal Example

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/composer-2.5-fast",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

Python ([OpenAI SDK](https://github.com/openai/openai-python)):

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:8080/v1", api_key="local")
print(client.chat.completions.create(
    model="cursor/composer-2.5-fast",
    messages=[{"role": "user", "content": "Hello!"}],
).choices[0].message.content)
```

## Architecture

```
Client (OpenAI / Anthropic / Gemini SDK)
        │
        ▼
  CLI Agent Gateway  ── HTTP/SSE ──►  handler
        │
        ▼
  AgentRouter  ── JSON-RPC stdio ──►  ACP agent (cursor-agent, claude-acp, …)
        │
        ▼
  toolloop  (ACP events → IR → SSE)
```

## API Overview

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI Chat (streaming) |
| `POST` | `/v1/messages` | Anthropic Messages |
| `POST` | `/v1/responses` | OpenAI Responses |
| `POST` | `/v1beta/models/*` | Gemini generateContent |
| `GET` | `/v1/models` | Model list (`agent/model` IDs) |
| `GET` | `/healthz` · `/status` · `/metrics` | Health, stats, Prometheus |
| `GET` | `/` | Web UI trace viewer |

Full endpoint list and config reference: [**Usage Guide →**](docs/guide.md)

## Configuration

Load order: `config.yaml` → `config.local.yaml` → `CG_*` environment variables.

```yaml
# config.local.yaml
cursor:
  binary_path: cursor-agent
  default_model: cursor/composer-2.5-fast
  workspace: /path/to/project
  client_tools_agent_mode: plan   # for OpenCode
```

| Section | Purpose |
|---------|---------|
| `cursor` | Default agent, model, profile/mode, concurrency |
| `agents` | Multi-agent discovery & profiles |
| `auth` | API key gate |
| `session` | Persistent `sessions.json` |
| `webui` | Trace retention, admin API |

## Releases & Development

Prebuilt binaries for **Linux / Windows / macOS** (amd64 & arm64) are published on **[Releases](https://github.com/chaojimct/cli-agent-gateway/releases)** with `SHA256SUMS.txt`.

```bash
make test        # unit tests
make build-all   # cross-compile locally → dist/
```

Push a `v*` tag to trigger a new release (e.g. `v0.1.2`).

## Project Layout

| Package | Role |
|---------|------|
| `internal/acp/` | ACP JSON-RPC client |
| `internal/runner/` | AgentRouter, ACPGateway, turn scheduling |
| `internal/agent/` | Discovery, registry, model listing |
| `internal/toolloop/` | ACP events → IR translation |
| `internal/handler/` | HTTP handlers & SSE |
| `internal/webui/` | Trace store & embedded UI |

## License

[MIT](LICENSE)
