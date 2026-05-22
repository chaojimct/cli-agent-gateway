# 使用指南

[English](guide.md) | **简体中文**

CLI Agent Gateway 完整安装与接入说明。

## 目录

- [安装](#安装)
- [首次运行](#首次运行)
- [配置 Agent](#配置-agent)
- [客户端接入](#客户端接入)
- [多轮对话](#多轮对话)
- [OpenCode 与 Client Tools](#opencode-与-client-tools)
- [Web UI](#web-ui)
- [鉴权与安全](#鉴权与安全)
- [运维监控](#运维监控)
- [常见问题](#常见问题)

---

## 安装

### 从 GitHub Release 安装（推荐）

1. 打开 [Releases](https://github.com/chaojimct/cli-agent-gateway/releases)
2. 下载对应平台的压缩包
3. 校验（可选）：

```bash
sha256sum -c SHA256SUMS.txt
```

4. 解压，将 `config.yaml` 放在可执行文件同目录

| 平台 | 压缩包 | 可执行文件 |
|------|--------|------------|
| Linux x64 | `*_linux-amd64.tar.gz` | `cli-agent-gateway` |
| Linux ARM64 | `*_linux-arm64.tar.gz` | `cli-agent-gateway` |
| Windows x64 | `*_windows-amd64.zip` | `cli-agent-gateway.exe` |
| Windows ARM64 | `*_windows-arm64.zip` | `cli-agent-gateway.exe` |
| macOS Intel | `*_darwin-amd64.tar.gz` | `cli-agent-gateway` |
| macOS Apple Silicon | `*_darwin-arm64.tar.gz` | `cli-agent-gateway` |

### 从 npm 安装（Node 18+）

```bash
npm install -g cli-agent-gateway
```

安装脚本会从 GitHub Release 下载与 npm 包版本一致的平台二进制（如 `v0.1.1`）。开发时可跳过下载并使用本地构建：

```bash
set CG_SKIP_BINARY_INSTALL=1
set CG_BINARY_PATH=C:\path\to\bin\cli-agent-gateway.exe
npm install -g ./packages/cli-agent-gateway
```

详见 [packages/cli-agent-gateway/README.md](../packages/cli-agent-gateway/README.md)。

### 从源码编译

```bash
git clone https://github.com/chaojimct/cli-agent-gateway.git
cd cli-agent-gateway
make build
```

需要 **Go 1.23+** 及本机 ACP Agent。

---

## 首次运行

### 1. 安装 ACP Agent

至少安装 **cursor-agent**（[Cursor CLI](https://cursor.com)）：

```bash
cursor-agent --version
```

或使用 Claude Code（需 Node.js）：

```bash
npx -y @agentclientprotocol/claude-agent-acp --help
```

### 2. 配置凭据

```bash
# Cursor — 跳过交互式登录
set CURSOR_API_KEY=your-key          # Windows
export CURSOR_API_KEY="your-key"     # Linux/macOS

# Claude ACP
export ANTHROPIC_API_KEY="your-key"
```

### 3. 本地配置（可选）

```bash
cp config.local.yaml.example config.local.yaml
```

编辑 `config.local.yaml`：

```yaml
cursor:
  binary_path: cursor-agent.cmd      # 或完整路径
  default_model: cursor/composer-2.5-fast
  workspace: C:\path\to\your\project  # Agent 工作目录
  # proxy: http://127.0.0.1:7890
```

### 4. 启动网关

```bash
cli-agent-gateway.exe    # Windows：自动解析配置路径
./cli-agent-gateway      # Linux/macOS
# 也可显式指定：-config /path/to/config.yaml
```

日志出现 `starting cli-agent-gateway`，默认监听 `127.0.0.1:8080`。

### 5. 验证

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/v1/models
```

浏览器打开 **http://127.0.0.1:8080/** 查看 Web UI。

---

## 配置 Agent

### 默认（仅 Cursor）

自带 `config.yaml` 在 `cursor-agent` 于 PATH 中即可运行，模型 ID 为 `cursor/*`。

### 多 Agent

在 `config.local.yaml` 中添加：

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

`GET /v1/models` 返回 `cursor/composer-2.5-fast`、`claude/sonnet` 等。

### 常用 `cursor` 配置

| 字段 | 说明 |
|------|------|
| `agent_profile: model` | 默认，通用补全 API（推荐） |
| `agent_profile: agent` | 完整 Agent 行为 |
| `agent_mode: plan` | 允许执行修改（相对只读 `ask`） |
| `client_tools_agent_mode: plan` | **OpenCode** 场景用这个，不是 `agent_mode` |
| `workspace` | Agent 操作的项目根目录 |
| `max_concurrent: 8` | 最大并发 ACP turn |

**配置文件位置**（未指定 `-config` 或值为默认 `config.yaml` 时）：

1. 当前工作目录下的 `config.yaml`（存在则优先，适合项目目录或 Release 解压目录）
2. 否则使用用户配置目录（首次启动自动创建并写入默认文件）：
   - Linux/macOS：`~/.config/cli-agent-gateway/`
   - Windows：`%AppData%\cli-agent-gateway\`
   - 可用 `CG_CONFIG_HOME` 覆盖整个目录路径

同目录内加载顺序：`config.yaml` → `config.local.yaml` → `CG_*` 环境变量。用户目录下的 `sessions.json` 路径在初始化时写为绝对路径。

---

## 客户端接入

将任意 OpenAI 兼容客户端的 Base URL 指向 `http://127.0.0.1:8080/v1`。

### curl

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"cursor/composer-2.5-fast\",\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}],\"stream\":true}"
```

PowerShell 可用：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/v1/chat/completions -Method POST `
  -ContentType "application/json" `
  -Body '{"model":"cursor/composer-2.5-fast","messages":[{"role":"user","content":"你好"}]}'
```

### Python（OpenAI SDK）

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8080/v1",
    api_key="not-needed",
)

resp = client.chat.completions.create(
    model="cursor/composer-2.5-fast",
    messages=[{"role": "user", "content": "用一句话解释 ACP"}],
)
print(resp.choices[0].message.content)
```

### Anthropic SDK

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://127.0.0.1:8080",
    api_key="not-needed",
)

msg = client.messages.create(
    model="claude/sonnet",
    max_tokens=1024,
    messages=[{"role": "user", "content": "你好"}],
)
print(msg.content[0].text)
```

### Continue / IDE 插件

| 设置 | 值 |
|------|-----|
| 提供商 | OpenAI 兼容 |
| Base URL | `http://127.0.0.1:8080/v1` |
| 模型 | `cursor/composer-2.5-fast` |
| API Key | 任意字符串（或配置的 `auth.api_key`） |

### 环境变量（快速测试）

```bash
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
export OPENAI_API_KEY=local
```

---

## 多轮对话

相同 conversation id 会复用 ACP session，仅发送增量 prompt。

**HTTP Header（通用）：**

```http
X-Conversation-Id: feature-auth-module
```

**OpenAI metadata：**

```json
{
  "model": "cursor/composer-2.5-fast",
  "metadata": { "conversation_id": "feature-auth-module" },
  "messages": []
}
```

**Anthropic** 还支持 `metadata.user_id`。

未指定时，会从 system prompt + 首条 user 消息自动派生 id（适合单会话脚本，不适合并行多聊）。

---

## OpenCode 与 Client Tools

带 `tools` 的客户端（OpenCode 等）：

1. 保持 `client_tools_mode: auto`（默认）
2. 设置 `client_tools_agent_mode: plan`，让 Agent 能执行工具
3. 带 tools 的请求**不要**指望 `agent_mode`，该字段只影响普通补全

网关将 ACP `session/request_permission` 译为 OpenAI `tool_calls`，下一轮请求需带回 tool result。

---

## Web UI

| 地址 | 功能 |
|------|------|
| `/` | Tap 风格实时 trace |
| `/legacy` | 旧版紧凑 UI |
| `/api/traces` | Trace 列表 JSON |
| `/api/config` | 读取运行时配置 |

可查看 prompt、tool call、token 用量、SSE 事件。Trace 存于内存（默认最多 2000 条）。

运行时改配置：

```bash
curl -X PUT http://127.0.0.1:8080/api/config \
  -H "Content-Type: application/json" \
  -d '{"logging": {"level": "debug"}}'
```

优雅重启：

```bash
curl -X POST http://127.0.0.1:8080/api/admin/restart
```

---

## 鉴权与安全

对外暴露时启用 API Key：

```yaml
auth:
  enabled: true
  api_key: "your-secret-key"

server:
  host: 0.0.0.0
  allowed_origins:
    - "https://your-app.example.com"
```

客户端请求头：

```http
Authorization: Bearer your-secret-key
```

**默认仅本机访问**（`127.0.0.1`，无鉴权）。切勿将未鉴权实例暴露到公网。

---

## 运维监控

| 端点 | 用途 |
|------|------|
| `GET /healthz` | 存活探测，`ready: true` 表示 Agent 就绪 |
| `GET /status` | 各 Agent 统计、运行时间、并发 |
| `GET /metrics` | Prometheus 指标 |

调试 ACP 原始 JSON-RPC：

```bash
set CG_ACP_RAW_DEBUG=1
cli-agent-gateway.exe -config config.yaml
```

查看版本：

```bash
cli-agent-gateway.exe -version
```

---

## 常见问题

### `GET /v1/models` 返回 503

Agent 仍在 probe，等几秒重试。查看 `/status` 中 `acp.agents`。

### 找不到 `cursor-agent`

在 `config.local.yaml` 指定完整路径：

```yaml
cursor:
  binary_path: C:\Users\you\AppData\Local\cursor-agent\cursor-agent.cmd
```

修改 `binary_path` 后需**重启**网关。

### 认证 / 代理失败

- 设置 `CURSOR_API_KEY`，或先交互式登录一次 `cursor-agent`
- 需要代理时配置 `cursor.proxy`

### model profile 下 tool call 被拒绝

`agent_profile: model` 且 `on_tool_call: abort` 时会熔断。可改用 client tools 流程，或切换 `agent_profile: agent`。

### 多轮对话上下文丢失

确保每次请求携带相同的 `X-Conversation-Id` 或 `metadata.conversation_id`。

### Windows 防火墙

局域网访问需放行 8080 端口。

---

## 相关链接

- [README](../README.zh-CN.md) — 概览与 API 参考
- [CHANGELOG](../CHANGELOG.md) — 版本历史
- [config.local.yaml.example](../config.local.yaml.example) — 配置模板
