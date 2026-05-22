# cli-agent-gateway (npm)

npm 分发包：安装时从 [GitHub Releases](https://github.com/chaojimct/cli-agent-gateway/releases) 下载对应平台的 Go 二进制，并提供 `cli-agent-gateway` / `cag` 命令。

完整文档见仓库根目录 [README.zh-CN.md](https://github.com/chaojimct/cli-agent-gateway/blob/main/README.zh-CN.md)。

## 安装

```bash
npm install -g cli-agent-gateway
```

或一次性运行：

```bash
npx cli-agent-gateway init
npx cli-agent-gateway
```

**注意**：本包只安装网关，不包含 `cursor-agent`。请先安装 Cursor CLI 或配置 `config.local.yaml` 中的 `cursor.binary_path`。

## 命令

| 命令 | 说明 |
|------|------|
| `cli-agent-gateway` | 启动 HTTP 网关（默认 127.0.0.1:8080） |
| `cli-agent-gateway init` | 初始化用户配置目录 |
| `cli-agent-gateway doctor` | 检查二进制与 cursor-agent |
| `cag` | 同上（短别名） |

## 配置

npm 启动器会自动传入 `-config`：

1. 当前工作目录存在 `config.yaml` → 使用该文件
2. 否则使用用户配置目录（`init` 或首次启动时自动创建）

与 Go 二进制内置解析逻辑一致；旧版 Release 二进制也通过 npm 层兼容。

- Linux/macOS：`~/.config/cli-agent-gateway/`
- Windows：`%AppData%\cli-agent-gateway\`

## 环境变量

| 变量 | 说明 |
|------|------|
| `CG_BINARY_PATH` | 使用本地二进制，跳过 vendor 下载 |
| `CG_SKIP_BINARY_INSTALL` | `postinstall` 不下载二进制 |
| `CG_BINARY_VERSION` | 指定 Release 标签（如 `v0.1.1`） |
| `CG_GITHUB_REPO` | 默认 `chaojimct/cli-agent-gateway` |
| `CG_CONFIG_HOME` | 覆盖用户配置目录 |

## 发布（维护者）

### 配置 GitHub Secret（首次必做）

CI 报错 `ENEEDAUTH` 表示 **未配置或配错了 `NPM_TOKEN`**。

1. 打开 https://www.npmjs.com → 右上角头像 → **Access Tokens** → **Generate New Token**
2. 选 **Granular Access Token**（npm 新版已取消单独的 “Automation” 类型）
3. **Packages**：勾选 `cli-agent-gateway`（或 All packages）
4. **Permissions**：**Read and write**
5. 若账号开了 2FA：勾选 **Bypass two-factor authentication**（CI 发布必须，否则 Actions 无法 publish）
6. 设置过期时间 → **Generate Token** → 复制 token（只显示一次）
7. GitHub 仓库 → **Settings** → **Secrets and variables** → **Actions** → **New repository secret**
   - Name 必须为：`NPM_TOKEN`（大小写一致）
   - Value：粘贴 npm token

配置好后：**每次推送 `v*` tag**，`build` 工作流会创建 GitHub Release，并在同一运行末尾 **`npm publish`**，无需单独点 Actions。

npm 版本需与 Release tag 一致（如 `v0.1.3` → 包版本 `0.1.3`）。

若二进制已就绪但只有一次 npm 失败，可用 Actions → **npm-publish-manual**，输入与 tag 一致的版本号做补救。

本地打包检查：

```bash
cd packages/cli-agent-gateway
npm pack
```
