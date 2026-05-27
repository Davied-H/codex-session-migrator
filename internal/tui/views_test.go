package tui

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codex-session-migrator/internal/codex"
	"codex-session-migrator/internal/migrate"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	_ "modernc.org/sqlite"
)

func TestViewDoesNotExceedWindowHeight(t *testing.T) {
	for _, tc := range []struct {
		name    string
		width   int
		height  int
		message string
	}{
		{name: "normal", width: 200, height: 45},
		{name: "short", width: 100, height: 18},
		{name: "with message", width: 120, height: 24, message: "dry-run:\nline 1\nline 2"},
		{name: "detail page", width: 120, height: 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel(tc.width, tc.height)
			m.message = tc.message
			if tc.name == "detail page" {
				m.detailOpen = true
				m.detailThread = m.sessions[0]
				m.detail = codex.ConversationInfo{
					LineCount:     3,
					UserMessages:  1,
					AgentMessages: 1,
					Items: []codex.ConversationItem{
						{Role: "user", Text: "hello"},
						{Role: "assistant", Text: "world"},
					},
				}
			}
			if got := lipgloss.Height(m.View()); got > tc.height {
				t.Fatalf("view height = %d, want <= %d", got, tc.height)
			}
			assertViewFitsWidth(t, m.View(), tc.width)
		})
	}
}

func TestViewDoesNotWrapLongRows(t *testing.T) {
	m := testModel(88, 24)
	m.providers = []providerRow{
		{Name: "very-long-provider-name-that-used-to-wrap-counts", Total: 123456},
		{Name: "openai", Total: 1},
	}
	m.projects = []projectRow{
		{Key: allProjectsKey, Name: "全部项目", Count: 123456},
		{Key: "/Users/dong/Develop/Code/project/estha/extremely/deep/worktree/path", Name: "estha-go-api", Root: "/Users/dong/Develop/Code/project/estha/extremely/deep/worktree/path", Count: 99},
	}
	m.sessions = []codex.Thread{
		{
			ID:            "long",
			RolloutPath:   "/Users/dong/.codex/sessions/2026/05/25/rollout.jsonl",
			UpdatedAt:     1770000000,
			ModelProvider: "openai",
			CWD:           "/Users/dong/Develop/Code/project/estha/extremely/deep/worktree/path",
			Title:         "The following is the Codex agent history whose request action you are assessing. Treat the transcript as very long text.",
		},
	}
	m.allSessions = m.sessions

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, want <= %d", got, m.height)
	}
	assertViewFitsWidth(t, view, m.width)
}

func TestSessionRowsUseSingleLineTitles(t *testing.T) {
	m := testModel(110, 24)
	m.sessions = []codex.Thread{
		{
			ID:            "stack-title",
			UpdatedAt:     1770000000,
			ModelProvider: "openai",
			Title:         "ERROR:CONSOLE:1 Cannot read properties of undefined\n    at App.tsx:68:52\n    at react-dom_client.js:997:72",
		},
		{
			ID:            "preview-fallback",
			UpdatedAt:     1770000001,
			ModelProvider: "openai",
			Preview:       "兼容 gpt response 接口\r\n{\"error\":{\"message\":\"Images API is not supported\"}}\t后续内容",
		},
		{
			ID:            "mixed-long",
			UpdatedAt:     1770000002,
			ModelProvider: "openai",
			Title:         "中英文 mixed https://example.com/this/is/a/very/long/path/that/has/no/spaces/and-keeps-going",
		},
	}
	m.allSessions = m.sessions

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, want <= %d\n%s", got, m.height, view)
	}
	assertViewFitsWidth(t, view, m.width)
	for _, unexpected := range []string{"at App.tsx", "react-dom_client"} {
		if strings.Contains(view, unexpected) {
			t.Fatalf("session list leaked multiline detail %q:\n%s", unexpected, view)
		}
	}
	if strings.Contains(view, "\t") || strings.Contains(view, "\r") {
		t.Fatalf("session list should not contain raw control whitespace:\n%q", view)
	}
	if !strings.Contains(view, "兼容 gpt response 接口") {
		t.Fatalf("session list should use preview fallback:\n%s", view)
	}
}

