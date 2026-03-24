package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Config / IO ---

func loadScripts(configFile string) []ScriptEntry {
	var manager ScriptManager
	data, err := os.ReadFile(configFile)
	if err != nil {
		os.MkdirAll(filepath.Dir(configFile), 0755)
		return []ScriptEntry{
			{
				Name:        "Git Status All",
				Command:     "bash",
				Args:        []string{"-c", "find . -name .git -type d | while read dir; do echo \"$(dirname \"$dir\")\"; cd \"$(dirname \"$dir\")\" && git status -s; cd - > /dev/null; echo; done"},
				WorkDir:     "~/projects",
				Category:    "Development",
				Description: "Check git status in all repos",
			},
		}
	}
	json.Unmarshal(data, &manager)
	return manager.Scripts
}

func (m *model) saveScripts() {
	manager := ScriptManager{Scripts: m.scripts}
	data, err := json.MarshalIndent(manager, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(m.configFile), 0755)
	os.WriteFile(m.configFile, data, 0644)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// --- Output file management ---

func (m *model) saveOutputToFile(scriptName, output, workDir string, execErr error) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	outputDir := filepath.Join(homeDir, ".local", "share", "runx")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.txt", scriptName, timestamp)
	filename = strings.ReplaceAll(filename, " ", "_")
	filename = strings.ReplaceAll(filename, "/", "_")

	filePath := filepath.Join(outputDir, filename)

	content := fmt.Sprintf("Script: %s\nExecuted: %s\nWorking Directory: %s\n\n",
		scriptName, time.Now().Format("2006-01-02 15:04:05"), workDir)

	if execErr != nil {
		content += fmt.Sprintf("Exit Status: Failed (%v)\n\n", execErr)
	} else {
		content += "Exit Status: Success\n\n"
	}

	content += "Output:\n" + strings.Repeat("-", 50) + "\n" + output

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (m *model) loadOutputFiles() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	outputDir := filepath.Join(homeDir, ".local", "share", "runx")

	files, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			m.outputFiles = []string{}
			return nil
		}
		return err
	}

	m.outputFiles = []string{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".txt") {
			m.outputFiles = append(m.outputFiles, file.Name())
		}
	}

	sort.Slice(m.outputFiles, func(i, j int) bool {
		filePathI := filepath.Join(outputDir, m.outputFiles[i])
		filePathJ := filepath.Join(outputDir, m.outputFiles[j])
		infoI, errI := os.Stat(filePathI)
		infoJ, errJ := os.Stat(filePathJ)
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	return nil
}

func (m *model) clearOldOutputFiles() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	outputDir := filepath.Join(homeDir, ".local", "share", "runx")
	files, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoffTime := time.Now().AddDate(0, 0, -m.clearDays)

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".txt") {
			filePath := filepath.Join(outputDir, file.Name())
			info, err := os.Stat(filePath)
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffTime) {
				os.Remove(filePath)
			}
		}
	}

	return nil
}

