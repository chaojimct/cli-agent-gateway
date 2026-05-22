# CLAUDE.md

## 项目概述

**CLI Agent Gateway** — Go API 网关：将 OpenAI Chat / Anthropic Messages / OpenAI Responses / Gemini 转为 CLI 编程 Agent（Cursor、Claude Code、Kimi 等），底层通过 **ACP**（Agent Client Protocol，JSON-RPC over stdio）通信。

v2 执行路径 **仅 ACP**；stream-json / daemon 已移除。内置多 Agent（cursor、claude），支持 `agents.profiles` 扩展。

默认 **Model Profile**（`agent_profile: model`）对外提供通用文本补全；OpenCode 等带 tools 的请求走 `client_tools_mode` + ACP `session/request_permission` → OpenAI `tool_calls` 翻译。

模型 ID 格式：`agent/model`（如 `cursor/composer-2.5-fast`）。

## 文档

- [README.md](README.md) / [README.zh-CN.md](README.zh-CN.md) — 概览
- [docs/guide.zh-CN.md](docs/guide.zh-CN.md) — 使用指南
- [CHANGELOG.md](CHANGELOG.md) — 版本历史

## 构建命令

```bash
make build    # → bin/cli-agent-gateway.exe
make run
make test
```

## 核心能力

- **多 Agent ACP**：`internal/agent` 发现 + `internal/runner.AgentRouter` 每 profile 一个长驻 ACP 子进程
- **Tool Loop 翻译**：`internal/toolloop` 将 `session/update` + permission + `cursor/*` 译为 IR → SSE
- **Session**：`conversation_id` → `internal/session.Pool`（内存 LRU）；可选 `SessionManager` 持久化到 `sessions.json`
- **Model Profile**：`agent_profile: model|agent`；plan/ask 模式；`on_tool_call: abort` 熔断
- **Web UI**：tap 查看器（`/`）、legacy（`/legacy`）；追踪 / 对比 / 在线配置 / 优雅重启
- **可观测**：`GET /metrics`（Prometheus）、`GET /status`（各 agent ACP 统计）
- **安全**：API/WS 统一 Auth、Origin 校验、请求体上限

## 配置

- `config.yaml` + `config.local.yaml` 覆盖 + `CG_*` 环境变量
- `agents.auto_discover`（默认 true）、`agents.default: cursor`
- `CG_CURSOR_AGENT_PROFILE` 覆盖 `cursor.agent_profile`（非 `agents.default`）
- `CG_ACP_RAW_DEBUG=1` — ACP JSON-RPC 调试
- `CURSOR_API_KEY` / `ANTHROPIC_API_KEY` — 跳过对应 agent 的 ACP authenticate

## 关键包

- `internal/acp/` — ACP JSON-RPC 协议与客户端
- `internal/runner/` — AgentRouter、ACPGateway、turn 调度
- `internal/toolloop/` — ACP 事件 → IR 翻译与 permission 策略
- `internal/session/` — conversation_id ↔ sessionId LRU 池
- `internal/agent/` — 发现、Registry、模型列表、probe
- `internal/cursor/` — 对外 Runner 包装、profile、SessionManager
- `internal/handler/` — API + `stream.go`（IR 事件驱动）
- `internal/webui/` — Store、对比、导出、内嵌 UI

## API

- `POST /v1/chat/completions` | `/v1/messages` | `/v1/responses`
- `POST /v1beta/models/*` — Gemini
- `GET /v1/models`（发现未完成 503）| `/metrics` | `/status` | `/healthz`
- `GET /api/traces`、`/api/traces/compare`、`/api/tap/*`、`/api/config`
- `PUT /api/config`、`POST /api/admin/restart`
- `GET /ws/events`
