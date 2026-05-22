# Cursor Gateway

**English** | [简体中文](README.zh-CN.md)

A Go API gateway that bridges popular LLM client APIs to **[Cursor Agent ACP](https://github.com/cursor/agent-client-protocol)** (Agent Client Protocol, JSON-RPC over stdio). Point your OpenAI-, Anthropic-, or Gemini-compatible clients at this gateway and run requests through `cursor-agent acp`.

## Features

- **Multi-protocol API** — OpenAI Chat Completions, Anthropic Messages, OpenAI Responses, and Gemini `generateContent` / `streamGenerateContent`
- **ACP backbone** — Long-lived `cursor-agent acp` processes with session reuse and turn scheduling
- **Tool loop translation** — Maps ACP `session/update` events and `cursor/*` extensions to a unified IR, then streams SSE to clients
- **Session reuse** — `X-Conversation-Id` or `metadata.conversation_id` maps to ACP sessions; multi-turn requests send incremental prompts only
- **Model profile** — Default `agent_profile: model` for general text completion; plan/ask modes, isolated workspace, tool-call circuit breaker
- **Client tools** — OpenCode and similar clients: ACP `session/request_permission` → OpenAI `tool_calls` translation
- **Multi-agent routing** — Optional agent profiles (Cursor, Claude ACP, Kimi, etc.) with auto-discovery
- **Web UI** — Request tracing, diff/compare, live config (`PUT /api/config`), graceful restart
- **Observability** — Prometheus metrics (`GET /metrics`), status with ACP session stats (`GET /status`)
- **Security** — Unified API/WS auth, Origin checks, request body size limits

## Architecture

```
Client (OpenAI / Anthropic / Gemini SDK)
        │
        ▼
  cursor-gateway  ── HTTP/SSE ──►  internal/handler
        │
        ▼
  internal/runner  ── JSON-RPC stdio ──►  cursor-agent acp
        │
        ▼
  internal/toolloop  (ACP events → IR → SSE)
```

## Requirements

- Go 1.23+
- [cursor-agent](https://cursor.com) installed and on `PATH`, or configured via `cursor.binary_path`
- Cursor account (or set `CURSOR_API_KEY` to skip ACP `authenticate`)

## Quick Start

```bash
# Clone
git clone https://github.com/chaojimct/cursor-gateway.git
cd cursor-gateway

# Copy local config (optional overrides)
cp config.local.yaml.example config.local.yaml

# Build & run
make build
make run
```

The server listens on `http://127.0.0.1:8080` by default. Open the Web UI at `/`.

### Minimal config

Edit `config.local.yaml` (not committed):

```yaml
cursor:
  binary_path: /path/to/cursor-agent   # or cursor-agent.cmd on Windows
  default_model: composer-2.5-fast
  # proxy: http://127.0.0.1:7890       # optional
```

Config load order: `config.yaml` → `config.local.yaml` (later keys override).

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions (streaming supported) |
| `POST` | `/v1/messages` | Anthropic Messages API |
| `POST` | `/v1/responses` | OpenAI Responses API |
| `POST` | `/v1beta/models/*` | Gemini generateContent / streamGenerateContent |
| `GET` | `/v1/models` | List available models |
| `GET` | `/healthz` | Health check |
| `GET` | `/status` | Gateway & ACP session status |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/api/traces` | Trace list (Web UI) |
| `GET` | `/api/config` | Current config |
| `PUT` | `/api/config` | Update config at runtime |
| `POST` | `/api/admin/restart` | Graceful restart |
| `GET` | `/ws/events` | WebSocket event stream |

### Session continuity

Send the same conversation id across turns:

```http
X-Conversation-Id: my-thread-001
```

Or in request metadata:

```json
{ "metadata": { "conversation_id": "my-thread-001" } }
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CURSOR_API_KEY` | Skip ACP interactive authenticate when set |
| `CG_CURSOR_AGENT_PROFILE` | Override default cursor agent profile |
| `CG_ACP_RAW_DEBUG=1` | Log raw ACP JSON-RPC for debugging |

## Development

```bash
make test              # unit tests
make test-integration  # integration tests (tagged)
make lint              # golangci-lint
```

CI runs `go test ./...` on Ubuntu, Windows, and macOS.

## Project Layout

| Package | Role |
|---------|------|
| `internal/acp/` | ACP JSON-RPC protocol & client |
| `internal/runner/` | ACP gateway, process keep-alive, turn dispatch |
| `internal/toolloop/` | ACP events → IR translation & permission policy |
| `internal/session/` | `conversation_id` ↔ sessionId LRU pool |
| `internal/cursor/` | Runner wrapper, profiles, session manager |
| `internal/handler/` | HTTP handlers & SSE streaming |
| `internal/webui/` | Trace store, compare, export, embedded UI |
| `internal/agent/` | Multi-agent registry & discovery |
| `internal/translator/` | Request/response format translators |

## License

MIT