func (m *model) viewOutputFile() {
	if len(m.outputFiles) == 0 {
		return
	}

	selectedIndex := m.outputTable.Cursor()
	if selectedIndex >= len(m.outputFiles) {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	outputDir := filepath.Join(homeDir, ".local", "share", "runx")
	filePath := filepath.Join(outputDir, m.outputFiles[selectedIndex])

	content, err := os.ReadFile(filePath)
	if err != nil {
		m.runOutput = fmt.Sprintf("Error reading file: %v", err)
	} else {
		m.runOutput = string(content)
	}

	m.running = true
	m.viewingOutput = false
	m.outputScroll = 0
}

// --- Table management ---

func (m *model) updateTable() {
	sortedScripts := m.getSortedScripts()
	visibleColumns := m.table.Columns()

	var rows []table.Row
	m.scriptIndices = []int{}

	var lastCategory string
	scriptIndex := 0

	for _, script := range sortedScripts {
		displayCategory := script.Category
		if displayCategory == "" {
			displayCategory = "General"
		}

		if displayCategory != lastCategory {
			headerRow := make(table.Row, len(visibleColumns))
			headerRow[0] = fmt.Sprintf("── %s", displayCategory)
			for i := 1; i < len(headerRow); i++ {
				headerRow[i] = ""
			}
			rows = append(rows, headerRow)
			m.scriptIndices = append(m.scriptIndices, -1)
			lastCategory = displayCategory
		}

		argsStr := strings.Join(script.Args, " ")
		fullCommand := fmt.Sprintf("%s %s", script.Command, argsStr)

		fullRowData := []string{
			script.Name,
			displayCategory,
			fullCommand,
			script.WorkDir,
			script.Description,
			script.LastRun,
		}

		visibleRow := make(table.Row, len(visibleColumns))
		for i, col := range visibleColumns {
			columnIndex := m.getColumnIndex(col.Title)
			if columnIndex >= 0 && columnIndex < len(fullRowData) {
				visibleRow[i] = fullRowData[columnIndex]
			} else {
				visibleRow[i] = ""
			}
		}

		rows = append(rows, visibleRow)
		m.scriptIndices = append(m.scriptIndices, scriptIndex)
		scriptIndex++
	}
	m.table.SetRows(rows)
}

func (m *model) updateOutputTable() {
	columns := []table.Column{
		{Title: "Output File", Width: m.width - 10},
	}

	var rows []table.Row
	for _, filename := range m.outputFiles {
		parts := strings.Split(strings.TrimSuffix(filename, ".txt"), "_")
		if len(parts) >= 4 {
			scriptName := strings.Join(parts[:len(parts)-3], "_")
			timestamp := strings.Join(parts[len(parts)-3:], "_")
			displayName := fmt.Sprintf("%s (%s)", scriptName, timestamp)
			rows = append(rows, table.Row{displayName})
		} else {
			rows = append(rows, table.Row{filename})
		}
	}

	m.outputTable.SetColumns(columns)
	m.outputTable.SetRows(rows)
}

func (m *model) getColumnIndex(title string) int {
	switch title {
	case "Name":
		return 0
	case "Category":
		return 1
	case "Command":
		return 2
	case "Work Dir":
		return 3
	case "Description":
		return 4
	case "Last Run":
		return 5
	default:
		return -1
	}
}

func (m *model) adjustLayout() {
	tableHeight := m.height - 6
	if tableHeight < 5 {
		tableHeight = 5
	}

	availableWidth := m.width - 6

	totalWidth := 0
	visibleCols := 0
	for i, col := range m.allColumns {
		if totalWidth+col.Width <= availableWidth {
			totalWidth += col.Width
			visibleCols++
		} else {
			break
		}
		if i >= len(m.allColumns)-1 {
			break
		}
	}

	if visibleCols == 0 {
		visibleCols = 1
		firstCol := m.allColumns[0]
		firstCol.Width = availableWidth
		m.allColumns[0] = firstCol
	}

	startCol := m.scrollOffset
	endCol := startCol + visibleCols
	if endCol > len(m.allColumns) {
		endCol = len(m.allColumns)
		startCol = endCol - visibleCols
		if startCol < 0 {
			startCol = 0
		}
		m.scrollOffset = startCol
	}

	var visibleColumns []table.Column
	for i := startCol; i < endCol && i < len(m.allColumns); i++ {
		visibleColumns = append(visibleColumns, m.allColumns[i])
	}

	if len(visibleColumns) > 0 {
		usedWidth := 0
		for _, col := range visibleColumns {
			usedWidth += col.Width
		}
		if extraWidth := availableWidth - usedWidth; extraWidth > 0 {
			visibleColumns[len(visibleColumns)-1].Width += extraWidth
		}
	}

	m.table.SetColumns(visibleColumns)
	m.table.SetHeight(tableHeight)
	m.maxCols = len(m.allColumns)
}

// --- Edit helpers ---

func (m *model) startEdit() {
	if len(m.scripts) == 0 {
		return
	}

	m.editMode = true
	displayIndex := m.table.Cursor()
	m.editRow = m.getOriginalIndexByDisplayIndex(displayIndex)
	if m.editRow == -1 {
		return
	}
	m.editCol = 0
	m.loadEditField()
}

func (m *model) loadEditField() {
	if m.editRow < 0 || m.editRow >= len(m.scripts) {
		return
	}
	script := m.scripts[m.editRow]
	var value string
	switch m.editCol {
	case 0:
		value = script.Name
	case 1:
		value = script.Category
	case 2:
		if len(script.Args) > 0 {
			value = script.Command + " " + strings.Join(script.Args, " ")
		} else {
			value = script.Command
		}
	case 3:
		value = script.WorkDir
	case 4:
		value = script.Description
	}
	m.textInput.SetValue(value)
	m.textInput.SetCursor(len(value))
	m.textInput.Focus()
}

func (m *model) saveEdit() {
	if !m.editMode || m.editRow < 0 || m.editRow >= len(m.scripts) {
		return
	}

	value := m.textInput.Value()
	switch m.editCol {
	case 0:
		m.scripts[m.editRow].Name = value
	case 1:
		m.scripts[m.editRow].Category = value
	case 2:
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "bash -c ") {
			m.scripts[m.editRow].Command = "bash"
			script := strings.TrimPrefix(value, "bash -c ")
			script = strings.Trim(script, "\"'")
			m.scripts[m.editRow].Args = []string{"-c", script}
		} else {
			parts := strings.SplitN(value, " ", 2)
			m.scripts[m.editRow].Command = parts[0]
			if len(parts) > 1 {
				m.scripts[m.editRow].Args = []string{parts[1]}
			} else {
				m.scripts[m.editRow].Args = []string{}
			}
		}
	case 3:
		m.scripts[m.editRow].WorkDir = expandPath(value)
	case 4:
		m.scripts[m.editRow].Description = value
	}

	m.saveScripts()
	m.updateTable()
}

