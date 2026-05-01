package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// reposViewMsg is a no-op message used to trigger textarea.repositionView()
// after manual cursor navigation (CursorUp/CursorDown don't update the viewport).
type reposViewMsg struct{}

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
			rs.cmd = msg.cmd
			rs.ch = msg.ch
			rs.stdin = msg.stdin
			return m, listenForOutput(msg.scriptID, msg.ch)
		}
		return m, nil

	case scriptLineMsg:
		if rs := m.findRunningScript(msg.scriptID); rs != nil {
			rs.Lines = append(rs.Lines, msg.line)
			visH := m.height - 9 // matches renderRunningPage visibleHeight
			maxScroll := len(rs.Lines) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if rs.Scroll >= maxScroll-1 {
				rs.Scroll = maxScroll
			}
			// Detect interactive prompts and show stdin input
			if rs.stdin != nil && !rs.stdinVisible {
				if promptType, label, ok := detectStdinPrompt(msg.line); ok {
					rs.stdinVisible = true
					rs.stdinPrompt = promptType
					rs.stdinLabel = label
					if promptType == "password" {
						m.stdinInput.EchoMode = textinput.EchoPassword
					} else {
						m.stdinInput.EchoMode = textinput.EchoNormal
					}
					m.stdinInput.Width = m.width - 30
					m.stdinInput.Focus()
				}
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
		m.cronTable.SetHeight(m.height - 9)
		m.outputTable.SetHeight(m.height - 9)
		return m, nil

	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode
		if msg.String() == "ctrl+c" {
			m.killRunningScripts()
			return m, tea.Quit
		}
		// Handle modal modes first
		switch m.mode {
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
		}

		// If stdin overlay is active, swallow all keys except ctrl+c (handled above) and
		// route them to the running page handler — prevents 1/2/3/4 from switching pages.
		if m.page == pageRunning && len(m.runningScripts) > 0 {
			if rs := &m.runningScripts[m.activeRunTab]; !rs.Done && rs.stdin != nil && rs.stdinVisible {
				return m.updateRunningPage(msg)
			}
		}

		// Normal mode — global keys
		switch msg.String() {
		case "q":
			if m.page == pageScripts {
				m.killRunningScripts()
				return m, tea.Quit
			}
			m.page = pageScripts
			return m, nil
		case "?":
			m.mode = modeHelp
			m.helpScroll = 0
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

	return m, nil
}

// --- Help ---

func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q":
		m.mode = modeNormal
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	case "g":
		m.helpScroll = 0
	case "G":
		m.helpScroll = 9999 // clamped in renderHelp
	case "ctrl+d", "pagedown":
		m.helpScroll += (m.height - 6) / 2
	case "ctrl+u", "pageup":
		m.helpScroll -= (m.height - 6) / 2
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
	}
	return m, nil
}

// --- Parameterized script prompt ---

// paramIsEnum returns true if the field at idx is an enum picker.
func (m model) paramIsEnum(idx int) bool {
	return idx < len(m.paramOptions) && len(m.paramOptions[idx]) > 0
}

// paramFocusField sets up textInput (or blurs it) when moving to field idx.
func (m *model) paramFocusField(idx int) {
	if m.paramIsEnum(idx) {
		m.textInput.Blur()
		m.textInput.SetValue("")
		return
	}
	m.textInput.SetValue(m.paramValues[idx])
	m.textInput.Placeholder = m.paramFields[idx]
	m.textInput.SetCursor(len(m.paramValues[idx]))
	m.textInput.Focus()
}

// paramSaveCurrentField saves the textInput value for text fields (enum fields self-update).
func (m *model) paramSaveCurrentField() {
	if !m.paramIsEnum(m.paramCursor) {
		m.paramValues[m.paramCursor] = m.textInput.Value()
	}
}

