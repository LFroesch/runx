package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	// Full-screen overlay modes
	if m.mode == modeHelp {
		return m.renderHelp()
	}
	if m.mode == modeDeleteConfirm {
		return m.renderDeleteDialog()
	}
	if m.mode == modeClear {
		return m.renderClearDialog()
	}
	if m.mode == modeParamPrompt {
		return m.renderParamDialog()
	}
	if m.mode == modeScheduleEdit {
		return m.renderScheduleEditDialog()
	}
	// Stdin prompt overlay — shown over running page when script needs input
	if m.page == pageRunning && len(m.runningScripts) > 0 {
		rs := m.runningScripts[m.activeRunTab]
		if !rs.Done && rs.stdinVisible {
			return m.renderStdinDialog(rs)
		}
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
	title := titleStyle.Render("Runx") + " " + dimTextStyle.Render(version)

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
		// Not enough room for right stats — drop them to avoid wrapping
		right = ""
		rightWidth = 0
		gap = m.width - leftWidth
		if gap < 0 {
			gap = 0
		}
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
		if m.mode == modeEdit {
			add("tab", "next")
			add("enter", "save")
			add("esc", "cancel")
			add("ctrl+d", "del line")
		} else if m.mode == modeScriptEdit {
			add("ctrl+s", "save")
			add("esc", "cancel")
			add("ctrl+d", "del line")
			add("ctrl+home/end", "top/bottom")
		} else {
			add("enter", "run")
			add("D", "dry run")
			add("e/E", "edit")
			add("n", "add")
			add("d", "delete")
			add("/", "search")
			add("s", "sort")
		}
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
		add("y", "copy")
		if m.activeRunTab < len(m.runningScripts) && m.runningScripts[m.activeRunTab].Done {
			add("r", "rerun")
		}
		add("x", "close tab")
	}

	add("1-4", "pages")
	add("?", "help")
	if m.page == pageScripts {
		add("q", "quit")
	} else {
		add("q", "scripts")
	}

	return " " + strings.Join(parts, "")
}

// --- Scripts page: split panel ---

