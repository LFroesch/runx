package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
			rs.stdin = msg.stdin
			return m, listenForOutput(msg.scriptID, msg.ch)
		}
		return m, nil

	case scriptLineMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.Lines = append(rs.Lines, msg.line)
			visH := m.height - 8 // matches renderRunningPage visibleHeight
			maxScroll := len(rs.Lines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if rs.Scroll >= maxScroll-1 {
				rs.Scroll = maxScroll
			}
			// Detect password/passphrase prompts and mask stdin input
			lower := strings.ToLower(msg.line)
			if rs.stdin != nil && (strings.Contains(lower, "password") || strings.Contains(lower, "passphrase")) {
				m.stdinInput.EchoMode = textinput.EchoPassword
				m.stdinInput.Focus()
			}
			return m, listenForOutput(msg.scriptID, rs.ch)
		}
		return m, nil

	case scriptFinishedMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.Done = true
			rs.Err = msg.err
			rs.EndTime = time.Now()
			if rs.stdin != nil {
				rs.stdin.Close()
				rs.stdin = nil
			}
			for i := range m.scripts {
				if m.scripts[i].Name == rs.Name {
					m.scripts[i].LastRun = time.Now().Format("2006-01-02 15:04")
					m.scripts[i].RunCount++
					break
				}
			}
			m.saveScripts()
			m.updateVisibleScripts()
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

	case editorDoneMsg:
		m.scripts = loadScripts(m.configFile)
		m.updateVisibleScripts()
		m.updateScheduleTable()
		return m, showStatus("Scripts reloaded")

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateVisibleScripts()
		m.updateScheduleTable()
		if len(m.outputFiles) > 0 {
			m.updateOutputTable()
		}
		m.cronTable.SetHeight(m.height - 8)
		m.outputTable.SetHeight(m.height - 8)
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
		case modeScriptEdit:
			return m.updateScriptEdit(msg)
		case modeDeleteConfirm:
			return m.updateDeleteConfirm(msg)
		case modeClear:
			return m.updateClearMode(msg)
		case modeSearch:
			return m.updateSearch(msg)
		case modeScheduleEdit:
			return m.updateScheduleEdit(msg)
		case modeFilePicker:
			return m.updateFilePicker(msg)
		}

		// Normal mode — global keys
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.page == pageScripts {
				return m, tea.Quit
			}
			m.page = pageScripts
			return m, nil
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

	// Route non-key messages to textarea when in script edit mode
	if m.mode == modeScriptEdit {
		var cmd tea.Cmd
		m.scriptEditArea, cmd = m.scriptEditArea.Update(msg)
		return m, cmd
	}

	// Handle filepicker messages (non-key)
	if m.mode == modeFilePicker {
		var cmd tea.Cmd
		m.filePicker, cmd = m.filePicker.Update(msg)
		if m.filePicker.Path != "" {
			path := m.filePicker.Path
			m.filePicker.Path = ""
			m.mode = modeEdit
			m.textInput.SetValue(path)
			return m, nil
		}
		return m, cmd
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
			m.stdinInput.EchoMode = textinput.EchoNormal
		}
		return m, nil
	case "shift+tab":
		if len(m.runningScripts) > 1 {
			m.activeRunTab = (m.activeRunTab - 1 + len(m.runningScripts)) % len(m.runningScripts)
			m.stdinInput.EchoMode = textinput.EchoNormal
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
		maxScroll := len(rs.Lines) - (m.height - 8)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if rs.Scroll < maxScroll {
			rs.Scroll++
		}
	case "G":
		maxScroll := len(rs.Lines) - (m.height - 8)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll = maxScroll
	case "g":
		rs.Scroll = 0
	case "ctrl+d", "pagedown":
		pageSize := (m.height - 8) / 2
		maxScroll := len(rs.Lines) - (m.height - 8)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll += pageSize
		if rs.Scroll > maxScroll {
			rs.Scroll = maxScroll
		}
	case "ctrl+u", "pageup":
		pageSize := (m.height - 8) / 2
		rs.Scroll -= pageSize
		if rs.Scroll < 0 {
			rs.Scroll = 0
		}
	case "y":
		if err := copyToClipboard(rs.Output()); err != nil {
			return m, showStatus("Copy failed: no clipboard tool found")
		}
		return m, showStatus(fmt.Sprintf("Copied %d lines", len(rs.Lines)))
	case "enter":
		if !rs.Done && rs.stdin != nil {
			text := m.stdinInput.Value() + "\n"
			rs.stdin.Write([]byte(text)) //nolint:errcheck
			m.stdinInput.SetValue("")
			m.stdinInput.EchoMode = textinput.EchoNormal
			return m, nil
		}
	}

	// Route unhandled keys to stdin input when script is running interactively
	if !rs.Done && rs.stdin != nil {
		var cmd tea.Cmd
		m.stdinInput, cmd = m.stdinInput.Update(msg)
		return m, cmd
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
			m.updateVisibleScripts()
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
		m.editCol = (m.editCol + 1) % editFieldCount
		m.loadEditField()
		return m, nil
	case "shift+tab":
		m.saveEdit()
		m.editCol = (m.editCol - 1 + editFieldCount) % editFieldCount
		m.loadEditField()
		return m, nil
	case "ctrl+f", "ctrl+o":
		// Open file picker — navigate to Work Dir field first if not already there
		m.saveEdit()
		m.editCol = 4
		m.loadEditField()
		m.mode = modeFilePicker
		m.filePicker.Path = ""
		if m.editRow >= 0 && m.editRow < len(m.scripts) && m.scripts[m.editRow].WorkDir != "" {
			m.filePicker.CurrentDirectory = expandPath(m.scripts[m.editRow].WorkDir)
		}
		return m, m.filePicker.Init()
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// --- Script textarea editor (E key) ---

func (m model) updateScriptEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.scriptEditArea.Blur()
		return m, nil
	case "ctrl+s":
		if m.scriptEditFile != "" {
			err := os.WriteFile(m.scriptEditFile, []byte(m.scriptEditArea.Value()), 0644)
			if err != nil {
				return m, showStatus(fmt.Sprintf("Save failed: %v", err))
			}
		}
		m.mode = modeNormal
		m.scriptEditArea.Blur()
		return m, showStatus("Script saved")
	}

	var cmd tea.Cmd
	m.scriptEditArea, cmd = m.scriptEditArea.Update(msg)
	return m, cmd
}

