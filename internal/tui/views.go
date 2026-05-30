package tui

import (
	"fmt"
	"strings"
	"time"

	"codex-session-migrator/internal/migrate"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
}

var (
	colorText    = lipgloss.Color("252")
	colorMuted   = lipgloss.Color("245")
	colorDim     = lipgloss.Color("240")
	colorBorder  = lipgloss.Color("238")
	colorActive  = lipgloss.Color("81")
	colorWarn    = lipgloss.Color("214")
	colorOK      = lipgloss.Color("114")
	colorBad     = lipgloss.Color("203")
	colorSurface = lipgloss.Color("236")

	baseStyle  = lipgloss.NewStyle().Foreground(colorText)
	titleStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true)
	appTitleAccentStyle = lipgloss.NewStyle().
				Foreground(colorActive).
				Bold(true)
	appSubtitleStyle = lipgloss.NewStyle().
				Foreground(colorDim)
	statusPillStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorSurface).
			Padding(0, 1)
	okPillStyle = statusPillStyle.Copy().
			Foreground(colorOK)
	warnPillStyle = statusPillStyle.Copy().
			Foreground(colorWarn)
	badPillStyle = statusPillStyle.Copy().
			Foreground(colorBad).
			Bold(true).
			Padding(0, 1)
	mutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	dimStyle     = lipgloss.NewStyle().Foreground(colorDim)
	keyStyle     = lipgloss.NewStyle().Foreground(colorActive).Bold(true)
	projectStyle = lipgloss.NewStyle().
			Foreground(colorWarn).
			Bold(true)
	dateHeaderStyle = lipgloss.NewStyle().
			Foreground(colorWarn).
			Bold(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)
	activePanelStyle = panelStyle.Copy().
				BorderForeground(colorActive)
	errorPanelStyle = panelStyle.Copy().
			BorderForeground(colorBad)
	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)
	errorTitleStyle = lipgloss.NewStyle().
			Foreground(colorBad).
			Bold(true)
	errorBodyStyle = lipgloss.NewStyle().
			Foreground(colorBad)
	activePanelTitleStyle = lipgloss.NewStyle().
				Foreground(colorActive).
				Bold(true)

	rowStyle       = lipgloss.NewStyle()
	activeRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("31")).
			Bold(true)
	selectedRowStyle = lipgloss.NewStyle().
				Foreground(colorActive)

	badgeStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("232")).
			Background(colorMuted).
			Bold(true)
	okBadgeStyle       = badgeStyle.Copy().Background(colorOK)
	warnBadgeStyle     = badgeStyle.Copy().Background(colorWarn)
	badBadgeStyle      = badgeStyle.Copy().Background(colorBad)
	archivedBadgeStyle = badgeStyle.Copy().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("99"))
)

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 90
	}
	height := m.height
	if height <= 0 {
		height = 28
	}
	if m.errorMessage != "" {
		return m.renderErrorModal(width, height)
	}
	if m.onboardingOpen {
		return m.renderOnboardingModal(width, height)
	}
	if m.clearConfirm {
		return m.renderClearConfirm(width, height)
	}
	if m.migrateConfirm {
		return m.renderMigrateConfirm(width, height)
	}
	if m.pickerOpen {
		return m.renderProviderPicker(width, height)
	}
	if m.searchOpen {
		return m.renderSearchModal(width, height)
	}
	if m.inputMode != "" {
		return m.renderInput(width)
	}
	if m.detailOpen {
		return m.renderConversationDetail(width, height)
	}
	if m.settingsOpen {
		return m.renderSettingsModal(width, height)
	}
	return m.renderMain(width, height)
}

func (m Model) renderErrorModal(width, height int) string {
	base := m.renderMain(width, height)
	boxWidth, boxHeight := m.errorModalSize(width, height)
	contentWidth, contentHeight := panelContentSize(errorPanelStyle, boxWidth, boxHeight)
	title := m.errorTitle
	if strings.TrimSpace(title) == "" {
		title = "操作失败"
	}
	bodyWidth := max(1, contentWidth-2)
	bodyRows := wrapText(m.errorMessage, bodyWidth)
	maxBodyRows := max(1, contentHeight-5)
	if len(bodyRows) > maxBodyRows {
		bodyRows = append(bodyRows[:maxBodyRows-1], "...")
	}
	rows := []string{
		badBadgeStyle.Render("ERROR") + " " + errorTitleStyle.Render(title) + padRight("", max(1, contentWidth-lipgloss.Width(title)-10)) + keyStyle.Render("Esc"),
		dimStyle.Render(strings.Repeat("─", contentWidth)),
		"",
	}
	for _, row := range bodyRows {
		rows = append(rows, errorBodyStyle.Render(truncate(row, bodyWidth)))
	}
	rows = append(rows, "", dimStyle.Render("Enter/Esc 关闭"))
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	box := renderPanel(errorPanelStyle, boxWidth, boxHeight, strings.Join(rows, "\n"))
	left, top := modalOrigin(width, height, boxWidth, boxHeight)
	return baseStyle.Render(placeOverlay(base, box, width, height, left, top))
}

type onboardingStep struct {
	Title string
	Body  []string
	Hint  string
}

