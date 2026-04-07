package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	// Full-screen overlay modes
	if m.mode == modeHelp {
		return m.renderHelp()
	}
	if m.mode == modeEdit {
		return m.renderEditDialog()
	}
	if m.mode == modeDeleteConfirm {
		return m.renderDeleteDialog()
	}
	if m.mode == modeClear {
		return m.renderClearDialog()
	}
	if m.mode == modeDryRun {
		return m.renderDryRunDialog()
	}
	if m.mode == modeParamPrompt {
		return m.renderParamDialog()
	}
	if m.mode == modeScheduleEdit {
		return m.renderScheduleEditDialog()
	}

	// --- Main page view ---
	header := m.renderHeader()
	sep := dimTextStyle.Render(strings.Repeat("─", m.width))

	var content string
	switch m.page {
	case pageScripts:
		content = m.renderScriptsPage()
	case pageSchedules:
		content = m.renderSchedulesPage()
	case pageHistory:
		content = m.renderHistoryPage()
	case pageRunning:
		content = m.renderRunningPage()
	}

	// Status line
	var statusLine string
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		if strings.Contains(m.statusMsg, "fail") || strings.Contains(m.statusMsg, "Fail") || strings.Contains(m.statusMsg, "Error") {
			statusLine = errorTextStyle.Render("  " + m.statusMsg)
		} else {
			statusLine = statusMsgStyle.Render("  " + m.statusMsg)
		}
	}

	footer := m.renderFooter()

	var parts []string
	parts = append(parts, header, sep, content)
	if statusLine != "" {
		parts = append(parts, statusLine)
	}
	parts = append(parts, sep, footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// --- Header ---

func (m model) renderHeader() string {
	title := titleStyle.Render("Runx")

	pages := []struct {
		name string
		p    appPage
	}{
		{"Scripts", pageScripts},
		{"Schedules", pageSchedules},
		{"History", pageHistory},
		{"Running", pageRunning},
	}

	var tabs []string
	for i, pg := range pages {
		if i > 0 {
			tabs = append(tabs, dimTextStyle.Render(" │ "))
		}
		if pg.p == m.page {
			tabs = append(tabs, activePageStyle.Render(pg.name))
		} else {
			tabs = append(tabs, inactivePageStyle.Render(pg.name))
		}
	}

	left := title + "  " + strings.Join(tabs, "")

	// Right side stats
	var statParts []string
	statParts = append(statParts, dimTextStyle.Render(fmt.Sprintf("%d scripts", len(m.scripts))))

	running := 0
	for _, rs := range m.runningScripts {
		if !rs.Done {
			running++
		}
	}
	if running > 0 {
		statParts = append(statParts, warnTextStyle.Render(fmt.Sprintf("▶ %d running", running)))
	}

	activeScheds := 0
	for _, s := range m.scripts {
		if s.ScheduleOn && s.Schedule != "" {
			activeScheds++
		}
	}
	if activeScheds > 0 {
		statParts = append(statParts, dimTextStyle.Render(fmt.Sprintf("⏱ %d scheduled", activeScheds)))
	}

	if m.page == pageScripts && m.sortMode != sortByName {
		labels := []string{"", "runs", "recent"}
		statParts = append(statParts, dimTextStyle.Render(fmt.Sprintf("sort: %s", labels[m.sortMode])))
	}

	if m.searchFilter != "" && m.mode != modeSearch {
		statParts = append(statParts, dimTextStyle.Render(fmt.Sprintf("filter: \"%s\"", m.searchFilter)))
	}

	right := strings.Join(statParts, dimTextStyle.Render(" · "))

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := m.width - leftWidth - rightWidth
	if gap < 2 {
		gap = 2
	}

	headerLine := left + strings.Repeat(" ", gap) + right

	if m.mode == modeSearch {
		searchBar := "  " + keyStyle.Render("/") + " " + m.searchInput.View()
		return lipgloss.JoinVertical(lipgloss.Left, headerLine, searchBar)
	}

	return headerLine
}

// --- Footer ---

func (m model) renderFooter() string {
	var parts []string
	add := func(key, action string) {
		if len(parts) > 0 {
			parts = append(parts, bulletStyle.Render(" · "))
		}
		parts = append(parts, keyStyle.Render(key), " ", actionStyle.Render(action))
	}

	switch m.page {
	case pageScripts:
		add("enter", "run")
		add("D", "dry run")
		add("e", "edit")
		add("n", "add")
		add("d", "delete")
		add("/", "search")
		add("s", "sort")
	case pageSchedules:
		add("enter", "toggle")
		add("e", "set interval")
	case pageHistory:
		add("enter", "view")
		add("c", "clear")
	case pageRunning:
		add("j/k", "scroll")
		add("G/g", "end/top")
		if len(m.runningScripts) > 1 {
			add("tab", "switch")
		}
		add("x", "close tab")
	}

	add("1-4", "pages")
	add("?", "help")
	add("q", "quit")

	return " " + strings.Join(parts, "")
}

// --- Page content ---

func (m model) renderScriptsPage() string {
	if len(m.scripts) == 0 {
		return m.renderEmptyState("No scripts yet", "Press n to add your first script")
	}
	return m.table.View()
}

func (m model) renderSchedulesPage() string {
	if len(m.scripts) == 0 {
		return m.renderEmptyState("No scripts", "Add scripts on the Scripts page first")
	}
	return m.cronTable.View()
}

func (m model) renderHistoryPage() string {
	if len(m.outputFiles) == 0 {
		return m.renderEmptyState("No output history", "Run a script to see output here")
	}
	return m.outputTable.View()
}

func (m model) renderRunningPage() string {
	if len(m.runningScripts) == 0 {
		return m.renderEmptyState("No running scripts", "Run a script to see output here")
	}

	// Tab bar
	var tabs []string
	for i, rs := range m.runningScripts {
		name := rs.Name
		if len(name) > 16 {
			name = name[:13] + "..."
		}
		indicator := "▶"
		if rs.Done {
			if rs.Err != nil {
				indicator = "✗"
			} else {
				indicator = "✓"
			}
		}
		label := fmt.Sprintf("%s %s", indicator, name)
		if i == m.activeRunTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)

	// Content area
	rs := m.runningScripts[m.activeRunTab]
	visibleHeight := m.height - 12
	if visibleHeight < 3 {
		visibleHeight = 3
	}

	lines := rs.Lines
	startLine := rs.Scroll
	endLine := startLine + visibleHeight
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
		if startLine < 0 {
			startLine = 0
		}
	}

	var visibleContent string
	if len(lines) > 0 && startLine < endLine {
		visibleContent = strings.Join(lines[startLine:endLine], "\n")
	}
	if !rs.Done {
		if visibleContent != "" {
			visibleContent += "\n"
		}
		visibleContent += dimTextStyle.Render("running...")
	}

	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDim).
		Padding(0, 1).
		Height(visibleHeight).
		Width(m.width - 4)

	content := contentStyle.Render(visibleContent)

	// Elapsed time + status
	elapsed := time.Since(rs.StartTime).Truncate(time.Second)
	var statusInfo string
	if rs.Done {
		if rs.Err != nil {
			statusInfo = errorTextStyle.Render(fmt.Sprintf("✗ failed (%s)", formatElapsed(elapsed)))
		} else {
			statusInfo = successTextStyle.Render(fmt.Sprintf("✓ done (%s)", formatElapsed(elapsed)))
		}
	} else {
		statusInfo = warnTextStyle.Render(fmt.Sprintf("▶ running %s", formatElapsed(elapsed)))
	}

	lineInfo := dimTextStyle.Render(fmt.Sprintf("  %d lines", len(rs.Lines)))

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, "", content, "", statusInfo+lineInfo)
}

