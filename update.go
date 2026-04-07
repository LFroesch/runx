package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.statusMsg != "" && time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		cmds := []tea.Cmd{tickCmd()}
		cmds = append(cmds, m.checkSchedules()...)
		return m, tea.Batch(cmds...)

	case statusMsg:
		m.statusMsg = msg.message
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil

	case scriptStartedMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.ch = msg.ch
			return m, listenForOutput(msg.scriptID, msg.ch)
		}
		return m, nil

	case scriptLineMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.Lines = append(rs.Lines, msg.line)
			visH := m.height - 8
			maxScroll := len(rs.Lines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if rs.Scroll >= maxScroll-1 {
				rs.Scroll = maxScroll
			}
			return m, listenForOutput(msg.scriptID, rs.ch)
		}
		return m, nil

	case scriptFinishedMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.Done = true
			rs.Err = msg.err
			for i := range m.scripts {
				if m.scripts[i].Name == rs.Name {
					m.scripts[i].LastRun = time.Now().Format("2006-01-02 15:04")
					m.scripts[i].RunCount++
					break
				}
			}
			m.saveScripts()
			m.updateTable()
			m.updateScheduleTable()
			m.saveOutputToFile(rs.Name, rs.Output(), rs.WorkDir, msg.err)
			if msg.err != nil {
				m.statusMsg = fmt.Sprintf("%s failed: %v", rs.Name, msg.err)
			} else {
				m.statusMsg = fmt.Sprintf("%s completed", rs.Name)
			}
			m.statusExpiry = time.Now().Add(3 * time.Second)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustLayout()
		m.updateTable()
		m.updateScheduleTable()
		if len(m.outputFiles) > 0 {
			m.updateOutputTable()
		}
		return m, nil

	case tea.KeyMsg:
		// Handle modal modes first
		switch m.mode {
		case modeDryRun:
			m.mode = modeNormal
			return m, nil
		case modeParamPrompt:
			return m.updateParamMode(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeEdit:
			return m.updateEdit(msg)
		case modeDeleteConfirm:
			return m.updateDeleteConfirm(msg)
		case modeClear:
			return m.updateClearMode(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeScheduleEdit:
			return m.updateScheduleEdit(msg)
		}

		// Normal mode — global keys
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.mode = modeHelp
			return m, nil
		case "1":
			m.page = pageScripts
			return m, nil
		case "2":
			m.page = pageSchedules
			m.updateScheduleTable()
			return m, nil
		case "3":
			m.page = pageHistory
			m.loadOutputFiles()
			m.updateOutputTable()
			return m, nil
		case "4":
			m.page = pageRunning
			return m, nil
		}

		// Page-specific handling
		switch m.page {
		case pageScripts:
			return m.updateScriptsPage(msg)
		case pageSchedules:
			return m.updateSchedulesPage(msg)
		case pageHistory:
			return m.updateHistoryPage(msg)
		case pageRunning:
			return m.updateRunningPage(msg)
		}
	}

	return m, nil
}

// --- Help ---

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q":
		m.mode = modeNormal
	}
	return m, nil
}

// --- Parameterized script prompt ---