func onboardingSteps() []onboardingStep {
	return []onboardingStep{
		{
			Title: "先确认来源 Provider",
			Body: []string{
				"左侧 Providers 是当前 Codex 本地会话的来源分组。",
				"用 ↑/↓ 选择要迁移的 provider，Enter 会进入项目分组。",
			},
			Hint: "快捷键：p 回到 Providers，Tab 在面板间切换",
		},
		{
			Title: "再缩小到项目或会话",
			Body: []string{
				"Projects 会按 Codex 记录的工作区分组，默认优先当前项目。",
				"进入 Sessions 后可以用 Space 选择单条会话，a 选择当前列表。",
			},
			Hint: "快捷键：g 看项目，/ 搜索标题和对话内容",
		},
		{
			Title: "先 dry-run，再迁移",
			Body: []string{
				"d 只预览将要写入的内容，不会修改数据库或 rollout。",
				"确认范围和目标 provider 后，再按 m 打开迁移确认框。",
			},
			Hint: "目标 provider 用 e 选择，模式用 c 切换 retag / clone",
		},
		{
			Title: "安全退出和回滚",
			Body: []string{
				"真正 apply 前会创建 snapshot，之后可以从 r 进入 rollback。",
				"删除和迁移都会有确认框；不确定时先用演示模式练习。",
			},
			Hint: "快捷键：Ctrl+E 演示，r 回滚，q 退出，? 重新打开本引导",
		},
	}
}

func (m Model) renderOnboardingModal(width, height int) string {
	baseModel := m
	baseModel.onboardingOpen = false
	base := baseModel.renderMain(width, height)
	boxWidth, boxHeight := m.onboardingModalSize(width, height)
	contentWidth, contentHeight := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	steps := onboardingSteps()
	stepIndex := clamp(m.onboardingStep, 0, len(steps)-1)
	step := steps[stepIndex]

	bodyWidth := max(1, contentWidth-2)
	rows := []string{
		titleStyle.Render(fmt.Sprintf("首次使用引导  %d/%d", stepIndex+1, len(steps))) + padRight("", max(1, contentWidth-18)) + keyStyle.Render("Esc"),
		dimStyle.Render(strings.Repeat("─", contentWidth)),
		"",
		keyStyle.Render(step.Title),
		"",
	}
	for _, paragraph := range step.Body {
		for _, row := range wrapText(paragraph, bodyWidth) {
			rows = append(rows, "  "+row)
		}
	}
	rows = append(rows, "", mutedStyle.Render(step.Hint), "")
	progress := onboardingProgress(stepIndex, len(steps), max(8, min(24, contentWidth/3)))
	action := keyStyle.Render("Enter/→") + " 下一步"
	if stepIndex == len(steps)-1 {
		action = keyStyle.Render("Enter") + " 完成"
	}
	rows = append(rows,
		progress,
		dimStyle.Render("←/b 上一步 · ")+action+dimStyle.Render(" · Esc 跳过"),
	)
	for len(rows) > contentHeight {
		removeAt := len(rows) - 4
		if removeAt < 0 {
			break
		}
		rows = append(rows[:removeAt], rows[removeAt+1:]...)
	}
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	box := renderPanel(activePanelStyle, boxWidth, boxHeight, strings.Join(rows, "\n"))
	left, top := modalOrigin(width, height, boxWidth, boxHeight)
	return baseStyle.Render(placeOverlay(base, box, width, height, left, top))
}

func onboardingProgress(current, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	filled := int(float64(current+1) / float64(total) * float64(width))
	filled = clamp(filled, 1, width)
	return keyStyle.Render(strings.Repeat("━", filled)) + dimStyle.Render(strings.Repeat("─", width-filled))
}

func (m Model) renderMain(width, height int) string {
	if m.focus == focusRollback {
		return m.renderRollback(width, height)
	}

	header := m.renderHeader(width)
	footer := m.renderFooter(width)
	message := m.renderMessage(width)
	l := m.layout()

	sidebar := m.renderSidebar(l.sidebarWidth, l.mainHeight)
	sessions := m.renderSessions(l.mainWidth, l.sessionHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, strings.Repeat(" ", l.gutterWidth), sessions)

	parts := []string{header, body}
	if message != "" {
		parts = append(parts, message)
	}
	parts = append(parts, footer)
	return baseStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m Model) renderInput(width int) string {
	label := "搜索会话"
	box := activePanelStyle.Width(width-4).Padding(1, 2).Render(
		titleStyle.Render(label) + "\n\n" +
			mutedStyle.Render("输入后按 Enter 确认，Esc 取消") + "\n\n" +
			keyStyle.Render("> ") + m.input,
	)
	return lipgloss.Place(width, 12, lipgloss.Left, lipgloss.Top, box)
}

func (m Model) renderProviderPicker(width, height int) string {
	boxWidth, boxHeight := m.providerPickerSize(width, height)
	contentWidth, contentHeight := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	listHeight := max(0, contentHeight-6)
	choices := m.filteredTargetProviders()
	offset := clamp(m.pickerOffset, 0, max(0, len(choices)-listHeight))
	end := min(len(choices), offset+listHeight)
	query := m.pickerQuery
	if query == "" {
		query = "输入搜索 provider..."
	}
	rows := []string{
		titleStyle.Render("选择目标 Provider"),
		mutedStyle.Render("当前目标: ") + keyStyle.Render(m.target),
		keyStyle.Render("> ") + truncate(query, max(0, contentWidth-2)),
		dimStyle.Render(padRight("    "+padRight("Provider", max(1, contentWidth-14))+"  Sessions", contentWidth)),
	}
	for i := offset; i < end; i++ {
		p := choices[i]
		nameWidth := max(1, contentWidth-13)
		line := padRight(truncate(p.Name, nameWidth), nameWidth) + fmt.Sprintf(" %8d", p.Total)
		rows = append(rows, m.renderNavRow(line, contentWidth, i == m.pickerCursor, p.Name == m.target))
	}
	if len(choices) == 0 {
		rows = append(rows, mutedStyle.Render("没有匹配 provider；Enter 使用当前输入。"))
	}
	rows = append(rows, m.scrollHint(offset, listHeight, len(choices)))
	rows = append(rows, dimStyle.Render("Enter 选择 · Esc 取消 · ↑/↓ 移动"))
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	box := renderPanel(activePanelStyle, boxWidth, boxHeight, strings.Join(rows, "\n"))
	return baseStyle.Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box))
}

