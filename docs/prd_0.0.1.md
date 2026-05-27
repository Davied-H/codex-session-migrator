# Codex Session Migrator TUI 设计

## 背景

Codex 会话在切换认证方式或 cc switch provider 后可能不可见。验证结果显示：

- `state_5.sqlite.threads.model_provider` 是关键过滤字段。
- 原 thread id 已存在于 Codex Desktop 的全局索引时，直接把原会话重标记到目标 provider 可以生效。
- 复制成新 thread id 时，仅写 SQLite 和 rollout 不够；Codex Desktop 还依赖 `.codex-global-state.json` 里的顶层索引。
- Codex Desktop 运行中会把内存中的 `.codex-global-state.json` 覆盖回磁盘，因此涉及全局状态文件的复制/补索引操作必须要求 Codex 退出后执行。

所以 TUI 的 MVP 应同时支持 `retag` 和 `clone`，但默认仍推荐 `retag`。`clone` 和批量迁移必须带 dry-run、快照、二次确认，并在需要写 `.codex-global-state.json` 时强制要求 Codex Desktop 已退出。

## 工具定位

工具名：`codex-session-migrator`

目标：

- 查出不同 provider 下的会话数量。
- 让用户选择单个或批量会话。
- 把会话从 `openai` 迁移到 `sub2api` / `custom` 等目标 provider。
- 自动快照，支持回滚。
- 避免打印或读取敏感 token。

非目标：

- 不迁移 `auth.json`。
- 不修改 API key。
- 不尝试伪造 ChatGPT 登录态。
- 不默认创建新 thread id 副本。

## 信息架构

主界面分 4 个区域：

```text
┌─ Codex Session Migrator ─────────────────────────────────────────────┐
│ Current: sub2api        Codex: running        DB: ok                 │
├─ Providers ───────────┬─ Sessions ──────────────────────────────────┤
│ > openai      1166    │ [ ] 今天 codex 又更新什么了？                │
│   sub2api        6    │ [ ] 设计 Codex 会话迁移工具                  │
│   custom       361    │ [ ] 比较 GEO 与 SEO 优化                     │
│                       │                                              │
├─ Detail ──────────────┴──────────────────────────────────────────────┤
│ id: 019e6717-...                                                     │
│ cwd: /Users/dong/Documents/Codex                                     │
│ rollout: exists                                                      │
│ updated: 2026-05-27 09:41                                            │
├─ Actions ────────────────────────────────────────────────────────────┤
│ Space select  / search  m migrate  d dry-run  b backup  r rollback  │
└──────────────────────────────────────────────────────────────────────┘
```

## 核心流程

### 1. 启动诊断

启动后先只读检查：

- `~/.codex/state_5.sqlite` 是否存在。
- `threads` 表 schema 是否包含 `model_provider`。
- `~/.codex/session_index.jsonl` 是否存在。
- `~/.codex/.codex-global-state.json` 是否存在。
- Codex Desktop 是否正在运行。
- provider 计数：

```sql
select model_provider, archived, count(*)
from threads
group by model_provider, archived;
```

状态展示：

- `DB: ok`
- `Codex: running` 或 `Codex: stopped`
- `Writable: yes/no`

### 2. 浏览会话

左侧选择 provider，右侧列出该 provider 下会话。

默认过滤：

- `archived = 0`
- `source = vscode`
- `thread_source = user`

可切换：

- 显示 archived
- 显示 subagent
- 按 cwd 过滤
- 按标题搜索
- 按更新时间排序

### 3. 迁移确认

按 `m` 后弹出确认框：

```text
Migrate selected sessions

From: openai
To:   sub2api
Mode: retag original thread id

This will update:
- state_5.sqlite threads.model_provider
- rollout session_meta.model_provider

Snapshot will be created before changes.

[Dry Run] [Apply] [Cancel]
```

默认选中 `retag`，但 MVP 也提供 `clone`。

`clone` 必须二次确认：

```text
Clone mode is experimental.
Codex must be stopped because global state index must be updated.
```

批量迁移时确认框必须展示汇总：

```text
Batch migrate selected sessions

From: openai
To:   sub2api
Mode: clone

Selected: 18 sessions
Archived: 2 included
Subagents: excluded

Writes:
- state_5.sqlite
- session_index.jsonl
- rollout copies
- .codex-global-state.json

Codex Desktop must be stopped.

[Dry Run] [Apply] [Cancel]
```

### 4. 执行迁移

`retag` 步骤：

1. 创建快照目录：

```text
~/.codex/session-migrate-snapshots/YYYYMMDD-HHMMSS-retag-<thread-id>-to-<provider>/
```

2. 备份：

- `state_5.sqlite`
- `state_5.sqlite-wal`
- `state_5.sqlite-shm`
- 对应 `rollout-*.jsonl`

3. 更新 SQLite：

```sql
update threads
set model_provider = :target_provider
where id = :thread_id;
```

4. 更新 rollout 首行：

```json
{"type":"session_meta","payload":{"model_provider":"sub2api"}}
```

5. 校验：

- DB 中目标 thread provider 是否已变更。
- rollout 首行 provider 是否已变更。
- rollout 文件是否存在。
- `pragma integrity_check` 是否为 `ok`。

### 5. 回滚

回滚界面列出 snapshot：

```text
20260527-100711-retag-019e6717...-to-sub2api
20260527-100119-copy-019e6717...-to-sub2api
```

选择后恢复备份文件。

回滚前要求：

- Codex Desktop 已退出。
- 当前 DB 可读。

## 操作模式

### Safe Retag

默认模式。

适用场景：