func (m model) updateParamMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.paramScript = nil
		m.textInput.Blur()
		m.textInput.SetValue("")
		m.textInput.Placeholder = ""
		return m, showStatus("Cancelled")
	case "enter":
		m.paramValues[m.paramCursor] = m.textInput.Value()
		values := make(map[string]string)
		for i, field := range m.paramFields {
			if m.paramValues[i] != "" {
				values[field] = m.paramValues[i]
			}
		}
		resolved := substitutePlaceholders(*m.paramScript, values)
		m.mode = modeNormal
		m.paramScript = nil
		m.textInput.Blur()
		m.textInput.SetValue("")
		m.textInput.Placeholder = ""
		return m, m.runScript(resolved, true)
	case "tab":
		m.paramValues[m.paramCursor] = m.textInput.Value()
		m.paramCursor = (m.paramCursor + 1) % len(m.paramFields)
		m.textInput.SetValue(m.paramValues[m.paramCursor])
		m.textInput.Placeholder = m.paramFields[m.paramCursor]
		m.textInput.SetCursor(len(m.paramValues[m.paramCursor]))
		return m, nil
	case "shift+tab":
		m.paramValues[m.paramCursor] = m.textInput.Value()
		m.paramCursor = (m.paramCursor - 1 + len(m.paramFields)) % len(m.paramFields)
		m.textInput.SetValue(m.paramValues[m.paramCursor])
		m.textInput.Placeholder = m.paramFields[m.paramCursor]
		m.textInput.SetCursor(len(m.paramValues[m.paramCursor]))
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// --- Running page ---

func (m model) updateRunningPage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.runningScripts) == 0 {
		return m, nil
	}

	rs := &m.runningScripts[m.activeRunTab]

	switch msg.String() {
	case "tab":
		if len(m.runningScripts) > 1 {
			m.activeRunTab = (m.activeRunTab + 1) % len(m.runningScripts)
		}
		return m, nil
	case "shift+tab":
		if len(m.runningScripts) > 1 {
			m.activeRunTab = (m.activeRunTab - 1 + len(m.runningScripts)) % len(m.runningScripts)
		}
		return m, nil
	case "x":
		if rs.Done {
			m.runningScripts = append(m.runningScripts[:m.activeRunTab], m.runningScripts[m.activeRunTab+1:]...)
			if m.activeRunTab >= len(m.runningScripts) {
				m.activeRunTab = len(m.runningScripts) - 1
				if m.activeRunTab < 0 {
					m.activeRunTab = 0
				}
			}
		}
		return m, nil
	case "up", "k":
		if rs.Scroll > 0 {
			rs.Scroll--
		}
	case "down", "j":
		maxScroll := len(rs.Lines) - (m.height - 12)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if rs.Scroll < maxScroll {
			rs.Scroll++
		}
	case "G":
		maxScroll := len(rs.Lines) - (m.height - 12)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll = maxScroll
	case "g":
		rs.Scroll = 0
	case "ctrl+d", "pagedown":
		pageSize := (m.height - 12) / 2
		maxScroll := len(rs.Lines) - (m.height - 12)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll += pageSize
		if rs.Scroll > maxScroll {
			rs.Scroll = maxScroll
		}
	case "ctrl+u", "pageup":
		pageSize := (m.height - 12) / 2
		rs.Scroll -= pageSize
		if rs.Scroll < 0 {
			rs.Scroll = 0
		}
	}
	return m, nil
}

// --- Delete confirmation ---

func (m model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.deleteIndex >= 0 && m.deleteIndex < len(m.scripts) {
			scriptName := m.scripts[m.deleteIndex].Name
			m.scripts = append(m.scripts[:m.deleteIndex], m.scripts[m.deleteIndex+1:]...)
			m.saveScripts()
			m.updateTable()
			m.mode = modeNormal
			m.deleteIndex = -1
			return m, showStatus(fmt.Sprintf("Deleted %s", scriptName))
		}
		m.mode = modeNormal
		m.deleteIndex = -1
		return m, nil
	case "n", "N", "esc":
		m.mode = modeNormal
		m.deleteIndex = -1
		return m, showStatus("Cancelled")
	}
	return m, nil
}

// --- Clear mode ---

func (m model) updateClearMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeNormal
		return m, nil
	case "+", "=":
		if m.clearDays < 365 {
			m.clearDays++
		}
	case "-":
		if m.clearDays > 1 {
			m.clearDays--
		}
	case "enter", " ":
		err := m.clearOldOutputFiles()
		m.mode = modeNormal
		if err != nil {
			return m, showStatus(fmt.Sprintf("Failed: %v", err))
		}
		m.loadOutputFiles()
		m.updateOutputTable()
		return m, showStatus(fmt.Sprintf("Cleared files older than %d days", m.clearDays))
	}
	return m, nil
}