func (m Model) renderSearchModal(width, height int) string {
	base := m.renderMain(width, height)
	boxWidth, boxHeight := m.searchModalSize(width, height)
	box := m.renderSearchBox(boxWidth, boxHeight)
	left, top := modalOrigin(width, height, boxWidth, boxHeight)
	return baseStyle.Render(placeOverlay(base, box, width, height, left, top))
}

func (m Model) renderSearchBox(width, height int) string {
	contentWidth, contentHeight := panelContentSize(activePanelStyle, width, height)
	listRows := max(1, contentHeight-6)
	query := m.searchQuery
	if query == "" {
		query = "输入标题、user/assistant 对话内容..."
	}
	rowWidth := max(0, contentWidth-2)
	timeWidth := len("2006-01-02 15:04")
	projectWidth := min(18, max(8, rowWidth/5))
	hitWidth := 6
	bodyWidth := max(1, rowWidth-timeWidth-2-projectWidth-2-hitWidth-2)
	rows := []string{
		titleStyle.Render("搜索 Sessions"),
		keyStyle.Render("> ") + highlightMatches(truncate(query, max(0, contentWidth-2)), m.searchQuery),
		dimStyle.Render(m.searchStatusLine()),
		dimStyle.Render(padRight("    "+padRight("更新", timeWidth)+"  "+padRight("项目", projectWidth)+"  "+padRight("命中内容", bodyWidth)+"  类型", contentWidth)),
	}
	end := min(len(m.searchResults), m.searchOffset+listRows)
	for i := m.searchOffset; i < end; i++ {
		result := m.searchResults[i]
		role := result.Role
		if role == "assistant" {
			role = "assist"
		}
		updated := absoluteUpdatedString(result.Thread.UpdatedAt)
		project := projectStyle.Render(truncate(projectName(m.sessionProjectRoot(result.Thread)), projectWidth))
		text := searchResultText(result)
		text = highlightMatches(truncate(text, bodyWidth), m.searchQuery)
		line := padRight(truncate(updated, timeWidth), timeWidth) + "  " +
			padRight(project, projectWidth) + "  " +
			padRight(text, bodyWidth) + "  " +
			padRight(truncate(role, hitWidth), hitWidth)
		rows = append(rows, m.renderNavRow(line, contentWidth, i == m.searchCursor, false))
	}
	if len(m.searchResults) == 0 {
		rows = append(rows, mutedStyle.Render("没有匹配结果"))
	}
	rows = append(rows, m.scrollHint(m.searchOffset, listRows, len(m.searchResults)))
	rows = append(rows, dimStyle.Render("Enter 打开 Markdown · Esc 关闭 · ↑/↓ 移动"))
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	return renderPanel(activePanelStyle, width, height, strings.Join(rows, "\n"))
}

func (m Model) renderMigrateConfirm(width, height int) string {
	boxWidth, boxHeight := m.migrateConfirmSize(width, height)
	contentWidth, _ := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	scopeLabel := localizedMigrationLabel(m.migrateLabel)
	actionTitle := "迁移 " + scopeLabel
	modeText := "retag 原会话 ID"
	updateLines := []string{
		"1. 更新 state_5.sqlite 的 threads.model_provider",
		"2. 更新 rollout 首行 session_meta.model_provider",
		"3. 创建可回滚的 snapshot manifest",
	}
	if m.mode == migrate.ModeClone {
		actionTitle = "克隆 " + scopeLabel
		modeText = "clone 为新会话 ID"
		updateLines = []string{
			"1. 创建新的 rollout 文件和 thread id",
			"2. 写入 state_5.sqlite thread 记录",
			"3. 更新 session_index.jsonl 和 global-state 项目映射",
			"4. 创建可回滚的 snapshot manifest",
		}
	}
	leftWidth := max(12, (contentWidth-2)/2)
	rightWidth := max(12, contentWidth-leftWidth-2)
	fromTo := lipgloss.JoinHorizontal(
		lipgloss.Top,
		boxLine("来源", m.providerSourceLabel(), leftWidth),
		"  ",
		boxLine("目标", m.target, rightWidth),
	)
	buttons := lipgloss.JoinHorizontal(
		lipgloss.Top,
		buttonStyle("预览", false),
		"  ",
		buttonStyle("执行", true),
		"  ",
		buttonStyle("取消", false),
	)
	rows := []string{
		titleStyle.Render(actionTitle) + padRight("", max(1, contentWidth-lipgloss.Width(actionTitle)-3)) + keyStyle.Render("Esc"),
		dimStyle.Render(strings.Repeat("─", contentWidth)),
		"",
		fromTo,
		boxLine("模式", modeText, contentWidth),
		boxLines("将更新", updateLines, contentWidth),
		"",
		mutedStyle.Render(fmt.Sprintf("范围：%s · %d 条会话", scopeLabel, m.migrateCount)),
		"",
		buttons,
		dimStyle.Render("D 预览 · Enter/Y 执行 · Esc/N 取消"),
	}
	box := renderPanel(activePanelStyle, boxWidth, boxHeight, strings.Join(rows, "\n"))
	return baseStyle.Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box))
}

