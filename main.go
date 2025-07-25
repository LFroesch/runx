package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ScriptEntry struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	WorkDir     string   `json:"workdir"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	LastRun     string   `json:"last_run"`
	RunCount    int      `json:"run_count"`
}

type ScriptManager struct {
	Scripts []ScriptEntry `json:"scripts"`
}

type statusMsg struct {
	message string
}

func showStatus(msg string) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{message: msg}
	}
}

type model struct {
	scripts       []ScriptEntry
	table         table.Model
	editMode      bool
	editRow       int
	editCol       int
	textInput     textinput.Model
	configFile    string
	width         int
	height        int
	statusMsg     string
	statusExpiry  time.Time
	scrollOffset  int
	maxCols       int
	scriptIndices []int
	allColumns    []table.Column
	confirmDelete bool
	deleteIndex   int
	running       bool
	runOutput     string
	viewingOutput bool
	outputFiles   []string
	outputTable   table.Model
	outputScroll  int
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configFile := filepath.Join(homeDir, ".config", "bolt", "bolt-scripts.json")

	m := model{
		scripts:       loadScripts(configFile),
		configFile:    configFile,
		width:         100,
		height:        24,
		editMode:      false,
		editRow:       -1,
		editCol:       -1,
		scrollOffset:  0,
		maxCols:       6,
		confirmDelete: false,
		deleteIndex:   -1,
		running:       false,
	}

	// Define all possible columns
	m.allColumns = []table.Column{
		{Title: "Name", Width: 25},
		{Title: "Category", Width: 20},
		{Title: "Command", Width: 30},
		{Title: "Work Dir", Width: 25},
		{Title: "Description", Width: 30},
		{Title: "Last Run", Width: 20},
	}

	// Initialize text input for editing
	m.textInput = textinput.New()
	m.textInput.CharLimit = 300

	// Initialize table with initial columns
	t := table.New(
		table.WithColumns(m.allColumns[:4]),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#374151")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#F3F4F6")).
		Background(lipgloss.Color("#1F2937"))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#F3F4F6")).
		Background(lipgloss.Color("#F97316")).
		Bold(true)
	s.Cell = s.Cell.
		Foreground(lipgloss.Color("#E5E7EB"))
	t.SetStyles(s)

	m.table = t
	
	// Initialize output table
	outputTable := table.New(
		table.WithColumns([]table.Column{{Title: "Output File", Width: 50}}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	outputTable.SetStyles(s)
	m.outputTable = outputTable
	
	m.adjustLayout()
	m.updateTable()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func loadScripts(configFile string) []ScriptEntry {
	var manager ScriptManager
	data, err := os.ReadFile(configFile)
	if err != nil {
		// Create default config directory
		os.MkdirAll(filepath.Dir(configFile), 0755)
		// Return some example scripts
		return []ScriptEntry{
			{
				Name:        "System Update",
				Command:     "bash",
				Args:        []string{"-c", "sudo apt update && sudo apt upgrade"},
				WorkDir:     "~/",
				Category:    "System",
				Description: "Update system packages",
			},
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

func (m *model) saveOutputToFile(scriptName, output, workDir string, execErr error) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	
	outputDir := filepath.Join(homeDir, ".local", "share", "scriptgodx")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.txt", scriptName, timestamp)
	// Replace spaces and special characters in filename
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
	
	outputDir := filepath.Join(homeDir, ".local", "share", "scriptgodx")
	
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
	
	// Sort files by modification time (newest first)
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

func (m *model) updateOutputTable() {
	columns := []table.Column{
		{Title: "Output File", Width: m.width - 10},
	}
	
	var rows []table.Row
	for _, filename := range m.outputFiles {
		// Parse filename to extract script name and timestamp
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
	
	outputDir := filepath.Join(homeDir, ".local", "share", "scriptgodx")
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

		// Add category header if this is a new category
		if displayCategory != lastCategory {
			categoryHeader := fmt.Sprintf("⚡ %s", displayCategory)

			headerRow := make(table.Row, len(visibleColumns))
			headerRow[0] = categoryHeader
			for i := 1; i < len(headerRow); i++ {
				headerRow[i] = ""
			}

			rows = append(rows, headerRow)
			m.scriptIndices = append(m.scriptIndices, -1)
			lastCategory = displayCategory
		}

		// Create script row
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

	script := m.scripts[m.editRow]
	var initialValue string
	switch m.editCol {
	case 0:
		initialValue = script.Name
	case 1:
		initialValue = script.Category
	case 2:
		initialValue = script.Command
	case 3:
		initialValue = strings.Join(script.Args, " ")
	case 4:
		initialValue = script.WorkDir
	case 5:
		initialValue = script.Description
	}
	m.textInput.SetValue(initialValue)
	m.textInput.SetCursor(len(initialValue))
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
		m.scripts[m.editRow].Command = value
	case 3:
		m.scripts[m.editRow].Args = strings.Fields(value)
	case 4:
		m.scripts[m.editRow].WorkDir = expandPath(value)
	case 5:
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

func (m model) Init() tea.Cmd {
	return tea.SetWindowTitle("bolt - Script Manager")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case statusMsg:
		m.statusMsg = msg.message
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
			if msg.String() == "esc" || msg.String() == "q" {
				m.running = false
				m.runOutput = ""
				m.outputScroll = 0
				return m, nil
			}
			// Handle scrolling in output view
			switch msg.String() {
			case "up", "k":
				if m.outputScroll > 0 {
					m.outputScroll--
				}
				return m, nil
			case "down", "j":
				lines := strings.Split(m.runOutput, "\n")
				maxScroll := len(lines) - (m.height - 8)
				if maxScroll < 0 {
					maxScroll = 0
				}
				if m.outputScroll < maxScroll {
					m.outputScroll++
				}
				return m, nil
			case "pageup":
				pageSize := m.height - 8
				m.outputScroll -= pageSize
				if m.outputScroll < 0 {
					m.outputScroll = 0
				}
				return m, nil
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
				return m, nil
			}
			return m, nil
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
		return m.updateNormal(msg)
	}

	if !m.editMode && !m.running {
		m.table, cmd = m.table.Update(msg)
		return m, cmd
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
			return m, showStatus(fmt.Sprintf("🗑️ Deleted %s", scriptName))
		}
		m.confirmDelete = false
		m.deleteIndex = -1
		return m, nil
	case "n", "N", "esc":
		m.confirmDelete = false
		m.deleteIndex = -1
		return m, showStatus("❌ Deletion cancelled")
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
		return m, showStatus("✅ Script updated")
	case "tab":
		m.saveEdit()
		m.editCol = (m.editCol + 1) % 6
		script := m.scripts[m.editRow]
		var newValue string
		switch m.editCol {
		case 0:
			newValue = script.Name
		case 1:
			newValue = script.Category
		case 2:
			newValue = script.Command
		case 3:
			newValue = strings.Join(script.Args, " ")
		case 4:
			newValue = script.WorkDir
		case 5:
			newValue = script.Description
		}
		m.textInput.SetValue(newValue)
		m.textInput.SetCursor(len(newValue))
		return m, nil
	case "shift+tab":
		m.saveEdit()
		m.editCol = (m.editCol - 1 + 6) % 6
		script := m.scripts[m.editRow]
		var newValue string
		switch m.editCol {
		case 0:
			newValue = script.Name
		case 1:
			newValue = script.Category
		case 2:
			newValue = script.Command
		case 3:
			newValue = strings.Join(script.Args, " ")
		case 4:
			newValue = script.WorkDir
		case 5:
			newValue = script.Description
		}
		m.textInput.SetValue(newValue)
		m.textInput.SetCursor(len(newValue))
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
	return m, nil
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
		return m, showStatus("➕ New script added")
	case "d", "delete":
		if len(m.scripts) > 0 {
			displayIndex := m.table.Cursor()
			originalIndex := m.getOriginalIndexByDisplayIndex(displayIndex)
			if originalIndex == -1 {
				return m, nil
			}
			m.confirmDelete = true
			m.deleteIndex = originalIndex
			return m, showStatus(fmt.Sprintf("❓ Delete '%s'? (y/n)", m.scripts[originalIndex].Name))
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
		return m, showStatus("🔄 Refreshed")
	case "o":
		if err := m.loadOutputFiles(); err != nil {
			return m, showStatus(fmt.Sprintf("❌ Failed to load output files: %v", err))
		}
		m.updateOutputTable()
		m.viewingOutput = true
		return m, showStatus("📄 Viewing output history")
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

	return m, nil
}

func (m model) runScript(script ScriptEntry) tea.Cmd {
	return func() tea.Msg {
		// Update last run time and count
		for i := range m.scripts {
			if m.scripts[i].Name == script.Name && m.scripts[i].Command == script.Command {
				m.scripts[i].LastRun = time.Now().Format("2006-01-02 15:04")
				m.scripts[i].RunCount++
				m.saveScripts()
				break
			}
		}

		// Expand work directory
		workDir := expandPath(script.WorkDir)

		// Create command
		var cmd *exec.Cmd
		if len(script.Args) > 0 {
			cmd = exec.Command(script.Command, script.Args...)
		} else {
			cmd = exec.Command(script.Command)
		}
		cmd.Dir = workDir

		// Run command
		output, err := cmd.CombinedOutput()
		outputStr := string(output)
		
		// Save output to file
		if saveErr := m.saveOutputToFile(script.Name, outputStr, workDir, err); saveErr != nil {
			return statusMsg{message: fmt.Sprintf("❌ Failed to save output: %v", saveErr)}
		}
		
		if err != nil {
			return statusMsg{message: fmt.Sprintf("❌ Script failed: %v (output saved)", err)}
		}

		return statusMsg{message: fmt.Sprintf("✅ Executed %s (output saved)", script.Name)}
	}
}

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

func (m model) View() string {
	if m.running {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F97316")).
			Bold(true)

		contentStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Padding(1).
			Height(m.height - 8).
			Width(m.width - 4)

		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Render("esc/q: close • ↑↓/j/k: scroll • pageup/pagedown: page scroll")

		title := titleStyle.Render("🚀 Script Output")
		
		// Handle scrolling by splitting content into lines and showing visible portion
		lines := strings.Split(m.runOutput, "\n")
		visibleHeight := m.height - 8
		startLine := m.outputScroll
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
		if len(lines) > 0 {
			visibleLines := lines[startLine:endLine]
			visibleContent = strings.Join(visibleLines, "\n")
		}
		
		content := contentStyle.Render(visibleContent)

		return lipgloss.JoinVertical(lipgloss.Left, title, "", content, "", footer)
	}

	if m.viewingOutput {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F97316")).
			Bold(true)
		header := titleStyle.Render("📄 Output History")

		if len(m.outputFiles) == 0 {
			emptyStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")).
				MarginTop(1).
				MarginBottom(1)

			content := emptyStyle.Render("No output files found.")
			footer := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#60A5FA")).
				Render("Commands: ") +
				lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("esc/q: back")

			return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
		}

		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Render("Commands: ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")).Render("space/enter: view") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(" • ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("esc/q: back")

		return lipgloss.JoinVertical(lipgloss.Left, header, m.outputTable.View(), footer)
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F97316")).
		Bold(true)
	header := titleStyle.Render("⚡ bolt - Script Manager")

	if len(m.scripts) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			MarginTop(1).
			MarginBottom(1)

		content := emptyStyle.Render("📋 No scripts registered yet.\n\n💡 Press 'n' to add your first script!")
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Render("Commands: ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Render("n/a: add script") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(" • ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("q: quit")

		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			content,
			footer,
		)
	}

	var statusMessage string
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		if strings.Contains(m.statusMsg, "❌") || strings.Contains(m.statusMsg, "Failed") {
			statusMessage = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EF4444")).
				Bold(true).
				Render("Status: " + m.statusMsg)
		} else {
			statusMessage = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")).
				Bold(true).
				Render("Status: " + m.statusMsg)
		}
	}

	var footer string
	if m.editMode {
		colNames := []string{"Name", "Category", "Command", "Args", "Work Dir", "Description"}
		colName := colNames[m.editCol]

		editStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Bold(true)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

		footer = editStyle.Render(fmt.Sprintf("✏️  Editing %s: %s", colName, m.textInput.View())) +
			helpStyle.Render("\nCommands: tab: next field • enter: save • esc: cancel")
	} else if m.confirmDelete {
		deleteStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DC2626")).
			Bold(true)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

		footer = deleteStyle.Render(fmt.Sprintf("🗑️  Delete '%s'? ", m.scripts[m.deleteIndex].Name)) +
			helpStyle.Render("y: yes • n/esc: no")
	} else {
		navStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA"))
		actionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
		editStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
		systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171"))
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

		scrollHint := ""
		if m.maxCols > len(m.table.Columns()) {
			scrollHint = " • " + navStyle.Render("←→: scroll columns")
		}

		commands := []string{
			navStyle.Render("↑↓: navigate") + scrollHint,
			actionStyle.Render("space/enter: run"),
			editStyle.Render("e: edit fields"),
			editStyle.Render("n/a: add"),
			systemStyle.Render("o: output history"),
			systemStyle.Render("d: delete"),
			systemStyle.Render("r: refresh"),
			systemStyle.Render("q: quit"),
		}
		footer = helpStyle.Render("Commands: " + strings.Join(commands, " • "))
	}

	var parts []string
	parts = append(parts, header)
	parts = append(parts, m.table.View())
	if statusMessage != "" {
		parts = append(parts, statusMessage)
	}
	parts = append(parts, footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