// --- Edit mode ---

func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "enter":
		m.saveEdit()
		m.cancelEdit()
		return m, showStatus("Script saved")
	case "tab":
		m.saveEdit()
		m.editCol = (m.editCol + 1) % 7
		m.loadEditField()
		return m, nil
	case "shift+tab":
		m.saveEdit()
		m.editCol = (m.editCol - 1 + 7) % 7
		m.loadEditField()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// --- Search ---

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.searchFilter = ""
		m.searchInput.SetValue("")
		m.updateTable()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.searchFilter = m.searchInput.Value()
		m.updateTable()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchFilter = m.searchInput.Value()
	m.updateTable()
	return m, cmd
}

// --- Schedule edit ---

func (m model) updateScheduleEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.textInput.Blur()
		m.textInput.SetValue("")
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.textInput.Value())
		if val == "" {
			m.scripts[m.schedEditIndex].Schedule = ""
			m.scripts[m.schedEditIndex].ScheduleOn = false
			m.saveScripts()
			m.updateScheduleTable()
			m.mode = modeNormal
			m.textInput.Blur()
			m.textInput.SetValue("")
			return m, showStatus("Schedule cleared")
		}
		dur, err := time.ParseDuration(val)
		if err != nil || dur < time.Minute {
			return m, showStatus("Invalid interval (min 1m). Examples: 5m, 30m, 1h")
		}
		m.scripts[m.schedEditIndex].Schedule = val
		m.scripts[m.schedEditIndex].ScheduleOn = true
		m.saveScripts()
		m.updateScheduleTable()
		m.mode = modeNormal
		m.textInput.Blur()
		m.textInput.SetValue("")
		return m, showStatus(fmt.Sprintf("Schedule set: every %s", val))
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// --- Scripts page ---

