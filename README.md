# CLI Agent Gateway

**English** | [简体中文](README.zh-CN.md)

Route **OpenAI Chat**, **Anthropic Messages**, **OpenAI Responses**, and **Gemini** APIs to CLI coding agents — Cursor, Claude Code, Kimi, and others — via **[ACP](https://github.com/agentclientprotocol/agent-client-protocol)** (Agent Client Protocol, JSON-RPC over stdio).

## Features

- **Multi-protocol frontends** — `/v1/chat/completions`, `/v1/messages`, `/v1/responses`, Gemini `generateContent` / `streamGenerateContent`
- **Multi-agent ACP backend** — One long-lived ACP subprocess per agent profile; model IDs use `agent/model` (e.g. `cursor/composer-2.5-fast`, `claude/sonnet`)
- **Auto-discovery** — Probes installed agents at startup; optional `agents.profiles` overrides
- **Tool loop translation** — ACP `session/update`, `session/request_permission`, and `cursor/*` extensions → unified IR → SSE
- **Session continuity** — Explicit `X-Conversation-Id` / `metadata.conversation_id`, or auto-derived thread keys; in-memory ACP session pool + optional `sessions.json` persistence
- **Model profile** — Default `agent_profile: model` for drop-in text completion; `agent` profile for full agent behavior
- **Client tools** — OpenCode-style clients: ACP permission requests translated to OpenAI `tool_calls` (`client_tools_mode`, `client_tools_agent_mode`)
- **Web UI** — Live trace viewer (tap-style), diff/compare, export, runtime config, graceful restart
- **Observability** — Prometheus (`GET /metrics`), detailed status (`GET /status`)
- **Security** — Optional API key auth, CORS / Origin checks, request body limits

## Architecture

```
Client (OpenAI / Anthropic / Gemini SDK)
        │
        ▼
  CLI Agent Gateway  ── HTTP/SSE ──►  internal/handler
        │
        ▼
  internal/cursor.Runner  ──►  internal/runner.AgentRouter
        │
        ▼
  internal/runner.ACPGateway  ── JSON-RPC stdio ──►  ACP agent subprocess
        │                                              (cursor-agent, claude-acp, …)
        ▼
  internal/toolloop  (ACP events → IR → SSE)
```

Execution path is **ACP-only** (v2). Legacy stream-json / daemon modes are removed; `use_daemon` remains in config but is unused.

## Requirements

- Go 1.23+
- At least one ACP agent on the host (default: [cursor-agent](https://cursor.com))
- Credentials for your agents (e.g. `CURSOR_API_KEY`, `ANTHROPIC_API_KEY`, or interactive login)

## Quick Start

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway

cp config.local.yaml.example config.local.yaml   # optional local overrides

make build
make run
```

Default listen address: `http://127.0.0.1:8080`. Web UI: `/` (tap viewer), legacy compact UI: `/legacy`.

### Minimal config

Edit `config.local.yaml` (gitignored):

```yaml
cursor:
  binary_path: cursor-agent          # or full path / cursor-agent.cmd on Windows
  default_model: cursor/composer-2.5-fast
  # proxy: http://127.0.0.1:7890
```

Load order: `config.yaml` → `config.local.yaml` → environment variables.

### Multi-agent example

```yaml
agents:
  auto_discover: true
  default: cursor
  model_cache_ttl: 5m
  profiles:
    claude:
      enabled: true
      spawn:
        command: npx
        args: ["-y", "@agentclientprotocol/claude-agent-acp"]
      models:
        source: session_new
```

Request Claude models as `claude/sonnet`, Cursor models as `cursor/composer-2.5-fast` (prefix optional when using the default agent).

## API Endpoints

### LLM compatibility

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions (streaming) |
| `POST` | `/v1/messages` | Anthropic Messages API |
| `POST` | `/v1/responses` | OpenAI Responses API |
| `POST` | `/v1beta/models/*` | Gemini generateContent / streamGenerateContent |
| `GET` | `/v1/models` | Aggregated model list (`agent/model` IDs); **503** until discovery finishes |

### Ops & observability

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health: `status`, `ready`, `version` |
| `GET` | `/status` | Uptime, concurrency, per-agent ACP stats |
| `GET` | `/metrics` | Prometheus text metrics |

### Web UI & admin

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Tap-style trace viewer |
| `GET` | `/legacy` | Legacy compact trace UI |
| `GET` | `/ws/events` | WebSocket live trace events |
| `GET` | `/api/traces` | Trace list (`q`, `endpoint`, `model`, `status` filters) |
| `GET` | `/api/traces/compare` | Compare two traces (`a`, `b`) |
| `GET` | `/api/traces/{id}` | Single trace |
| `GET` | `/api/traces/{id}/export` | Export (`format=html` or JSON) |
| `GET` | `/api/stats` | Aggregated stats |
| `GET` | `/api/tap/events` | Tap-style SSE stream |
| `GET` | `/api/tap/records` | Tap records JSON |
| `GET` | `/api/config` | Current config (secrets masked) |
| `PUT` | `/api/config` | Merge-update config (writes `config.yaml`) |
| `POST` | `/api/admin/restart` | Graceful restart |

### Session continuity

Explicit id (recommended for multi-turn):

```http
X-Conversation-Id: my-thread-001
```

Or in metadata:

```json
{ "metadata": { "conversation_id": "my-thread-001" } }
```

Anthropic also accepts `metadata.user_id`. Without an explicit id, the gateway derives a stable key from the system prompt and first user message.

## Configuration

| Section | Purpose |
|---------|---------|
| `server` | Host, port, timeouts, CORS origins, max body size |
| `cursor` | Default agent binary, model, profile/mode, concurrency, client-tools behavior |
| `agents` | Multi-agent discovery, default agent, per-profile spawn/probe/models |
| `session` | Persistent session store (`sessions.json`) |
| `auth` | API key gate |
| `logging` | Level & format (`json` / `text`) |
| `webui` | Trace retention, config API policy |
| `admin` | Graceful restart timeout |

Key `cursor` fields:

| Field | Default | Notes |
|-------|---------|-------|
| `agent_profile` | `model` | `model` = completion API; `agent` = full agent |
| `agent_mode` | `ask` | `plan` or `ask` for the default cursor agent |
| `client_tools_mode` | `auto` | `auto` / `off` / `always` |
| `client_tools_agent_mode` | `ask` | Mode used when client sends tools (OpenCode) |
| `on_tool_call` | `abort` | Circuit-break tool calls in model profile |
| `default_model` | `cursor/composer-2.5-fast` | Used when request omits model |

Runtime config changes to `server.host/port`, `cursor.binary_path`, or `logging.format` require a restart; most other fields hot-reload.

## Environment Variables

| Variable | Effect |
|----------|--------|
| `CURSOR_API_KEY` / `CURSOR_AUTH_TOKEN` | Skip Cursor ACP authenticate |
| `ANTHROPIC_API_KEY` | Skip Claude ACP authenticate |
| `CG_ACP_RAW_DEBUG=1` | Log raw ACP JSON-RPC |
| `CG_SERVER_HOST` / `CG_SERVER_PORT` | Bind address |
| `CG_CURSOR_BINARY_PATH` | Cursor agent binary |
| `CG_CURSOR_DEFAULT_MODEL` | Default model id |
| `CG_CURSOR_MAX_CONCURRENT` | Max concurrent ACP turns |
| `CG_CURSOR_PROXY` | HTTP proxy for agent subprocesses |
| `CG_CURSOR_AGENT_PROFILE` | Override `cursor.agent_profile` (`model` / `agent`) |
| `CG_CURSOR_THINKING_VISIBILITY` | Reasoning field exposure |
| `CG_CURSOR_STREAM_PENDING_MODE` | `optimistic` / `buffer` |
| `CG_SESSION_ENABLED` / `CG_SESSION_STORAGE_PATH` | Session store |
| `CG_AUTH_ENABLED` / `CG_AUTH_API_KEY` | API auth |
| `CG_LOGGING_LEVEL` | Log level |

## Development

```bash
make test              # unit tests
make test-integration  # integration tests (build tag)
make lint              # golangci-lint
make build-all         # cross-compile all platforms locally → dist/
```

### Prebuilt binaries

GitHub Actions (`.github/workflows/build.yml`) cross-compiles for **6 platforms** on every push to `main` and PR:

| Platform | Archive |
|----------|---------|
| Linux amd64 / arm64 | `.tar.gz` |
| Windows amd64 / arm64 | `.zip` |
| macOS amd64 / arm64 | `.tar.gz` |

Download from the **Actions** run → **Artifacts**, or get a versioned build from **[Releases](https://github.com/chaojimct/cli-agent-gateway/releases)**. Push a `v*` tag (e.g. `v0.1.1`) to publish a new release automatically.

CI (`.github/workflows/ci.yml`) runs `go test ./...` on Ubuntu, Windows, and macOS.

## Project Layout

| Package | Role |
|---------|------|
| `internal/acp/` | ACP JSON-RPC client & protocol |
| `internal/acpsession/` | `session/new` helpers |
| `internal/runner/` | `AgentRouter`, per-agent `ACPGateway`, turn scheduling |
| `internal/toolloop/` | ACP events → IR, permission policy |
| `internal/ir/` | Unified streaming event types |
| `internal/session/` | In-memory `conversation_id` ↔ ACP sessionId pool |
| `internal/agent/` | Discovery, registry, model listing, probe |
| `internal/cursor/` | Public `Runner` facade, profiles, `SessionManager` |
| `internal/handler/` | HTTP handlers & SSE |
| `internal/translator/` | OpenAI / Anthropic / Gemini format adapters |
| `internal/webui/` | Trace store, embedded UI, admin API |
| `internal/config/` | Config load, defaults, hot reload |

## License

MIT