- 旧会话已经在 Codex UI 的全局索引里。
- 只是切换 provider 后不可见。
- 目标是“移动过去”，不要求保留旧 provider 下同一条可见记录。

优点：

- 实测可行。
- 不需要生成新 thread id。
- 不依赖 `.codex-global-state.json` 增补索引。

风险：

- 原 provider 下不再显示该会话。

### Clone

MVP 支持模式，但安全等级高于 `retag`。

适用场景：

- 想同时在旧 provider 和新 provider 下看到同一会话。

需要修改：

- 新 rollout 文件
- `state_5.sqlite.threads`
- `session_index.jsonl`
- `.codex-global-state.json` 顶层：
  - `projectless-thread-ids`
  - `thread-workspace-root-hints`
- `.codex-global-state.json.electron-persisted-atom-state`：
  - `prompt-history`
  - `heartbeat-thread-permissions-by-id`

强约束：

- 必须在 Codex Desktop 退出后执行。
- 执行后再启动 Codex。
- 每个 clone 生成新的 thread id。
- 新 rollout 文件名必须替换为新 thread id。
- rollout 内所有等于旧 thread id 的字段必须替换为新 thread id。
- `session_meta.model_provider` 必须改为目标 provider。
- 失败时不能留下半条 DB 记录；DB 写入和文件写入需要可回滚。

### Batch

MVP 支持批量迁移。

批量选择来源：

- 当前 provider 列表中用 `space` 多选。
- 搜索结果中用 `a` 全选当前过滤结果。
- 按日期范围筛选后全选。
- 按 cwd 筛选后全选。

批量执行规则：

- `retag` 批量：允许 Codex 运行中执行 SQLite/rollout 修改，但 TUI 必须提示“刷新/重启后 UI 更稳定”。
- `clone` 批量：必须检测 Codex Desktop 已退出，否则禁止 Apply。
- 每批迁移只创建一个 batch snapshot 目录。
- `manifest.json` 记录每条会话的 old id、新 id、旧 provider、新 provider、rollout 路径。
- 批量过程中任一条失败，进入 rollback 提示，不继续静默迁移后续会话。

批量 snapshot 结构：

```text
~/.codex/session-migrate-snapshots/YYYYMMDD-HHMMSS-batch-clone-openai-to-sub2api/
  manifest.json
  state_5.sqlite
  state_5.sqlite-wal
  state_5.sqlite-shm
  session_index.jsonl
  .codex-global-state.json
  rollouts/
    rollout-2026-05-27T09-40-44-019e6717....jsonl
```

## TUI 快捷键

```text
q       退出
j/k     上下移动
tab     切换区域
space   选择/取消选择
/       搜索
p       provider 选择
a       显示/隐藏 archived
s       显示/隐藏 subagent
d       dry-run
m       migrate
c       clone mode
b       创建快照
r       rollback
v       查看详情
?       帮助
```

## 技术选型

建议使用 Go + Bubble Tea：

- 单文件二进制，适合放到 PATH。
- TUI 表格、确认框、进度条成熟。
- SQLite 可用 `modernc.org/sqlite` 做纯 Go 实现，避免 CGO。
- 文件操作和 JSON patch 易控。

推荐结构：

```text
cmd/codex-session-migrator/main.go
internal/codex/paths.go
internal/codex/state_db.go
internal/codex/rollout.go
internal/codex/global_state.go
internal/migrate/retag.go
internal/migrate/clone.go
internal/migrate/snapshot.go
internal/tui/model.go
internal/tui/views.go
```

## MVP 版本

第一版必须做：

- provider 会话计数
- 会话列表
- 搜索标题
- 单选 retag
- 单选 clone
- 批量 retag
- 批量 clone
- dry-run
- apply
- snapshot
- rollback

暂不做：

- 编辑 provider 配置
- 图形化 diff
- 跨机器导入导出
- 自动关闭 Codex Desktop

## 验收标准

用刚才验证过的会话作为测试：

```text
019e6717-5a68-7b11-aaa4-6505d503f4da
今天 codex 又更新什么了？
```

验收步骤：

1. 从 `sub2api` retag 回 `openai`。
2. Codex UI 确认从 sub2api 不可见。
3. TUI 选择该会话，retag 到 `sub2api`。
4. Codex UI 刷新/重启后确认可见。
5. 用 snapshot rollback。
6. 确认 provider 恢复。

批量 retag 验收：

1. 选择 3 条 `openai` 会话。
2. dry-run 显示 3 条将从 `openai` 改到 `sub2api`。
3. apply 后 3 条 DB provider 和 rollout provider 都变为 `sub2api`。
4. rollback 后恢复原 provider。

clone 验收：

1. 退出 Codex Desktop。
2. 选择 1 条 `openai` 会话，clone 到 `sub2api`。
3. 验证生成新 thread id、新 rollout 文件、新 DB 记录、新 `session_index.jsonl` 记录。
4. 验证 `.codex-global-state.json` 顶层 `projectless-thread-ids` 和 `thread-workspace-root-hints` 包含新 id。
5. 启动 Codex Desktop，确认新副本可见。

批量 clone 验收：

1. 退出 Codex Desktop。
2. 选择 3 条 `openai` 会话，clone 到 `sub2api`。
3. apply 后生成 3 个新 thread id。
4. 启动 Codex Desktop，确认 3 条副本可见。
5. rollback 后 3 条新副本消失，原会话仍存在。

## 后续增强

- 批量迁移：按 provider / 日期 / cwd 选择。
- 自动检测当前 cc switch provider。
- 显示“为什么不可见”的诊断原因。
- Clone 模式完整支持，但要求 Codex 已退出。
- 导出迁移报告 Markdown。
