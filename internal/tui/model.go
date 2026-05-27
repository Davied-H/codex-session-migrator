package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"codex-session-migrator/internal/codex"
	"codex-session-migrator/internal/migrate"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

type focus int

const (
	focusProviders focus = iota
	focusProjects
	focusSessions
	focusRollback
)

type Model struct {
	paths          codex.Paths
	diag           codex.Diagnostics
	globalIndex    codex.GlobalIndex
	sessionNames   map[string]string
	providers      []providerRow
	projects       []projectRow
	allSessions    []codex.Thread
	sessions       []codex.Thread
	selected       map[string]bool
	cursorP        int
	cursorG        int
	cursorS        int
	target         string
	mode           migrate.Mode
	search         string
	searchOpen     bool
	searchQuery    string
	searchCursor   int
	searchOffset   int
	searchResults  []searchResult
	searchDocs     map[string]searchDoc
	searchIndexSeq int
	searchIndexPos int
	searchIndexing bool
	input          string
	inputMode      string
	pickerOpen     bool
	pickerQuery    string
	pickerCursor   int
	pickerOffset   int
	migrateConfirm bool
	migrateLabel   string
	migrateCount   int
	migrateIDs     []string
	clearConfirm   bool
	clearScope     string
	clearLabel     string
	clearCount     int
	clearIDs       []string
	clearExpected  string
	clearInput     string
	settingsOpen   bool
	settingsCursor int
	includeA       bool
	includeS       bool
	focus          focus
	message        string
	snapshots      []string
	width          int
	height         int
	offsetP        int
	offsetG        int
	offsetS        int
	offsetR        int
	detailOpen     bool
	detail         codex.ConversationInfo
	detailThread   codex.Thread
	detailErr      string
	detailOffset   int
	titleFrame     int
	demoMode       bool
}

type providerRow struct {
	Name  string
	Total int
}

type projectRow struct {
	Key   string
	Name  string
	Root  string
	Count int
}

func (m Model) viewProviders() []providerRow {
	if m.demoMode {
		return demoProviders()
	}
	return m.providers
}

func (m Model) viewProjects() []projectRow {
	if m.demoMode {
		return demoProjects()
	}
	return m.projects
}

func (m Model) viewSessions() []codex.Thread {
	if m.demoMode {
		sessions := demoSessions()
		projects := demoProjects()
		if m.cursorG < 0 || m.cursorG >= len(projects) || projects[m.cursorG].Key == allProjectsKey {
			return sessions
		}
		project := projects[m.cursorG]
		var out []codex.Thread
		for _, s := range sessions {
			if filepath.Clean(s.CWD) == filepath.Clean(project.Root) {
				out = append(out, s)
			}
		}
		return out
	}
	return m.sessions
}

func (m Model) viewAllSessionCount() int {
	if m.demoMode {
		return len(demoSessions())
	}
	return len(m.allSessions)
}

func demoProviders() []providerRow {
	return []providerRow{
		{Name: "demo-openai", Total: 12},
		{Name: "demo-sub2api", Total: 6},
	}
}

func demoProjects() []projectRow {
	return []projectRow{
		{Key: allProjectsKey, Name: "全部项目", Count: 18},
		{Key: "/demo/customer-portal", Name: "customer-portal", Root: "/demo/customer-portal", Count: 7},
		{Key: "/demo/ops-console", Name: "ops-console", Root: "/demo/ops-console", Count: 6},
		{Key: "/demo/research-lab", Name: "research-lab", Root: "/demo/research-lab", Count: 5},
	}
}

func demoSessions() []codex.Thread {
	now := time.Now()
	return []codex.Thread{
		{ID: "demo-001", UpdatedAt: now.Add(-35 * time.Minute).Unix(), ModelProvider: "demo-openai", CWD: "/demo/customer-portal", Title: "梳理客户门户登录流程"},
		{ID: "demo-002", UpdatedAt: now.Add(-2 * time.Hour).Unix(), ModelProvider: "demo-openai", CWD: "/demo/customer-portal", Title: "优化订单列表筛选交互"},
		{ID: "demo-003", UpdatedAt: now.Add(-4 * time.Hour).Unix(), ModelProvider: "demo-sub2api", CWD: "/demo/ops-console", Title: "排查任务队列延迟告警"},
		{ID: "demo-004", UpdatedAt: now.Add(-7 * time.Hour).Unix(), ModelProvider: "demo-openai", CWD: "/demo/research-lab", Title: "设计实验数据对比视图", Archived: true},
		{ID: "demo-005", UpdatedAt: now.Add(-24 * time.Hour).Unix(), ModelProvider: "demo-sub2api", CWD: "/demo/ops-console", Title: "整理发布前检查清单"},
		{ID: "demo-006", UpdatedAt: now.Add(-26 * time.Hour).Unix(), ModelProvider: "demo-openai", CWD: "/demo/customer-portal", Title: "补充用户资料页边界状态"},
		{ID: "demo-007", UpdatedAt: now.Add(-48 * time.Hour).Unix(), ModelProvider: "demo-openai", CWD: "/demo/research-lab", Title: "生成周报摘要草稿"},
	}
}

type openMarkdownMsg struct {
	path string
	err  error
}

type clearProviderMsg struct {
	label string
	count int
	err   error
}

type titleTickMsg time.Time

type searchDoc struct {
	Title    string
	Messages []searchMessage
}

type searchMessage struct {
	Role string
	Text string
}

type searchResult struct {
	Thread  codex.Thread
	Title   string
	Role    string
	Snippet string
	Score   int
}

type searchIndexBatchMsg struct {
	Seq   int
	Start int
	Next  int
	Total int
	Docs  map[string]searchDoc
}

const allProjectsKey = "__all__"

const searchIndexBatchSize = 24

func New(paths codex.Paths) Model {
	m := Model{
		paths:        paths,
		selected:     map[string]bool{},
		sessionNames: map[string]string{},
		searchDocs:   map[string]searchDoc{},
		target:       "sub2api",
		mode:         migrate.ModeRetag,
		focus:        focusProviders,
	}
	m.reload()
	m.selectCurrentWorkspaceProject()
	return m
}

func (m Model) Init() tea.Cmd { return titleTickCmd() }