func (m model) renderEmptyState(title, subtitle string) string {
	content := lipgloss.JoinVertical(lipgloss.Center,
		"", "",
		dimTextStyle.Render(title),
		"",
		dimTextStyle.Render(subtitle),
	)
	h := m.height - 8
	if h < 5 {
		h = 5
	}
	return lipgloss.Place(m.width-4, h, lipgloss.Center, lipgloss.Center, content)
}

// --- Edit dialog ---

func (m model) renderEditDialog() string {
	if m.editRow < 0 || m.editRow >= len(m.scripts) {
		return ""
	}

	w := m.width - 6
	w = int(w * 3 / 4)

	script := m.scripts[m.editRow]
	cmdStr := script.Command
	if len(script.Args) > 0 {
		cmdStr = script.Command + " " + strings.Join(script.Args, " ")
	}

	tagsStr := strings.Join(script.Tags, ", ")
	var envPairs []string
	for k, v := range script.EnvVars {
		envPairs = append(envPairs, k+"="+v)
	}
	envStr := strings.Join(envPairs, ", ")

	fields := []struct {
		label string
		value string
	}{
		{"Name", script.Name},
		{"Category", script.Category},
		{"Command", cmdStr},
		{"Work Dir", script.WorkDir},
		{"Description", script.Description},
		{"Tags", tagsStr},
		{"Env Vars", envStr},
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Edit Script"), "")

	for i, f := range fields {
		label := fieldLabelStyle.Render(f.label)
		if i == m.editCol {
			lines = append(lines, label+m.textInput.View())
		} else {
			val := f.value
			if val == "" {
				val = dimTextStyle.Render("(empty)")
			} else {
				val = inactiveFieldStyle.Render(val)
			}
			lines = append(lines, label+val)
		}
	}

	lines = append(lines, "")
	hints := fmt.Sprintf("%s next  %s prev  %s save  %s cancel",
		keyStyle.Render("tab"), keyStyle.Render("shift+tab"),
		keyStyle.Render("enter"), keyStyle.Render("esc"))
	lines = append(lines, dimTextStyle.Render(hints))

	dialog := dialogStyle.Width(w).Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Delete confirmation dialog ---

func (m model) renderDeleteDialog() string {
	if m.deleteIndex < 0 || m.deleteIndex >= len(m.scripts) {
		return ""
	}

	script := m.scripts[m.deleteIndex]
	cmdStr := script.Command
	if len(script.Args) > 0 {
		cmdStr += " " + strings.Join(script.Args, " ")
	}

	var lines []string
	lines = append(lines, errorTextStyle.Render("Delete Script?"), "")
	lines = append(lines, fieldLabelStyle.Render("Name")+script.Name)
	lines = append(lines, fieldLabelStyle.Render("Command")+dimTextStyle.Render(truncate(cmdStr, 35)))
	if script.RunCount > 0 {
		lines = append(lines, fieldLabelStyle.Render("Runs")+fmt.Sprintf("%d", script.RunCount))
	}
	lines = append(lines, "")
	lines = append(lines, keyStyle.Render("Y")+" confirm   "+keyStyle.Render("N/esc")+" cancel")

	dialog := dialogStyle.
		BorderForeground(colorError).
		Width(50).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Clear dialog ---

func (m model) renderClearDialog() string {
	var lines []string
	lines = append(lines, warnTextStyle.Render("Clear Output History"), "")
	lines = append(lines, fmt.Sprintf("  Delete files older than %s",
		keyStyle.Render(fmt.Sprintf("%d days", m.clearDays))))
	lines = append(lines, "")
	lines = append(lines,
		keyStyle.Render("+/-")+" adjust   "+
			keyStyle.Render("enter")+" confirm   "+
			keyStyle.Render("esc")+" cancel")

	dialog := dialogStyle.
		BorderForeground(colorWarn).
		Width(45).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Dry run dialog ---

func (m model) renderDryRunDialog() string {
	script := m.getScriptByDisplayIndex(m.table.Cursor())
	if script == nil {
		return ""
	}

	cmdStr := script.Command
	if len(script.Args) > 0 {
		cmdStr += " " + strings.Join(script.Args, " ")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Dry Run Preview"), "")
	lines = append(lines, fieldLabelStyle.Render("Name")+script.Name)
	lines = append(lines, fieldLabelStyle.Render("Command")+cmdStr)
	lines = append(lines, fieldLabelStyle.Render("Work Dir")+expandPath(script.WorkDir))
	if len(script.Tags) > 0 {
		lines = append(lines, fieldLabelStyle.Render("Tags")+strings.Join(script.Tags, ", "))
	}
	if len(script.EnvVars) > 0 {
		var pairs []string
		for k, v := range script.EnvVars {
			pairs = append(pairs, k+"="+v)
		}
		lines = append(lines, fieldLabelStyle.Render("Env Vars")+strings.Join(pairs, ", "))
	}
	if script.Schedule != "" {
		status := "OFF"
		if script.ScheduleOn {
			status = "ON"
		}
		lines = append(lines, fieldLabelStyle.Render("Schedule")+
			fmt.Sprintf("every %s (%s)", script.Schedule, status))
	}
	if script.RunCount > 0 {
		lines = append(lines, fieldLabelStyle.Render("Run Count")+fmt.Sprintf("%d", script.RunCount))
	}
	if script.LastRun != "" {
		lines = append(lines, fieldLabelStyle.Render("Last Run")+script.LastRun)
	}
	lines = append(lines, "", dimTextStyle.Render("Press any key to close"))

	dialog := dialogStyle.
		BorderForeground(colorAccent).
		Width(60).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Parameterized script dialog ---

func (m model) renderParamDialog() string {
	if m.paramScript == nil {
		return ""
	}

	w := 60
	if m.width-8 < w {
		w = m.width - 8
	}

	cmdStr := m.paramScript.Command
	if len(m.paramScript.Args) > 0 {
		cmdStr += " " + strings.Join(m.paramScript.Args, " ")
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Parameters"), "")
	lines = append(lines, dimTextStyle.Render(m.paramScript.Name))
	lines = append(lines, dimTextStyle.Render(cmdStr), "")

	for i, field := range m.paramFields {
		label := fieldLabelStyle.Render(field)
		if i == m.paramCursor {
			lines = append(lines, label+m.textInput.View())
		} else {
			val := m.paramValues[i]
			if val == "" {
				val = dimTextStyle.Render("(empty)")
			} else {
				val = inactiveFieldStyle.Render(val)
			}
			lines = append(lines, label+val)
		}
	}

	lines = append(lines, "")
	hints := fmt.Sprintf("%s next  %s run  %s cancel",
		keyStyle.Render("tab"), keyStyle.Render("enter"), keyStyle.Render("esc"))
	lines = append(lines, dimTextStyle.Render(hints))

	dialog := dialogStyle.Width(w).Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Schedule edit dialog ---

func (m model) renderScheduleEditDialog() string {
	if m.schedEditIndex < 0 || m.schedEditIndex >= len(m.scripts) {
		return ""
	}

	script := m.scripts[m.schedEditIndex]
	w := 50
	if m.width-8 < w {
		w = m.width - 8
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Set Schedule"), "")
	lines = append(lines, fieldLabelStyle.Render("Script")+script.Name)
	lines = append(lines, fieldLabelStyle.Render("Interval")+m.textInput.View())
	lines = append(lines, "")
	lines = append(lines, dimTextStyle.Render("Examples: 1m, 5m, 30m, 1h, 6h, 24h"))
	lines = append(lines, dimTextStyle.Render("Leave empty to clear schedule"))
	lines = append(lines, "")
	lines = append(lines,
		keyStyle.Render("enter")+" save  "+keyStyle.Render("esc")+" cancel")

	dialog := dialogStyle.Width(w).Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Help overlay ---

func (m model) renderHelp() string {
	type helpKey struct{ key, desc string }
	sections := []struct {
		title string
		keys  []helpKey
	}{
		{"Navigation", []helpKey{
			{"1/2/3/4", "Scripts / Schedules / History / Running"},
			{"?", "Toggle help"},
			{"q", "Quit"},
		}},
		{"Scripts", []helpKey{
			{"j/k, ↑/↓", "Navigate"},
			{"G/g", "Bottom / top"},
			{"ctrl+d/u", "Page down / up"},
			{"enter", "Run script"},
			{"D", "Dry run preview"},
			{"e", "Edit script"},
			{"n/a", "New script"},
			{"d", "Delete script"},
			{"/", "Search / filter"},
			{"s", "Sort (name/runs/recent)"},
			{"←/→", "Scroll columns"},
		}},
		{"Schedules", []helpKey{
			{"enter", "Toggle on / off"},
			{"e", "Set interval"},
		}},
		{"History", []helpKey{
			{"enter", "View output"},
			{"c", "Clear old files"},
		}},
		{"Running", []helpKey{
			{"j/k", "Scroll"},
			{"G/g", "End / top"},
			{"tab", "Switch tabs"},
			{"x", "Close tab"},
		}},
	}

	keyCol := lipgloss.NewStyle().Foreground(colorAccent).Width(14)

	var lines []string
	lines = append(lines, titleStyle.Render("Help"), "")
	for _, section := range sections {
		lines = append(lines, keyStyle.Render(section.title))
		for _, k := range section.keys {
			lines = append(lines, fmt.Sprintf("  %s  %s",
				keyCol.Render(k.key), k.desc))
		}
		lines = append(lines, "")
	}
	lines = append(lines, dimTextStyle.Render("Press ? or esc to close"))

	dialog := dialogStyle.
		BorderForeground(colorPrimary).
		Width(55).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		dialog)
}

// --- Utility ---

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
