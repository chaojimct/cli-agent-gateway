# CLI Agent Gateway

[English](README.md) | **简体中文**

将 **OpenAI Chat**、**Anthropic Messages**、**OpenAI Responses**、**Gemini** 等 API 路由到 CLI 编程 Agent —— Cursor、Claude Code、Kimi 等 —— 底层通过 **[ACP](https://github.com/agentclientprotocol/agent-client-protocol)**（Agent Client Protocol，stdio JSON-RPC）通信。

## 核心能力

- **多协议前端** — `/v1/chat/completions`、`/v1/messages`、`/v1/responses`、Gemini `generateContent` / `streamGenerateContent`
- **多 Agent ACP 后端** — 每个 agent profile 维护一个长驻 ACP 子进程；模型 ID 格式为 `agent/model`（如 `cursor/composer-2.5-fast`、`claude/sonnet`）
- **自动发现** — 启动时 probe 本机已安装的 Agent；可通过 `agents.profiles` 覆盖
- **Tool Loop 翻译** — ACP `session/update`、`session/request_permission`、`cursor/*` 扩展 → 统一 IR → SSE
- **Session 连续性** — 显式 `X-Conversation-Id` / `metadata.conversation_id`，或从 system + 首条 user 消息自动派生；内存 ACP session 池 + 可选 `sessions.json` 持久化
- **Model Profile** — 默认 `agent_profile: model` 作为通用补全 API；`agent` profile 走完整 Agent 行为
- **Client Tools** — OpenCode 等客户端：ACP 权限请求翻译为 OpenAI `tool_calls`（`client_tools_mode`、`client_tools_agent_mode`）
- **Web UI** — 实时 trace 查看（tap 风格）、对比、导出、运行时改配置、优雅重启
- **可观测** — Prometheus（`GET /metrics`）、详细状态（`GET /status`）
- **安全** — 可选 API Key、CORS / Origin 校验、请求体大小限制

## 架构

```
客户端（OpenAI / Anthropic / Gemini SDK）
        │
        ▼
  CLI Agent Gateway  ── HTTP/SSE ──►  internal/handler
        │
        ▼
  internal/cursor.Runner  ──►  internal/runner.AgentRouter
        │
        ▼
  internal/runner.ACPGateway  ── JSON-RPC stdio ──►  ACP Agent 子进程
        │                                              (cursor-agent, claude-acp, …)
        ▼
  internal/toolloop  （ACP 事件 → IR → SSE）
```

执行路径 **仅 ACP**（v2）。旧的 stream-json / daemon 已移除；`use_daemon` 配置项仍存在但不再使用。

## 环境要求

- Go 1.23+
- 本机至少安装一个 ACP Agent（默认：[cursor-agent](https://cursor.com)）
- 对应 Agent 的凭据（如 `CURSOR_API_KEY`、`ANTHROPIC_API_KEY`，或交互式登录）

## 快速开始

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway

cp config.local.yaml.example config.local.yaml   # 可选本地覆盖

make build
make run
```

默认监听 `http://127.0.0.1:8080`。Web UI：`/`（tap 查看器），旧版紧凑 UI：`/legacy`。

### 最小配置

编辑 `config.local.yaml`（已 gitignore）：

```yaml
cursor:
  binary_path: cursor-agent          # Windows 可填完整路径或 cursor-agent.cmd
  default_model: cursor/composer-2.5-fast
  # proxy: http://127.0.0.1:7890
```

加载顺序：`config.yaml` → `config.local.yaml` → 环境变量。

### 多 Agent 示例

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

请求 Claude 模型用 `claude/sonnet`，Cursor 模型用 `cursor/composer-2.5-fast`（默认 agent 时可省略前缀）。

## API 端点

### LLM 兼容接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions（支持流式） |
| `POST` | `/v1/messages` | Anthropic Messages API |
| `POST` | `/v1/responses` | OpenAI Responses API |
| `POST` | `/v1beta/models/*` | Gemini generateContent / streamGenerateContent |
| `GET` | `/v1/models` | 聚合模型列表（`agent/model` 格式）；发现完成前返回 **503** |

### 运维与可观测

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/healthz` | 健康检查：`status`、`ready`、`version` |
| `GET` | `/status` | 运行时间、并发、各 Agent ACP 统计 |
| `GET` | `/metrics` | Prometheus 文本指标 |

### Web UI 与管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/` | Tap 风格 trace 查看器 |
| `GET` | `/legacy` | 旧版紧凑 trace UI |
| `GET` | `/ws/events` | WebSocket 实时 trace 事件 |
| `GET` | `/api/traces` | Trace 列表（支持 `q`、`endpoint`、`model`、`status` 过滤） |
| `GET` | `/api/traces/compare` | 对比两条 trace（`a`、`b`） |
| `GET` | `/api/traces/{id}` | 单条 trace |
| `GET` | `/api/traces/{id}/export` | 导出（`format=html` 或 JSON） |
| `GET` | `/api/stats` | 聚合统计 |
| `GET` | `/api/tap/events` | Tap 风格 SSE |
| `GET` | `/api/tap/records` | Tap 记录 JSON |
| `GET` | `/api/config` | 当前配置（密钥已掩码） |
| `PUT` | `/api/config` | 合并更新配置（写回 `config.yaml`） |
| `POST` | `/api/admin/restart` | 优雅重启 |

### 多轮对话

推荐显式指定 conversation id：

```http
X-Conversation-Id: my-thread-001
```

或在 metadata 中：

```json
{ "metadata": { "conversation_id": "my-thread-001" } }
```

Anthropic 还支持 `metadata.user_id`。未指定时，网关会从 system prompt 和首条 user 消息自动派生稳定 thread key。

## 配置说明

| 段 | 用途 |
|----|------|
| `server` | 监听地址、超时、CORS、请求体上限 |
| `cursor` | 默认 Agent 二进制、模型、profile/mode、并发、client-tools 行为 |
| `agents` | 多 Agent 发现、默认 agent、各 profile 的 spawn/probe/models |
| `session` | 持久化 session 存储（`sessions.json`） |
| `auth` | API Key 鉴权 |
| `logging` | 日志级别与格式（`json` / `text`） |
| `webui` | Trace 保留策略、配置 API 权限 |
| `admin` | 优雅重启超时 |

`cursor` 关键字段：

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `agent_profile` | `model` | `model` = 补全 API；`agent` = 完整 Agent |
| `agent_mode` | `ask` | 默认 cursor agent 的 `plan` / `ask` 模式 |
| `client_tools_mode` | `auto` | `auto` / `off` / `always` |
| `client_tools_agent_mode` | `ask` | 客户端带 tools 时使用的模式（OpenCode） |
| `on_tool_call` | `abort` | model profile 下 tool call 熔断 |
| `default_model` | `cursor/composer-2.5-fast` | 请求未指定 model 时使用 |

修改 `server.host/port`、`cursor.binary_path`、`logging.format` 需重启；其余大部分字段支持热更新。

## 环境变量

| 变量 | 作用 |
|------|------|
| `CURSOR_API_KEY` / `CURSOR_AUTH_TOKEN` | 跳过 Cursor ACP authenticate |
| `ANTHROPIC_API_KEY` | 跳过 Claude ACP authenticate |
| `CG_ACP_RAW_DEBUG=1` | 输出原始 ACP JSON-RPC |
| `CG_SERVER_HOST` / `CG_SERVER_PORT` | 监听地址 |
| `CG_CURSOR_BINARY_PATH` | Cursor agent 二进制路径 |
| `CG_CURSOR_DEFAULT_MODEL` | 默认 model id |
| `CG_CURSOR_MAX_CONCURRENT` | 最大并发 ACP turn 数 |
| `CG_CURSOR_PROXY` | Agent 子进程 HTTP 代理 |
| `CG_CURSOR_AGENT_PROFILE` | 覆盖 `cursor.agent_profile`（`model` / `agent`） |
| `CG_CURSOR_THINKING_VISIBILITY` | reasoning 字段暴露策略 |
| `CG_CURSOR_STREAM_PENDING_MODE` | `optimistic` / `buffer` |
| `CG_SESSION_ENABLED` / `CG_SESSION_STORAGE_PATH` | Session 存储 |
| `CG_AUTH_ENABLED` / `CG_AUTH_API_KEY` | API 鉴权 |
| `CG_LOGGING_LEVEL` | 日志级别 |

## 开发

```bash
make test              # 单元测试
make test-integration  # 集成测试（build tag）
make lint              # golangci-lint
```

CI（`.github/workflows/ci.yml`）在 Ubuntu、Windows、macOS 上运行 `go test ./...`。

## 项目结构

| 包 | 职责 |
|----|------|
| `internal/acp/` | ACP JSON-RPC 客户端与协议 |
| `internal/acpsession/` | `session/new` 辅助 |
| `internal/runner/` | `AgentRouter`、每 agent 的 `ACPGateway`、turn 调度 |
| `internal/toolloop/` | ACP 事件 → IR、permission 策略 |
| `internal/ir/` | 统一流式事件类型 |
| `internal/session/` | 内存 `conversation_id` ↔ ACP sessionId 池 |
| `internal/agent/` | 发现、注册表、模型列表、probe |
| `internal/cursor/` | 对外 `Runner` 门面、profile、`SessionManager` |
| `internal/handler/` | HTTP handler 与 SSE |
| `internal/translator/` | OpenAI / Anthropic / Gemini 格式适配 |
| `internal/webui/` | Trace 存储、内嵌 UI、管理 API |
| `internal/config/` | 配置加载、默认值、热更新 |

## 许可证

MIT
