# CLAUDE.md

## 项目概述

Go API 网关：将 OpenAI Chat / Anthropic Messages / OpenAI Responses / Gemini 转为 **`cursor-agent acp`**（Agent Client Protocol，JSON-RPC over stdio）调用。v2 已移除 stream-json / daemon 路径。

默认 **Model Profile**（`agent_profile: model`）对外提供通用文本补全；OpenCode 等客户端工具场景走 ACP `session/request_permission` → OpenAI `tool_calls` 翻译。

## 构建命令

```bash
make build
make run
make test
```

## 核心能力

- **ACP 主干**：`internal/acp` 客户端 + `internal/runner` 长驻 `cursor-agent acp` 进程
- **Tool Loop 翻译**：`internal/toolloop` 将 8 种 `session/update` + `cursor/*` 扩展译为 IR → SSE
- **Session 复用**：`X-Conversation-Id` / `metadata.conversation_id` → ACP session，多轮只发增量 prompt
- **Model Profile**：plan/ask 模式、隔离 workspace、tool_call 熔断
- **Web UI**：追踪 / 对比 / 在线配置（`PUT /api/config`、`POST /api/admin/restart`）
- **可观测**：`GET /metrics`（Prometheus）、`GET /status`（含 ACP sessions/restarts）
- **安全**：API/WS 统一 Auth、Origin 校验、请求体上限

## 配置

- `config.yaml` + `config.local.yaml` 覆盖
- 环境变量：`CG_CURSOR_AGENT_PROFILE`、`CG_ACP_RAW_DEBUG=1`（ACP JSON-RPC 调试）
- `CURSOR_API_KEY` 设置时跳过 ACP `authenticate`

## 关键包

- `internal/acp/` — ACP JSON-RPC 协议与客户端
- `internal/runner/` — ACPGateway、进程保活与 turn 调度
- `internal/toolloop/` — ACP 事件 → IR 翻译与 permission 策略
- `internal/session/` — conversation_id ↔ sessionId LRU 池
- `internal/cursor/` — 对外 Runner 包装、profile、旧 SessionManager
- `internal/handler/` — API + `stream.go`（IR 事件驱动）
- `internal/webui/` — Store、对比、导出

## API

- `POST /v1/chat/completions` | `/v1/messages` | `/v1/responses`
- `POST /v1beta/models/*` — Gemini generateContent / streamGenerateContent
- `GET /v1/models` | `GET /metrics` | `GET /status` | `GET /healthz`
- `GET /api/traces`、`/api/traces/compare`、`/api/config`
- `POST /api/admin/restart`
- `GET /ws/events`