func titleTickCmd() tea.Cmd {
	return tea.Tick(140*time.Millisecond, func(t time.Time) tea.Msg {
		return titleTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		m.ensureOffsets()
		return m, nil
	}
	if _, ok := msg.(titleTickMsg); ok {
		m.titleFrame++
		return m, titleTickCmd()
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if m.migrateConfirm {
			return m.updateMigrateConfirmMouse(mouse), nil
		}
		if m.clearConfirm {
			return m.updateClearConfirmMouse(mouse), nil
		}
		if m.pickerOpen {
			return m.updatePickerMouse(mouse), nil
		}
		if m.searchOpen {
			return m.updateSearchMouse(mouse), nil
		}
		if m.detailOpen {
			return m.updateDetailMouse(mouse)
		}
		if m.settingsOpen {
			return m.updateSettingsMouse(mouse), nil
		}
		return m.updateMouse(mouse)
	}
	if opened, ok := msg.(openMarkdownMsg); ok {
		if opened.err != nil {
			m.message = "打开 Markdown 失败: " + opened.err.Error()
		} else {
			m.message = "已打开 Markdown: " + opened.path
		}
		return m, nil
	}
	if cleared, ok := msg.(clearProviderMsg); ok {
		if cleared.err != nil {
			m.message = "删除 session 失败: " + cleared.err.Error()
		} else {
			m.message = fmt.Sprintf("已删除 %s: %d 条 session", cleared.label, cleared.count)
			m.selected = map[string]bool{}
			m.reload()
		}
		return m, nil
	}
	if batch, ok := msg.(searchIndexBatchMsg); ok {
		if batch.Seq != m.searchIndexSeq {
			return m, nil
		}
		if m.searchDocs == nil {
			m.searchDocs = map[string]searchDoc{}
		}
		for id, doc := range batch.Docs {
			m.searchDocs[id] = doc
		}
		m.searchIndexPos = batch.Next
		m.searchIndexing = batch.Next < batch.Total
		if m.searchOpen {
			m.refreshSearchResults()
		}
		if m.searchIndexing {
			return m, m.searchIndexBatchCmd(batch.Next, batch.Seq)
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.migrateConfirm {
		return m.updateMigrateConfirmKey(key)
	}
	if m.clearConfirm {
		return m.updateClearConfirmKey(key)
	}
	if m.pickerOpen {
		return m.updatePickerKey(key), nil
	}
	if m.searchOpen {
		return m.updateSearchKey(key)
	}
	if m.detailOpen {
		return m.updateDetailKey(key)
	}
	if m.settingsOpen {
		return m.updateSettingsKey(key), nil
	}
	if m.inputMode != "" {
		return m.updateInput(key), nil
	}
	var cmd tea.Cmd
	switch key.String() {
	case "esc":
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		switch m.focus {
		case focusProviders:
			m.focus = focusProjects
		case focusProjects:
			m.focus = focusSessions
		default:
			m.focus = focusProviders
		}
	case "j", "down":
		m.down()
	case "k", "up":
		m.up()
	case "pgdown":
		m.page(1)
	case "pgup":
		m.page(-1)
	case "home":
		m.jumpStart()
	case "end":
		m.jumpEnd()
	case "p":
		m.focus = focusProviders
	case "g":
		m.focus = focusProjects
	case " ":
		sessions := m.viewSessions()
		if m.focus == focusSessions && len(sessions) > 0 {
			id := sessions[m.cursorS].ID
			m.selected[id] = !m.selected[id]
		}
	case "a":
		if m.focus == focusSessions {
			m.toggleSelectVisibleSessions()
		} else {
			m.includeA = !m.includeA
			m.reload()
		}
	case "s":
		m.includeS = !m.includeS
		m.reloadProviderData()
	case "/":
		if m.demoMode {
			m.message = "演示模式不搜索真实会话；Ctrl+E 退出演示模式"
		} else {
			cmd = m.openSearchModal()
		}
	case "e":
		m.openProviderPicker()
	case "ctrl+e":
		m.toggleDemoMode()
	case "o":
		m.settingsOpen = true
	case "t":
		m.cycleTarget()
	case "c":
		m.toggleMode()
	case "x":
		if m.demoMode {
			m.message = "演示模式不会删除真实会话；Ctrl+E 退出演示模式"
		} else {
			m.openClearConfirm()
		}
	case "d":
		if m.demoMode {
			m.message = "演示模式不会迁移真实会话；Ctrl+E 退出演示模式"
		} else {
			m.run(true)
		}
	case "m":
		if m.demoMode {
			m.message = "演示模式不会迁移真实会话；Ctrl+E 退出演示模式"
		} else {
			m.openMigrateConfirm()
		}
	case "b":
		m.message = "迁移 apply 会自动创建 snapshot；独立 snapshot 请使用 dry-run 检查后执行迁移。"
	case "r":
		m.focus = focusRollback
		m.snapshots = listSnapshots(m.paths.Snapshots)
		m.cursorS = 0
	case "enter":
		if m.focus == focusProviders {
			if !m.demoMode {
				m.reloadProviderData()
			}
			m.focus = focusProjects
		} else if m.focus == focusProjects {
			if !m.demoMode {
				m.applyProjectFilter()
			}
			m.focus = focusSessions
		} else if m.focus == focusSessions && len(m.viewSessions()) > 0 {
			if m.demoMode {
				m.message = "演示模式不打开真实 Markdown；Ctrl+E 退出演示模式"
			} else {
				cmd = m.openConversationMarkdown()
			}
		} else if m.focus == focusRollback && len(m.snapshots) > 0 {
			name := m.snapshots[m.cursorS]
			if err := migrate.Rollback(m.paths, name); err != nil {
				m.message = "rollback 失败: " + err.Error()
			} else {
				m.message = "rollback 完成: " + name
				m.reload()
			}
		}
	case "v":
		if m.focus == focusSessions && len(m.viewSessions()) > 0 {
			if m.demoMode {
				m.message = "演示模式不打开真实 Markdown；Ctrl+E 退出演示模式"
			} else {
				cmd = m.openConversationMarkdown()
			}
		}
	case "?":
		m.message = "q quit | tab focus | p providers | g projects | j/k move | enter/v open markdown | space select | / search | o settings | Ctrl+E demo | e choose target | t cycle target | c mode | x clear provider | d dry-run | m apply | r rollback"
	}
	m.ensureOffsets()
	return m, cmd
}

func (m *Model) toggleSelectVisibleSessions() {
	sessions := m.viewSessions()
	if len(sessions) == 0 {
		m.message = "当前没有可选择的会话"
		return
	}
	allSelected := true
	for _, s := range sessions {
		if !m.selected[s.ID] {
			allSelected = false
			break
		}
	}
	if allSelected {
		for _, s := range sessions {
			delete(m.selected, s.ID)
		}
		m.message = fmt.Sprintf("已取消选择当前项目过滤结果 %d 条", len(sessions))
		return
	}
	for _, s := range sessions {
		m.selected[s.ID] = true
	}
	m.message = fmt.Sprintf("已选择当前项目过滤结果 %d 条", len(sessions))
}

func (m *Model) toggleDemoMode() {
	m.demoMode = !m.demoMode
	m.selected = map[string]bool{}
	m.searchOpen = false
	m.detailOpen = false
	m.pickerOpen = false
	m.migrateConfirm = false
	m.clearConfirm = false
	m.cursorP = 0
	m.cursorG = 0
	m.cursorS = 0
	m.offsetP = 0
	m.offsetG = 0
	m.offsetS = 0
	if m.demoMode {
		m.message = "已进入演示模式：项目和会话均为 mock 数据，Ctrl+E 退出"
		return
	}
	m.message = "已退出演示模式"
	m.ensureOffsets()
}

func (m Model) updateDetailKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "b", "left":
		m.detailOpen = false
	case "j", "down":
		m.scrollDetail(1)
	case "k", "up":
		m.scrollDetail(-1)
	case "pgdown":
		m.scrollDetail(max(1, m.detailVisibleRows()))
	case "pgup":
		m.scrollDetail(-max(1, m.detailVisibleRows()))
	case "home":
		m.detailOffset = 0
	case "end":
		m.detailOffset = max(0, len(m.detailRows())-m.detailVisibleRows())
	}
	return m, nil
}

func (m Model) updateDetailMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	mouse := tea.MouseEvent(msg)
	if mouse.IsWheel() {
		if mouse.Button == tea.MouseButtonWheelDown {
			m.scrollDetail(3)
		} else if mouse.Button == tea.MouseButtonWheelUp {
			m.scrollDetail(-3)
		}
		return m, nil
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.X < 12 && mouse.Y < 3 {
		m.detailOpen = false
	}
	return m, nil
}

func (m *Model) openConversationDetail() {
	if m.demoMode {
		m.message = "演示模式不读取真实会话详情；Ctrl+E 退出演示模式"
		return
	}
	if len(m.sessions) == 0 || m.cursorS >= len(m.sessions) {
		return
	}
	t := m.sessions[m.cursorS]
	m.detailThread = t
	m.detailOffset = 0
	m.detailErr = ""
	info, err := codex.ReadConversationInfo(t.RolloutPath, 500)
	if err != nil {
		m.detailErr = err.Error()
		info = codex.ConversationInfo{Path: t.RolloutPath}
	}
	m.detail = info
	m.detailOpen = true
}

func (m *Model) openConversationMarkdown() tea.Cmd {
	if m.demoMode {
		m.message = "演示模式不打开真实 Markdown；Ctrl+E 退出演示模式"
		return nil
	}
	if len(m.sessions) == 0 || m.cursorS >= len(m.sessions) {
		m.message = "没有可打开的会话"
		return nil
	}
	return m.openThreadMarkdown(m.sessions[m.cursorS])
}

func (m *Model) openThreadMarkdown(t codex.Thread) tea.Cmd {
	t.Title = m.displayThreadTitle(t)
	outputDir := filepath.Join(m.paths.Home, "session-details")
	m.message = "正在生成 Markdown..."
	return func() tea.Msg {
		path, err := codex.WriteConversationMarkdown(outputDir, t, 2000)
		if err == nil {
			err = openPath(path)
		}
		return openMarkdownMsg{path: path, err: err}
	}
}

func (m *Model) refreshSearchResults() {
	query := strings.TrimSpace(m.searchQuery)
	m.searchResults = m.searchResults[:0]
	if query == "" {
		for _, t := range m.sessions {
			m.searchResults = append(m.searchResults, searchResult{
				Thread:  t,
				Title:   m.displayThreadTitle(t),
				Role:    "title",
				Snippet: fallbackPreview(t),
			})
		}
		m.ensureSearchVisible()
		return
	}
	for _, t := range m.sessions {
		doc := m.searchDocumentCached(t)
		best, ok := bestSearchResult(t, doc, query)
		if ok {
			m.searchResults = append(m.searchResults, best)
		}
	}
	sort.SliceStable(m.searchResults, func(i, j int) bool {
		if m.searchResults[i].Score != m.searchResults[j].Score {
			return m.searchResults[i].Score > m.searchResults[j].Score
		}
		return m.searchResults[i].Thread.UpdatedAt > m.searchResults[j].Thread.UpdatedAt
	})
	m.searchCursor = clamp(m.searchCursor, 0, max(0, len(m.searchResults)-1))
	m.ensureSearchVisible()
}

func (m *Model) searchDocumentCached(t codex.Thread) searchDoc {
	if m.searchDocs == nil {
		m.searchDocs = map[string]searchDoc{}
	}
	if doc, ok := m.searchDocs[t.ID]; ok {
		return doc
	}
	doc := searchDocFromThread(t, m.sessionNames)
	m.searchDocs[t.ID] = doc
	return doc
}

func openPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Run()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Run()
	default:
		return exec.Command("xdg-open", path).Run()
	}
}

