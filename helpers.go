package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var placeholderRe = regexp.MustCompile(`\{\{(\w+)(?:=([^}]*))?\}\}`)

// shellSplit splits a command string into tokens, respecting single and double quotes.
func shellSplit(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		} else if c == '\'' || c == '"' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// categoryIcons maps category names (lowercase) to display icons.
var categoryIcons = map[string]string{
	"development": "⚙",
	"dev":         "⚙",
	"git":         "",
	"docker":      "🐳",
	"deploy":      "🚀",
	"deployment":  "🚀",
	"database":    "🗄",
	"db":          "🗄",
	"monitoring":  "📊",
	"network":     "🌐",
	"security":    "🔒",
	"backup":      "💾",
	"system":      "🖥",
	"test":        "🧪",
	"testing":     "🧪",
	"build":       "🔨",
	"ci":          "🔄",
	"cloud":       "☁",
	"general":     "📁",
}

func categoryIcon(name string) string {
	if icon, ok := categoryIcons[strings.ToLower(name)]; ok {
		return icon
	}
	return "📂"
}

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

// --- Parameterized scripts ---

type paramField struct {
	Name    string
	Default string
}

func extractPlaceholders(script ScriptEntry) []paramField {
	seen := map[string]bool{}
	var fields []paramField

	add := func(matches [][]string) {
		for _, match := range matches {
			if !seen[match[1]] {
				seen[match[1]] = true
				fields = append(fields, paramField{Name: match[1], Default: match[2]})
			}
		}
	}
	add(placeholderRe.FindAllStringSubmatch(script.Command, -1))
	for _, arg := range script.Args {
		add(placeholderRe.FindAllStringSubmatch(arg, -1))
	}
	return fields
}

func substitutePlaceholders(script ScriptEntry, values map[string]string) ScriptEntry {
	result := script
	replace := func(s string) string {
		return placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
			sub := placeholderRe.FindStringSubmatch(match)
			if v, ok := values[sub[1]]; ok {
				return v
			}
			if sub[2] != "" {
				return sub[2]
			}
			return match
		})
	}
	result.Command = replace(result.Command)
	newArgs := make([]string, len(result.Args))
	for i, arg := range result.Args {
		newArgs[i] = expandPath(replace(arg))
	}
	result.Args = newArgs
	return result
}

// runScript creates a RunningScript and returns the start command.
// If foreground is true, switches to the run panel.
func (m *model) runScript(script ScriptEntry, foreground bool) tea.Cmd {
	scriptID := m.nextRunID
	m.nextRunID++
	m.runningScripts = append(m.runningScripts, RunningScript{
		ID:        scriptID,
		Name:      script.Name,
		WorkDir:   script.WorkDir,
		Lines:     []string{},
		StartTime: time.Now(),
	})
	if foreground {
		m.activeRunTab = len(m.runningScripts) - 1
		m.page = pageRunning
	}
	return startScript(script, scriptID)
}

// --- Script execution (streaming) ---

func startScript(script ScriptEntry, scriptID int) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan outputLine, 100)

		go func() {
			defer close(ch)
			workDir := expandPath(script.WorkDir)

			var cmd *exec.Cmd
			if len(script.Args) > 0 {
				cmd = exec.Command(script.Command, script.Args...)
			} else {
				cmd = exec.Command(script.Command)
			}
			cmd.Dir = workDir

			if len(script.EnvVars) > 0 {
				cmd.Env = os.Environ()
				for k, v := range script.EnvVars {
					cmd.Env = append(cmd.Env, k+"="+v)
				}
			}

			r, w, err := os.Pipe()
			if err != nil {
				ch <- outputLine{done: true, err: err}
				return
			}
			cmd.Stdout = w
			cmd.Stderr = w

			if err := cmd.Start(); err != nil {
				w.Close()
				r.Close()
				ch <- outputLine{done: true, err: err}
				return
			}

			waitCh := make(chan error, 1)
			go func() {
				waitCh <- cmd.Wait()
				w.Close()
			}()

			scanner := bufio.NewScanner(r)
			buf := make([]byte, 0, 64*1024)
			scanner.Buffer(buf, 1024*1024)
			for scanner.Scan() {
				ch <- outputLine{text: scanner.Text()}
			}
			r.Close()

			cmdErr := <-waitCh
			ch <- outputLine{done: true, err: cmdErr}
		}()

		return scriptStartedMsg{scriptID: scriptID, ch: ch}
	}
}

