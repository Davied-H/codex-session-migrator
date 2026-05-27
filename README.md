# Codex Session Migrator

Codex Desktop 会话 provider 迁移工具。默认读取 `~/.codex`，支持按 provider / 项目浏览会话，执行 dry-run、snapshot、retag、clone、删除和 rollback。

它只修改 Codex Desktop 的本地会话元数据和 rollout 文件，不读取或迁移 `auth.json`，也不处理 API key。

## 安装

从 GitHub Release 一键安装，不需要本机安装 Go：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/codex-session-migrator/main/scripts/install-release.sh | sh
```

默认安装到 `~/.local/bin/codex-session-migrator`。指定版本或目录：

```bash
curl -fsSL https://raw.githubusercontent.com/Davied-H/codex-session-migrator/main/scripts/install-release.sh | CSM_VERSION=v0.0.1 sh
curl -fsSL https://raw.githubusercontent.com/Davied-H/codex-session-migrator/main/scripts/install-release.sh | INSTALL_DIR=/usr/local/bin sh
```

源码安装：

```bash
git clone https://github.com/Davied-H/codex-session-migrator.git
cd codex-session-migrator
./scripts/install.sh
```

## 快速使用

```bash
codex-session-migrator
codex-session-migrator -from openai -to sub2api -mode retag -dry-run
codex-session-migrator -ids <thread-id> -to sub2api -mode clone -dry-run
codex-session-migrator -rollback <snapshot-dir-or-name>
```

常用键：

- `Tab` / `p` / `g`：切换 Providers、Projects、Sessions 焦点
- `j/k` / `↑/↓` / `PgUp/PgDn`：移动
- `mouse wheel`：只滚动当前面板，不改变高亮行
- `Enter`：进入项目或打开会话 Markdown
- `Space`：选择当前 session
- `a`：在 Sessions 中选择/取消当前项目过滤结果；在其他面板切换 archived 显示
- `/`：搜索标题和对话内容
- `o`：设置
- `Ctrl+E`：演示模式，隐藏真实项目名和真实会话，改用 mock 数据
- `e` / `t`：选择或切换目标 provider
- `c`：切换 `retag` / `clone`
- `d`：dry-run
- `m`：确认并执行迁移；焦点在 Projects 时迁移/克隆当前项目下全部会话
- `x`：按当前焦点删除 provider / project / session
- `r`：rollback
- `q` / `Esc`：退出或返回

## 模式

`retag` 会保留原 thread id，只把会话 provider 改到目标 provider：

- 更新 `state_5.sqlite`
- 更新对应 rollout 首行 `session_meta.model_provider`
- apply 前自动创建 snapshot

`clone` 会复制出新的 thread id：

- 生成新 rollout 文件
- 写入 SQLite、`session_index.jsonl` 和 `.codex-global-state.json`
- 保留项目归属
- apply 时要求 Codex Desktop 已退出；运行中只允许 dry-run

## 项目分组

TUI 会在 provider 和 session 之间显示项目分组：

- `全部项目`：当前 provider 和过滤条件下的全部会话
- `普通对话`：来自 `.codex-global-state.json` 的 projectless 会话
- 其他项目：使用 `.codex-global-state.json` 中保存的 workspace roots

Sessions 列表按日期分组，今天显示为 `Today`，会话行缩进显示时间和标题。

## 发布资产

Release 会生成以下文件：

- `codex-session-migrator_<version>_darwin_amd64.tar.gz`
- `codex-session-migrator_<version>_darwin_arm64.tar.gz`
- `codex-session-migrator_<version>_linux_amd64.tar.gz`
- `codex-session-migrator_<version>_linux_arm64.tar.gz`
- `codex-session-migrator_<version>_windows_amd64.zip`
- `checksums.txt`

打 tag 前跑：

```bash
sh scripts/preflight-release.sh
```

发布 `v0.0.1`：

```bash
git tag v0.0.1
git push origin main --tags
```