func (m *Model) scrollDetail(delta int) {
	rows := m.detailRows()
	visible := m.detailVisibleRows()
	m.detailOffset = clamp(m.detailOffset+delta, 0, max(0, len(rows)-visible))
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	mouse := tea.MouseEvent(msg)
	target, row, ok := m.hitTest(mouse.X, mouse.Y)
	if mouse.IsWheel() {
		target, ok := m.hitPanel(mouse.X, mouse.Y)
		if !ok {
			return m, nil
		}
		if mouse.Button == tea.MouseButtonWheelDown {
			m.scrollPanel(target, 3)
		} else if mouse.Button == tea.MouseButtonWheelUp {
			m.scrollPanel(target, -3)
		}
		return m, nil
	}
	if mouse.Action != tea.MouseActionPress {
		return m, nil
	}
	if !ok {
		return m, nil
	}
	if mouse.Button != tea.MouseButtonLeft {
		return m, nil
	}
	m.activateHit(target, row)
	m.ensureOffsets()
	return m, nil
}

func (m Model) updateInput(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc":
		m.inputMode = ""
	case "enter":
		if m.inputMode == "search" {
			m.search = m.input
			m.reloadProviderData()
		}
		m.inputMode = ""
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(key.Runes) > 0 {
			m.input += string(key.Runes)
		}
	}
	return m
}

func (m Model) updateSearchKey(key tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch key.String() {
	case "esc":
		m.searchOpen = false
	case "enter":
		if len(m.searchResults) > 0 {
			m.search = m.searchQuery
			m.searchOpen = false
			result := m.searchResults[clamp(m.searchCursor, 0, len(m.searchResults)-1)]
			cmd = m.openThreadMarkdown(result.Thread)
		}
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = trimLastRune(m.searchQuery)
			m.searchCursor = 0
			m.searchOffset = 0
			m.refreshSearchResults()
		}
	case "ctrl+u":
		m.searchQuery = ""
		m.searchCursor = 0
		m.searchOffset = 0
		m.refreshSearchResults()
	case "j", "down":
		m.searchCursor = clamp(m.searchCursor+1, 0, max(0, len(m.searchResults)-1))
		m.ensureSearchVisible()
	case "k", "up":
		m.searchCursor = clamp(m.searchCursor-1, 0, max(0, len(m.searchResults)-1))
		m.ensureSearchVisible()
	case "pgdown":
		m.searchCursor = clamp(m.searchCursor+m.searchVisibleRows(), 0, max(0, len(m.searchResults)-1))
		m.ensureSearchVisible()
	case "pgup":
		m.searchCursor = clamp(m.searchCursor-m.searchVisibleRows(), 0, max(0, len(m.searchResults)-1))
		m.ensureSearchVisible()
	case "home":
		m.searchCursor = 0
		m.ensureSearchVisible()
	case "end":
		m.searchCursor = max(0, len(m.searchResults)-1)
		m.ensureSearchVisible()
	default:
		if len(key.Runes) > 0 {
			m.searchQuery += string(key.Runes)
			m.searchCursor = 0
			m.searchOffset = 0
			m.refreshSearchResults()
		}
	}
	return m, cmd
}

func (m Model) updateSearchMouse(msg tea.MouseMsg) Model {
	mouse := tea.MouseEvent(msg)
	if mouse.IsWheel() {
		if mouse.Button == tea.MouseButtonWheelDown {
			m.searchCursor = clamp(m.searchCursor+1, 0, max(0, len(m.searchResults)-1))
		} else if mouse.Button == tea.MouseButtonWheelUp {
			m.searchCursor = clamp(m.searchCursor-1, 0, max(0, len(m.searchResults)-1))
		}
		m.ensureSearchVisible()
		return m
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		width := m.width
		if width <= 0 {
			width = 90
		}
		height := m.height
		if height <= 0 {
			height = 28
		}
		boxWidth, boxHeight := m.searchModalSize(width, height)
		left, top := modalOrigin(width, height, boxWidth, boxHeight)
		if mouse.X < left || mouse.X >= left+boxWidth || mouse.Y < top || mouse.Y >= top+boxHeight {
			m.searchOpen = false
			return m
		}
		row := mouse.Y - top - 6
		if row >= 0 {
			idx := m.searchOffset + row
			if idx >= 0 && idx < len(m.searchResults) {
				m.searchCursor = idx
			}
		}
	}
	return m
}

func (m *Model) openProviderPicker() {
	m.pickerOpen = true
	m.pickerQuery = ""
	m.pickerCursor = 0
	m.pickerOffset = 0
	for i, p := range m.filteredTargetProviders() {
		if p.Name == m.target {
			m.pickerCursor = i
			break
		}
	}
	m.ensurePickerVisible()
}

func (m *Model) openSearchModal() tea.Cmd {
	m.searchOpen = true
	m.searchQuery = m.search
	m.searchCursor = 0
	m.searchOffset = 0
	m.searchIndexSeq++
	m.searchIndexPos = 0
	m.searchIndexing = len(m.sessions) > 0
	m.refreshSearchResults()
	if !m.searchIndexing {
		return nil
	}
	return m.searchIndexBatchCmd(0, m.searchIndexSeq)
}