func listenForOutput(scriptID int, ch <-chan outputLine) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return scriptFinishedMsg{scriptID: scriptID}
		}
		line, ok := <-ch
		if !ok {
			return scriptFinishedMsg{scriptID: scriptID}
		}
		if line.done {
			return scriptFinishedMsg{scriptID: scriptID, err: line.err}
		}
		return scriptLineMsg{scriptID: scriptID, line: line.text}
	}
}

func (m *model) findRunningScript(id int) *RunningScript {
	for i := range m.runningScripts {
		if m.runningScripts[i].ID == id {
			return &m.runningScripts[i]
		}
	}
	return nil
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
		return
	}

	// Parse filename for display name
	parts := strings.Split(strings.TrimSuffix(m.outputFiles[selectedIndex], ".txt"), "_")
	name := m.outputFiles[selectedIndex]
	if len(parts) >= 3 {
		name = strings.Join(parts[:len(parts)-2], " ")
	}

	lines := strings.Split(string(content), "\n")
	m.runningScripts = append(m.runningScripts, RunningScript{
		ID:    m.nextRunID,
		Name:  name,
		Lines: lines,
		Done:  true,
	})
	m.nextRunID++
	m.activeRunTab = len(m.runningScripts) - 1
	m.page = pageRunning
}

// --- Table management ---