func TestSessionRowsPreferSessionIndexThreadName(t *testing.T) {
	m := testModel(100, 24)
	m.sessionNames = map[string]string{
		m.sessions[0].ID: "排查日志代码问题",
	}
	m.sessions[0].Title = "[$20k-es-log-debugging](/Users/dong/Config/agents/skills/20k-es-log-debugging/SKILL.md) 结合日志代码排查下这个问题"
	m.allSessions = m.sessions
	m.focus = focusSessions

	view := m.View()
	if !strings.Contains(view, "排查日志代码问题") {
		t.Fatalf("view missing session_index title: %q", view)
	}
	if strings.Contains(view, "20k-es-log-debugging") || strings.Contains(view, "SKILL.md") {
		t.Fatalf("view leaked sqlite raw title: %q", view)
	}
}

func TestArchivedSessionRowsShowColoredBadge(t *testing.T) {
	m := testModel(88, 24)
	m.sessions = []codex.Thread{
		{
			ID:            "archived",
			UpdatedAt:     1770000000,
			ModelProvider: "openai",
			Title:         "archived session title",
			Archived:      true,
		},
	}
	m.allSessions = m.sessions

	view := m.View()
	if !strings.Contains(view, "归档") {
		t.Fatalf("archived session should show badge: %q", view)
	}
	if !strings.Contains(view, "\x1b[") {
		t.Fatalf("archived badge should be styled with ANSI color: %q", view)
	}
	assertViewFitsWidth(t, view, m.width)
}

func TestProjectsRenderOnlyProjectName(t *testing.T) {
	m := testModel(96, 24)
	m.focus = focusProjects
	m.projects = []projectRow{
		{Key: "/Users/dong/Desktop/Projects/example", Name: "example", Root: "/Users/dong/Desktop/Projects/example", Count: 3},
	}

	view := m.renderProjects(40, 8)
	if strings.Contains(view, "~/Desktop/Projects") || strings.Contains(view, "/Users/dong/Desktop/Projects") {
		t.Fatalf("project view should not include root path: %q", view)
	}
	if !strings.Contains(view, "example") {
		t.Fatalf("project view should include project name: %q", view)
	}
}

func TestFocusedPanelTitleIsHighlighted(t *testing.T) {
	m := testModel(100, 28)
	m.focus = focusProjects

	providers := m.renderProviders(32, 8)
	projects := m.renderProjects(32, 8)
	if strings.Contains(providers, "> Providers") {
		t.Fatalf("providers title should not be focused: %q", providers)
	}
	if !strings.Contains(projects, "> Projects") {
		t.Fatalf("projects title should be focused: %q", projects)
	}
}

func TestRebuildProjectsUsesProjectMarkersOnly(t *testing.T) {
	m := testModel(120, 30)
	m.globalIndex = codex.GlobalIndex{
		Projectless:         map[string]bool{},
		ThreadWorkspaceRoot: map[string]string{},
		ProjectRoots:        []string{"/Users/dong/Desktop/codex-session-migrator"},
	}
	m.allSessions = []codex.Thread{
		{ID: "real", CWD: "/Users/dong/Desktop/codex-session-migrator", UpdatedAt: 3},
		{ID: "cwd-only-project", CWD: "/Users/dong/Desktop/looks-like-project", UpdatedAt: 2},
		{ID: "numeric", CWD: "/Users/dong/Desktop/6-6", UpdatedAt: 1},
	}

	m.rebuildProjects()

	counts := map[string]int{}
	for _, p := range m.projects {
		counts[p.Name] = p.Count
	}
	if counts["codex-session-migrator"] != 1 {
		t.Fatalf("real project missing from projects: %+v", m.projects)
	}
	if counts[codex.OrdinaryConversationGroup] != 2 {
		t.Fatalf("sessions without project markers should be ordinary: %+v", m.projects)
	}
	for _, unexpected := range []string{"looks-like-project", "6-6"} {
		if counts[unexpected] != 0 {
			t.Fatalf("unexpected project %q in projects: %+v", unexpected, m.projects)
		}
	}
}