// --- File picker ---

func (m model) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeEdit
		m.loadEditField()
		return m, nil
	}

	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)
	if m.filePicker.Path != "" {
		path := m.filePicker.Path
		m.filePicker.Path = ""
		m.mode = modeEdit
		m.textInput.SetValue(path)
		return m, nil
	}
	return m, cmd
}

// --- Search ---

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.searchFilter = ""
		m.searchInput.SetValue("")
		m.updateVisibleScripts()
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.searchFilter = m.searchInput.Value()
		m.updateVisibleScripts()
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	m.searchFilter = m.searchInput.Value()
	m.updateVisibleScripts()
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
			m.updateVisibleScripts()
			return m, nil
		}
	case "e":
		m.startEdit()
		return m, nil
	case "E":
		script := m.currentScript()
		if script != nil {
			origIdx := m.currentScriptIndex()
			filePath := findScriptFile(*script)
			if filePath == "" {
				return m, showStatus("No script file found in command")
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				return m, showStatus(fmt.Sprintf("Cannot read file: %v", err))
			}
			m.scriptEditIdx = origIdx
			m.scriptEditFile = filePath
			m.scriptEditArea.SetValue(string(content))
			rightW := m.width - 28 - 5
			if m.width < 80 {
				rightW = m.width - (m.width/3) - 5
			}
			m.scriptEditArea.SetWidth(rightW - 2)
			m.scriptEditArea.SetHeight(m.height - 8)
			m.scriptEditArea.Focus()
			m.mode = modeScriptEdit
			return m, m.scriptEditArea.Cursor.BlinkCmd()
		}
	case "n", "a":
		newScript := ScriptEntry{
			Name:    "New Script",
			Command: "echo",
			Args:    []string{"Hello, World!"},
			WorkDir: "~/",
		}
		m.scripts = append(m.scripts, newScript)
		m.saveScripts()
		m.updateVisibleScripts()
		// Move cursor to the new script
		for i, idx := range m.visibleScripts {
			if m.scripts[idx].Name == newScript.Name &&
				m.scripts[idx].Command == newScript.Command {
				m.scriptCursor = i
				break
			}
		}
		m.startEdit()
		return m, showStatus("New script — fill in fields, enter to save")
	case "d", "delete":
		if len(m.visibleScripts) > 0 {
			origIdx := m.currentScriptIndex()
			if origIdx == -1 {
				return m, nil
			}
			m.mode = modeDeleteConfirm
			m.deleteIndex = origIdx
		}
		return m, nil
	case " ", "enter":
		script := m.currentScript()
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
		return m, nil
	case "D":
		script := m.currentScript()
		if script != nil {
			m.mode = modeDryRun
		}
		return m, nil
	case "s":
		m.sortMode = (m.sortMode + 1) % 3
		m.updateVisibleScripts()
		labels := []string{"name", "run count", "last run"}
		return m, showStatus(fmt.Sprintf("Sort: %s", labels[m.sortMode]))
	case "r":
		m.scripts = loadScripts(m.configFile)
		m.updateVisibleScripts()
		return m, showStatus("Refreshed")
	case "v":
		m.page = pageHistory
		m.loadOutputFiles()
		m.updateOutputTable()
		return m, nil
	case "up", "k":
		if m.scriptCursor > 0 {
			m.scriptCursor--
			m.ensureCursorVisible()
		}
		return m, nil
	case "down", "j":
		if m.scriptCursor < len(m.visibleScripts)-1 {
			m.scriptCursor++
			m.ensureCursorVisible()
		}
		return m, nil
	case "G":
		m.scriptCursor = len(m.visibleScripts) - 1
		if m.scriptCursor < 0 {
			m.scriptCursor = 0
		}
		m.ensureCursorVisible()
		return m, nil
	case "g":
		m.scriptCursor = 0
		m.leftScroll = 0
		return m, nil
	case "ctrl+d", "pagedown":
		pageSize := (m.height - 10) / 2
		m.scriptCursor += pageSize
		if m.scriptCursor >= len(m.visibleScripts) {
			m.scriptCursor = len(m.visibleScripts) - 1
		}
		if m.scriptCursor < 0 {
			m.scriptCursor = 0
		}
		m.ensureCursorVisible()
		return m, nil
	case "ctrl+u", "pageup":
		pageSize := (m.height - 10) / 2
		m.scriptCursor -= pageSize
		if m.scriptCursor < 0 {
			m.scriptCursor = 0
		}
		m.ensureCursorVisible()
		return m, nil
	}
	return m, nil
}

// ensureCursorVisible adjusts leftScroll so the cursor is visible.
func (m *model) ensureCursorVisible() {
	contentH := m.height - 8
	if contentH < 5 {
		contentH = 5
	}

	// Find position of cursor in the left panel items list
	items := m.buildLeftPanelItems()
	cursorItemIdx := -1
	for i, item := range items {
		if !item.isHeader && item.scriptIdx == m.scriptCursor {
			cursorItemIdx = i
			break
		}
	}
	if cursorItemIdx == -1 {
		return
	}

	visibleH := contentH - 2
	if cursorItemIdx < m.leftScroll {
		m.leftScroll = cursorItemIdx
	}
	if cursorItemIdx >= m.leftScroll+visibleH {
		m.leftScroll = cursorItemIdx - visibleH + 1
	}
	if m.leftScroll < 0 {
		m.leftScroll = 0
	}
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