func (m *model) updateTable() {
	sortedScripts := m.getSortedScripts()
	visibleColumns := m.table.Columns()

	var rows []table.Row
	m.scriptIndices = []int{}

	filter := strings.ToLower(m.searchFilter)
	var lastCategory string
	scriptIndex := 0

	showCategoryHeaders := m.sortMode == sortByName

	for _, script := range sortedScripts {
		if filter != "" {
			tagStr := strings.ToLower(strings.Join(script.Tags, " "))
			match := strings.Contains(strings.ToLower(script.Name), filter) ||
				strings.Contains(strings.ToLower(script.Category), filter) ||
				strings.Contains(strings.ToLower(script.Description), filter) ||
				strings.Contains(strings.ToLower(script.Command), filter) ||
				strings.Contains(tagStr, filter)
			if !match {
				scriptIndex++
				continue
			}
		}

		if showCategoryHeaders {
			displayCategory := script.Category
			if displayCategory == "" {
				displayCategory = "General"
			}

			if displayCategory != lastCategory {
				headerRow := make(table.Row, len(visibleColumns))
				headerRow[0] = fmt.Sprintf("%s %s", categoryIcon(displayCategory), displayCategory)
				for i := 1; i < len(headerRow); i++ {
					headerRow[i] = ""
				}
				rows = append(rows, headerRow)
				m.scriptIndices = append(m.scriptIndices, -1)
				lastCategory = displayCategory
			}
		}

		argsStr := strings.Join(script.Args, " ")
		fullCommand := fmt.Sprintf("%s %s", script.Command, argsStr)

		// Build display name with tags and schedule indicator
		displayName := script.Name
		if len(script.Tags) > 0 {
			displayName += "  #" + strings.Join(script.Tags, " #")
		}
		if script.ScheduleOn && script.Schedule != "" {
			displayName += "  ⏱" + script.Schedule
		}

		fullRowData := []string{
			displayName,
			fullCommand,
			fmt.Sprintf("%d", script.RunCount),
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
	m.skipHeaderRow(1)
}

func (m *model) updateOutputTable() {
	w := m.width - 10
	scriptW := w - 24
	if scriptW < 10 {
		scriptW = 10
	}
	columns := []table.Column{
		{Title: "Script", Width: scriptW},
		{Title: "Date", Width: 12},
		{Title: "Time", Width: 10},
	}

	var rows []table.Row
	for _, filename := range m.outputFiles {
		parts := strings.Split(strings.TrimSuffix(filename, ".txt"), "_")
		if len(parts) >= 3 {
			timePart := parts[len(parts)-1]
			datePart := parts[len(parts)-2]
			scriptName := strings.Join(parts[:len(parts)-2], " ")
			timeFormatted := strings.ReplaceAll(timePart, "-", ":")
			rows = append(rows, table.Row{scriptName, datePart, timeFormatted})
		} else {
			rows = append(rows, table.Row{filename, "", ""})
		}
	}

	m.outputTable.SetColumns(columns)
	m.outputTable.SetRows(rows)
	m.outputTable.SetHeight(m.height - 8)
}

func (m *model) updateScheduleTable() {
	availWidth := m.width - 6
	columns := []table.Column{
		{Title: "Name", Width: 22},
		{Title: "Category", Width: 12},
		{Title: "Schedule", Width: 10},
		{Title: "Status", Width: 8},
		{Title: "Last Run", Width: 16},
		{Title: "Next Run", Width: 18},
	}

	totalW := 0
	for _, c := range columns {
		totalW += c.Width
	}
	if extra := availWidth - totalW; extra > 0 {
		columns[0].Width += extra
	}

	now := time.Now()
	var rows []table.Row
	for _, s := range m.scripts {
		schedule := s.Schedule
		if schedule == "" {
			schedule = "—"
		}

		status := "—"
		if s.Schedule != "" {
			if s.ScheduleOn {
				status = "● ON"
			} else {
				status = "○ OFF"
			}
		}

		lastRun := s.LastRun
		if lastRun == "" {
			lastRun = "—"
		}

		nextRun := "—"
		if s.ScheduleOn && s.Schedule != "" {
			dur, err := time.ParseDuration(s.Schedule)
			if err == nil {
				if s.LastRun == "" {
					nextRun = "pending"
				} else {
					last, err := time.ParseInLocation("2006-01-02 15:04", s.LastRun, time.Local)
					if err == nil {
						next := last.Add(dur)
						if next.Before(now) {
							nextRun = "due now"
						} else {
							nextRun = next.Format("15:04") + " (" + humanDuration(time.Until(next)) + ")"
						}
					}
				}
			}
		}

		cat := s.Category
		if cat == "" {
			cat = "General"
		}

		rows = append(rows, table.Row{s.Name, cat, schedule, status, lastRun, nextRun})
	}

	m.cronTable.SetColumns(columns)
	m.cronTable.SetRows(rows)
	m.cronTable.SetHeight(m.height - 8)
}

func (m *model) getColumnIndex(title string) int {
	switch title {
	case "Name":
		return 0
	case "Command":
		return 1
	case "Runs":
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

// expandableCols lists column titles that should absorb extra horizontal space.
var expandableCols = map[string]bool{
	"Name": true, "Command": true, "Work Dir": true, "Description": true,
}

func (m *model) adjustLayout() {
	tableHeight := m.height - 8
	if tableHeight < 3 {
		tableHeight = 3
	}
	availableWidth := m.width - 6

	// Reset to base widths before recomputing
	baseWidths := map[string]int{
		"Name": 22, "Command": 40, "Runs": 6,
		"Work Dir": 30, "Description": 28, "Last Run": 18,
	}
	for i := range m.allColumns {
		if w, ok := baseWidths[m.allColumns[i].Title]; ok {
			m.allColumns[i].Width = w
		}
	}

	// Determine how many columns fit starting from scrollOffset
	startCol := m.scrollOffset
	endCol := startCol
	used := 0
	for endCol < len(m.allColumns) {
		w := m.allColumns[endCol].Width
		if used+w > availableWidth && endCol > startCol {
			break
		}
		used += w
		endCol++
	}
	if endCol == startCol {
		endCol = startCol + 1
	}
	if endCol > len(m.allColumns) {
		endCol = len(m.allColumns)
	}

	visibleColumns := make([]table.Column, endCol-startCol)
	for i := range visibleColumns {
		visibleColumns[i] = m.allColumns[startCol+i]
	}

	// Distribute extra width to expandable columns
	usedWidth := 0
	for _, col := range visibleColumns {
		usedWidth += col.Width
	}
	extra := availableWidth - usedWidth
	if extra > 0 {
		var expandIdx []int
		for i, col := range visibleColumns {
			if expandableCols[col.Title] {
				expandIdx = append(expandIdx, i)
			}
		}
		if len(expandIdx) == 0 {
			// Fallback: expand last column
			visibleColumns[len(visibleColumns)-1].Width += extra
		} else {
			each := extra / len(expandIdx)
			rem := extra % len(expandIdx)
			for _, i := range expandIdx {
				visibleColumns[i].Width += each
			}
			visibleColumns[expandIdx[len(expandIdx)-1]].Width += rem
		}
	}

	m.table.SetColumns(visibleColumns)
	m.table.SetHeight(tableHeight)
	m.maxCols = len(m.allColumns)
	m.cronTable.SetHeight(tableHeight)
	m.outputTable.SetHeight(tableHeight)
}

// --- Edit helpers ---

func (m *model) startEdit() {
	if len(m.scripts) == 0 {
		return
	}

	displayIndex := m.table.Cursor()
	origIdx := m.getOriginalIndexByDisplayIndex(displayIndex)
	if origIdx == -1 {
		return
	}

	m.mode = modeEdit
	m.editRow = origIdx
	m.editCol = 0

	w := m.width - 6
	if w > 100 {
		w = 100
	}
	if w < 50 {
		w = 50
	}
	m.textInput.Width = w - 16
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
	case 5:
		value = strings.Join(script.Tags, ", ")
	case 6:
		var pairs []string
		for k, v := range script.EnvVars {
			pairs = append(pairs, k+"="+v)
		}
		sort.Strings(pairs)
		value = strings.Join(pairs, ", ")
	}
	m.textInput.SetValue(value)
	m.textInput.SetCursor(len(value))
	m.textInput.Focus()
}

func (m *model) saveEdit() {
	if m.mode != modeEdit || m.editRow < 0 || m.editRow >= len(m.scripts) {
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
		tokens := shellSplit(value)
		if len(tokens) > 0 {
			m.scripts[m.editRow].Command = tokens[0]
			m.scripts[m.editRow].Args = tokens[1:]
		} else {
			m.scripts[m.editRow].Command = value
			m.scripts[m.editRow].Args = []string{}
		}
	case 3:
		m.scripts[m.editRow].WorkDir = expandPath(value)
	case 4:
		m.scripts[m.editRow].Description = value
	case 5:
		var tags []string
		for _, t := range strings.Split(value, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		m.scripts[m.editRow].Tags = tags
	case 6:
		envVars := make(map[string]string)
		for _, pair := range strings.Split(value, ",") {
			pair = strings.TrimSpace(pair)
			if k, v, ok := strings.Cut(pair, "="); ok && k != "" {
				envVars[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		if len(envVars) > 0 {
			m.scripts[m.editRow].EnvVars = envVars
		} else {
			m.scripts[m.editRow].EnvVars = nil
		}
	}

	m.saveScripts()
	m.updateTable()
}

func (m *model) cancelEdit() {
	m.mode = modeNormal
	m.editRow = -1
	m.editCol = -1
	m.textInput.Blur()
	m.textInput.SetValue("")
}

// --- Sorting / Lookup ---

func (m *model) getSortedScripts() []ScriptEntry {
	sorted := make([]ScriptEntry, len(m.scripts))
	copy(sorted, m.scripts)

	switch m.sortMode {
	case sortByRunCount:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].RunCount > sorted[j].RunCount
		})
	case sortByLastRun:
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].LastRun == sorted[j].LastRun {
				return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
			}
			return sorted[i].LastRun > sorted[j].LastRun
		})
	default: // sortByName — group by category, then name
		sort.Slice(sorted, func(i, j int) bool {
			catI := sorted[i].Category
			if catI == "" {
				catI = "General"
			}
			catJ := sorted[j].Category
			if catJ == "" {
				catJ = "General"
			}
			if !strings.EqualFold(catI, catJ) {
				return strings.ToLower(catI) < strings.ToLower(catJ)
			}
			return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
		})
	}

	return sorted
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

// --- Cursor helpers (skip category headers) ---

func (m *model) moveCursor(dir int) {
	cursor := m.table.Cursor()
	total := len(m.scriptIndices)
	if total == 0 {
		return
	}

	cursor += dir
	for cursor >= 0 && cursor < total && m.scriptIndices[cursor] == -1 {
		cursor += dir
	}
	if cursor < 0 {
		cursor = 0
		for cursor < total && m.scriptIndices[cursor] == -1 {
			cursor++
		}
	}
	if cursor >= total {
		cursor = total - 1
		for cursor >= 0 && m.scriptIndices[cursor] == -1 {
			cursor--
		}
	}
	if cursor >= 0 && cursor < total {
		m.table.SetCursor(cursor)
	}
}

func (m *model) moveCursorTo(target, skipDir int) {
	total := len(m.scriptIndices)
	if total == 0 {
		return
	}
	if target < 0 {
		target = 0
	}
	if target >= total {
		target = total - 1
	}
	for target >= 0 && target < total && m.scriptIndices[target] == -1 {
		target += skipDir
	}
	if target >= 0 && target < total {
		m.table.SetCursor(target)
	}
}

func (m *model) moveCursorBy(delta int) {
	total := len(m.scriptIndices)
	if total == 0 {
		return
	}
	cursor := m.table.Cursor() + delta
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	for cursor >= 0 && cursor < total && m.scriptIndices[cursor] == -1 {
		cursor += dir
	}
	if cursor >= 0 && cursor < total {
		m.table.SetCursor(cursor)
	}
}

func (m *model) skipHeaderRow(dir int) {
	cursor := m.table.Cursor()
	total := len(m.scriptIndices)
	if total == 0 || cursor < 0 || cursor >= total {
		return
	}
	if m.scriptIndices[cursor] != -1 {
		return
	}
	m.moveCursor(dir)
}

// --- Schedule helpers ---

func (m *model) checkSchedules() []tea.Cmd {
	now := time.Now()
	if !m.lastCronCheck.IsZero() && now.Sub(m.lastCronCheck) < 30*time.Second {
		return nil
	}
	m.lastCronCheck = now

	var cmds []tea.Cmd
	var started []string
	for i := range m.scripts {
		s := &m.scripts[i]
		if !s.ScheduleOn || s.Schedule == "" {
			continue
		}
		dur, err := time.ParseDuration(s.Schedule)
		if err != nil || dur < time.Minute {
			continue
		}
		// Skip if already running
		running := false
		for _, rs := range m.runningScripts {
			if rs.Name == s.Name && !rs.Done {
				running = true
				break
			}
		}
		if running {
			continue
		}
		// Check if due
		isDue := false
		if s.LastRun == "" {
			isDue = true
		} else {
			lastRun, err := time.ParseInLocation("2006-01-02 15:04", s.LastRun, time.Local)
			if err == nil && now.Sub(lastRun) >= dur {
				isDue = true
			}
		}
		if isDue {
			cmds = append(cmds, m.runScript(*s, false))
			started = append(started, s.Name)
		}
	}
	if len(started) > 0 {
		m.statusMsg = fmt.Sprintf("⏱ %s (scheduled)", strings.Join(started, ", "))
		m.statusExpiry = now.Add(3 * time.Second)
	}
	return cmds
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	min := int(d.Minutes()) % 60
	if min == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, min)
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