func TestRebuildProjectsIncludesSavedProjectsWithNoSessions(t *testing.T) {
	m := testModel(120, 30)
	m.globalIndex = codex.GlobalIndex{
		Projectless:         map[string]bool{},
		ThreadWorkspaceRoot: map[string]string{},
		ProjectRoots: []string{
			"/Users/dong/Desktop/codex-session-migrator",
			"/Users/dong/Develop/Code/work/mbl-ai-workbench",
		},
	}
	m.allSessions = []codex.Thread{
		{ID: "real", CWD: "/Users/dong/Desktop/codex-session-migrator/internal", UpdatedAt: 3},
	}

	m.rebuildProjects()

	counts := map[string]int{}
	for _, p := range m.projects {
		counts[p.Name] = p.Count
	}
	if counts["codex-session-migrator"] != 1 {
		t.Fatalf("real project count missing: %+v", m.projects)
	}
	if _, ok := counts["mbl-ai-workbench"]; !ok {
		t.Fatalf("saved project with no sessions missing: %+v", m.projects)
	}
}

func TestSelectCurrentWorkspaceProjectFiltersSessions(t *testing.T) {
	m := testModel(120, 30)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	m.globalIndex = codex.GlobalIndex{
		Projectless:         map[string]bool{},
		ThreadWorkspaceRoot: map[string]string{},
		ProjectRoots:        []string{wd},
	}
	m.allSessions = []codex.Thread{
		{ID: "current", CWD: wd, UpdatedAt: 2},
		{ID: "other", CWD: filepath.Dir(wd), UpdatedAt: 1},
	}
	m.rebuildProjects()
	m.selectCurrentWorkspaceProject()

	if len(m.sessions) != 1 || m.sessions[0].ID != "current" {
		t.Fatalf("sessions = %+v, want current project only", m.sessions)
	}
	if m.projects[m.cursorG].Key == allProjectsKey {
		t.Fatalf("cursor stayed on all projects: %+v", m.projects[m.cursorG])
	}
}

func TestProviderPickerFiltersAndFits(t *testing.T) {
	m := testModel(100, 30)
	m.providers = []providerRow{
		{Name: "openai", Total: 687},
		{Name: "sub2api", Total: 7},
		{Name: "custom", Total: 256},
	}
	m.openProviderPicker()
	m.pickerQuery = "sub"
	m.ensurePickerVisible()

	choices := m.filteredTargetProviders()
	if len(choices) == 0 || choices[0].Name != "sub2api" {
		t.Fatalf("filtered choices = %+v, want sub2api first", choices)
	}
	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("picker view height = %d, want <= %d", got, m.height)
	}
	assertViewFitsWidth(t, view, m.width)
	if !strings.Contains(view, "选择目标 Provider") || !strings.Contains(view, "sub2api") {
		t.Fatalf("picker view missing expected content: %q", view)
	}
}

func TestHeaderAlwaysShowsTargetProvider(t *testing.T) {
	m := testModel(72, 20)
	m.target = "dong_s_sub2api"

	header := m.renderHeader(m.width)
	if !strings.Contains(header, "目标") || !strings.Contains(header, "dong_s_sub2api") {
		t.Fatalf("header should show target provider: %q", header)
	}
	assertViewFitsWidth(t, header, m.width)
}

func TestAppTitleUsesAnimatedGradient(t *testing.T) {
	first := renderAppTitle(80, 0)
	next := renderAppTitle(80, 1)
	if !strings.Contains(xansi.Strip(first), "Codex Session Migrator") {
		t.Fatalf("title should show full tool name: %q", first)
	}
	if !strings.Contains(first, "\x1b[") {
		t.Fatalf("title should be rendered with ANSI color: %q", first)
	}
	if first == next {
		t.Fatalf("title should change across animation frames: %q", first)
	}
}

