# Usage Guide

**English** | [简体中文](guide.zh-CN.md)

Complete setup and integration guide for CLI Agent Gateway.

## Table of Contents

- [Install](#install)
- [First Run](#first-run)
- [Configure Agents](#configure-agents)
- [Client Integration](#client-integration)
- [Multi-turn Conversations](#multi-turn-conversations)
- [OpenCode & Client Tools](#opencode--client-tools)
- [Web UI](#web-ui)
- [Auth & Security](#auth--security)
- [Operations](#operations)
- [Troubleshooting](#troubleshooting)

---

## Install

### From GitHub Release (recommended)

1. Open [Releases](https://github.com/chaojimct/cli-agent-gateway/releases)
2. Download the archive for your platform (see table below)
3. Verify checksum (optional):

```bash
sha256sum -c SHA256SUMS.txt
```

4. Extract and place `config.yaml` next to the binary

| Platform | Archive | Binary |
|----------|---------|--------|
| Linux x64 | `*_linux-amd64.tar.gz` | `cli-agent-gateway` |
| Linux ARM64 | `*_linux-arm64.tar.gz` | `cli-agent-gateway` |
| Windows x64 | `*_windows-amd64.zip` | `cli-agent-gateway.exe` |
| Windows ARM64 | `*_windows-arm64.zip` | `cli-agent-gateway.exe` |
| macOS Intel | `*_darwin-amd64.tar.gz` | `cli-agent-gateway` |
| macOS Apple Silicon | `*_darwin-arm64.tar.gz` | `cli-agent-gateway` |

### From source

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway
make build    # → bin/cli-agent-gateway.exe (Windows) or bin/cli-agent-gateway
```

Requires **Go 1.23+** and a local ACP agent (see [Configure Agents](#configure-agents)).

---

## First Run

### 1. Install an ACP agent

At minimum, install **cursor-agent** ([Cursor CLI](https://cursor.com)):

```bash
# macOS / Linux — follow Cursor docs for your OS
cursor-agent --version
```

Or use Claude Code via npm (no separate install if Node.js is present):

```bash
npx -y @agentclientprotocol/claude-agent-acp --help
```

### 2. Set credentials

```bash
# Cursor — skip interactive login
export CURSOR_API_KEY="your-key"

# Claude ACP — if not logged in via Claude Code
export ANTHROPIC_API_KEY="your-key"
```

### 3. Local overrides (optional)

```bash
cp config.local.yaml.example config.local.yaml
```

Edit `config.local.yaml`:

```yaml
cursor:
  binary_path: cursor-agent          # Windows: C:\...\cursor-agent.cmd
  default_model: cursor/composer-2.5-fast
  workspace: /path/to/your/project   # agent working directory
  # proxy: http://127.0.0.1:7890
```

### 4. Start the gateway

```bash
./cli-agent-gateway -config config.yaml
# Windows:
cli-agent-gateway.exe -config config.yaml
```

Expected log: `starting cli-agent-gateway` on `127.0.0.1:8080`.

### 5. Smoke test

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/v1/models
```

Open **http://127.0.0.1:8080/** for the trace Web UI.

---

## Configure Agents

### Default (Cursor only)

Shipped `config.yaml` works if `cursor-agent` is on `PATH`. Models appear as `cursor/*`.

### Multi-agent

Add to `config.local.yaml`:

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
      probe:
        reject_auth_methods: [cursor_login]
      models:
        source: session_new
    kimi:
      enabled: true
      spawn:
        command: kimi-cli
        args: ["acp"]
      models:
        source: session_new
```

List models: `GET /v1/models` → `cursor/composer-2.5-fast`, `claude/sonnet`, etc.

### Key `cursor` settings

| Field | When to change |
|-------|----------------|
| `agent_profile: model` | Default — OpenAI-compatible completion (recommended) |
| `agent_profile: agent` | Full agent behavior (tools, plan mode) |
| `agent_mode: plan` | Allow agent to execute changes (vs read-only `ask`) |
| `client_tools_agent_mode: plan` | **OpenCode** — use `plan`, not `agent_mode` |
| `workspace` | Project root the agent operates in |
| `max_concurrent: 8` | Max parallel ACP turns |

Config load order: `config.yaml` → `config.local.yaml` → `CG_*` env vars.

---

## Client Integration

Point any OpenAI-compatible client at the gateway base URL (`http://127.0.0.1:8080/v1`).

### curl

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "cursor/composer-2.5-fast",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1",
    api_key="not-needed",  # set auth.api_key if enabled
)

stream = client.chat.completions.create(
    model="cursor/composer-2.5-fast",
    messages=[{"role": "user", "content": "Explain ACP in one sentence."}],
    stream=True,
)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="")
```

### Anthropic SDK

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://127.0.0.1:8080",
    api_key="not-needed",
)

message = client.messages.create(
    model="claude/sonnet",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello"}],
)
print(message.content[0].text)
```

### Continue / IDE extensions

Set in your IDE LLM settings:

| Setting | Value |
|---------|-------|
| Provider | OpenAI-compatible |
| Base URL | `http://127.0.0.1:8080/v1` |
| Model | `cursor/composer-2.5-fast` |
| API Key | any string (or your `auth.api_key`) |

### Environment override (quick test)

```bash
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
export OPENAI_API_KEY=local
```

---

## Multi-turn Conversations

The gateway reuses ACP sessions when the same conversation id is sent.

**Header (all APIs):**

```http
X-Conversation-Id: project-feature-auth
```

**OpenAI metadata:**

```json
{
  "model": "cursor/composer-2.5-fast",
  "metadata": { "conversation_id": "project-feature-auth" },
  "messages": [...]
}
```

**Anthropic metadata** — also accepts `metadata.user_id`.

If omitted, a stable id is derived from the system prompt + first user message (good for single-session scripts, not for parallel chats).

---

## OpenCode & Client Tools

For clients that send `tools` (OpenCode, custom agents):

1. Keep `client_tools_mode: auto` (default)
2. Set `client_tools_agent_mode: plan` so the agent can execute tools
3. Do **not** rely on `agent_mode` for tool requests — that field applies to plain completions only

The gateway translates ACP `session/request_permission` into OpenAI `tool_calls` and expects tool results in the next request.

---

## Web UI

| URL | Purpose |
|-----|---------|
| `/` | Tap-style live trace viewer |
| `/legacy` | Compact legacy viewer |
| `/api/traces` | Trace list JSON |
| `/api/config` | Read runtime config |

Use the Web UI to inspect prompts, tool calls, token usage, and SSE events. Traces are stored in memory (`webui.max_traces`, default 2000).

Runtime config update:

```bash
curl -X PUT http://127.0.0.1:8080/api/config \
  -H "Content-Type: application/json" \
  -d '{"logging": {"level": "debug"}}'
```

Graceful restart (reload binary / agent processes):

```bash
curl -X POST http://127.0.0.1:8080/api/admin/restart
```

---

## Auth & Security

Enable API key for non-local access:

```yaml
auth:
  enabled: true
  api_key: "your-secret-key"

server:
  host: 0.0.0.0          # listen on all interfaces
  allowed_origins:
    - "https://your-app.example.com"
```

Clients send:

```http
Authorization: Bearer your-secret-key
```

**Defaults are local-only** (`127.0.0.1`, auth off). Do not expose an unauthenticated instance to the internet.

---

## Operations

| Endpoint | Use |
|----------|-----|
| `GET /healthz` | Liveness — `ready: true` when agents discovered |
| `GET /status` | Per-agent ACP stats, uptime, concurrency |
| `GET /metrics` | Prometheus scrape target |

Debug ACP JSON-RPC:

```bash
CG_ACP_RAW_DEBUG=1 ./cli-agent-gateway -config config.yaml
```

Check version:

```bash
./cli-agent-gateway -version
# cli-agent-gateway v0.1.1
```

---

## Troubleshooting

### `GET /v1/models` returns 503

Agents are still probing. Wait a few seconds and retry. Check `/status` → `acp.agents`.

### `cursor-agent` not found

Set absolute path in `config.local.yaml`:

```yaml
cursor:
  binary_path: C:\Users\you\AppData\Local\cursor-agent\cursor-agent.cmd
```

Restart required after changing `binary_path`.

### Authentication / proxy errors

- Set `CURSOR_API_KEY` or log in via `cursor-agent` interactively once
- Set `cursor.proxy` if agents need an HTTP proxy

### Tool calls rejected in model profile

Expected when `agent_profile: model` and `on_tool_call: abort`. Either:
- Use a client-tools-aware flow (`client_tools_mode: auto`), or
- Switch to `agent_profile: agent`

### Session not continuing across turns

Ensure the same `X-Conversation-Id` or `metadata.conversation_id` on every request.

### Windows firewall

Allow inbound on port `8080` if accessing from another machine on the LAN.

---

## See Also

- [README](../README.md) — overview & API reference
- [CHANGELOG](../CHANGELOG.md) — release history
- [config.local.yaml.example](../config.local.yaml.example) — config template