func (m model) updateParamMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.paramScript = nil
		m.textInput.Blur()
		m.textInput.SetValue("")
		m.textInput.Placeholder = ""
		return m, showStatus("Cancelled")

	case "up", "ctrl+p":
		if m.paramIsEnum(m.paramCursor) {
			opts := m.paramOptions[m.paramCursor]
			n := len(opts)
			m.paramOptionCursors[m.paramCursor] = (m.paramOptionCursors[m.paramCursor] - 1 + n) % n
			m.paramValues[m.paramCursor] = opts[m.paramOptionCursors[m.paramCursor]]
			return m, nil
		}

	case "down", "ctrl+n":
		if m.paramIsEnum(m.paramCursor) {
			opts := m.paramOptions[m.paramCursor]
			n := len(opts)
			m.paramOptionCursors[m.paramCursor] = (m.paramOptionCursors[m.paramCursor] + 1) % n
			m.paramValues[m.paramCursor] = opts[m.paramOptionCursors[m.paramCursor]]
			return m, nil
		}

	case "enter":
		m.paramSaveCurrentField()
		if m.paramCursor < len(m.paramFields)-1 {
			m.paramCursor++
			m.paramFocusField(m.paramCursor)
			return m, nil
		}
		values := make(map[string]string)
		for i, field := range m.paramFields {
			values[field] = m.paramValues[i]
		}
		resolved := substitutePlaceholders(*m.paramScript, values)
		if unresolved := unresolvedPlaceholders(resolved); len(unresolved) > 0 {
			return m, showStatus(fmt.Sprintf("Unresolved placeholders: %s", strings.Join(unresolved, ", ")))
		}
		m.mode = modeNormal
		m.paramScript = nil
		m.textInput.Blur()
		m.textInput.SetValue("")
		m.textInput.Placeholder = ""
		return m, m.runScript(resolved, true)

	case "tab":
		m.paramSaveCurrentField()
		m.paramCursor = (m.paramCursor + 1) % len(m.paramFields)
		m.paramFocusField(m.paramCursor)
		return m, nil

	case "shift+tab":
		m.paramSaveCurrentField()
		m.paramCursor = (m.paramCursor - 1 + len(m.paramFields)) % len(m.paramFields)
		m.paramFocusField(m.paramCursor)
		return m, nil
	}

	if !m.paramIsEnum(m.paramCursor) {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Running page ---

func (m model) updateRunningPage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.runningScripts) == 0 {
		return m, nil
	}

	rs := &m.runningScripts[m.activeRunTab]

	// Stdin overlay active — intercept all keys before normal page handling.
	// Prevents 1-4/tab/r/x/y/etc. from firing while user is typing input.
	if !rs.Done && rs.stdin != nil && rs.stdinVisible {
		switch msg.String() {
		case "esc":
			cancelInput := "\n"
			if rs.stdinPrompt == "confirm" {
				cancelInput = "no\n"
			}
			rs.stdin.Write([]byte(cancelInput)) //nolint:errcheck
			m.stdinInput.SetValue("")
			m.stdinInput.EchoMode = textinput.EchoNormal
			rs.stdinVisible = false
			rs.stdinPrompt = ""
			rs.stdinLabel = ""
		case "enter":
			text := m.stdinInput.Value() + "\n"
			rs.stdin.Write([]byte(text)) //nolint:errcheck
			m.stdinInput.SetValue("")
			m.stdinInput.EchoMode = textinput.EchoNormal
			rs.stdinVisible = false
			rs.stdinPrompt = ""
			rs.stdinLabel = ""
		case "y", "Y":
			if rs.stdinPrompt == "confirm" {
				rs.stdin.Write([]byte("yes\n")) //nolint:errcheck
				m.stdinInput.SetValue("")
				m.stdinInput.EchoMode = textinput.EchoNormal
				rs.stdinVisible = false
				rs.stdinPrompt = ""
				rs.stdinLabel = ""
				return m, nil
			}
			var cmd tea.Cmd
			m.stdinInput, cmd = m.stdinInput.Update(msg)
			return m, cmd
		case "n", "N":
			if rs.stdinPrompt == "confirm" {
				rs.stdin.Write([]byte("no\n")) //nolint:errcheck
				m.stdinInput.SetValue("")
				m.stdinInput.EchoMode = textinput.EchoNormal
				rs.stdinVisible = false
				rs.stdinPrompt = ""
				rs.stdinLabel = ""
				return m, nil
			}
			var cmd tea.Cmd
			m.stdinInput, cmd = m.stdinInput.Update(msg)
			return m, cmd
		default:
			var cmd tea.Cmd
			m.stdinInput, cmd = m.stdinInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

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
	case "s":
		if !rs.Done && rs.cmd != nil && rs.cmd.Process != nil {
			rs.cmd.Process.Kill()
		}
		return m, nil
	case "r":
		if rs.Done {
			// Re-prompt for params/flags so user can toggle dry-run, change values, etc.
			// Resolve a live pointer by ID so edits since launch are honored.
			var live *ScriptEntry
			for i := range m.scripts {
				if m.scripts[i].ID == rs.Script.ID {
					live = &m.scripts[i]
					break
				}
			}
			if live == nil {
				cp := rs.Script
				live = &cp
			}
			return m, m.promptOrRun(live)
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
		maxScroll := len(rs.Lines) - (m.height - 9)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if rs.Scroll < maxScroll {
			rs.Scroll++
		}
	case "G":
		maxScroll := len(rs.Lines) - (m.height - 9)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll = maxScroll
	case "g":
		rs.Scroll = 0
	case "ctrl+d", "pagedown":
		pageSize := (m.height - 9) / 2
		maxScroll := len(rs.Lines) - (m.height - 9)
		if maxScroll < 0 {
			maxScroll = 0
		}
		rs.Scroll += pageSize
		if rs.Scroll > maxScroll {
			rs.Scroll = maxScroll
		}
	case "ctrl+u", "pageup":
		pageSize := (m.height - 9) / 2
		rs.Scroll -= pageSize
		if rs.Scroll < 0 {
			rs.Scroll = 0
		}
	case "y", "Y":
		if err := copyToClipboard(rs.Output()); err != nil {
			return m, showStatus("Copy failed: no clipboard tool found")
		}
		return m, showStatus(fmt.Sprintf("Copied %d lines", len(rs.Lines)))
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
	case "tab":
		m.clearBySize = !m.clearBySize
		return m, nil
	case "+", "=":
		if m.clearBySize {
			if m.clearSizeMB < 1000 {
				m.clearSizeMB += 10
			}
		} else {
			if m.clearDays < 365 {
				m.clearDays++
			}
		}
	case "-":
		if m.clearBySize {
			if m.clearSizeMB > 10 {
				m.clearSizeMB -= 10
			}
		} else {
			if m.clearDays > 1 {
				m.clearDays--
			}
		}
	case "enter", " ":
		var err error
		var msg string
		if m.clearBySize {
			err = m.pruneBySize(m.clearSizeMB)
			msg = fmt.Sprintf("Pruned history to %dMB", m.clearSizeMB)
		} else {
			err = m.clearOldOutputFiles()
			msg = fmt.Sprintf("Cleared files older than %d days", m.clearDays)
		}
		m.mode = modeNormal
		if err != nil {
			return m, showStatus(fmt.Sprintf("Failed: %v", err))
		}
		m.loadOutputFiles()
		m.updateOutputTable()
		return m, showStatus(msg)
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
	// Explicit line nav (guarantees these work regardless of terminal)
	case "home", "ctrl+a":
		m.textInput.CursorStart()
		return m, nil
	case "end", "ctrl+e":
		m.textInput.CursorEnd()
		return m, nil
	// Clear entire field
	case "ctrl+d":
		m.textInput.SetValue("")
		m.textInput.SetCursor(0)
		return m, nil
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
	case "ctrl+d":
		// Delete current line, reposition cursor to same line number.
		value := m.scriptEditArea.Value()
		lineNum := m.scriptEditArea.Line()
		lines := strings.Split(value, "\n")
		if lineNum < len(lines) {
			newLines := append(lines[:lineNum], lines[lineNum+1:]...)
			m.scriptEditArea.SetValue(strings.Join(newLines, "\n"))
			// SetValue leaves cursor at end; move up to target line.
			target := lineNum
			if target >= len(newLines) {
				target = len(newLines) - 1
			}
			if target < 0 {
				target = 0
			}
			for i := 0; i < len(newLines)-1-target; i++ {
				m.scriptEditArea.CursorUp()
			}
			m.scriptEditArea.CursorStart()
			// CursorUp doesn't call repositionView; pass a no-op msg to trigger it.
			m.scriptEditArea, _ = m.scriptEditArea.Update(reposViewMsg{})
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.scriptEditArea, cmd = m.scriptEditArea.Update(msg)
	return m, cmd
}

// --- File picker ---

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
				rightW = m.width - (m.width / 3) - 5
			}
			m.scriptEditArea.SetWidth(rightW - 2)
			m.scriptEditArea.SetHeight(m.height - 9)
			m.scriptEditArea.Focus()
			m.mode = modeScriptEdit
			return m, m.scriptEditArea.Cursor.BlinkCmd()
		}
	case "n", "a":
		newScript := ScriptEntry{
			ID:      generateID(),
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
			if m.scripts[idx].ID == newScript.ID {
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
				// Always show dialog — defaults are pre-filled, not auto-submitted
				m.mode = modeParamPrompt
				m.paramScript = script
				m.paramFields = make([]string, len(params))
				m.paramDescs = make([]string, len(params))
				m.paramValues = make([]string, len(params))
				m.paramOptions = make([][]string, len(params))
				m.paramOptionCursors = make([]int, len(params))
				for i, p := range params {
					m.paramFields[i] = p.Name
					m.paramDescs[i] = p.Desc
					m.paramOptions[i] = p.Options
					if len(p.Options) > 0 {
						defIdx := 0
						for j, opt := range p.Options {
							if opt == p.Default {
								defIdx = j
								break
							}
						}
						m.paramOptionCursors[i] = defIdx
						m.paramValues[i] = p.Options[defIdx]
					} else {
						m.paramValues[i] = p.Default
					}
				}
				m.paramCursor = 0
				m.paramFocusField(0)
				return m, nil
			}
			if unresolved := unresolvedPlaceholders(*script); len(unresolved) > 0 {
				return m, showStatus(fmt.Sprintf("Unresolved placeholders: %s", strings.Join(unresolved, ", ")))
			}
			return m, m.runScript(*script, true)
		}
		return m, nil
	case "s":
		m.sortMode = (m.sortMode + 1) % 3
		m.updateVisibleScripts()
		labels := []string{"name", "run count", "last run"}
		return m, showStatus(fmt.Sprintf("Sort: %s", labels[m.sortMode]))
	case ",":
		return m, openInEditor(m.configFile)
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
		pageSize := (m.height - 9) / 2
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
		pageSize := (m.height - 9) / 2
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
	contentH := m.height - 9
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
		// If a category header sits directly above, pull it into view too
		if m.leftScroll > 0 && items[m.leftScroll-1].isHeader {
			m.leftScroll--
		}
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
		m.clearSizeMB = 50
		m.clearBySize = false
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