func TestFooterShowsRollbackShortcut(t *testing.T) {
	m := testModel(120, 24)

	footer := m.renderFooter(m.width)
	if !strings.Contains(footer, "回滚") {
		t.Fatalf("footer should show rollback shortcut: %q", footer)
	}
	if !strings.Contains(footer, "Ctrl+E") {
		t.Fatalf("footer should show demo shortcut: %q", footer)
	}
}

func TestDemoModeUsesMockDataAndHidesRealNames(t *testing.T) {
	m := testModel(120, 28)
	m.projects = []projectRow{{Key: "/real/private-project", Name: "private-project", Root: "/real/private-project", Count: 1}}
	m.sessions = []codex.Thread{{ID: "real", UpdatedAt: 1770000000, CWD: "/real/private-project", Title: "真实客户会话标题"}}
	m.allSessions = m.sessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)

	if !m.demoMode {
		t.Fatal("Ctrl+E should enable demo mode")
	}
	view := xansi.Strip(m.View())
	for _, hidden := range []string{"private-project", "真实客户会话标题"} {
		if strings.Contains(view, hidden) {
			t.Fatalf("demo mode leaked real data %q: %q", hidden, view)
		}
	}
	for _, want := range []string{"Demo", "customer-portal", "梳理客户门户登录流程"} {
		if !strings.Contains(view, want) {
			t.Fatalf("demo mode missing mock data %q: %q", want, view)
		}
	}
}

func TestDemoModeBlocksRealSessionActions(t *testing.T) {
	m := testModel(120, 28)
	m.demoMode = true
	m.focus = focusSessions

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("demo migrate should not return a command")
	}
	if !strings.Contains(m.message, "演示模式不会迁移真实会话") {
		t.Fatalf("demo migrate message = %q", m.message)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("demo open markdown should not return a command")
	}
	if !strings.Contains(m.message, "演示模式不打开真实 Markdown") {
		t.Fatalf("demo open message = %q", m.message)
	}
}

func TestClearConfirmModalFits(t *testing.T) {
	m := testModel(90, 24)
	m.clearConfirm = true
	m.clearLabel = "provider openai"
	m.clearCount = 42
	m.clearExpected = "openai"

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("clear modal height = %d, want <= %d", got, m.height)
	}
	assertViewFitsWidth(t, view, m.width)
	for _, want := range []string{"危险操作", "openai", "42"} {
		if !strings.Contains(view, want) {
			t.Fatalf("clear modal missing %q: %q", want, view)
		}
	}
}

func TestMigrateConfirmModalFits(t *testing.T) {
	m := testModel(90, 24)
	m.migrateConfirm = true
	m.migrateLabel = "selected sessions"
	m.migrateCount = 3

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("migrate modal height = %d, want <= %d", got, m.height)
	}
	assertViewFitsWidth(t, view, m.width)
	for _, want := range []string{"确认操作", "selected sessions", "3", "openai", "sub2api", "retag"} {
		if !strings.Contains(view, want) {
			t.Fatalf("migrate modal missing %q: %q", want, view)
		}
	}
}

func TestCloneConfirmModalNamesCloneAction(t *testing.T) {
	m := testModel(90, 24)
	m.mode = migrate.ModeClone
	m.migrateConfirm = true
	m.migrateLabel = "selected sessions"
	m.migrateCount = 3

	view := m.View()
	for _, want := range []string{"克隆 Sessions", "将克隆 3 条 session", "保留项目归属", "确认克隆"} {
		if !strings.Contains(view, want) {
			t.Fatalf("clone modal missing %q: %q", want, view)
		}
	}
}

func TestMigrationTargetUsesSelectedOrCurrentSession(t *testing.T) {
	m := testModel(100, 28)
	m.sessions = append(m.sessions, codex.Thread{ID: "second", ModelProvider: "openai"})
	m.allSessions = m.sessions
	m.cursorS = 1

	label, ids := m.migrationTarget()
	if label != "current session second" || len(ids) != 1 || ids[0] != "second" {
		t.Fatalf("fallback migration target = %q %+v, want current second", label, ids)
	}

	m.selected = map[string]bool{m.sessions[0].ID: true}
	label, ids = m.migrationTarget()
	if label != "selected sessions" || len(ids) != 1 || ids[0] != m.sessions[0].ID {
		t.Fatalf("selected migration target = %q %+v, want selected first session", label, ids)
	}
}