func localizedMigrationLabel(label string) string {
	switch {
	case label == "selected sessions":
		return "已选会话"
	case strings.HasPrefix(label, "current session "):
		return "当前会话 " + strings.TrimPrefix(label, "current session ")
	case strings.HasPrefix(label, "provider "):
		return "Provider " + strings.TrimPrefix(label, "provider ")
	case strings.HasPrefix(label, "providers "):
		return "Providers " + strings.TrimPrefix(label, "providers ")
	case strings.HasPrefix(label, "project "):
		return "项目 " + strings.TrimPrefix(label, "project ")
	default:
		return label
	}
}

func boxLine(label, value string, width int) string {
	content := titleStyle.Render(label) + "\n" + value
	return lipgloss.NewStyle().
		Width(max(1, width-2)).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Padding(0, 1).
		Render(content)
}

func boxLines(label string, values []string, width int) string {
	content := titleStyle.Render(label) + "\n" + strings.Join(values, "\n")
	return lipgloss.NewStyle().
		Width(max(1, width-2)).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Padding(0, 1).
		Render(content)
}

func buttonStyle(label string, primary bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 2)
	if primary {
		style = style.BorderForeground(colorActive).Bold(true)
	} else {
		style = style.BorderForeground(colorMuted)
	}
	return style.Render(label)
}

func (m Model) renderClearConfirm(width, height int) string {
	boxWidth, boxHeight := m.clearConfirmSize(width, height)
	contentWidth, _ := panelContentSize(activePanelStyle, boxWidth, boxHeight)
	rows := []string{
		badBadgeStyle.Render("危险操作") + " " + titleStyle.Render("删除 Sessions"),
		"",
		mutedStyle.Render("范围: ") + keyStyle.Render(m.clearLabel),
		mutedStyle.Render(fmt.Sprintf("将删除 %d 条 session，并创建 snapshot 可回滚。", m.clearCount)),
		mutedStyle.Render("包含 SQLite、session_index、global-state 和 rollout 文件。"),
		"",
	}
	if m.clearExpected != "" {
		rows = append(rows,
			mutedStyle.Render("输入名称确认: ")+keyStyle.Render(m.clearExpected),
			keyStyle.Render("> ")+m.clearInput,
			"",
			keyStyle.Render("Enter")+" 确认删除    "+keyStyle.Render("Esc")+" 取消",
		)
	} else {
		rows = append(rows, keyStyle.Render("Enter/Y")+" 确认删除    "+keyStyle.Render("Esc/N")+" 取消")
	}
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	box := renderPanel(activePanelStyle, boxWidth, boxHeight, strings.Join(rows, "\n"))
	return baseStyle.Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box))
}

func (m Model) renderSettingsModal(width, height int) string {
	base := m.renderMain(width, height)
	boxWidth, boxHeight := m.settingsModalSize(width, height)
	box := m.renderSettingsBox(boxWidth, boxHeight)
	left, top := modalOrigin(width, height, boxWidth, boxHeight)
	return baseStyle.Render(placeOverlay(base, box, width, height, left, top))
}

func (m Model) renderSettingsBox(width, height int) string {
	contentWidth, _ := panelContentSize(activePanelStyle, width, height)
	rows := []string{
		panelTitleStyle.Render("Settings"),
		dimStyle.Render("常用配置"),
		"",
	}
	items := []struct {
		label string
		value string
		hint  string
	}{
		{"显示归档", onOff(m.includeA), "默认隐藏；开启后显示 archived sessions"},
		{"显示子代理", onOff(m.includeS), "控制 sub-agent / 非 user 线程是否出现在列表"},
		{"目标 Provider", m.target, "选择迁移目标 provider"},
		{"迁移模式", string(m.mode), "retag 修改原会话，clone 复制新会话"},
		{"清理归档", fmt.Sprintf("%d 条", m.archivedSessionCount()), "删除所有 archived sessions，并创建 snapshot"},
		{"清理子代理", fmt.Sprintf("%d 条", m.subagentSessionCount()), "删除所有 sub-agent sessions，并创建 snapshot"},
	}
	labelWidth := max(10, min(18, contentWidth/4))
	valueWidth := max(10, min(20, contentWidth/4))
	descWidth := max(1, contentWidth-labelWidth-valueWidth-8)
	rows = append(rows,
		dimStyle.Render("  "+padRight("配置", labelWidth)+"  "+padRight("状态", valueWidth)+"  "+"说明"),
		dimStyle.Render("  "+strings.Repeat("─", max(0, contentWidth-2))),
	)
	for i, item := range items {
		label := padRight(truncate(item.label, labelWidth), labelWidth)
		value := padRight(truncate(item.value, valueWidth), valueWidth)
		line := label + "  " + keyStyle.Render(value)
		rows = append(rows, m.renderNavRow(line, contentWidth, i == m.settingsCursor, false))
		rows = append(rows, dimStyle.Render("    "+padRight("", labelWidth)+padRight("", 2)+padRight("", valueWidth)+padRight("", 2)+truncate(item.hint, descWidth)))
	}
	rows = append(rows, "")
	rows = append(rows, dimStyle.Render("Enter/Space 执行 · Esc/b 返回 · e 也可直接选择目标 provider"))
	for i := range rows {
		rows[i] = truncate(rows[i], contentWidth)
	}
	return renderPanel(activePanelStyle, width, height, strings.Join(rows, "\n"))
}

