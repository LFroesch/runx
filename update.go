package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case statusMsg:
		m.statusMsg = msg.message
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil

	case scriptDoneMsg:
		for i := range m.scripts {
			if m.scripts[i].Name == msg.scriptName {
				m.scripts[i].LastRun = time.Now().Format("2006-01-02 15:04")
				m.scripts[i].RunCount++
				break
			}
		}
		m.saveScripts()
		m.updateTable()

		m.runOutput = msg.output
		m.running = true
		m.outputScroll = 0

		if msg.saveErr != nil {
			m.statusMsg = fmt.Sprintf("Failed to save output: %v", msg.saveErr)
		} else if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Script failed: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Executed %s", msg.scriptName)
		}
		m.statusExpiry = time.Now().Add(3 * time.Second)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustLayout()
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		if m.running {
			return m.updateRunning(msg)
		}
		if m.viewingOutput {
			return m.updateOutputView(msg)
		}
		if m.editMode {
			return m.updateEdit(msg)
		}
		if m.confirmDelete {
			return m.updateDeleteConfirm(msg)
		}
		if m.clearMode {
			return m.updateClearMode(msg)
		}
		return m.updateNormal(msg)
	}

	if !m.editMode && !m.running {
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) updateRunning(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.running = false
		m.runOutput = ""
		m.outputScroll = 0
		return m, nil
	case "up", "k":
		if m.outputScroll > 0 {
			m.outputScroll--
		}
	case "down", "j":
		lines := strings.Split(m.runOutput, "\n")
		maxScroll := len(lines) - (m.height - 8)
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.outputScroll < maxScroll {
			m.outputScroll++
		}
	case "pageup":
		pageSize := m.height - 8
		m.outputScroll -= pageSize
		if m.outputScroll < 0 {
			m.outputScroll = 0
		}
	case "pagedown":
		pageSize := m.height - 8
		lines := strings.Split(m.runOutput, "\n")
		maxScroll := len(lines) - pageSize
		if maxScroll < 0 {
			maxScroll = 0
		}
		m.outputScroll += pageSize
		if m.outputScroll > maxScroll {
			m.outputScroll = maxScroll
		}
	}
	return m, nil
}

func (m model) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.deleteIndex >= 0 && m.deleteIndex < len(m.scripts) {
			scriptName := m.scripts[m.deleteIndex].Name
			m.scripts = append(m.scripts[:m.deleteIndex], m.scripts[m.deleteIndex+1:]...)
			m.saveScripts()
			m.updateTable()
			m.confirmDelete = false
			m.deleteIndex = -1
			return m, showStatus(fmt.Sprintf("Deleted %s", scriptName))
		}
		m.confirmDelete = false
		m.deleteIndex = -1
		return m, nil
	case "n", "N", "esc":
		m.confirmDelete = false
		m.deleteIndex = -1
		return m, showStatus("Deletion cancelled")
	}
	return m, nil
}

func (m model) updateClearMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.clearMode = false
		return m, nil
	case "+", "=":
		if m.clearDays < 365 {
			m.clearDays++
		}
		return m, nil
	case "-":
		if m.clearDays > 1 {
			m.clearDays--
		}
		return m, nil
	case "enter", " ":
		err := m.clearOldOutputFiles()
		m.clearMode = false
		if err != nil {
			return m, showStatus(fmt.Sprintf("Failed to clear files: %v", err))
		}
		return m, showStatus(fmt.Sprintf("Cleared files older than %d days", m.clearDays))
	}
	return m, nil
}

func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "enter":
		m.saveEdit()
		m.cancelEdit()
		return m, showStatus("Script updated")
	case "tab":
		m.saveEdit()
		m.editCol = (m.editCol + 1) % 5
		m.loadEditField()
		return m, nil
	case "shift+tab":
		m.saveEdit()
		m.editCol = (m.editCol - 1 + 5) % 5
		m.loadEditField()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) updateOutputView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.viewingOutput = false
		return m, nil
	case " ", "enter":
		m.viewOutputFile()
		return m, nil
	default:
		var cmd tea.Cmd
		m.outputTable, cmd = m.outputTable.Update(msg)
		return m, cmd
	}
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "e":
		m.startEdit()
		return m, nil
	case "n", "a":
		newScript := ScriptEntry{
			Name:        "New Script",
			Command:     "echo",
			Args:        []string{"Hello, World!"},
			WorkDir:     "~/",
			Category:    "",
			Description: "Script description",
		}
		m.scripts = append(m.scripts, newScript)
		m.saveScripts()
		m.updateTable()
		displayIndex := m.findScriptDisplayIndex(newScript)
		if displayIndex != -1 {
			m.table.SetCursor(displayIndex)
			m.startEdit()
		}
		return m, showStatus("New script added")
	case "d", "delete":
		if len(m.scripts) > 0 {
			displayIndex := m.table.Cursor()
			originalIndex := m.getOriginalIndexByDisplayIndex(displayIndex)
			if originalIndex == -1 {
				return m, nil
			}
			m.confirmDelete = true
			m.deleteIndex = originalIndex
			return m, showStatus(fmt.Sprintf("Delete '%s'? (y/n)", m.scripts[originalIndex].Name))
		}
		return m, nil
	case " ", "enter":
		if len(m.scripts) > 0 {
			displayIndex := m.table.Cursor()
			script := m.getScriptByDisplayIndex(displayIndex)
			if script != nil {
				return m, m.runScript(*script)
			}
		}
		return m, nil
	case "r":
		m.scripts = loadScripts(m.configFile)
		m.updateTable()
		return m, showStatus("Refreshed")
	case "v":
		if err := m.loadOutputFiles(); err != nil {
			return m, showStatus(fmt.Sprintf("Failed to load output files: %v", err))
		}
		m.updateOutputTable()
		m.viewingOutput = true
		return m, showStatus("Viewing output history")
	case "c":
		m.clearMode = true
		return m, showStatus(fmt.Sprintf("Clear outputs older than %d days (+/- to adjust, enter to confirm, esc to cancel)", m.clearDays))
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
		return m, cmd
	}
}