func (m Model) searchIndexBatchCmd(start, seq int) tea.Cmd {
	sessions := append([]codex.Thread(nil), m.sessions...)
	names := map[string]string{}
	for id, name := range m.sessionNames {
		names[id] = name
	}
	return func() tea.Msg {
		total := len(sessions)
		end := min(total, start+searchIndexBatchSize)
		docs := make(map[string]searchDoc, max(0, end-start))
		for _, t := range sessions[start:end] {
			docs[t.ID] = readSearchDocument(t, names)
		}
		return searchIndexBatchMsg{Seq: seq, Start: start, Next: end, Total: total, Docs: docs}
	}
}

func (m Model) updatePickerKey(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc":
		m.pickerOpen = false
	case "enter":
		choices := m.filteredTargetProviders()
		if len(choices) > 0 {
			m.target = choices[clamp(m.pickerCursor, 0, len(choices)-1)].Name
			m.message = "目标 Provider: " + m.target
		} else if strings.TrimSpace(m.pickerQuery) != "" {
			m.target = strings.TrimSpace(m.pickerQuery)
			m.message = "目标 Provider: " + m.target
		}
		m.pickerOpen = false
	case "j", "down":
		m.pickerCursor = clamp(m.pickerCursor+1, 0, max(0, len(m.filteredTargetProviders())-1))
	case "k", "up":
		m.pickerCursor = clamp(m.pickerCursor-1, 0, max(0, len(m.filteredTargetProviders())-1))
	case "pgdown":
		m.pickerCursor = clamp(m.pickerCursor+8, 0, max(0, len(m.filteredTargetProviders())-1))
	case "pgup":
		m.pickerCursor = clamp(m.pickerCursor-8, 0, max(0, len(m.filteredTargetProviders())-1))
	case "home":
		m.pickerCursor = 0
	case "end":
		m.pickerCursor = max(0, len(m.filteredTargetProviders())-1)
	case "backspace":
		if len(m.pickerQuery) > 0 {
			m.pickerQuery = trimLastRune(m.pickerQuery)
			m.pickerCursor = 0
			m.pickerOffset = 0
		}
	default:
		if len(key.Runes) > 0 {
			m.pickerQuery += string(key.Runes)
			m.pickerCursor = 0
			m.pickerOffset = 0
		}
	}
	m.ensurePickerVisible()
	return m
}

func (m Model) updatePickerMouse(msg tea.MouseMsg) Model {
	mouse := tea.MouseEvent(msg)
	if mouse.IsWheel() {
		if mouse.Button == tea.MouseButtonWheelDown {
			m.pickerCursor = clamp(m.pickerCursor+3, 0, max(0, len(m.filteredTargetProviders())-1))
		} else if mouse.Button == tea.MouseButtonWheelUp {
			m.pickerCursor = clamp(m.pickerCursor-3, 0, max(0, len(m.filteredTargetProviders())-1))
		}
		m.ensurePickerVisible()
		return m
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		width := m.width
		if width <= 0 {
			width = 90
		}
		height := m.height
		if height <= 0 {
			height = 28
		}
		boxWidth, boxHeight := m.providerPickerSize(width, height)
		left := max(0, (width-boxWidth)/2)
		top := max(0, (height-boxHeight)/2)
		row := mouse.Y - top - 5
		choices := m.filteredTargetProviders()
		idx := m.pickerOffset + row
		if mouse.X >= left && mouse.X < left+boxWidth && row >= 0 && idx >= 0 && idx < len(choices) {
			m.pickerCursor = idx
			m.target = choices[idx].Name
			m.message = "目标 Provider: " + m.target
			m.pickerOpen = false
		}
	}
	return m
}

func (m Model) updateSettingsKey(key tea.KeyMsg) Model {
	switch key.String() {
	case "esc", "b", "left", "o":
		m.settingsOpen = false
	case "q", "ctrl+c":
		// q remains handled only from the main view; leave settings with Esc/b.
	case "j", "down":
		m.settingsCursor = clamp(m.settingsCursor+1, 0, settingsItemCount()-1)
	case "k", "up":
		m.settingsCursor = clamp(m.settingsCursor-1, 0, settingsItemCount()-1)
	case "home":
		m.settingsCursor = 0
	case "end":
		m.settingsCursor = settingsItemCount() - 1
	case "enter", " ":
		m.activateSetting()
	}
	return m
}

func (m Model) updateSettingsMouse(msg tea.MouseMsg) Model {
	mouse := tea.MouseEvent(msg)
	if mouse.IsWheel() {
		if mouse.Button == tea.MouseButtonWheelDown {
			m.settingsCursor = clamp(m.settingsCursor+1, 0, settingsItemCount()-1)
		} else if mouse.Button == tea.MouseButtonWheelUp {
			m.settingsCursor = clamp(m.settingsCursor-1, 0, settingsItemCount()-1)
		}
		return m
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		width := m.width
		if width <= 0 {
			width = 90
		}
		height := m.height
		if height <= 0 {
			height = 28
		}
		boxWidth, boxHeight := m.settingsModalSize(width, height)
		left, top := modalOrigin(width, height, boxWidth, boxHeight)
		if mouse.X < left || mouse.X >= left+boxWidth || mouse.Y < top || mouse.Y >= top+boxHeight {
			m.settingsOpen = false
			return m
		}
		row := mouse.Y - top - 5
		if row >= 0 {
			idx := row / 2
			if idx < settingsItemCount() {
				m.settingsCursor = idx
				m.activateSetting()
			}
		}
	}
	return m
}

func (m *Model) activateSetting() {
	switch m.settingsCursor {
	case 0:
		m.includeA = !m.includeA
		m.reload()
	case 1:
		m.includeS = !m.includeS
		m.reloadProviderData()
	case 2:
		m.openProviderPicker()
	case 3:
		m.toggleMode()
	case 4:
		m.openClearArchivedConfirm()
	}
}

func settingsItemCount() int {
	return 5
}

func (m *Model) openClearArchivedConfirm() {
	if !m.diag.DBExists {
		m.message = "数据库不存在，无法清理归档会话"
		return
	}
	db, err := codex.OpenDB(m.paths)
	if err != nil {
		m.message = "读取归档会话失败: " + err.Error()
		return
	}
	defer db.Close()
	threads, err := codex.ListArchivedThreads(db)
	if err != nil {
		m.message = "读取归档会话失败: " + err.Error()
		return
	}
	if len(threads) == 0 {
		m.message = "没有归档会话可清理"
		return
	}
	ids := make([]string, 0, len(threads))
	for _, t := range threads {
		ids = append(ids, t.ID)
	}
	m.clearConfirm = true
	m.clearScope = "archived"
	m.clearLabel = "archived sessions"
	m.clearCount = len(ids)
	m.clearIDs = ids
	m.clearExpected = ""
	m.clearInput = ""
}

func (m Model) archivedSessionCount() int {
	total := 0
	for _, c := range m.diag.Counts {
		if c.Archived {
			total += c.Count
		}
	}
	return total
}

func (m *Model) toggleMode() {
	if m.mode == migrate.ModeRetag {
		m.mode = migrate.ModeClone
	} else {
		m.mode = migrate.ModeRetag
	}
}

func (m *Model) openMigrateConfirm() {
	label, ids := m.migrationTarget()
	if len(ids) == 0 {
		m.message = "没有可迁移的会话"
		return
	}
	m.migrateConfirm = true
	m.migrateLabel = label
	m.migrateCount = len(ids)
	m.migrateIDs = ids
}

func (m Model) updateMigrateConfirmKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "n", "N":
		m.migrateConfirm = false
	case "enter", "y", "Y":
		m.migrateConfirm = false
		m.runIDs(false, append([]string{}, m.migrateIDs...))
	}
	return m, nil
}