func (m Model) renderHeader(width int) string {
	contentWidth := max(0, width-2)
	dbBadge := okPillStyle.Render("DB ok")
	if !(m.diag.DBExists && m.diag.HasModelProvider && m.diag.Integrity == "ok") {
		dbBadge = badPillStyle.Render("DB bad")
	}
	codexBadge := okPillStyle.Render("Codex stopped")
	if m.diag.CodexRunning {
		codexBadge = warnPillStyle.Render("Codex running")
	}
	modeBadge := okPillStyle.Render("retag")
	if m.mode == "clone" {
		modeBadge = warnPillStyle.Render("clone")
	}
	search := "无"
	if m.search != "" {
		search = m.search
	}
	brand := renderAppTitle(contentWidth, m.titleFrame)
	statusParts := []string{
		codexBadge,
		dbBadge,
		mutedStyle.Render(fmt.Sprintf("已选 %d", len(m.selectedIDs()))),
	}
	if providerCount := len(m.selectedProviderNames()); providerCount > 0 {
		statusParts = append(statusParts, mutedStyle.Render(fmt.Sprintf("来源 %d", providerCount)))
	}
	if m.demoMode {
		statusParts = append(statusParts, warnPillStyle.Render("Demo"))
	}
	statusWidth := max(0, contentWidth-lipgloss.Width(brand)-2)
	status := truncate(strings.Join(statusParts, " "), statusWidth)
	line1 := brand
	if status != "" && statusWidth > 0 {
		line1 = padRight(brand, max(lipgloss.Width(brand), contentWidth-lipgloss.Width(status)-2)) + "  " + status
	}
	line2 := truncate(
		mutedStyle.Render("目标 ")+keyStyle.Render(m.target)+
			mutedStyle.Render(" · 模式 ")+modeBadge+
			mutedStyle.Render(fmt.Sprintf(" · 过滤：归档 %s · 子代理 %s · 搜索 %q", onOff(m.includeA), onOff(m.includeS), search)),
		contentWidth,
	)
	return lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Render(truncate(line1, contentWidth) + "\n" + line2)
}

func renderAppTitle(width int, frame int) string {
	title := "Codex Session Migrator"
	prefix := animatedTitleAccent(frame) + " "
	if width < 30 {
		title = "CSM"
		prefix = ""
	}
	if width < 44 && title != "CSM" {
		title = "Codex Migrator"
	}
	return truncate(prefix+animatedTitle(title, frame), width)
}

func animatedTitleAccent(frame int) string {
	colors := []lipgloss.Color{
		lipgloss.Color("#00D7FF"),
		lipgloss.Color("#5DFFB1"),
		lipgloss.Color("#FFF06A"),
		lipgloss.Color("#FF5DA2"),
	}
	return lipgloss.NewStyle().
		Foreground(colors[frame%len(colors)]).
		Bold(true).
		Render("▌")
}

func animatedTitle(title string, frame int) string {
	palette := []lipgloss.Color{
		lipgloss.Color("#00D7FF"),
		lipgloss.Color("#5DFFB1"),
		lipgloss.Color("#FFF06A"),
		lipgloss.Color("#FF9F43"),
		lipgloss.Color("#FF5DA2"),
		lipgloss.Color("#9B7BFF"),
	}
	var b strings.Builder
	for i, r := range title {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		color := palette[(i+frame)%len(palette)]
		b.WriteString(lipgloss.NewStyle().
			Foreground(color).
			Background(lipgloss.Color("#1F2937")).
			Bold(true).
			Render(string(r)))
	}
	return b.String()
}

func (m Model) renderSidebar(width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	providerHeight := min(height, min(8, max(4, len(m.viewProviders())+4)))
	projectHeight := max(0, height-providerHeight-1)
	providers := m.renderProviders(width, providerHeight)
	projects := m.renderProjects(width, projectHeight)
	if projects == "" {
		return providers
	}
	return lipgloss.JoinVertical(lipgloss.Left, providers, strings.Repeat(" ", width), projects)
}

func (m Model) renderProviders(width, height int) string {
	style := m.panelFor(focusProviders)
	contentWidth, contentHeight := panelContentSize(style, width, height)
	rowWidth := max(0, contentWidth-2)
	providers := m.viewProviders()
	rows := []string{m.renderPanelTitle(focusProviders, "Providers", contentWidth)}
	limit := max(0, contentHeight-2)
	end := min(len(providers), m.offsetP+limit)
	for i := m.offsetP; i < end; i++ {
		p := providers[i]
		selected := m.selectedProviders[p.Name]
		check := "[ ]"
		if selected {
			check = "[x]"
		}
		nameWidth := max(1, rowWidth-lipgloss.Width(check)-1-6)
		line := check + " " + padRight(truncate(p.Name, nameWidth), nameWidth) + fmt.Sprintf(" %5d", p.Total)
		rows = append(rows, m.renderNavRow(line, contentWidth, m.focus == focusProviders && i == m.cursorP, selected))
	}
	rows = append(rows, m.scrollHint(m.offsetP, limit, len(providers)))
	if len(providers) == 0 {
		rows = append(rows, truncate(mutedStyle.Render("没有 provider"), contentWidth))
	}
	return renderPanel(style, width, height, strings.Join(rows, "\n"))
}

func (m Model) renderProjects(width, height int) string {
	style := m.panelFor(focusProjects)
	contentWidth, contentHeight := panelContentSize(style, width, height)
	rowWidth := max(0, contentWidth-2)
	projects := m.viewProjects()
	rows := []string{m.renderPanelTitle(focusProjects, "Projects", contentWidth)}
	limit := max(0, contentHeight-2)
	end := min(len(projects), m.offsetG+limit)
	for i := m.offsetG; i < end; i++ {
		p := projects[i]
		indent := ""
		if p.Key != allProjectsKey {
			indent = "  "
		}
		nameWidth := max(1, rowWidth-lipgloss.Width(indent)-6)
		line := indent + padRight(truncate(p.Name, nameWidth), nameWidth) + fmt.Sprintf(" %5d", p.Count)
		rows = append(rows, m.renderNavRow(line, contentWidth, m.focus == focusProjects && i == m.cursorG, false))
	}
	rows = append(rows, m.scrollHint(m.offsetG, limit, len(projects)))
	if len(projects) == 0 {
		rows = append(rows, truncate(mutedStyle.Render("没有项目"), contentWidth))
	}
	return renderPanel(style, width, height, strings.Join(rows, "\n"))
}