func TestMigrationTargetUsesFocusedProjectSessions(t *testing.T) {
	m := testModel(100, 28)
	projectRoot := "/tmp/project"
	otherRoot := "/tmp/other"
	m.globalIndex = codex.GlobalIndex{
		ProjectRoots: []string{projectRoot, otherRoot},
		ThreadWorkspaceRoot: map[string]string{
			"project-one": projectRoot,
			"project-two": projectRoot,
			"other":       otherRoot,
		},
	}
	m.allSessions = []codex.Thread{
		{ID: "project-one", CWD: projectRoot, UpdatedAt: 3},
		{ID: "project-two", CWD: projectRoot, UpdatedAt: 2},
		{ID: "other", CWD: otherRoot, UpdatedAt: 1},
	}
	m.selected = map[string]bool{"other": true}
	m.rebuildProjects()
	for i, project := range m.projects {
		if project.Key == projectRoot {
			m.cursorG = i
			break
		}
	}
	m.focus = focusProjects

	label, ids := m.migrationTarget()
	if label != "project project" || len(ids) != 2 || ids[0] != "project-one" || ids[1] != "project-two" {
		t.Fatalf("project migration target = %q %+v, want focused project sessions", label, ids)
	}
}

func TestAKeyTogglesVisibleSessionSelection(t *testing.T) {
	m := testModel(100, 28)
	m.focus = focusSessions
	m.sessions = append(m.sessions, codex.Thread{ID: "second", ModelProvider: "openai"})
	m.allSessions = m.sessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if got := len(m.selectedIDs()); got != 2 {
		t.Fatalf("selected after first a = %d, want 2", got)
	}
	if !strings.Contains(m.message, "已选择") {
		t.Fatalf("first a message = %q, want selected message", m.message)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if got := len(m.selectedIDs()); got != 0 {
		t.Fatalf("selected after second a = %d, want 0", got)
	}
	if !strings.Contains(m.message, "已取消选择") {
		t.Fatalf("second a message = %q, want deselected message", m.message)
	}
}

func TestMouseWheelScrollsSessionsWithoutChangingSelection(t *testing.T) {
	m := testModel(100, 18)
	m.focus = focusSessions
	m.sessions = nil
	for i := 0; i < 20; i++ {
		m.sessions = append(m.sessions, codex.Thread{ID: "session-" + string(rune('a'+i)), UpdatedAt: 1770000000 + int64(i)})
	}
	m.allSessions = m.sessions
	m.cursorS = 2

	updated, _ := m.Update(tea.MouseMsg{X: 60, Y: 6, Type: tea.MouseWheelDown, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.cursorS != 2 {
		t.Fatalf("mouse wheel changed session cursor = %d, want 2", m.cursorS)
	}
	if m.focus != focusSessions {
		t.Fatalf("mouse wheel changed focus = %v, want sessions", m.focus)
	}
	if m.offsetS == 0 {
		t.Fatalf("mouse wheel should scroll session offset")
	}
}

func TestMouseWheelScrollsProjectsWithoutChangingSelection(t *testing.T) {
	m := testModel(100, 18)
	m.focus = focusProjects
	m.projects = nil
	for i := 0; i < 20; i++ {
		m.projects = append(m.projects, projectRow{Key: "project", Name: "project", Count: i})
	}
	m.cursorG = 2

	updated, _ := m.Update(tea.MouseMsg{X: 4, Y: 12, Type: tea.MouseWheelDown, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = updated.(Model)

	if m.cursorG != 2 {
		t.Fatalf("mouse wheel changed project cursor = %d, want 2", m.cursorG)
	}
	if m.focus != focusProjects {
		t.Fatalf("mouse wheel changed focus = %v, want projects", m.focus)
	}
	if m.offsetG == 0 {
		t.Fatalf("mouse wheel should scroll project offset")
	}
}

func TestDeleteTargetByFocus(t *testing.T) {
	m := testModel(100, 28)
	m.providers = []providerRow{{Name: "openai", Total: 2}}
	m.allSessions = append(m.allSessions, codex.Thread{ID: "second", ModelProvider: "openai"})
	m.sessions = m.allSessions

	m.focus = focusProviders
	scope, label, ids, expected := m.deleteTarget()
	if scope != "provider" || label != "provider openai" || len(ids) != 2 || expected != "openai" {
		t.Fatalf("provider delete target = %q %q %+v %q", scope, label, ids, expected)
	}

	m.focus = focusSessions
	m.selected = map[string]bool{"second": true}
	scope, label, ids, expected = m.deleteTarget()
	if scope != "sessions" || label != "selected sessions" || len(ids) != 1 || ids[0] != "second" || expected != "" {
		t.Fatalf("session delete target = %q %q %+v %q", scope, label, ids, expected)
	}
}

func TestSettingsModalFitsAndShowsCommonConfig(t *testing.T) {
	m := testModel(100, 28)
	m.includeA = true
	m.settingsOpen = true

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("settings height = %d, want <= %d", got, m.height)
	}
	assertViewFitsWidth(t, view, m.width)
	for _, want := range []string{"Settings", "配置", "状态", "说明", "显示归档", "显示子代理", "目标 Provider", "迁移模式", "清理归档"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings missing %q: %q", want, view)
		}
	}
	for _, want := range []string{"Codex", "Providers", "Sessions"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings modal should keep main view visible, missing %q: %q", want, view)
		}
	}
}

func TestSettingsClearArchivedOpensConfirmForArchivedSessions(t *testing.T) {
	paths := newTUITestDB(t)
	m := New(paths)
	m.settingsCursor = 4

	m.activateSetting()

	if !m.clearConfirm {
		t.Fatal("clear archived should open delete confirmation")
	}
	if m.clearScope != "archived" || m.clearLabel != "archived sessions" {
		t.Fatalf("clear archived scope = %q label = %q", m.clearScope, m.clearLabel)
	}
	if m.clearCount != 1 || len(m.clearIDs) != 1 || m.clearIDs[0] != "archived" {
		t.Fatalf("clear archived ids = %d %+v, want archived only", m.clearCount, m.clearIDs)
	}
	if m.clearExpected != "" {
		t.Fatalf("clear archived should not require typed provider confirmation: %q", m.clearExpected)
	}
}

func TestAbsoluteUpdatedStringShowsDateAndTime(t *testing.T) {
	got := absoluteUpdatedString(1770000000)
	if got != time.Unix(1770000000, 0).Format("2006-01-02 15:04") {
		t.Fatalf("absoluteUpdatedString = %q", got)
	}
}

func TestSessionRowsGroupByDateAndIndentTimeBeforeTitle(t *testing.T) {
	m := testModel(118, 28)
	view := xansi.Strip(m.renderSessions(90, 10))
	date := sessionDateLabel(m.sessions[0].UpdatedAt, time.Now())
	updated := sessionTimeString(m.sessions[0].UpdatedAt)
	title := m.displayThreadTitle(m.sessions[0])
	if !strings.Contains(view, "时间") || !strings.Contains(view, "标题") || !strings.Contains(view, date) {
		t.Fatalf("session list should show time/title headers: %q", view)
	}
	if !strings.Contains(view, "[ ] "+updated+" "+title) {
		t.Fatalf("session row should show indented time before title: %q", view)
	}
}

func TestSessionRowsShowTodayForCurrentDateGroup(t *testing.T) {
	m := testModel(118, 28)
	m.sessions[0].UpdatedAt = time.Now().Unix()
	m.allSessions = m.sessions

	view := xansi.Strip(m.renderSessions(90, 10))
	if !strings.Contains(view, "Today") {
		t.Fatalf("today session group should show Today: %q", view)
	}
}

func TestSessionVisualRowsDoNotSelectDateHeaders(t *testing.T) {
	m := testModel(118, 28)
	m.sessions = []codex.Thread{
		{ID: "first", UpdatedAt: time.Now().Unix()},
		{ID: "second", UpdatedAt: time.Now().Add(-24 * time.Hour).Unix()},
	}

	if _, ok := m.sessionIndexAtVisualRow(0); ok {
		t.Fatal("date header should not map to a session")
	}
	if idx, ok := m.sessionIndexAtVisualRow(1); !ok || idx != 0 {
		t.Fatalf("first session visual row = %d %v, want 0 true", idx, ok)
	}
	if _, ok := m.sessionIndexAtVisualRow(2); ok {
		t.Fatal("second date header should not map to a session")
	}
	if idx, ok := m.sessionIndexAtVisualRow(3); !ok || idx != 1 {
		t.Fatalf("second session visual row = %d %v, want 1 true", idx, ok)
	}
}

func TestSearchModalMatchesConversationAndShowsPreview(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout.jsonl")
	body := `{"type":"response_item","payload":{"item":{"type":"message","role":"developer","content":[{"type":"input_text","text":"hidden rules"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"user","content":[{"type":"input_text","text":"please migrate this session"}]}}}` + "\n" +
		`{"type":"response_item","payload":{"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant answer includes markdown preview"}]}}}` + "\n"
	if err := os.WriteFile(rollout, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := testModel(100, 28)
	m.sessions[0].RolloutPath = rollout
	m.allSessions = m.sessions

	m.openSearchModal()
	m.searchDocs = map[string]searchDoc{}
	m.searchDocs[m.sessions[0].ID] = readSearchDocument(m.sessions[0], nil)
	m.searchQuery = "markdown"
	m.refreshSearchResults()

	if len(m.searchResults) != 1 {
		t.Fatalf("search results = %d, want 1", len(m.searchResults))
	}
	if m.searchResults[0].Role != "assistant" || !strings.Contains(m.searchResults[0].Snippet, "markdown") {
		t.Fatalf("unexpected search result: %+v", m.searchResults[0])
	}
	view := m.View()
	for _, want := range []string{"搜索 Sessions", "assistant answer includes", "markdown", "Codex"} {
		if !strings.Contains(view, want) {
			t.Fatalf("search modal missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "hidden rules") {
		t.Fatalf("search modal should not show developer text: %q", view)
	}
}

func TestSearchModalUsesSingleStructuredRows(t *testing.T) {
	m := testModel(118, 28)
	root := "/Users/dong/Documents/Codex"
	m.globalIndex = codex.GlobalIndex{
		Projectless:         map[string]bool{},
		ThreadWorkspaceRoot: map[string]string{m.sessions[0].ID: root},
		ProjectRoots:        []string{root},
	}
	m.openSearchModal()
	m.searchQuery = "优化"
	m.refreshSearchResults()

	view := m.View()
	updated := absoluteUpdatedString(m.sessions[0].UpdatedAt)
	for _, want := range []string{"更新", "项目", "命中内容", "类型", "Codex", updated} {
		if !strings.Contains(view, want) {
			t.Fatalf("search modal missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "\n    帮我优化") {
		t.Fatalf("search result should stay on one structured row: %q", view)
	}
	assertViewFitsWidth(t, view, m.width)
}

func TestSearchMultiWordQueryRequiresEveryWord(t *testing.T) {
	if _, ok := fuzzyScore("The user interrupted the previous turn on purpose", "user not found"); ok {
		t.Fatal("multi-word search should not match text unless every word appears")
	}
	if _, ok := fuzzyScore("login failed: user not found in database", "user not found"); !ok {
		t.Fatal("multi-word search should match when every word appears")
	}
}

func TestCleanSearchMessageTextDropsTurnAbortedNoise(t *testing.T) {
	got := cleanSearchMessageText("检查接口 · <turn_aborted> The user interrupted the previous turn on purpose.")
	if got != "检查接口" {
		t.Fatalf("cleanSearchMessageText = %q, want %q", got, "检查接口")
	}
}

func TestNewHidesArchivedByDefault(t *testing.T) {
	m := New(codex.NewPaths(t.TempDir()))
	if m.includeA {
		t.Fatal("includeA should default to false")
	}
}

func TestArchivedToggleReloadsVisibleData(t *testing.T) {
	paths := newTUITestDB(t)
	m := New(paths)

	if m.includeA {
		t.Fatal("archived sessions should be hidden by default")
	}
	if got := len(m.providers); got != 1 {
		t.Fatalf("providers = %d, want 1", got)
	}
	if got := m.providers[0].Total; got != 1 {
		t.Fatalf("provider total = %d, want only non-archived count 1", got)
	}
	if got := len(m.allSessions); got != 1 {
		t.Fatalf("allSessions = %d, want 1", got)
	}
	if m.allSessions[0].ID != "active" || m.allSessions[0].Archived {
		t.Fatalf("default sessions = %+v, want only active", m.allSessions)
	}

	m.settingsCursor = 0
	m.activateSetting()

	if !m.includeA {
		t.Fatal("archived sessions should be visible after toggle")
	}
	if got := m.providers[0].Total; got != 2 {
		t.Fatalf("provider total after toggle = %d, want 2", got)
	}
	if got := len(m.allSessions); got != 2 {
		t.Fatalf("allSessions after toggle = %d, want 2", got)
	}
}

func newTUITestDB(t *testing.T) codex.Paths {
	t.Helper()
	home := t.TempDir()
	paths := codex.NewPaths(home)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`create table threads (
		id text primary key,
		rollout_path text,
		created_at integer,
		updated_at integer,
		updated_at_ms integer,
		source text,
		model_provider text,
		cwd text,
		title text,
		archived integer,
		thread_source text,
		preview text
	)`)
	if err != nil {
		t.Fatal(err)
	}
	activeRollout := filepath.Join(home, "active.jsonl")
	archivedRollout := filepath.Join(home, "archived.jsonl")
	if err := os.WriteFile(activeRollout, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivedRollout, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`insert into threads values
		('active', ?, 1, 2, 2, '', 'openai', '/tmp/project', 'active title', 0, 'user', ''),
		('archived', ?, 1, 3, 3, '', 'openai', '/tmp/project', 'archived title', 1, 'user', '')`, activeRollout, archivedRollout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GlobalState, []byte(`{
		"projectless-thread-ids": [],
		"thread-workspace-root-hints": {"active": "/tmp/project", "archived": "/tmp/project"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return paths
}

func assertViewFitsWidth(t *testing.T, view string, width int) {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width = %d, want <= %d: %q", i+1, got, width, line)
		}
	}
}

func testModel(width, height int) Model {
	threads := []codex.Thread{
		{
			ID:            "019e5e8b-2eb9-7461-9dfa-979f8e4ec932",
			RolloutPath:   "/Users/dong/.codex/sessions/2026/05/25/rollout-2026-05-25T17-50-40-019e5e8b-2eb9-7461-9dfa-979f8e4ec932.jsonl",
			UpdatedAt:     1770000000,
			ModelProvider: "openai",
			CWD:           "/Users/dong/Documents/Codex/2026-05-25/codex-skill",
			Title:         "帮我优化 TUI 并修复顶出屏幕的问题",
		},
	}
	return Model{
		diag: codex.Diagnostics{
			DBExists:         true,
			HasModelProvider: true,
			Integrity:        "ok",
		},
		providers:   []providerRow{{Name: "openai", Total: 1}},
		projects:    []projectRow{{Key: allProjectsKey, Name: "全部项目", Count: 1}},
		allSessions: threads,
		sessions:    threads,
		selected:    map[string]bool{},
		target:      "sub2api",
		mode:        migrate.ModeRetag,
		includeA:    true,
		width:       width,
		height:      height,
	}
}