func (m Model) updateMigrateConfirmMouse(msg tea.MouseMsg) Model {
	mouse := tea.MouseEvent(msg)
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		width := m.width
		if width <= 0 {
			width = 90
		}
		height := m.height
		if height <= 0 {
			height = 28
		}
		boxWidth, boxHeight := m.migrateConfirmSize(width, height)
		left := max(0, (width-boxWidth)/2)
		top := max(0, (height-boxHeight)/2)
		if mouse.X < left || mouse.X >= left+boxWidth || mouse.Y < top || mouse.Y >= top+boxHeight {
			m.migrateConfirm = false
		}
	}
	return m
}

func (m *Model) openClearConfirm() {
	scope, label, ids, expected := m.deleteTarget()
	if len(ids) == 0 {
		m.message = "没有可删除的 session"
		return
	}
	m.clearConfirm = true
	m.clearScope = scope
	m.clearLabel = label
	m.clearCount = len(ids)
	m.clearIDs = ids
	m.clearExpected = expected
	m.clearInput = ""
}

func (m Model) updateClearConfirmKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "n", "N":
		m.clearConfirm = false
	case "enter", "y", "Y":
		if m.clearExpected != "" && m.clearInput != m.clearExpected {
			m.message = "输入名称不匹配，未删除"
			return m, nil
		}
		label := m.clearLabel
		count := m.clearCount
		ids := append([]string{}, m.clearIDs...)
		m.clearConfirm = false
		m.message = "正在删除 " + label + "..."
		return m, func() tea.Msg {
			_, err := migrate.ClearThreads(m.paths, migrate.ClearThreadsOptions{IDs: ids, Label: label})
			return clearProviderMsg{label: label, count: count, err: err}
		}
	case "backspace":
		if m.clearExpected != "" && m.clearInput != "" {
			m.clearInput = trimLastRune(m.clearInput)
		}
	default:
		if m.clearExpected != "" && len(key.Runes) > 0 {
			m.clearInput += string(key.Runes)
		}
	}
	return m, nil
}

func (m Model) updateClearConfirmMouse(msg tea.MouseMsg) Model {
	mouse := tea.MouseEvent(msg)
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		width := m.width
		if width <= 0 {
			width = 90
		}
		height := m.height
		if height <= 0 {
			height = 28
		}
		boxWidth, boxHeight := m.clearConfirmSize(width, height)
		left := max(0, (width-boxWidth)/2)
		top := max(0, (height-boxHeight)/2)
		if mouse.X < left || mouse.X >= left+boxWidth || mouse.Y < top || mouse.Y >= top+boxHeight {
			m.clearConfirm = false
		}
	}
	return m
}

func (m *Model) down() {
	if m.focus == focusProviders && m.cursorP < len(m.viewProviders())-1 {
		m.cursorP++
		if !m.demoMode {
			m.reloadProviderData()
		}
	}
	if m.focus == focusProjects && m.cursorG < len(m.viewProjects())-1 {
		m.cursorG++
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	}
	if (m.focus == focusSessions || m.focus == focusRollback) && m.cursorS < m.currentLen()-1 {
		m.cursorS++
	}
	m.ensureOffsets()
}

func (m *Model) up() {
	if m.focus == focusProviders && m.cursorP > 0 {
		m.cursorP--
		if !m.demoMode {
			m.reloadProviderData()
		}
	}
	if m.focus == focusProjects && m.cursorG > 0 {
		m.cursorG--
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	}
	if (m.focus == focusSessions || m.focus == focusRollback) && m.cursorS > 0 {
		m.cursorS--
	}
	m.ensureOffsets()
}

func (m *Model) page(dir int) {
	step := m.visibleRows(m.focus)
	if step < 1 {
		step = 8
	}
	m.moveFocus(dir * step)
}

func (m *Model) moveFocus(delta int) {
	switch m.focus {
	case focusProviders:
		m.cursorP = clamp(m.cursorP+delta, 0, max(0, len(m.viewProviders())-1))
		if !m.demoMode {
			m.reloadProviderData()
		}
	case focusProjects:
		m.cursorG = clamp(m.cursorG+delta, 0, max(0, len(m.viewProjects())-1))
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	case focusSessions:
		m.cursorS = clamp(m.cursorS+delta, 0, max(0, len(m.viewSessions())-1))
	case focusRollback:
		m.cursorS = clamp(m.cursorS+delta, 0, max(0, len(m.snapshots)-1))
	}
	m.ensureOffsets()
}

func (m *Model) scrollPanel(target focus, delta int) {
	switch target {
	case focusProviders:
		m.offsetP = scrollOffset(m.offsetP, delta, m.visibleRows(focusProviders), len(m.viewProviders()))
	case focusProjects:
		m.offsetG = scrollOffset(m.offsetG, delta, m.visibleRows(focusProjects), len(m.viewProjects()))
	case focusSessions:
		m.offsetS = scrollOffset(m.offsetS, delta, m.visibleRows(focusSessions), len(m.viewSessions()))
	case focusRollback:
		m.offsetR = scrollOffset(m.offsetR, delta, m.visibleRows(focusRollback), len(m.snapshots))
	}
}

func (m *Model) jumpStart() {
	switch m.focus {
	case focusProviders:
		m.cursorP = 0
		if !m.demoMode {
			m.reloadProviderData()
		}
	case focusProjects:
		m.cursorG = 0
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	case focusSessions, focusRollback:
		m.cursorS = 0
	}
	m.ensureOffsets()
}

func (m *Model) jumpEnd() {
	switch m.focus {
	case focusProviders:
		m.cursorP = max(0, len(m.viewProviders())-1)
		if !m.demoMode {
			m.reloadProviderData()
		}
	case focusProjects:
		m.cursorG = max(0, len(m.viewProjects())-1)
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	case focusSessions:
		m.cursorS = max(0, len(m.viewSessions())-1)
	case focusRollback:
		m.cursorS = max(0, len(m.snapshots)-1)
	}
	m.ensureOffsets()
}

func (m Model) currentLen() int {
	if m.focus == focusRollback {
		return len(m.snapshots)
	}
	if m.focus == focusProjects {
		return len(m.viewProjects())
	}
	return len(m.viewSessions())
}

func (m *Model) reload() {
	diag, err := codex.Diagnose(m.paths)
	if err != nil {
		m.message = err.Error()
	}
	m.diag = diag
	if idx, err := codex.ReadGlobalIndex(m.paths.GlobalState); err == nil {
		m.globalIndex = idx
	} else {
		m.globalIndex = codex.GlobalIndex{
			Projectless:         map[string]bool{},
			ThreadWorkspaceRoot: map[string]string{},
			ProjectRoots:        nil,
		}
	}
	if names, err := codex.ReadSessionNames(m.paths.SessionIdx); err == nil {
		m.sessionNames = names
	} else if m.sessionNames == nil {
		m.sessionNames = map[string]string{}
	}
	byName := map[string]int{}
	for _, c := range diag.Counts {
		if !m.includeA && c.Archived {
			continue
		}
		byName[c.Provider] += c.Count
	}
	m.providers = m.providers[:0]
	for name, count := range byName {
		m.providers = append(m.providers, providerRow{name, count})
	}
	sort.Slice(m.providers, func(i, j int) bool { return m.providers[i].Name < m.providers[j].Name })
	if m.cursorP >= len(m.providers) {
		m.cursorP = 0
	}
	m.reloadProviderData()
	m.ensureOffsets()
}

func (m *Model) reloadProviderData() {
	if len(m.providers) == 0 || !m.diag.DBExists {
		m.allSessions = nil
		m.projects = nil
		m.sessions = nil
		return
	}
	db, err := codex.OpenDB(m.paths)
	if err != nil {
		m.message = err.Error()
		return
	}
	defer db.Close()
	sessions, err := codex.ListThreads(db, m.providers[m.cursorP].Name, m.search, m.includeA, m.includeS, 0)
	if err != nil {
		m.message = err.Error()
		return
	}
	m.allSessions = sessions
	m.rebuildProjects()
	m.applyProjectFilter()
	m.ensureOffsets()
}