func (m Model) renderSessions(width, height int) string {
	style := m.panelFor(focusSessions)
	contentWidth, contentHeight := panelContentSize(style, width, height)
	rowWidth := max(0, contentWidth-2)
	sessions := m.viewSessions()
	timeHeader := "时间"
	timeWidth := len("15:04")
	sessionIndent := "    "
	titleWidth := max(1, rowWidth-lipgloss.Width(sessionIndent)-lipgloss.Width("[ ] ")-2-timeWidth)
	title := fmt.Sprintf("Sessions  %s", mutedStyle.Render(fmt.Sprintf("%d/%d", len(sessions), m.viewAllSessionCount())))
	rows := []string{
		m.renderPanelTitle(focusSessions, title, contentWidth),
		dimStyle.Render(padRight(truncatePreserveSpace(sessionIndent+padRight(timeHeader, timeWidth)+"  标题", contentWidth), contentWidth)),
	}
	limit := max(0, contentHeight-3)
	now := time.Now()
	lastGroup := ""
	visibleRows := 0
	for i := m.offsetS; i < len(sessions) && visibleRows < limit; i++ {
		s := sessions[i]
		group := sessionDateLabel(s.UpdatedAt, now)
		if group != lastGroup {
			rows = append(rows, truncatePreserveSpace(dateHeaderStyle.Render("  ─ "+group), contentWidth))
			visibleRows++
			lastGroup = group
			if visibleRows >= limit {
				break
			}
		}
		check := "[ ]"
		if m.selected[s.ID] {
			check = "[x]"
		}
		updated := padRight(truncate(sessionTimeString(s.UpdatedAt), timeWidth), timeWidth)
		title := renderSessionTitleCell(s.Archived, m.displayThreadTitle(s), titleWidth)
		line := sessionIndent + check + " " + updated + "  " + title
		active := m.focus == focusSessions && i == m.cursorS
		selected := m.selected[s.ID]
		rows = append(rows, m.renderNavRow(line, contentWidth, active, selected))
		visibleRows++
	}
	rows = append(rows, m.scrollHint(m.offsetS, limit, len(sessions)))
	if len(sessions) == 0 {
		rows = append(rows, truncate(mutedStyle.Render("没有匹配会话。可以调整搜索、归档或项目分组。"), contentWidth))
	}
	return renderPanel(style, width, height, strings.Join(rows, "\n"))
}

