# Changelog

All notable changes to this project are documented here.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
 versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

[简体中文摘要](#简体中文摘要) · [Unreleased](#unreleased) · [0.1.1](#011---2026-05-22)

## [Unreleased]

### Planned
- Docker image
- Homebrew / Scoop install scripts

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

### [0.1.1] - 2026-05-22 · 初始版本

- 支持 OpenAI / Anthropic / Gemini 四种 API 前端
- 多 Agent 后端（Cursor、Claude Code 等），ACP stdio 通信
- 多轮对话 session 复用、OpenCode client tools 翻译
- Web UI 追踪与在线配置
- 六平台预编译产物与 GitHub Release 自动发布

详细使用说明见 [docs/guide.zh-CN.md](docs/guide.zh-CN.md)。