func (m *Model) rebuildProjects() {
	type agg struct {
		projectRow
		latest int64
	}
	byKey := map[string]*agg{}
	total := &agg{projectRow: projectRow{Key: allProjectsKey, Name: "全部项目", Count: len(m.allSessions)}}
	byKey[allProjectsKey] = total
	for _, root := range m.globalIndex.ProjectRoots {
		key := filepath.Clean(root)
		byKey[key] = &agg{projectRow: projectRow{Key: key, Name: projectName(key), Root: key}}
	}
	ordinary := &agg{projectRow: projectRow{Key: codex.OrdinaryConversationGroup, Name: codex.OrdinaryConversationGroup, Root: codex.OrdinaryConversationGroup}}
	byKey[codex.OrdinaryConversationGroup] = ordinary
	for _, s := range m.allSessions {
		key := m.sessionProjectRoot(s)
		row := byKey[key]
		if row == nil {
			row = ordinary
			key = codex.OrdinaryConversationGroup
			byKey[key] = row
		}
		row.Count++
		if s.UpdatedAt > row.latest {
			row.latest = s.UpdatedAt
		}
		if s.UpdatedAt > total.latest {
			total.latest = s.UpdatedAt
		}
	}
	m.projects = m.projects[:0]
	for _, row := range byKey {
		m.projects = append(m.projects, row.projectRow)
	}
	sort.Slice(m.projects, func(i, j int) bool {
		if m.projects[i].Key == allProjectsKey {
			return true
		}
		if m.projects[j].Key == allProjectsKey {
			return false
		}
		if m.projects[i].Key == codex.OrdinaryConversationGroup {
			return true
		}
		if m.projects[j].Key == codex.OrdinaryConversationGroup {
			return false
		}
		leftOrder := projectOrder(m.globalIndex.ProjectRoots, m.projects[i].Key)
		rightOrder := projectOrder(m.globalIndex.ProjectRoots, m.projects[j].Key)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return strings.ToLower(m.projects[i].Name) < strings.ToLower(m.projects[j].Name)
	})
	if m.cursorG >= len(m.projects) {
		m.cursorG = 0
	}
	m.offsetG = clamp(m.offsetG, 0, max(0, len(m.projects)-1))
}

func (m *Model) applyProjectFilter() {
	if len(m.projects) == 0 {
		m.sessions = nil
		m.cursorS = 0
		return
	}
	project := m.projects[m.cursorG]
	m.sessions = m.sessions[:0]
	for _, s := range m.allSessions {
		if project.Key == allProjectsKey || m.sessionProjectRoot(s) == project.Key {
			m.sessions = append(m.sessions, s)
		}
	}
	if m.cursorS >= len(m.sessions) {
		m.cursorS = 0
	}
	m.offsetS = clamp(m.offsetS, 0, max(0, len(m.sessions)-1))
}

func (m *Model) selectCurrentWorkspaceProject() {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" || len(m.projects) == 0 {
		return
	}
	cwd = filepath.Clean(cwd)
	bestIndex := -1
	bestLen := -1
	for i, project := range m.projects {
		if project.Key == allProjectsKey || project.Key == codex.OrdinaryConversationGroup {
			continue
		}
		root := filepath.Clean(project.Root)
		if pathWithinRoot(cwd, root) && len(root) > bestLen {
			bestIndex = i
			bestLen = len(root)
		}
	}
	if bestIndex < 0 {
		return
	}
	m.cursorG = bestIndex
	m.applyProjectFilter()
}

