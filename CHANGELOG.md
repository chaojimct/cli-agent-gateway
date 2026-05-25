# Changelog

All notable changes to this project are documented here.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
 versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

[简体中文摘要](#简体中文摘要) · [Unreleased](#unreleased) · [0.1.5](#015---2026-05-25) · [0.1.1](#011---2026-05-22)

## [Unreleased]

### Planned
- Docker image
- Homebrew / Scoop install scripts

---

## [0.1.5] - 2026-05-25

### Added
- **Startup banner** — Clean service info panel (URLs, agents, config) replaces raw JSON logs on startup
- **First-run wizard** — Interactive setup on first launch: port, proxy, model, agent mode, auth
- **`--debug` flag** — Enables detailed JSON log output; default is quiet (errors only)

### Changed
- Agent discovery and router logs downgraded to DEBUG level

[0.1.5]: https://github.com/chaojimct/cli-agent-gateway/releases/tag/v0.1.5

---

## [0.1.4] - 2026-05-25

### Fixed
- **Cursor agent auto-discovery** — Probe now skips `authenticate` step; the 4 s timeout was exceeded because `cursor_login` requires a network round-trip to Cursor backend. Authentication still happens lazily on first real request.

### Changed
- Default model changed to `auto` (agent-native default) in example configs

[0.1.4]: https://github.com/chaojimct/cli-agent-gateway/releases/tag/v0.1.4

---

## [0.1.2] - 2026-05-22

### Added
- **npm package** — `cli-agent-gateway` on npm; postinstall downloads platform binary from GitHub Releases
- **Config resolution** — Prefer `./config.yaml` in cwd, else auto-init user config dir (`~/.config/cli-agent-gateway` or `%AppData%\cli-agent-gateway`)
- **CLI subcommands** — `init`, `doctor` on the Go binary

### Changed
- Default startup no longer requires `-config` when user or project config is discoverable

[0.1.2]: https://github.com/chaojimct/cli-agent-gateway/releases/tag/v0.1.2

---

## [0.1.1] - 2026-05-22

First public release.

### Added
- **Multi-protocol API** — OpenAI Chat, Anthropic Messages, OpenAI Responses, Gemini
- **Multi-agent ACP backend** — Cursor (`cursor-agent acp`) and Claude Code (`claude-agent-acp`) with auto-discovery; custom profiles for Kimi and others
- **Model routing** — `agent/model` IDs (e.g. `cursor/composer-2.5-fast`, `claude/sonnet`)
- **Session continuity** — `X-Conversation-Id`, `metadata.conversation_id`, auto-derived thread keys; ACP session pool + optional `sessions.json`
- **Client tools** — OpenCode-style tool loops via ACP permission → OpenAI `tool_calls`
- **Web UI** — Live trace viewer, diff/compare, export, runtime config, graceful restart
- **Observability** — `/healthz`, `/status`, Prometheus `/metrics`
- **Security** — Optional API key auth, CORS, request body limits
- **CI/CD** — Cross-compile 6 platforms; GitHub Releases with `SHA256SUMS.txt` on `v*` tags
- **Documentation** — Bilingual README, usage guide, changelog

### Notes
- Execution path is **ACP-only** (v2). Legacy stream-json / daemon modes are removed.
- Default profile: `agent_profile: model` for drop-in text completion APIs.

[0.1.1]: https://github.com/chaojimct/cli-agent-gateway/releases/tag/v0.1.1

---

## 简体中文摘要

### [0.1.5] - 2026-05-25

- 启动 Banner：美观的服务信息面板替代原始 JSON 日志
- 首次运行引导：端口、代理、模型、认证等交互式配置
- `--debug` 标志：开启详细日志；默认安静模式

### [0.1.4] - 2026-05-25

- 修复 Cursor agent 自动发现：probe 跳过 authenticate，避免网络超时导致发现失败
- 默认模型改为 `auto`

### [0.1.2] - 2026-05-22

- npm 安装：`npm install -g cli-agent-gateway`
- 配置：当前目录 `config.yaml` 优先，否则自动初始化用户配置目录
- `init` / `doctor` 子命令

### [0.1.1] - 2026-05-22 · 初始版本

- 支持 OpenAI / Anthropic / Gemini 四种 API 前端
- 多 Agent 后端（Cursor、Claude Code 等），ACP stdio 通信
- 多轮对话 session 复用、OpenCode client tools 翻译
- Web UI 追踪与在线配置
- 六平台预编译产物与 GitHub Release 自动发布

详细使用说明见 [docs/guide.zh-CN.md](docs/guide.zh-CN.md)。