func (m Model) renderConversationDetail(width, height int) string {
	header := m.renderHeader(width)
	footer := m.renderDetailFooter(width)
	bodyHeight := max(0, height-lipgloss.Height(header)-lipgloss.Height(footer))
	contentWidth, contentHeight := panelContentSize(activePanelStyle, width, bodyHeight)
	rows := m.detailRows()
	visible := max(0, contentHeight-2)
	offset := clamp(m.detailOffset, 0, max(0, len(rows)-visible))
	end := min(len(rows), offset+visible)
	bodyRows := []string{panelTitleStyle.Render("Session Detail")}
	if len(rows) == 0 {
		bodyRows = append(bodyRows, mutedStyle.Render("没有可显示的对话信息"))
	} else {
		bodyRows = append(bodyRows, rows[offset:end]...)
	}
	bodyRows = append(bodyRows, m.scrollHint(offset, visible, len(rows)))
	for i := range bodyRows {
		bodyRows[i] = truncate(bodyRows[i], contentWidth)
	}
	body := renderPanel(activePanelStyle, width, bodyHeight, strings.Join(bodyRows, "\n"))
	return baseStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

func (m Model) detailRows() []string {
	t := m.detailThread
	title := m.displayThreadTitle(t)
	if t.Archived {
		title = archivedBadgeStyle.Render("归档") + " " + title
	}
	rows := []string{
		titleStyle.Render(title),
		fmt.Sprintf("%s  %s", dimStyle.Render("id"), t.ID),
		fmt.Sprintf("%s %s    %s %s    %s %s", dimStyle.Render("provider"), t.ModelProvider, dimStyle.Render("updated"), t.UpdatedString(), dimStyle.Render("archived"), fmt.Sprint(t.Archived)),
		fmt.Sprintf("%s %s", dimStyle.Render("cwd"), rel(t.CWD)),
		fmt.Sprintf("%s %s", dimStyle.Render("rollout"), rel(t.RolloutPath)),
		fmt.Sprintf("%s %d    %s %d    %s %d    %s %d", dimStyle.Render("lines"), m.detail.LineCount, dimStyle.Render("user"), m.detail.UserMessages, dimStyle.Render("assistant"), m.detail.AgentMessages, dimStyle.Render("tools/events"), m.detail.ToolEvents),
		"",
	}
	if m.detailErr != "" {
		rows = append(rows, badBadgeStyle.Render("读取失败")+" "+m.detailErr)
		return rows
	}
	if len(m.detail.Items) == 0 {
		rows = append(rows, mutedStyle.Render("rollout 中没有解析到可展示的消息。"))
		return rows
	}
	for _, item := range m.detail.Items {
		label := item.Role
		switch item.Role {
		case "user":
			label = keyStyle.Render("user")
		case "assistant":
			label = okBadgeStyle.Render("assistant")
		case "tool":
			label = warnBadgeStyle.Render("tool")
		default:
			label = dimStyle.Render(item.Role)
		}
		rows = append(rows, label)
		for _, line := range wrapText(strings.TrimSpace(item.Text), 100) {
			rows = append(rows, "  "+line)
		}
		rows = append(rows, "")
	}
	if m.detail.Truncated {
		rows = append(rows, mutedStyle.Render("已截断，仅显示前 500 条可解析消息。"))
	}
	return rows
}

func (m Model) detailVisibleRows() int {
	width := m.width
	if width <= 0 {
		width = 90
	}
	height := m.height
	if height <= 0 {
		height = 28
	}
	header := m.renderHeader(width)
	footer := m.renderDetailFooter(width)
	bodyHeight := max(0, height-lipgloss.Height(header)-lipgloss.Height(footer))
	_, contentHeight := panelContentSize(activePanelStyle, width, bodyHeight)
	return max(0, contentHeight-2)
}

func (m Model) renderDetailFooter(width int) string {
	help := strings.Join([]string{
		keyStyle.Render("Esc/b") + " 返回",
		keyStyle.Render("↑/↓") + " 滚动",
		keyStyle.Render("PgUp/PgDn") + " 翻页",
		keyStyle.Render("q") + " 退出",
	}, dimStyle.Render(" · "))
	return lipgloss.NewStyle().Width(max(0, width-2)).Padding(0, 1).Render(truncate(help, width-2))
}

func (m Model) renderSettingsFooter(width int) string {
	help := strings.Join([]string{
		keyStyle.Render("↑/↓") + " 移动",
		keyStyle.Render("Enter/Space") + " 修改",
		keyStyle.Render("Esc/b") + " 返回",
	}, dimStyle.Render(" · "))
	return lipgloss.NewStyle().Width(max(0, width-2)).Padding(0, 1).Render(truncate(help, width-2))
}

func (m Model) renderRollback(width, height int) string {
	header := m.renderHeader(width)
	rows := []string{panelTitleStyle.Render("Rollback snapshots")}
	limit := max(0, height-lipgloss.Height(header)-7)
	end := min(len(m.snapshots), m.offsetR+limit)
	for i := m.offsetR; i < end; i++ {
		s := m.snapshots[i]
		rows = append(rows, m.renderNavRow(s, width-4, i == m.cursorS, false))
	}
	rows = append(rows, m.scrollHint(m.offsetR, limit, len(m.snapshots)))
	if len(m.snapshots) == 0 {
		rows = append(rows, mutedStyle.Render("没有可回滚的 snapshot"))
	}
	bodyHeight := max(0, height-lipgloss.Height(header)-lipgloss.Height(m.renderFooter(width)))
	body := renderPanel(activePanelStyle, width, bodyHeight, strings.Join(rows, "\n"))
	footer := m.renderFooter(width)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) renderFooter(width int) string {
	help := []string{
		keyStyle.Render("↑/↓") + " 移动",
		keyStyle.Render("滚轮/PgUp/PgDn") + " 滚动",
		keyStyle.Render("Tab") + " 切换",
		keyStyle.Render("Space") + " 选择",
		keyStyle.Render("a") + " 全选",
		keyStyle.Render("/") + " 搜索",
		keyStyle.Render("o") + " 设置",
		keyStyle.Render("Ctrl+E") + " 演示",
		keyStyle.Render("e") + " 目标",
		keyStyle.Render("r") + " 回滚",
		keyStyle.Render("x") + " 清空",
		keyStyle.Render("d") + " 预览",
		keyStyle.Render("m") + " 迁移",
		keyStyle.Render("?") + " 帮助",
	}
	if m.focus == focusProjects {
		help = append(help, keyStyle.Render("Enter")+" 查看项目")
	}
	if m.focus == focusProviders {
		help = append(help, keyStyle.Render("Enter")+" 查看分组")
	}
	if m.focus == focusSessions {
		help = append(help, keyStyle.Render("Enter/v")+" 打开 Markdown")
	}
	text := strings.Join(help, dimStyle.Render(" · "))
	return lipgloss.NewStyle().Width(max(0, width-2)).Padding(0, 1).Render(truncate(text, width-2))
}

func (m Model) renderMessage(width int) string {
	if strings.TrimSpace(m.message) == "" {
		return ""
	}
	msg := truncate(strings.ReplaceAll(m.message, "\n", "  "), width-8)
	return lipgloss.NewStyle().
		Width(width-4).
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(colorBorder).
		Foreground(colorMuted).
		Padding(0, 1).
		Render(msg)
}

func (m Model) scrollHint(offset, visible, total int) string {
	if total <= visible || visible <= 0 {
		return dimStyle.Render("")
	}
	from := offset + 1
	to := min(total, offset+visible)
	return dimStyle.Render(fmt.Sprintf("  %d-%d / %d", from, to, total))
}

func (m Model) renderNavRow(text string, width int, active, selected bool) string {
	prefix := "  "
	if active {
		prefix = "› "
	}
	bodyWidth := max(0, width-lipgloss.Width(prefix))
	line := prefix + padRight(truncatePreserveSpace(text, bodyWidth), bodyWidth)
	if active {
		return activeRowStyle.Render(line)
	}
	if selected {
		return selectedRowStyle.Render(line)
	}
	return rowStyle.Render(line)
}

func (m Model) renderPanelTitle(target focus, title string, width int) string {
	if m.focus == target {
		return truncate(activePanelTitleStyle.Render("> "+title), width)
	}
	return truncate(panelTitleStyle.Render("  "+title), width)
}

func (m Model) panelFor(target focus) lipgloss.Style {
	if m.focus == target {
		return activePanelStyle
	}
	return panelStyle
}

func running(v bool) string {
	if v {
		return "running"
	}
	return "stopped"
}

func ok(v bool) string {
	if v {
		return "ok"
	}
	return "bad"
}

func absoluteUpdatedString(updatedAt int64) string {
	if updatedAt <= 0 {
		return ""
	}
	return time.Unix(updatedAt, 0).Format("2006-01-02 15:04")
}

func sessionDateLabel(updatedAt int64, now time.Time) string {
	if updatedAt <= 0 {
		return "无日期"
	}
	updated := time.Unix(updatedAt, 0)
	uy, um, ud := updated.Date()
	ny, nm, nd := now.Date()
	if uy == ny && um == nm && ud == nd {
		return "Today"
	}
	return updated.Format("2006-01-02")
}

func sessionTimeString(updatedAt int64) string {
	if updatedAt <= 0 {
		return ""
	}
	return time.Unix(updatedAt, 0).Format("15:04")
}

func onOff(v bool) string {
	if v {
		return "显示"
	}
	return "隐藏"
}

func (m Model) searchStatusLine() string {
	sessions := m.viewSessions()
	indexed := min(len(sessions), m.searchIndexPos)
	state := "索引完成"
	if m.searchIndexing {
		state = fmt.Sprintf("索引中 %d/%d", indexed, len(sessions))
	}
	return fmt.Sprintf("范围: 当前列表 %d 条 · 标题即时 / user / assistant 后台补全 · 模糊搜索 · %s", len(sessions), state)
}

func highlightMatches(text, query string) string {
	query = strings.ToLower(singleLineDisplay(query))
	if query == "" || text == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	if idx := strings.Index(lowerText, query); idx >= 0 {
		end := idx + len(query)
		return text[:idx] + keyStyle.Render(text[idx:end]) + text[end:]
	}
	queryRunes := []rune(query)
	q := 0
	var b strings.Builder
	for _, r := range text {
		if q < len(queryRunes) && strings.ToLower(string(r)) == string(queryRunes[q]) {
			b.WriteString(keyStyle.Render(string(r)))
			q++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func truncate(s string, width int) string {
	s = singleLineDisplay(s)
	if width <= 0 {
		return ""
	}
	if xansi.StringWidth(s) <= width {
		return s
	}
	return xansi.Truncate(s, width, "…")
}

func truncatePreserveSpace(s string, width int) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	if width <= 0 {
		return ""
	}
	if xansi.StringWidth(s) <= width {
		return s
	}
	return xansi.Truncate(s, width, "…")
}

func renderSessionTitleCell(archived bool, title string, width int) string {
	if width <= 0 {
		return ""
	}
	if !archived {
		return padRight(truncate(title, width), width)
	}
	badge := archivedBadgeStyle.Render("归档")
	prefix := badge + " "
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= width {
		return truncate(badge, width)
	}
	titleWidth := width - prefixWidth
	return prefix + padRight(truncate(title, titleWidth), titleWidth)
}

func singleLineDisplay(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func searchResultText(result searchResult) string {
	title := strings.TrimSpace(result.Title)
	snippet := strings.TrimSpace(result.Snippet)
	if snippet == "" || snippet == title {
		return title
	}
	if title == "" {
		return snippet
	}
	if result.Role != "title" {
		return snippet + " · " + title
	}
	return title + " · " + snippet
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	current := lipgloss.Width(s)
	if current >= width {
		return s
	}
	return s + strings.Repeat(" ", width-current)
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

func shortDisplayID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func wrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var rows []string
	for _, paragraph := range strings.Split(s, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		for lipgloss.Width(paragraph) > width {
			var b strings.Builder
			for _, r := range paragraph {
				if lipgloss.Width(b.String()+string(r)) > width {
					break
				}
				b.WriteRune(r)
			}
			line := b.String()
			if line == "" {
				break
			}
			rows = append(rows, line)
			paragraph = strings.TrimSpace(strings.TrimPrefix(paragraph, line))
		}
		if paragraph != "" {
			rows = append(rows, paragraph)
		}
	}
	return rows
}

func panelContentSize(style lipgloss.Style, outerWidth, outerHeight int) (int, int) {
	return max(0, outerWidth-style.GetHorizontalFrameSize()), max(0, outerHeight-style.GetVerticalFrameSize())
}

func renderPanel(style lipgloss.Style, outerWidth, outerHeight int, content string) string {
	if outerWidth <= 0 || outerHeight <= 0 {
		return ""
	}
	contentWidth, contentHeight := panelContentSize(style, outerWidth, outerHeight)
	blockWidth := max(0, contentWidth+style.GetHorizontalPadding())
	blockHeight := max(0, contentHeight+style.GetVerticalPadding())
	return style.Width(blockWidth).Height(blockHeight).Render(fitPanelContent(content, contentWidth, contentHeight))
}

func modalOrigin(width, height, boxWidth, boxHeight int) (int, int) {
	return max(0, (width-boxWidth)/2), max(0, (height-boxHeight)/2)
}

func placeOverlay(base, overlay string, width, height, left, top int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	for i, overlayLine := range overlayLines {
		row := top + i
		if row < 0 || row >= height {
			continue
		}
		overlayWidth := lipgloss.Width(overlayLine)
		prefix := xansi.Cut(baseLines[row], 0, left)
		suffix := xansi.Cut(baseLines[row], left+overlayWidth, width)
		baseLines[row] = padRight(prefix, left) + overlayLine + suffix
	}
	return strings.Join(baseLines, "\n")
}

func visibleRowsInPanel(style lipgloss.Style, outerHeight, fixedContentRows int) int {
	_, contentHeight := panelContentSize(style, 1, outerHeight)
	return max(0, contentHeight-fixedContentRows)
}

func fitPanelContent(content string, width, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = truncatePreserveSpace(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func lipglossHeight(s string) int {
	if s == "" {
		return 0
	}
	return lipgloss.Height(s)
}

func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