func (m *Model) cycleTarget() {
	providers := m.viewProviders()
	if len(providers) == 0 {
		return
	}
	names := []string{"sub2api", "openai", "custom"}
	for _, p := range providers {
		names = append(names, p.Name)
	}
	from := providers[m.cursorP].Name
	seen := map[string]bool{}
	var uniq []string
	for _, n := range names {
		if n != from && !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	for i, n := range uniq {
		if n == m.target {
			m.target = uniq[(i+1)%len(uniq)]
			return
		}
	}
	if len(uniq) > 0 {
		m.target = uniq[0]
	}
}

func (m Model) currentProvider() string {
	providers := m.viewProviders()
	if len(providers) == 0 || m.cursorP < 0 || m.cursorP >= len(providers) {
		return ""
	}
	return providers[m.cursorP].Name
}

func (m Model) filteredTargetProviders() []providerRow {
	query := strings.ToLower(strings.TrimSpace(m.pickerQuery))
	byName := map[string]int{}
	for _, name := range []string{"sub2api", "openai", "custom", m.target} {
		if strings.TrimSpace(name) != "" {
			byName[name] += 0
		}
	}
	for _, p := range m.viewProviders() {
		if strings.TrimSpace(p.Name) != "" {
			byName[p.Name] += p.Total
		}
	}
	var out []providerRow
	for name, total := range byName {
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			continue
		}
		out = append(out, providerRow{Name: name, Total: total})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == m.target {
			return true
		}
		if out[j].Name == m.target {
			return false
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m Model) deleteTarget() (scope, label string, ids []string, expected string) {
	switch m.focus {
	case focusProviders:
		provider := m.currentProvider()
		if provider == "" {
			return "", "", nil, ""
		}
		for _, s := range m.allSessions {
			ids = append(ids, s.ID)
		}
		return "provider", "provider " + provider, ids, provider
	case focusProjects:
		if len(m.projects) == 0 || m.cursorG >= len(m.projects) {
			return "", "", nil, ""
		}
		project := m.projects[m.cursorG]
		for _, s := range m.sessions {
			ids = append(ids, s.ID)
		}
		return "project", "project " + project.Name, ids, ""
	case focusSessions:
		ids = m.selectedIDs()
		if len(ids) > 0 {
			return "sessions", "selected sessions", ids, ""
		}
		if len(m.sessions) == 0 || m.cursorS >= len(m.sessions) {
			return "", "", nil, ""
		}
		return "session", "session " + shortDisplayID(m.sessions[m.cursorS].ID), []string{m.sessions[m.cursorS].ID}, ""
	default:
		return "", "", nil, ""
	}
}

func (m *Model) ensurePickerVisible() {
	visible := max(1, m.providerPickerVisibleRows())
	total := len(m.filteredTargetProviders())
	m.pickerCursor = clamp(m.pickerCursor, 0, max(0, total-1))
	m.pickerOffset = ensureVisible(m.pickerCursor, m.pickerOffset, visible, total)
}

func (m *Model) ensureSearchVisible() {
	visible := max(1, m.searchVisibleRows())
	total := len(m.searchResults)
	m.searchCursor = clamp(m.searchCursor, 0, max(0, total-1))
	m.searchOffset = ensureVisible(m.searchCursor, m.searchOffset, visible, total)
}

func (m Model) searchVisibleRows() int {
	width := m.width
	if width <= 0 {
		width = 90
	}
	height := m.height
	if height <= 0 {
		height = 28
	}
	boxWidth, boxHeight := m.searchModalSize(width, height)
	_, contentHeight := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	return max(1, contentHeight-6)
}

func (m Model) providerPickerVisibleRows() int {
	width := m.width
	if width <= 0 {
		width = 90
	}
	height := m.height
	if height <= 0 {
		height = 28
	}
	boxWidth, boxHeight := m.providerPickerSize(width, height)
	_, contentHeight := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	return max(1, contentHeight-6)
}

func (m Model) providerPickerSize(width, height int) (int, int) {
	boxWidth := min(76, max(32, width-8))
	if width < 40 {
		boxWidth = max(20, width)
	}
	boxHeight := min(20, max(12, height-6))
	if height < 14 {
		boxHeight = max(8, height)
	}
	return boxWidth, boxHeight
}

func (m Model) clearConfirmSize(width, height int) (int, int) {
	boxWidth := min(72, max(36, width-10))
	if width < 44 {
		boxWidth = max(24, width)
	}
	boxHeight := min(12, max(10, height-8))
	if height < 12 {
		boxHeight = max(8, height)
	}
	return boxWidth, boxHeight
}

func (m Model) migrateConfirmSize(width, height int) (int, int) {
	boxWidth := min(74, max(38, width-10))
	if width < 44 {
		boxWidth = max(24, width)
	}
	boxHeight := min(12, max(10, height-8))
	if height < 12 {
		boxHeight = max(8, height)
	}
	return boxWidth, boxHeight
}

func (m Model) settingsModalSize(width, height int) (int, int) {
	boxWidth := min(92, max(48, width-16))
	if width < 56 {
		boxWidth = max(28, width)
	}
	boxHeight := min(17, max(14, height-8))
	if height < 16 {
		boxHeight = max(8, height)
	}
	return boxWidth, boxHeight
}

func (m Model) searchModalSize(width, height int) (int, int) {
	boxWidth := min(150, max(72, width-8))
	if width < 64 {
		boxWidth = max(30, width)
	}
	boxHeight := min(32, max(18, height-2))
	if height < 16 {
		boxHeight = max(10, height)
	}
	return boxWidth, boxHeight
}

func (m *Model) run(dry bool) {
	_, ids := m.migrationTarget()
	if len(ids) == 0 {
		m.message = "没有可迁移的会话"
		return
	}
	m.runIDs(dry, ids)
}

func (m *Model) runIDs(dry bool, ids []string) {
	res, err := migrate.Run(m.paths, migrate.Options{
		IDs: ids, Target: m.target, Mode: m.mode, DryRun: dry, RequireFrom: m.providers[m.cursorP].Name,
	})
	if err != nil {
		m.message = err.Error()
		return
	}
	prefix := "apply 完成"
	if dry {
		prefix = "dry-run"
	}
	m.message = prefix + ":\n" + strings.Join(res.Lines, "\n")
	if !dry {
		m.selected = map[string]bool{}
		m.reload()
	}
}

func (m Model) migrationTarget() (label string, ids []string) {
	if m.focus == focusProjects {
		return m.projectMigrationTarget()
	}
	ids = m.selectedIDs()
	if len(ids) > 0 {
		return "selected sessions", ids
	}
	if len(m.sessions) == 0 || m.cursorS < 0 || m.cursorS >= len(m.sessions) {
		return "", nil
	}
	return "current session " + shortDisplayID(m.sessions[m.cursorS].ID), []string{m.sessions[m.cursorS].ID}
}

func (m Model) projectMigrationTarget() (label string, ids []string) {
	if len(m.projects) == 0 || m.cursorG < 0 || m.cursorG >= len(m.projects) {
		return "", nil
	}
	project := m.projects[m.cursorG]
	for _, s := range m.allSessions {
		if project.Key == allProjectsKey || m.sessionProjectRoot(s) == project.Key {
			ids = append(ids, s.ID)
		}
	}
	return "project " + project.Name, ids
}

func (m Model) selectedIDs() []string {
	var ids []string
	for id, ok := range m.selected {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func listSnapshots(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

func bestSearchResult(t codex.Thread, doc searchDoc, query string) (searchResult, bool) {
	best := searchResult{}
	bestOK := false
	if score, ok := fuzzyScore(doc.Title, query); ok {
		best = searchResult{
			Thread:  t,
			Title:   doc.Title,
			Role:    "title",
			Snippet: doc.Title,
			Score:   score + 20,
		}
		bestOK = true
	}
	for _, msg := range doc.Messages {
		score, ok := fuzzyScore(msg.Text, query)
		if !ok {
			continue
		}
		result := searchResult{
			Thread:  t,
			Title:   doc.Title,
			Role:    msg.Role,
			Snippet: snippetForQuery(msg.Text, query, 96),
			Score:   score,
		}
		if !bestOK || result.Score > best.Score {
			best = result
			bestOK = true
		}
	}
	return best, bestOK
}

func (m Model) displayThreadTitle(t codex.Thread) string {
	return displayThreadTitle(t, m.sessionNames)
}

func displayThreadTitle(t codex.Thread, names map[string]string) string {
	if name := strings.TrimSpace(names[t.ID]); name != "" {
		t.Title = name
		t.Preview = ""
	}
	return codex.DisplayThreadTitle(t)
}

func searchDocFromThread(t codex.Thread, names map[string]string) searchDoc {
	return searchDoc{Title: displayThreadTitle(t, names)}
}

func readSearchDocument(t codex.Thread, names map[string]string) searchDoc {
	doc := searchDoc{Title: displayThreadTitle(t, names)}
	info, err := codex.ReadConversationInfo(t.RolloutPath, 2000)
	if err != nil {
		return searchDocFromThread(t, names)
	}
	for _, item := range info.Items {
		if item.Role == "user" || item.Role == "assistant" {
			text := cleanSearchMessageText(item.Text)
			if text != "" {
				doc.Messages = append(doc.Messages, searchMessage{Role: item.Role, Text: text})
			}
		}
	}
	if len(doc.Messages) == 0 {
		return searchDocFromThread(t, names)
	}
	return doc
}

func cleanSearchMessageText(text string) string {
	text = singleLineDisplay(text)
	if idx := strings.Index(text, "<turn_aborted>"); idx >= 0 {
		text = strings.TrimSpace(text[:idx])
		text = strings.TrimRight(text, " ·,;:|-/")
	}
	return strings.TrimSpace(text)
}

func fuzzyScore(text, query string) (int, bool) {
	text = singleLineDisplay(text)
	query = singleLineDisplay(query)
	if query == "" {
		return 1, true
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	if idx := strings.Index(lowerText, lowerQuery); idx >= 0 {
		return 1000 + len([]rune(lowerQuery))*10 - idx, true
	}
	tokens := queryTokens(lowerQuery)
	if len(tokens) > 1 {
		score := 900
		last := -1
		for _, token := range tokens {
			idx := strings.Index(lowerText, token)
			if idx < 0 {
				return 0, false
			}
			score += len([]rune(token)) * 12
			if last >= 0 {
				score -= abs(idx-last) / 8
			}
			last = idx
		}
		return max(1, score), true
	}
	textRunes := []rune(lowerText)
	queryRunes := []rune(lowerQuery)
	q := 0
	last := -1
	gaps := 0
	for i, r := range textRunes {
		if q < len(queryRunes) && r == queryRunes[q] {
			if last >= 0 {
				gaps += i - last - 1
			}
			last = i
			q++
		}
		if q == len(queryRunes) {
			break
		}
	}
	if q != len(queryRunes) {
		return 0, false
	}
	return max(1, 500-gaps-len(textRunes)/12), true
}

func queryTokens(query string) []string {
	var tokens []string
	for _, token := range strings.Fields(query) {
		token = strings.TrimSpace(token)
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func snippetForQuery(text, query string, width int) string {
	text = singleLineDisplay(text)
	if xansi.StringWidth(text) <= width {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(singleLineDisplay(query))
	idx := strings.Index(lowerText, lowerQuery)
	if idx < 0 {
		for _, token := range queryTokens(lowerQuery) {
			idx = strings.Index(lowerText, token)
			if idx >= 0 {
				break
			}
		}
	}
	if idx < 0 {
		return truncate(text, width)
	}
	start := max(0, idx-width/3)
	snippet := xansi.Cut(text, start, start+width)
	if start > 0 {
		snippet = "..." + snippet
	}
	if xansi.StringWidth(snippet) > width {
		snippet = truncate(snippet, width)
	}
	return snippet
}

func fallbackPreview(t codex.Thread) string {
	if s := strings.TrimSpace(t.Preview); s != "" {
		return s
	}
	if s := strings.TrimSpace(t.CWD); s != "" {
		return rel(s)
	}
	return shortDisplayID(t.ID)
}

func rel(path string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return filepath.Clean(path)
}

func projectName(root string) string {
	if root == "" {
		return codex.OrdinaryConversationGroup
	}
	if root == codex.OrdinaryConversationGroup {
		return root
	}
	clean := filepath.Clean(root)
	base := filepath.Base(clean)
	if base == "." || base == "/" {
		return clean
	}
	return base
}

func (m Model) sessionProjectRoot(t codex.Thread) string {
	if m.globalIndex.Projectless[t.ID] {
		return codex.OrdinaryConversationGroup
	}
	if root := filepath.Clean(m.globalIndex.ThreadWorkspaceRoot[t.ID]); root != "." && m.isSavedProjectRoot(root) {
		return root
	}
	cwd := filepath.Clean(t.CWD)
	for _, root := range m.globalIndex.ProjectRoots {
		clean := filepath.Clean(root)
		if pathWithinRoot(cwd, clean) {
			return clean
		}
	}
	return codex.OrdinaryConversationGroup
}

func (m Model) isSavedProjectRoot(root string) bool {
	for _, projectRoot := range m.globalIndex.ProjectRoots {
		if filepath.Clean(projectRoot) == root {
			return true
		}
	}
	return false
}

func pathWithinRoot(path, root string) bool {
	if path == root {
		return true
	}
	if root == "." || root == string(filepath.Separator) {
		return false
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

func projectOrder(roots []string, key string) int {
	cleanKey := filepath.Clean(key)
	for i, root := range roots {
		if filepath.Clean(root) == cleanKey {
			return i
		}
	}
	return len(roots)
}

func (m *Model) ensureOffsets() {
	m.offsetP = ensureVisible(m.cursorP, m.offsetP, m.visibleRows(focusProviders), len(m.viewProviders()))
	m.offsetG = ensureVisible(m.cursorG, m.offsetG, m.visibleRows(focusProjects), len(m.viewProjects()))
	if m.focus == focusRollback {
		m.offsetR = ensureVisible(m.cursorS, m.offsetR, m.visibleRows(focusRollback), len(m.snapshots))
	} else {
		m.offsetS = ensureVisible(m.cursorS, m.offsetS, m.visibleRows(focusSessions), len(m.viewSessions()))
	}
}

func ensureVisible(cursor, offset, visible, total int) int {
	if total <= 0 || visible <= 0 {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	return clamp(offset, 0, max(0, total-visible))
}

func scrollOffset(offset, delta, visible, total int) int {
	if total <= 0 || visible <= 0 {
		return 0
	}
	return clamp(offset+delta, 0, max(0, total-visible))
}

func (m Model) visibleRows(target focus) int {
	layout := m.layout()
	switch target {
	case focusProviders:
		return max(1, layout.providerRows)
	case focusProjects:
		return max(1, layout.projectRows)
	case focusSessions:
		return max(1, layout.sessionRows)
	case focusRollback:
		return max(1, layout.rollbackRows)
	default:
		return 1
	}
}

type layoutInfo struct {
	width          int
	height         int
	headerRows     int
	sidebarWidth   int
	gutterWidth    int
	mainWidth      int
	bodyY          int
	mainHeight     int
	providerHeight int
	projectHeight  int
	sessionHeight  int
	detailHeight   int
	providerRows   int
	projectRows    int
	sessionRows    int
	rollbackRows   int
}

func (m Model) layout() layoutInfo {
	width := m.width
	if width <= 0 {
		width = 90
	}
	height := m.height
	if height <= 0 {
		height = 28
	}
	headerRows := lipglossHeight(m.renderHeader(width))
	footerRows := lipglossHeight(m.renderFooter(width))
	messageRows := lipglossHeight(m.renderMessage(width))
	mainHeight := max(0, height-headerRows-footerRows-messageRows)
	gutterWidth := 1
	if width < 70 {
		gutterWidth = 0
	}
	sidebarWidth := clamp(width/3, 28, 52)
	if width < 90 {
		sidebarWidth = clamp(width/3, 20, 30)
	}
	if width < 56 {
		sidebarWidth = clamp(width/3, 16, 22)
	}
	mainWidth := max(0, width-sidebarWidth-gutterWidth)
	providerHeight := min(mainHeight, min(8, max(4, len(m.viewProviders())+4)))
	projectHeight := max(0, mainHeight-providerHeight-1)
	detailHeight := 0
	sessionHeight := mainHeight
	return layoutInfo{
		width:          width,
		height:         height,
		headerRows:     headerRows,
		sidebarWidth:   sidebarWidth,
		gutterWidth:    gutterWidth,
		mainWidth:      mainWidth,
		bodyY:          headerRows,
		mainHeight:     mainHeight,
		providerHeight: providerHeight,
		projectHeight:  projectHeight,
		sessionHeight:  sessionHeight,
		detailHeight:   detailHeight,
		providerRows:   visibleRowsInPanel(m.panelFor(focusProviders), providerHeight, 2),
		projectRows:    visibleRowsInPanel(m.panelFor(focusProjects), projectHeight, 2),
		sessionRows:    visibleRowsInPanel(m.panelFor(focusSessions), sessionHeight, 3),
		rollbackRows:   max(0, height-headerRows-footerRows-7),
	}
}

func (m Model) hitTest(x, y int) (focus, int, bool) {
	l := m.layout()
	if y < l.bodyY {
		return 0, 0, false
	}
	if x < l.sidebarWidth {
		if y < l.bodyY+l.providerHeight {
			row := y - l.bodyY - 2
			idx := m.offsetP + row
			return focusProviders, idx, row >= 0 && idx >= 0 && idx < len(m.viewProviders())
		}
		projectY := l.bodyY + l.providerHeight + 1
		row := y - projectY - 2
		idx := m.offsetG + row
		return focusProjects, idx, row >= 0 && idx >= 0 && idx < len(m.viewProjects())
	}
	if x < l.sidebarWidth+l.gutterWidth {
		return 0, 0, false
	}
	if y < l.bodyY+l.sessionHeight {
		row := y - l.bodyY - 3
		idx, ok := m.sessionIndexAtVisualRow(row)
		return focusSessions, idx, ok
	}
	return 0, 0, false
}

func (m Model) hitPanel(x, y int) (focus, bool) {
	l := m.layout()
	if y < l.bodyY {
		return 0, false
	}
	if x < l.sidebarWidth {
		if y < l.bodyY+l.providerHeight {
			return focusProviders, true
		}
		projectY := l.bodyY + l.providerHeight + 1
		if y >= projectY && y < projectY+l.projectHeight {
			return focusProjects, true
		}
		return 0, false
	}
	if x < l.sidebarWidth+l.gutterWidth {
		return 0, false
	}
	if y < l.bodyY+l.sessionHeight {
		return focusSessions, true
	}
	return 0, false
}

func (m Model) sessionIndexAtVisualRow(row int) (int, bool) {
	if row < 0 {
		return 0, false
	}
	now := time.Now()
	lastGroup := ""
	visualRow := 0
	sessions := m.viewSessions()
	for i := m.offsetS; i < len(sessions); i++ {
		group := sessionDateLabel(sessions[i].UpdatedAt, now)
		if group != lastGroup {
			if visualRow == row {
				return 0, false
			}
			visualRow++
			lastGroup = group
		}
		if visualRow == row {
			return i, true
		}
		visualRow++
	}
	return 0, false
}

func (m *Model) activateHit(target focus, index int) {
	switch target {
	case focusProviders:
		m.focus = focusProviders
		m.cursorP = index
		if !m.demoMode {
			m.reloadProviderData()
		}
	case focusProjects:
		m.focus = focusProjects
		m.cursorG = index
		if !m.demoMode {
			m.applyProjectFilter()
		} else {
			m.cursorS = 0
			m.offsetS = 0
		}
	case focusSessions:
		m.focus = focusSessions
		m.cursorS = index
	}
}