func (m *model) cancelEdit() {
	m.editMode = false
	m.editRow = -1
	m.editCol = -1
	m.textInput.Blur()
	m.textInput.SetValue("")
}

// --- Script execution ---

func (m model) runScript(script ScriptEntry) tea.Cmd {
	return func() tea.Msg {
		workDir := expandPath(script.WorkDir)

		var cmd *exec.Cmd
		if len(script.Args) > 0 {
			cmd = exec.Command(script.Command, script.Args...)
		} else {
			cmd = exec.Command(script.Command)
		}
		cmd.Dir = workDir

		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		saveErr := m.saveOutputToFile(script.Name, outputStr, workDir, err)

		return scriptDoneMsg{
			scriptName: script.Name,
			output:     outputStr,
			err:        err,
			saveErr:    saveErr,
		}
	}
}

// --- Sorting / Lookup ---

func (m *model) getSortedScripts() []ScriptEntry {
	sortedScripts := make([]ScriptEntry, len(m.scripts))
	copy(sortedScripts, m.scripts)

	sort.Slice(sortedScripts, func(i, j int) bool {
		categoryI := sortedScripts[i].Category
		if categoryI == "" {
			categoryI = "General"
		}
		categoryJ := sortedScripts[j].Category
		if categoryJ == "" {
			categoryJ = "General"
		}

		if !strings.EqualFold(categoryI, categoryJ) {
			return strings.ToLower(categoryI) < strings.ToLower(categoryJ)
		}

		return strings.ToLower(sortedScripts[i].Name) < strings.ToLower(sortedScripts[j].Name)
	})

	return sortedScripts
}

func (m *model) getScriptByDisplayIndex(displayIndex int) *ScriptEntry {
	if displayIndex < 0 || displayIndex >= len(m.scriptIndices) {
		return nil
	}

	scriptIndex := m.scriptIndices[displayIndex]
	if scriptIndex == -1 {
		return nil
	}

	sortedScripts := m.getSortedScripts()
	if scriptIndex >= len(sortedScripts) {
		return nil
	}

	sortedScript := sortedScripts[scriptIndex]
	for i := range m.scripts {
		if m.scripts[i].Name == sortedScript.Name &&
			m.scripts[i].Command == sortedScript.Command &&
			m.scripts[i].Category == sortedScript.Category {
			return &m.scripts[i]
		}
	}
	return nil
}

func (m *model) findScriptDisplayIndex(targetScript ScriptEntry) int {
	for i, scriptIndex := range m.scriptIndices {
		if scriptIndex == -1 {
			continue
		}

		sortedScripts := m.getSortedScripts()
		if scriptIndex < len(sortedScripts) {
			script := sortedScripts[scriptIndex]
			if script.Name == targetScript.Name &&
				script.Command == targetScript.Command &&
				script.Category == targetScript.Category {
				return i
			}
		}
	}
	return -1
}

func (m *model) getOriginalIndexByDisplayIndex(displayIndex int) int {
	if displayIndex < 0 || displayIndex >= len(m.scriptIndices) {
		return -1
	}

	scriptIndex := m.scriptIndices[displayIndex]
	if scriptIndex == -1 {
		return -1
	}

	sortedScripts := m.getSortedScripts()
	if scriptIndex >= len(sortedScripts) {
		return -1
	}

	sortedScript := sortedScripts[scriptIndex]
	for i := range m.scripts {
		if m.scripts[i].Name == sortedScript.Name &&
			m.scripts[i].Command == sortedScript.Command &&
			m.scripts[i].Category == sortedScript.Category {
			return i
		}
	}
	return -1
}

// --- Table styling ---

func styledTable(t table.Model) table.Model {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorDim).
		BorderBottom(true).
		Bold(true).
		Foreground(colorAccent).
		Align(lipgloss.Left).
		PaddingLeft(0)
	s.Selected = s.Selected.
		Foreground(colorText).
		Background(lipgloss.Color("#3A3A5C")).
		Bold(true).
		Align(lipgloss.Left).
		PaddingLeft(0)
	s.Cell = s.Cell.
		Align(lipgloss.Left).
		PaddingLeft(0)
	t.SetStyles(s)
	return t
}
