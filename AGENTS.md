# AGENTS.md

## Global Preferences

- 总是使用中文回复。
- 遇到相关场景时，优先参考 `~/.codex/memories/` 下的经验文件。
- 涉及飞书/Lark 操作时，优先查看 `~/.codex/memories/feishu.md`。
- 涉及 `rtk` 命令或命令代理时，优先查看 `~/.codex/memories/rtk.md`。

## Project Notes

- 修改代码前先阅读相关模块与既有测试，保持改动范围尽量小。
- 搜索文件或文本时优先使用 `rg` / `rg --files`。
- 这是 Go 项目；涉及代码变更时优先运行相关 `go test`，必要时再扩大到 `go test ./...`。
