# CLI Agent Gateway

[![Release](https://img.shields.io/github/v/release/chaojimct/cli-agent-gateway?label=release)](https://github.com/chaojimct/cli-agent-gateway/releases)
[![CI](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/ci.yml)
[![Build](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/build.yml/badge.svg)](https://github.com/chaojimct/cli-agent-gateway/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | **简体中文**

将 **OpenAI Chat**、**Anthropic Messages**、**OpenAI Responses**、**Gemini** 等 API 路由到 CLI 编程 Agent —— Cursor、Claude Code、Kimi 等 —— 底层通过 **[ACP](https://github.com/agentclientprotocol/agent-client-protocol)**（Agent Client Protocol，stdio JSON-RPC）通信。

## 文档

| 文档 | 说明 |
|------|------|
| [**使用指南**](docs/guide.zh-CN.md) · [English](docs/guide.md) | 安装、配置、客户端接入、故障排查 |
| [**CHANGELOG**](CHANGELOG.md) | 版本更新记录 |
| [config.local.yaml.example](config.local.yaml.example) | 本地配置模板 |

## 核心能力

- **多协议前端** — OpenAI / Anthropic / Gemini 四种 API
- **多 Agent 后端** — 每个 profile 一个长驻 ACP 子进程；模型 ID 格式 `agent/model`
- **自动发现** — 启动时 probe 本机 Agent，支持 `agents.profiles` 扩展
- **Tool Loop** — ACP 事件 → IR → SSE；OpenCode client tools 翻译
- **Session 复用** — `X-Conversation-Id` / `metadata.conversation_id` 多轮对话
- **Web UI** — 实时 trace、对比、导出、在线改配置
- **可观测** — Prometheus、`/status` 各 Agent 统计

## 快速开始

### 方式 A — 下载 Release（推荐）

1. 从 **[Releases](https://github.com/chaojimct/cli-agent-gateway/releases)** 下载最新版（当前 **v0.1.2**）
2. 解压对应平台压缩包
3. 设置 `CURSOR_API_KEY`（或先运行 `cursor-agent` 完成登录）
4. 启动：

```bash
cli-agent-gateway.exe    # Windows（自动：当前目录 config.yaml，否则用户配置目录）
./cli-agent-gateway      # Linux/macOS
```

5. 验证：`curl http://127.0.0.1:8080/healthz` · Web UI：http://127.0.0.1:8080/

完整步骤见 [**使用指南**](docs/guide.zh-CN.md)。

### 方式 B — npm（Node 18+）

```bash
npm install -g cli-agent-gateway
cli-agent-gateway init
cli-agent-gateway
```

或：`npx cli-agent-gateway`。安装时会从 GitHub Release 拉取当前平台二进制；需已发布对应版本的 Release（与 npm 包版本一致）。详见 [packages/cli-agent-gateway/README.md](packages/cli-agent-gateway/README.md)。

### 方式 C — 源码编译

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway
cp config.local.yaml.example config.local.yaml
make build && make run
```

需要 Go 1.23+ 及本机 [cursor-agent](https://cursor.com)。

## 最小示例

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"cursor/composer-2.5-fast\",\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}"
```

Python（OpenAI SDK）：

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:8080/v1", api_key="local")
print(client.chat.completions.create(
    model="cursor/composer-2.5-fast",
    messages=[{"role": "user", "content": "你好"}],
).choices[0].message.content)
```

## 架构

```
客户端（OpenAI / Anthropic / Gemini SDK）
        │
        ▼
  CLI Agent Gateway  ── HTTP/SSE ──►  handler
        │
        ▼
  AgentRouter  ── JSON-RPC stdio ──►  ACP Agent（cursor-agent, claude-acp, …）
        │
        ▼
  toolloop  （ACP 事件 → IR → SSE）
```

## API 概览

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/v1/chat/completions` | OpenAI Chat（流式） |
| `POST` | `/v1/messages` | Anthropic Messages |
| `POST` | `/v1/responses` | OpenAI Responses |
| `POST` | `/v1beta/models/*` | Gemini |
| `GET` | `/v1/models` | 模型列表 |
| `GET` | `/healthz` · `/status` · `/metrics` | 健康检查、状态、Prometheus |
| `GET` | `/` | Web UI |

完整 API 与配置说明：[**使用指南 →**](docs/guide.zh-CN.md)

## 配置

加载顺序：`config.yaml` → `config.local.yaml` → `CG_*` 环境变量。

```yaml
# config.local.yaml
cursor:
  binary_path: cursor-agent.cmd
  default_model: cursor/composer-2.5-fast
  workspace: C:\path\to\project
  client_tools_agent_mode: plan   # OpenCode 场景
```

| 配置段 | 用途 |
|--------|------|
| `cursor` | 默认 Agent、模型、模式、并发 |
| `agents` | 多 Agent 发现与 profile |
| `auth` | API Key 鉴权 |
| `session` | `sessions.json` 持久化 |
| `webui` | Trace 与管理 API |

## 发布与开发

**Linux / Windows / macOS**（amd64 & arm64）预编译包发布在 **[Releases](https://github.com/chaojimct/cli-agent-gateway/releases)**，含 `SHA256SUMS.txt` 校验文件。

```bash
make test        # 单元测试
make build-all   # 本地交叉编译 → dist/
```

推送 `v*` 标签自动发版（如 `v0.1.2`）。发布 GitHub Release 后，若配置了仓库 Secret `NPM_TOKEN`，会同步发布 npm 包 `cli-agent-gateway`。

## 项目结构

| 包 | 职责 |
|----|------|
| `internal/acp/` | ACP JSON-RPC 客户端 |
| `internal/runner/` | AgentRouter、ACPGateway、turn 调度 |
| `internal/agent/` | 发现、注册表、模型列表 |
| `internal/toolloop/` | ACP 事件 → IR 翻译 |
| `internal/handler/` | HTTP handler 与 SSE |
| `internal/webui/` | Trace 存储与内嵌 UI |

## 许可证

[MIT](LICENSE)