func (m model) updateScriptsPage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.mode = modeSearch
		m.searchInput.SetValue(m.searchFilter)
		m.searchInput.Focus()
		return m, nil
	case "esc":
		if m.searchFilter != "" {
			m.searchFilter = ""
			m.searchInput.SetValue("")
			m.updateTable()
			return m, nil
		}
	case "e":
		m.startEdit()
		return m, nil
	case "n", "a":
		newScript := ScriptEntry{
			Name:    "New Script",
			Command: "echo",
			Args:    []string{"Hello, World!"},
			WorkDir: "~/",
		}
		m.scripts = append(m.scripts, newScript)
		m.saveScripts()
		m.updateTable()
		displayIndex := m.findScriptDisplayIndex(newScript)
		if displayIndex != -1 {
			m.table.SetCursor(displayIndex)
			m.startEdit()
		}
		return m, showStatus("New script — fill in fields, enter to save")
	case "d", "delete":
		if len(m.scripts) > 0 {
			displayIndex := m.table.Cursor()
			originalIndex := m.getOriginalIndexByDisplayIndex(displayIndex)
			if originalIndex == -1 {
				return m, nil
			}
			m.mode = modeDeleteConfirm
			m.deleteIndex = originalIndex
		}
		return m, nil
	case " ", "enter":
		if len(m.scripts) > 0 {
			displayIndex := m.table.Cursor()
			script := m.getScriptByDisplayIndex(displayIndex)
			if script != nil {
				params := extractPlaceholders(*script)
				if len(params) > 0 {
					allHaveDefaults := true
					for _, p := range params {
						if p.Default == "" {
							allHaveDefaults = false
							break
						}
					}
					if allHaveDefaults {
						values := make(map[string]string)
						for _, p := range params {
							values[p.Name] = p.Default
						}
						resolved := substitutePlaceholders(*script, values)
						return m, m.runScript(resolved, true)
					}
					m.mode = modeParamPrompt
					m.paramScript = script
					m.paramFields = make([]string, len(params))
					m.paramValues = make([]string, len(params))
					for i, p := range params {
						m.paramFields[i] = p.Name
						m.paramValues[i] = p.Default
					}
					m.paramCursor = 0
					m.textInput.SetValue(m.paramValues[0])
					m.textInput.Placeholder = m.paramFields[0]
					m.textInput.SetCursor(len(m.paramValues[0]))
					m.textInput.Focus()
					return m, nil
				}
				return m, m.runScript(*script, true)
			}
		}
		return m, nil
	case "D":
		if len(m.scripts) > 0 {
			script := m.getScriptByDisplayIndex(m.table.Cursor())
			if script != nil {
				m.mode = modeDryRun
			}
		}
		return m, nil
	case "s":
		m.sortMode = (m.sortMode + 1) % 3
		m.updateTable()
		labels := []string{"name", "run count", "last run"}
		return m, showStatus(fmt.Sprintf("Sort: %s", labels[m.sortMode]))
	case "r":
		m.scripts = loadScripts(m.configFile)
		m.updateTable()
		return m, showStatus("Refreshed")
	case "v":
		m.page = pageHistory
		m.loadOutputFiles()
		m.updateOutputTable()
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "G":
		m.moveCursorTo(len(m.scriptIndices)-1, -1)
		return m, nil
	case "g":
		m.moveCursorTo(0, 1)
		return m, nil
	case "ctrl+d", "pagedown":
		pageSize := (m.height - 6) / 2
		m.moveCursorBy(pageSize)
		return m, nil
	case "ctrl+u", "pageup":
		pageSize := (m.height - 6) / 2
		m.moveCursorBy(-pageSize)
		return m, nil
	case "left":
		if m.scrollOffset > 0 {
			m.scrollOffset--
			m.adjustLayout()
			m.updateTable()
		}
		return m, nil
	case "right":
		maxOffset := m.maxCols - len(m.table.Columns())
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.scrollOffset < maxOffset {
			m.scrollOffset++
			m.adjustLayout()
			m.updateTable()
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		m.skipHeaderRow(1)
		return m, cmd
	}
	return m, nil
}

// --- Schedules page ---

func (m model) updateSchedulesPage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", " ":
		idx := m.cronTable.Cursor()
		if idx >= 0 && idx < len(m.scripts) {
			if m.scripts[idx].Schedule == "" {
				return m, showStatus("Set a schedule first (press e)")
			}
			m.scripts[idx].ScheduleOn = !m.scripts[idx].ScheduleOn
			m.saveScripts()
			m.updateScheduleTable()
			if m.scripts[idx].ScheduleOn {
				return m, showStatus(fmt.Sprintf("● %s: every %s", m.scripts[idx].Name, m.scripts[idx].Schedule))
			}
			return m, showStatus(fmt.Sprintf("○ %s: schedule paused", m.scripts[idx].Name))
		}
		return m, nil
	case "e":
		idx := m.cronTable.Cursor()
		if idx >= 0 && idx < len(m.scripts) {
			m.schedEditIndex = idx
			m.mode = modeScheduleEdit
			m.textInput.SetValue(m.scripts[idx].Schedule)
			m.textInput.Placeholder = "e.g. 5m, 1h, 30m"
			m.textInput.Focus()
			m.textInput.SetCursor(len(m.scripts[idx].Schedule))
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.cronTable, cmd = m.cronTable.Update(msg)
		return m, cmd
	}
}

// --- History page ---

func (m model) updateHistoryPage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", " ":
		m.viewOutputFile()
		return m, nil
	case "c":
		m.mode = modeClear
		m.clearDays = 7
		return m, nil
	case "r":
		m.loadOutputFiles()
		m.updateOutputTable()
		return m, showStatus("Refreshed")
	default:
		var cmd tea.Cmd
		m.outputTable, cmd = m.outputTable.Update(msg)
		return m, cmd
	}
}