func (m model) renderScriptsPage() string {
	if len(m.scripts) == 0 {
		return m.renderEmptyState("No scripts yet", "Press n to add your first script")
	}

	headerH := 1
	if m.mode == modeSearch {
		headerH = 2
	}
	hasStatus := m.statusMsg != "" && time.Now().Before(m.statusExpiry)
	fixedH := headerH + 1 + 1 + 1 // header + sep + sep + footer
	if hasStatus {
		fixedH++
	}
	contentH := m.height - fixedH
	if contentH < 5 {
		contentH = 5
	}

	leftW := 28
	if m.width < 80 {
		leftW = m.width / 3
	}
	rightW := m.width - leftW - 5 // account for borders + gap
	if rightW < 8 {
		rightW = 8
		leftW = m.width - rightW - 5
		if leftW < 5 {
			leftW = 5
		}
	}

	// Build left panel items
	items := m.buildLeftPanelItems()

	// Render left panel
	var leftLines []string
	visibleH := contentH - 2 // panel inner height (border eats 2)

	// Clamp scroll — account for indicator lines that consume visible rows.
	// When scroll > 0, a top indicator takes 1 line. If items still overflow
	// after the top indicator, a bottom indicator takes another line. We need
	// maxScroll to reflect these reductions so the last item is reachable.
	maxScroll := len(items) - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if maxScroll > 0 {
		effH := visibleH - 1 // top indicator consumes 1 line
		if maxScroll+effH < len(items) {
			// Bottom indicator also needed at this scroll; raise maxScroll
			maxScroll = len(items) - effH + 1
			if maxScroll < 0 {
				maxScroll = 0
			}
		}
	}
	scroll := m.leftScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Reserve a line for top indicator if needed
	if scroll > 0 {
		visibleH--
	}
	// Check if a bottom indicator is needed (before committing endIdx)
	endIdx := scroll + visibleH
	if endIdx > len(items) {
		endIdx = len(items)
	}
	needBottom := len(items) > endIdx
	if needBottom {
		visibleH--
		endIdx = scroll + visibleH
		if endIdx > len(items) {
			endIdx = len(items)
		}
	}

	// Top indicator
	if scroll > 0 {
		leftLines = append(leftLines, dimTextStyle.Render(fmt.Sprintf("  ▲ %d more", scroll)))
	}

	for i := scroll; i < endIdx; i++ {
		item := items[i]
		if item.isHeader {
			label := xansi.Truncate(item.label, leftW-2, "...")
			leftLines = append(leftLines, categoryHeaderStyle.Render(label))
		} else {
			name := item.label
			if len(name) > leftW-4 {
				name = name[:leftW-7] + "..."
			}
			if item.scriptIdx == m.scriptCursor {
				leftLines = append(leftLines, selectedItemStyle.Render("▸ "+name))
			} else {
				leftLines = append(leftLines, dimTextStyle.Render("  "+name))
			}
		}
	}

	// Bottom indicator
	if needBottom {
		remaining := len(items) - endIdx
		leftLines = append(leftLines, dimTextStyle.Render(fmt.Sprintf("  ▼ %d more", remaining)))
	}

	// Pad to height
	for len(leftLines) < contentH-2 {
		leftLines = append(leftLines, "")
	}

	leftContent := strings.Join(leftLines, "\n")
	leftPanel := panelActiveStyle.Width(leftW).Height(contentH - 2).Render(leftContent)

	// Render right panel
	var rightContent string
	switch m.mode {
	case modeEdit:
		rightContent = m.renderEditPanel(rightW)
	case modeDryRun:
		rightContent = m.renderDryRunPanel(rightW)
	case modeScriptEdit:
		rightContent = m.renderScriptEditPanel(rightW, contentH-2)
	default:
		rightContent = m.renderDetailPanel(rightW)
	}
	// Clip right content to panel height so it never overflows on short terminals
	panelInnerH := contentH - 2
	rightLines := strings.Split(rightContent, "\n")
	if len(rightLines) > panelInnerH {
		rightLines = rightLines[:panelInnerH]
	}
	rightContent = strings.Join(rightLines, "\n")
	rightPanel := panelStyle.Width(rightW).Height(panelInnerH).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

// buildLeftPanelItems creates the display list with category headers interleaved.
func (m model) buildLeftPanelItems() []leftPanelItem {
	var items []leftPanelItem
	lastCat := ""

	for i, idx := range m.visibleScripts {
		script := m.scripts[idx]
		cat := script.Category
		if cat == "" {
			cat = "General"
		}
		if m.sortMode == sortByName && cat != lastCat {
			items = append(items, leftPanelItem{isHeader: true, label: cat, scriptIdx: -1})
			lastCat = cat
		}
		items = append(items, leftPanelItem{isHeader: false, label: script.Name, scriptIdx: i})
	}
	return items
}

// renderDetailPanel shows script details in the right panel.
func (m model) renderDetailPanel(w int) string {
	if m.scriptCursor < 0 || m.scriptCursor >= len(m.visibleScripts) {
		return dimTextStyle.Render("No script selected")
	}

	script := m.scripts[m.visibleScripts[m.scriptCursor]]
	valW := w - 14

	var lines []string
	lines = append(lines, titleStyle.Render(script.Name))
	lines = append(lines, dimTextStyle.Render(strings.Repeat("─", w-4)))
	lines = append(lines, "")

	addField := func(label, value string) {
		if value == "" {
			value = dimTextStyle.Render("—")
		} else {
			value = xansi.Truncate(value, valW, "...")
		}
		lines = append(lines, detailLabelStyle.Render(label)+value)
	}

	addField("Category", script.Category)
	addField("Command", script.Command)
	if len(script.Flags) > 0 {
		addField("Flags", strings.Join(script.Flags, " "))
	}
	if len(script.Args) > 0 {
		addField("Args", strings.Join(script.Args, " "))
	}
	addField("Work Dir", script.WorkDir)
	addField("Desc", script.Description)
	if len(script.Tags) > 0 {
		addField("Tags", strings.Join(script.Tags, ", "))
	}
	if len(script.EnvVars) > 0 {
		var pairs []string
		for k, v := range script.EnvVars {
			pairs = append(pairs, k+"="+v)
		}
		addField("Env Vars", strings.Join(pairs, ", "))
	}
	if script.Schedule != "" {
		status := "OFF"
		if script.ScheduleOn {
			status = "ON"
		}
		addField("Schedule", fmt.Sprintf("every %s (%s)", script.Schedule, status))
	}

	lines = append(lines, "")
	lines = append(lines, dimTextStyle.Render(strings.Repeat("─", w-4)))

	if script.LastRun != "" {
		addField("Last Run", script.LastRun)
	}
	if script.RunCount > 0 {
		addField("Runs", fmt.Sprintf("%d", script.RunCount))
	}

	params := extractPlaceholders(script)
	if len(params) > 0 {
		lines = append(lines, "")
		var pNames []string
		for _, p := range params {
			s := p.Name
			if p.Desc != "" {
				s += " (" + p.Desc + ")"
			}
			if p.Default != "" {
				s += "=" + p.Default
			}
			pNames = append(pNames, s)
		}
		lines = append(lines, detailLabelStyle.Render("Params")+dimTextStyle.Render(xansi.Truncate(strings.Join(pNames, ", "), valW, "...")))
	}

	lines = append(lines, "")
	hints := keyStyle.Render("enter") + " run  " +
		keyStyle.Render("e") + " edit  " +
		keyStyle.Render("D") + " dry run"
	lines = append(lines, hints)

	return strings.Join(lines, "\n")
}

// renderScriptEditPanel shows the textarea editor (E key) in the right panel.
func (m model) renderScriptEditPanel(w, h int) string {
	label := ""
	if m.scriptEditFile != "" {
		label = "  " + dimTextStyle.Render(m.scriptEditFile)
	}
	header := warnTextStyle.Render("EDITING") + label + "  " + dimTextStyle.Render("ctrl+s save · esc cancel")
	m.scriptEditArea.SetWidth(w - 2)
	m.scriptEditArea.SetHeight(h - 3)
	return lipgloss.JoinVertical(lipgloss.Left, header, "", m.scriptEditArea.View())
}

// renderEditPanel shows inline edit form in the right panel.
func (m model) renderEditPanel(w int) string {
	if m.editRow < 0 || m.editRow >= len(m.scripts) {
		return ""
	}

	script := m.scripts[m.editRow]

	fields := m.editFields(script)

	var lines []string
	lines = append(lines, titleStyle.Render("Edit Script"))
	lines = append(lines, "")

	for i, f := range fields {
		label := fieldLabelStyle.Render(f.label)
		if i == m.editCol {
			lines = append(lines, label+m.textInput.View())
		} else {
			val := f.value
			if val == "" {
				val = dimTextStyle.Render("(empty)")
			} else {
				val = inactiveFieldStyle.Render(xansi.Truncate(val, w-16, "..."))
			}
			lines = append(lines, label+val)
		}
	}

	lines = append(lines, "")
	hints := fmt.Sprintf("%s next  %s prev  %s save  %s cancel",
		keyStyle.Render("tab"), keyStyle.Render("shift+tab"),
		keyStyle.Render("enter"), keyStyle.Render("esc"))
	lines = append(lines, dimTextStyle.Render(hints))

	return strings.Join(lines, "\n")
}

// renderDryRunPanel shows dry run preview in the right panel.
func (m model) renderDryRunPanel(w int) string {
	if m.scriptCursor < 0 || m.scriptCursor >= len(m.visibleScripts) {
		return ""
	}

	script := m.scripts[m.visibleScripts[m.scriptCursor]]

	var lines []string
	lines = append(lines, titleStyle.Render("Dry Run Preview"))
	lines = append(lines, "")

	addField := func(label, value string) {
		lines = append(lines, fieldLabelStyle.Render(label)+value)
	}

	addField("Name", script.Name)
	addField("Command", script.FullCommand())
	addField("Work Dir", expandPath(script.WorkDir))
	if len(script.Tags) > 0 {
		addField("Tags", strings.Join(script.Tags, ", "))
	}
	if len(script.EnvVars) > 0 {
		var pairs []string
		for k, v := range script.EnvVars {
			pairs = append(pairs, k+"="+v)
		}
		addField("Env Vars", strings.Join(pairs, ", "))
	}
	if script.Schedule != "" {
		status := "OFF"
		if script.ScheduleOn {
			status = "ON"
		}
		addField("Schedule", fmt.Sprintf("every %s (%s)", script.Schedule, status))
	}
	if script.RunCount > 0 {
		addField("Run Count", fmt.Sprintf("%d", script.RunCount))
	}
	if script.LastRun != "" {
		addField("Last Run", script.LastRun)
	}
	lines = append(lines, "", dimTextStyle.Render("enter/space to run · any other key to close"))

	return strings.Join(lines, "\n")
}

// --- Other pages ---

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
	// tabBar(1) + blank(1) + content(N) + blank(1) + status(1) = N+4
	// outer: header(1)+sep(1)+[N+4]+[statusLine up to 1]+sep(1)+footer(1) = N+8/9
	// use N=m.height-9 so layout is stable regardless of status
	visibleHeight := m.height - 9
	if visibleHeight < 3 {
		visibleHeight = 3
	}

	lines := rs.Lines
	// Truncate lines to avoid wrapping messing up layout
	maxLineW := m.width - 2
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

	var visibleLines []string
	if len(lines) > 0 && startLine < endLine {
		for _, l := range lines[startLine:endLine] {
			// strip carriage returns so they don't corrupt line display
			l = strings.ReplaceAll(l, "\r", "")
			visibleLines = append(visibleLines, xansi.Truncate(l, maxLineW, ""))
		}
	}
	var visibleContent string
	if len(visibleLines) > 0 {
		visibleContent = strings.Join(visibleLines, "\n")
	}

	contentStyle := lipgloss.NewStyle().
		Height(visibleHeight).
		MaxHeight(visibleHeight)

	content := contentStyle.Render(visibleContent)

	// Elapsed time + status
	elapsed := rs.Elapsed()
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

	parts := []string{tabBar, "", content, "", statusInfo + lineInfo}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
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

// --- Delete confirmation dialog ---

func (m model) renderDeleteDialog() string {
	if m.deleteIndex < 0 || m.deleteIndex >= len(m.scripts) {
		return ""
	}

	script := m.scripts[m.deleteIndex]

	var lines []string
	lines = append(lines, errorTextStyle.Render("Delete Script?"), "")
	lines = append(lines, fieldLabelStyle.Render("Name")+script.Name)
	lines = append(lines, fieldLabelStyle.Render("Command")+dimTextStyle.Render(truncate(script.FullCommand(), 35)))
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

	ageTab := "age"
	sizeTab := "size"
	if m.clearBySize {
		ageTab = dimTextStyle.Render(ageTab)
		sizeTab = keyStyle.Render(sizeTab)
	} else {
		ageTab = keyStyle.Render(ageTab)
		sizeTab = dimTextStyle.Render(sizeTab)
	}
	lines = append(lines, "  Mode: "+ageTab+" · "+sizeTab+"  "+dimTextStyle.Render("(tab to switch)"))
	lines = append(lines, "")

	if m.clearBySize {
		lines = append(lines, fmt.Sprintf("  Prune to %s total",
			keyStyle.Render(fmt.Sprintf("%dMB", m.clearSizeMB))))
		lines = append(lines, dimTextStyle.Render("  Deletes oldest files first"))
	} else {
		lines = append(lines, fmt.Sprintf("  Delete files older than %s",
			keyStyle.Render(fmt.Sprintf("%d days", m.clearDays))))
	}

	lines = append(lines, "")
	lines = append(lines,
		keyStyle.Render("+/-")+" adjust   "+
			keyStyle.Render("enter")+" confirm   "+
			keyStyle.Render("esc")+" cancel")

	dialog := dialogStyle.
		BorderForeground(colorWarn).
		Width(48).
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

	w := 64
	if m.width-8 < w {
		w = m.width - 8
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Run: ")+dimTextStyle.Render(m.paramScript.Name), "")
	lines = append(lines, dimTextStyle.Render(fmt.Sprintf("Field %d/%d", m.paramCursor+1, len(m.paramFields))), "")

	for i, field := range m.paramFields {
		desc := ""
		if i < len(m.paramDescs) && m.paramDescs[i] != "" {
			desc = "  " + dimTextStyle.Render(m.paramDescs[i])
		}
		label := fieldLabelStyle.Render(field)
		isEnum := i < len(m.paramOptions) && len(m.paramOptions[i]) > 0
		isActive := i == m.paramCursor

		if isEnum {
			if isActive {
				lines = append(lines, label+desc)
				for j, opt := range m.paramOptions[i] {
					display := opt
					if display == "" {
						display = dimTextStyle.Render("(none)")
					}
					if j == m.paramOptionCursors[i] {
						lines = append(lines, "  "+titleStyle.Render("▸ ")+selectedItemStyle.Render(display))
					} else {
						lines = append(lines, "  "+dimTextStyle.Render("  "+display))
					}
				}
			} else {
				val := m.paramValues[i]
				if val == "" {
					val = dimTextStyle.Render("(none)")
				} else {
					val = inactiveFieldStyle.Render(val)
				}
				lines = append(lines, label+val+desc)
			}
		} else if isActive {
			lines = append(lines, label+m.textInput.View()+desc)
		} else {
			val := m.paramValues[i]
			if val == "" {
				val = dimTextStyle.Render("(empty)")
			} else {
				val = inactiveFieldStyle.Render(val)
			}
			lines = append(lines, label+val+desc)
		}
	}

	lines = append(lines, "")
	isEnum := m.paramCursor < len(m.paramOptions) && len(m.paramOptions[m.paramCursor]) > 0
	var hints string
	if isEnum {
		hints = fmt.Sprintf("%s/%s pick  %s next/run  %s prev  %s cancel",
			keyStyle.Render("↑"), keyStyle.Render("↓"), keyStyle.Render("enter"), keyStyle.Render("shift+tab"), keyStyle.Render("esc"))
	} else {
		hints = fmt.Sprintf("%s next  %s prev  %s next/run  %s cancel",
			keyStyle.Render("tab"), keyStyle.Render("shift+tab"), keyStyle.Render("enter"), keyStyle.Render("esc"))
	}
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

// --- Stdin prompt overlay ---

func (m model) renderStdinDialog(rs RunningScript) string {
	w := 56
	if m.width-8 < w {
		w = m.width - 8
	}

	prompt := "Input"
	if rs.stdinPrompt == "password" {
		prompt = "Password"
	} else if rs.stdinPrompt == "confirm" {
		prompt = "Confirm"
	}

	var lines []string
	lines = append(lines, warnTextStyle.Render("Script requires input"), "")
	lines = append(lines, dimTextStyle.Render(rs.Name), "")
	if rs.stdinLabel != "" {
		lines = append(lines, dimTextStyle.Render(truncate(rs.stdinLabel, w-8)), "")
	}
	lines = append(lines, fieldLabelStyle.Render(prompt)+m.stdinInput.View())
	lines = append(lines, "")
	if rs.stdinPrompt == "confirm" {
		lines = append(lines, dimTextStyle.Render(
			keyStyle.Render("y/n")+" → yes/no  "+keyStyle.Render("enter")+" submit  "+keyStyle.Render("esc")+" cancel"))
	} else {
		lines = append(lines, dimTextStyle.Render(
			keyStyle.Render("enter")+" submit  "+keyStyle.Render("esc")+" cancel"))
	}

	dialog := dialogStyle.
		BorderForeground(colorWarn).
		Width(w).
		Render(strings.Join(lines, "\n"))

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
			{"e", "Edit (field form)"},
			{"E", "Edit (textarea)"},
			{"n/a", "New script"},
			{"d", "Delete script"},
			{"/", "Search / filter"},
			{"s", "Sort (name/runs/recent)"},
			{",", "Open config file"},
		}},
		{"Params ({{}})", []helpKey{
			{"{{name}}", "Prompt for value before run"},
			{"{{name=default}}", "Prompt with pre-filled default"},
			{"{{name:Desc=default}}", "Prompt with description label"},
			{"{{name:a|b|c=a}}", "Enum picker (↑/↓ to select)"},
			{"{{ name }}", "Spaces inside braces are allowed"},
		}},
		{"Edit", []helpKey{
			{"tab/shift+tab", "Next / prev field"},
			{"enter", "Save"},
			{"esc", "Cancel"},
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
			{"ctrl+d/u", "Page down / up"},
			{"tab", "Switch tabs"},
			{"r", "Rerun script"},
			{"y", "Copy output to clipboard"},
			{"y/n", "Quick confirm reply when prompted"},
			{"x", "Close tab"},
		}},
	}

	keyCol := lipgloss.NewStyle().Foreground(colorAccent).Width(16)

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
	if max < 4 {
		return ""
	}
	return xansi.Truncate(s, max, "...")
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
