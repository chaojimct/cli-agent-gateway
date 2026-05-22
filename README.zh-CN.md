# Cursor Gateway

[English](README.md) | **简体中文**

Go 语言编写的 API 网关，将 OpenAI Chat / Anthropic Messages / OpenAI Responses / Gemini 等主流客户端协议，统一转发到 **[Cursor Agent ACP](https://github.com/cursor/agent-client-protocol)**（Agent Client Protocol，基于 stdio 的 JSON-RPC）。把你的 OpenAI / Anthropic / Gemini 兼容客户端指向本网关，即可通过 `cursor-agent acp` 运行请求。

## 核心能力

- **多协议 API** — OpenAI Chat Completions、Anthropic Messages、OpenAI Responses、Gemini `generateContent` / `streamGenerateContent`
- **ACP 主干** — 长驻 `cursor-agent acp` 进程，支持 session 复用与 turn 调度
- **Tool Loop 翻译** — 将 ACP `session/update` 事件与 `cursor/*` 扩展译为统一 IR，再以 SSE 流式输出
- **Session 复用** — `X-Conversation-Id` 或 `metadata.conversation_id` 映射到 ACP session，多轮对话仅发送增量 prompt
- **Model Profile** — 默认 `agent_profile: model` 提供通用文本补全；支持 plan/ask 模式、隔离 workspace、tool_call 熔断
- **Client Tools** — OpenCode 等客户端：ACP `session/request_permission` → OpenAI `tool_calls` 翻译
- **多 Agent 路由** — 可选 agent profile（Cursor、Claude ACP、Kimi 等），支持自动发现
- **Web UI** — 请求追踪、对比、在线配置（`PUT /api/config`）、优雅重启
- **可观测** — Prometheus 指标（`GET /metrics`）、含 ACP session 统计的状态页（`GET /status`）
- **安全** — API/WS 统一鉴权、Origin 校验、请求体大小限制

## 架构

```
客户端（OpenAI / Anthropic / Gemini SDK）
        │
        ▼
  cursor-gateway  ── HTTP/SSE ──►  internal/handler
        │
        ▼
  internal/runner  ── JSON-RPC stdio ──►  cursor-agent acp
        │
        ▼
  internal/toolloop （ACP 事件 → IR → SSE）
```

## 环境要求

- Go 1.23+
- 已安装 [cursor-agent](https://cursor.com) 并在 `PATH` 中，或通过 `cursor.binary_path` 配置
- Cursor 账号（或设置 `CURSOR_API_KEY` 跳过 ACP `authenticate`）

## 快速开始

```bash
# 克隆
git clone https://github.com/chaojimct/cursor-gateway.git
cd cursor-gateway

# 复制本地配置（可选覆盖项）
cp config.local.yaml.example config.local.yaml

# 构建并运行
make build
make run
```

默认监听 `http://127.0.0.1:8080`，浏览器访问 `/` 打开 Web UI。

### 最小配置

编辑 `config.local.yaml`（不会提交到 Git）：

```yaml
cursor:
  binary_path: C:\path\to\cursor-agent.cmd   # Linux/macOS 填 cursor-agent 路径
  default_model: composer-2.5-fast
  # proxy: http://127.0.0.1:7890             # 可选代理
```

配置加载顺序：`config.yaml` → `config.local.yaml`（同键后者覆盖）。

## API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions（支持流式） |
| `POST` | `/v1/messages` | Anthropic Messages API |
| `POST` | `/v1/responses` | OpenAI Responses API |
| `POST` | `/v1beta/models/*` | Gemini generateContent / streamGenerateContent |
| `GET` | `/v1/models` | 列出可用模型 |
| `GET` | `/healthz` | 健康检查 |
| `GET` | `/status` | 网关与 ACP session 状态 |
| `GET` | `/metrics` | Prometheus 指标 |
| `GET` | `/api/traces` | 追踪列表（Web UI） |
| `GET` | `/api/config` | 当前配置 |
| `PUT` | `/api/config` | 运行时更新配置 |
| `POST` | `/api/admin/restart` | 优雅重启 |
| `GET` | `/ws/events` | WebSocket 事件流 |

### 多轮对话

在请求中携带相同的 conversation id：

```http
X-Conversation-Id: my-thread-001
```

或在 metadata 中：

```json
{ "metadata": { "conversation_id": "my-thread-001" } }
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `CURSOR_API_KEY` | 设置后跳过 ACP 交互式 authenticate |
| `CG_CURSOR_AGENT_PROFILE` | 覆盖默认 cursor agent profile |
| `CG_ACP_RAW_DEBUG=1` | 输出原始 ACP JSON-RPC 便于调试 |

## 开发

```bash
make test              # 单元测试
make test-integration  # 集成测试（带 tag）
make lint              # golangci-lint
```

CI 在 Ubuntu、Windows、macOS 上运行 `go test ./...`。

## 项目结构

| 包 | 职责 |
|----|------|
| `internal/acp/` | ACP JSON-RPC 协议与客户端 |
| `internal/runner/` | ACP 网关、进程保活、turn 调度 |
| `internal/toolloop/` | ACP 事件 → IR 翻译与 permission 策略 |
| `internal/session/` | `conversation_id` ↔ sessionId LRU 池 |
| `internal/cursor/` | Runner 包装、profile、SessionManager |
| `internal/handler/` | HTTP handler 与 SSE 流式输出 |
| `internal/webui/` | 追踪存储、对比、导出、内嵌 UI |
| `internal/agent/` | 多 Agent 注册与发现 |
| `internal/translator/` | 请求/响应格式翻译 |

## 许可证

MIT
