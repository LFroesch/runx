package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// Placeholder syntax: {{name}}, {{name=default}}, {{name:Description=default}}
// Supports optional inner whitespace and names like "foo.bar" or "open-job".
var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9][A-Za-z0-9_.-]*)(?:\s*:\s*([^=}]*?))?(?:\s*=\s*([^}]*?))?\s*\}\}`)
var ttyInteractiveRe = regexp.MustCompile(`(?i)\b(fzf|gum|whiptail|dialog|vim|nvim|less|more|top|htop|watch|tmux)\b`)

// shellSplit splits a command string into tokens, respecting quotes.
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
	if err := json.Unmarshal(data, &manager); err == nil {
		return manager.Scripts
	}
	backupFile := configFile + ".bak"
	backupData, backupErr := os.ReadFile(backupFile)
	if backupErr == nil {
		var backupManager ScriptManager
		if json.Unmarshal(backupData, &backupManager) == nil {
			fmt.Fprintf(os.Stderr, "runx: config parse failed, recovered from backup: %s\n", backupFile)
			return backupManager.Scripts
		}
	}
	fmt.Fprintf(os.Stderr, "runx: config parse failed and no valid backup found: %s\n", configFile)
	return []ScriptEntry{}
}

func (m *model) saveScripts() {
	manager := ScriptManager{Scripts: m.scripts}
	data, err := json.MarshalIndent(manager, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(m.configFile), 0755)
	if existing, readErr := os.ReadFile(m.configFile); readErr == nil {
		_ = os.WriteFile(m.configFile+".bak", existing, 0644)
	}
	os.WriteFile(m.configFile, data, 0644)
}

// findScriptFile looks through a script's command+args for a real file on disk.
func findScriptFile(s ScriptEntry) string {
	candidates := append([]string{s.Command}, s.Flags...)
	candidates = append(candidates, s.Args...)
	for _, c := range candidates {
		expanded := expandPath(c)
		if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
			return expanded
		}
	}
	return ""
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

// --- Visible scripts (replaces old table management) ---

func (m *model) updateVisibleScripts() {
	sorted := m.getSortedScripts()
	filter := strings.ToLower(m.searchFilter)

	m.visibleScripts = m.visibleScripts[:0]

	for _, script := range sorted {
		if filter != "" {
			tagStr := strings.ToLower(strings.Join(script.Tags, " "))
			match := strings.Contains(strings.ToLower(script.Name), filter) ||
				strings.Contains(strings.ToLower(script.Category), filter) ||
				strings.Contains(strings.ToLower(script.Description), filter) ||
				strings.Contains(strings.ToLower(script.Command), filter) ||
				strings.Contains(tagStr, filter)
			if !match {
				continue
			}
		}
		// Find original index
		for i := range m.scripts {
			if m.scripts[i].Name == script.Name &&
				m.scripts[i].Command == script.Command &&
				m.scripts[i].Category == script.Category {
				m.visibleScripts = append(m.visibleScripts, i)
				break
			}
		}
	}

	// Clamp cursor
	if m.scriptCursor >= len(m.visibleScripts) {
		m.scriptCursor = len(m.visibleScripts) - 1
	}
	if m.scriptCursor < 0 {
		m.scriptCursor = 0
	}
}

// currentScript returns the currently selected script, or nil.
func (m *model) currentScript() *ScriptEntry {
	if m.scriptCursor < 0 || m.scriptCursor >= len(m.visibleScripts) {
		return nil
	}
	return &m.scripts[m.visibleScripts[m.scriptCursor]]
}

// currentScriptIndex returns the original index of the selected script.
func (m *model) currentScriptIndex() int {
	if m.scriptCursor < 0 || m.scriptCursor >= len(m.visibleScripts) {
		return -1
	}
	return m.visibleScripts[m.scriptCursor]
}

// --- Parameterized scripts ---

type paramField struct {
	Name    string
	Desc    string   // optional: from {{name:Description=default}}
	Default string
	Options []string // non-nil when desc is "opt1|opt2|opt3" (enum picker)
}

func normalizePlaceholderParts(name, desc, def string) (string, string, string) {
	return strings.TrimSpace(name), strings.TrimSpace(desc), strings.TrimSpace(def)
}

func extractPlaceholders(script ScriptEntry) []paramField {
	seen := map[string]int{}
	var fields []paramField

	add := func(matches [][]string) {
		for _, match := range matches {
			name, desc, def := normalizePlaceholderParts(match[1], match[2], match[3])
			// If desc contains "|", treat as enum options list
			var opts []string
			if strings.Contains(desc, "|") {
				opts = strings.Split(desc, "|")
				desc = ""
			}
			if idx, ok := seen[name]; ok {
				// Merge repeated placeholder metadata so later declarations can
				// fill missing description/default from earlier ones.
				if fields[idx].Desc == "" && desc != "" {
					fields[idx].Desc = desc
				}
				if fields[idx].Default == "" && def != "" {
					fields[idx].Default = def
				}
				if fields[idx].Options == nil && opts != nil {
					fields[idx].Options = opts
				}
				continue
			}
			seen[name] = len(fields)
			fields = append(fields, paramField{Name: name, Desc: desc, Default: def, Options: opts})
		}
	}
	add(placeholderRe.FindAllStringSubmatch(script.Command, -1))
	add(placeholderRe.FindAllStringSubmatch(script.WorkDir, -1))
	for _, arg := range script.Flags {
		add(placeholderRe.FindAllStringSubmatch(arg, -1))
	}
	for _, arg := range script.Args {
		add(placeholderRe.FindAllStringSubmatch(arg, -1))
	}
	for _, v := range script.EnvVars {
		add(placeholderRe.FindAllStringSubmatch(v, -1))
	}
	return fields
}

func unresolvedPlaceholders(script ScriptEntry) []string {
	seen := map[string]bool{}
	var unresolved []string
	addMatches := func(s string) {
		for _, match := range placeholderRe.FindAllStringSubmatch(s, -1) {
			name, _, _ := normalizePlaceholderParts(match[1], "", "")
			if name != "" && !seen[name] {
				seen[name] = true
				unresolved = append(unresolved, name)
			}
		}
	}
	addMatches(script.Command)
	addMatches(script.WorkDir)
	for _, f := range script.Flags {
		addMatches(f)
	}
	for _, a := range script.Args {
		addMatches(a)
	}
	for _, v := range script.EnvVars {
		addMatches(v)
	}
	sort.Strings(unresolved)
	return unresolved
}

func substitutePlaceholders(script ScriptEntry, values map[string]string) ScriptEntry {
	result := script
	replace := func(s string) string {
		return placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
			sub := placeholderRe.FindStringSubmatch(match)
			name, _, def := normalizePlaceholderParts(sub[1], "", sub[3])
			if v, ok := values[name]; ok {
				return v
			}
			if def != "" {
				return def
			}
			return match
		})
	}
	result.Command = replace(result.Command)
	newFlags := make([]string, len(result.Flags))
	for i, f := range result.Flags {
		newFlags[i] = replace(f)
	}
	result.Flags = newFlags
	newArgs := make([]string, len(result.Args))
	for i, arg := range result.Args {
		newArgs[i] = expandPath(replace(arg))
	}
	result.Args = newArgs
	result.WorkDir = expandPath(replace(result.WorkDir))
	if len(result.EnvVars) > 0 {
		newEnv := make(map[string]string, len(result.EnvVars))
		for k, v := range result.EnvVars {
			newEnv[k] = replace(v)
		}
		result.EnvVars = newEnv
	}
	return result
}

// runScript creates a RunningScript and returns the start command.
func (m *model) runScript(script ScriptEntry, foreground bool) tea.Cmd {
	scriptID := m.nextRunID
	m.nextRunID++
	ttyWarning := ttyInteractiveWarning(script)
	lines := []string{}
	if ttyWarning != "" {
		lines = append(lines, ttyWarning)
	}
	m.runningScripts = append(m.runningScripts, RunningScript{
		ID:        scriptID,
		Name:      script.Name,
		WorkDir:   script.WorkDir,
		Lines:     lines,
		StartTime: time.Now(),
	})
	if foreground {
		m.activeRunTab = len(m.runningScripts) - 1
		m.page = pageRunning
	}
	return startScript(script, scriptID)
}

// --- Script execution (streaming) ---

func ttyInteractiveWarning(script ScriptEntry) string {
	full := script.FullCommand()
	if ttyInteractiveRe.MatchString(full) {
		return "⚠ Script may require a full TTY UI; if interaction looks broken, run it in a normal terminal."
	}
	return ""
}

func preflightScript(script ScriptEntry) error {
	workDir := expandPath(script.WorkDir)
	if workDir != "" {
		info, err := os.Stat(workDir)
		if err != nil {
			return fmt.Errorf("invalid working directory %q: %w", workDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("working directory is not a directory: %q", workDir)
		}
	}
	if strings.Contains(script.Command, "/") {
		cmdPath := expandPath(script.Command)
		info, err := os.Stat(cmdPath)
		if err != nil {
			return fmt.Errorf("command not found: %q", cmdPath)
		}
		if info.IsDir() {
			return fmt.Errorf("command path is a directory: %q", cmdPath)
		}
		return nil
	}
	if _, err := exec.LookPath(script.Command); err != nil {
		return fmt.Errorf("command not found in PATH: %q", script.Command)
	}
	return nil
}

func startScript(script ScriptEntry, scriptID int) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan outputLine, 100)
		if err := preflightScript(script); err != nil {
			go func() {
				ch <- outputLine{done: true, err: err}
				close(ch)
			}()
			return scriptStartedMsg{scriptID: scriptID, ch: ch}
		}
		workDir := expandPath(script.WorkDir)

		args := script.FullArgs()
		var cmd *exec.Cmd
		if len(args) > 0 {
			cmd = exec.Command(script.Command, args...)
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

		stdinPipe, stdinErr := cmd.StdinPipe()

		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			go func() {
				ch <- outputLine{done: true, err: pipeErr}
				close(ch)
			}()
			return scriptStartedMsg{scriptID: scriptID, ch: ch}
		}
		cmd.Stdout = w
		cmd.Stderr = w

		if err := cmd.Start(); err != nil {
			w.Close()
			r.Close()
			go func() {
				ch <- outputLine{done: true, err: err}
				close(ch)
			}()
			return scriptStartedMsg{scriptID: scriptID, ch: ch}
		}

		go func() {
			defer close(ch)
			waitCh := make(chan error, 1)
			go func() {
				waitCh <- cmd.Wait()
				w.Close()
			}()

			// Read in a separate goroutine so we can use select+timer below.
			readCh := make(chan []byte, 32)
			go func() {
				tmp := make([]byte, 4096)
				for {
					n, err := r.Read(tmp)
					if n > 0 {
						data := make([]byte, n)
						copy(data, tmp[:n])
						readCh <- data
					}
					if err != nil {
						close(readCh)
						return
					}
				}
			}()

			var pending []byte
			// flushTimer fires when no new data arrives for 100ms — emits any
			// unterminated line (e.g. "read -rp" prompts that have no trailing \n).
			flushTimer := time.NewTimer(100 * time.Millisecond)
			defer flushTimer.Stop()

			emitLines := func(flushPartial bool) {
				for {
					idx := bytes.IndexByte(pending, '\n')
					if idx == -1 {
						break
					}
					line := strings.TrimRight(string(pending[:idx]), "\r")
					pending = pending[idx+1:]
					ch <- outputLine{text: line}
				}
				if flushPartial && len(pending) > 0 {
					line := strings.TrimRight(string(pending), "\r")
					if line != "" {
						ch <- outputLine{text: line}
					}
					pending = nil
				}
			}

			resetTimer := func() {
				if !flushTimer.Stop() {
					select {
					case <-flushTimer.C:
					default:
					}
				}
				flushTimer.Reset(100 * time.Millisecond)
			}

			for {
				select {
				case data, ok := <-readCh:
					if !ok {
						emitLines(true)
						r.Close()
						cmdErr := <-waitCh
						ch <- outputLine{done: true, err: cmdErr}
						return
					}
					resetTimer()
					pending = append(pending, data...)
					emitLines(false)
				case <-flushTimer.C:
					emitLines(true)
				}
			}
		}()

		var stdin io.WriteCloser
		if stdinErr == nil {
			stdin = stdinPipe
		}
		return scriptStartedMsg{scriptID: scriptID, ch: ch, stdin: stdin}
	}
}

// openInEditor opens the given file in $VISUAL/$EDITOR, or probes for an available editor.
func openInEditor(path string) tea.Cmd {
	// Check env vars first
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(env); e != "" {
			if _, err := exec.LookPath(e); err == nil {
				return tea.ExecProcess(exec.Command(e, path), func(err error) tea.Msg {
					return editorDoneMsg{}
				})
			}
		}
	}
	// Fallback: probe
	for _, e := range []string{"nvim", "vim", "nano", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return tea.ExecProcess(exec.Command(e, path), func(err error) tea.Msg {
				return editorDoneMsg{}
			})
		}
	}
	return nil
}

func copyToClipboard(text string) error {
	tools := [][]string{
		{"clip.exe"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, args := range tools {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no clipboard tool found (tried clip.exe, wl-copy, xclip, xsel)")
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

var confirmPromptRe = regexp.MustCompile(`(?i)(\[\s*y(?:es)?\s*/\s*n(?:o)?\s*\]|\(\s*y(?:es)?\s*/\s*n(?:o)?\s*\))`)

// detectStdinPrompt returns prompt kind ("password", "confirm", "input") and
// a cleaned prompt label from output lines.
func detectStdinPrompt(line string) (string, string, bool) {
	clean := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if clean == "" {
		return "", "", false
	}
	lower := strings.ToLower(clean)
	hasPromptKeyword := strings.Contains(lower, "enter ") ||
		strings.Contains(lower, "input") ||
		strings.Contains(lower, "type ") ||
		strings.Contains(lower, "provide ") ||
		strings.Contains(lower, "select ") ||
		strings.Contains(lower, "choice") ||
		strings.Contains(lower, "value") ||
		strings.Contains(lower, "token")
	switch {
	case strings.Contains(lower, "password"), strings.Contains(lower, "passphrase"):
		return "password", clean, true
	case confirmPromptRe.MatchString(lower):
		return "confirm", clean, true
	case strings.HasSuffix(clean, "?"):
		// Best-effort generic prompt detection for progressive scripts.
		return "input", clean, true
	case strings.HasSuffix(clean, ":") && hasPromptKeyword:
		return "input", clean, true
	default:
		return "", "", false
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

// pruneBySize deletes the oldest output files until total size is under maxMB.
func (m *model) pruneBySize(maxMB int) error {
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

	type fileInfo struct {
		path string
		mod  time.Time
		size int64
	}

	var infos []fileInfo
	var totalBytes int64
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
			continue
		}
		p := filepath.Join(outputDir, f.Name())
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		infos = append(infos, fileInfo{path: p, mod: info.ModTime(), size: info.Size()})
		totalBytes += info.Size()
	}

	// Sort oldest first so we delete them first
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].mod.Before(infos[j].mod)
	})

	maxBytes := int64(maxMB) * 1024 * 1024
	for _, fi := range infos {
		if totalBytes <= maxBytes {
			break
		}
		os.Remove(fi.path)
		totalBytes -= fi.size
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

// --- Output/Schedule table management ---

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

// --- Edit helpers ---

type editField struct {
	label string
	value string
}

// editFields returns the fields for the edit form.
// Field indices: 0=Name, 1=Category, 2=Command, 3=Flags, 4=WorkDir, 5=Desc, 6=Tags, 7=EnvVars
func (m model) editFields(script ScriptEntry) []editField {
	tagsStr := strings.Join(script.Tags, ", ")
	var envPairs []string
	for k, v := range script.EnvVars {
		envPairs = append(envPairs, k+"="+v)
	}
	envStr := strings.Join(envPairs, ", ")

	cmdWithArgs := script.Command
	if len(script.Args) > 0 {
		cmdWithArgs += " " + strings.Join(script.Args, " ")
	}

	return []editField{
		{"Name", script.Name},
		{"Category", script.Category},
		{"Command", cmdWithArgs},
		{"Flags", strings.Join(script.Flags, " ")},
		{"Work Dir", script.WorkDir},
		{"Description", script.Description},
		{"Tags", tagsStr},
		{"Env Vars", envStr},
	}
}

const editFieldCount = 8

func (m *model) startEdit() {
	if len(m.visibleScripts) == 0 {
		return
	}

	origIdx := m.currentScriptIndex()
	if origIdx == -1 {
		return
	}

	m.mode = modeEdit
	m.editRow = origIdx
	m.editCol = 0

	w := m.width - 34 // right panel width roughly
	if w > 100 {
		w = 100
	}
	if w < 40 {
		w = 40
	}
	m.textInput.Width = w - 16
	m.loadEditField()
}

func (m *model) loadEditField() {
	if m.editRow < 0 || m.editRow >= len(m.scripts) {
		return
	}
	script := m.scripts[m.editRow]
	fields := m.editFields(script)

	if m.editCol >= 0 && m.editCol < len(fields) {
		value := fields[m.editCol].value
		m.textInput.SetValue(value)
		m.textInput.SetCursor(len(value))
		m.textInput.Focus()
	}
}

func (m *model) saveEdit() {
	if m.mode != modeEdit || m.editRow < 0 || m.editRow >= len(m.scripts) {
		return
	}

	value := m.textInput.Value()
	switch m.editCol {
	case 0: // Name
		m.scripts[m.editRow].Name = value
	case 1: // Category
		m.scripts[m.editRow].Category = value
	case 2: // Command + Args
		value = strings.TrimSpace(value)
		tokens := shellSplit(value)
		if len(tokens) > 0 {
			m.scripts[m.editRow].Command = tokens[0]
			m.scripts[m.editRow].Args = tokens[1:]
		} else {
			m.scripts[m.editRow].Command = value
			m.scripts[m.editRow].Args = []string{}
		}
	case 3: // Flags
		value = strings.TrimSpace(value)
		if value == "" {
			m.scripts[m.editRow].Flags = nil
		} else {
			m.scripts[m.editRow].Flags = shellSplit(value)
		}
	case 4: // Work Dir
		m.scripts[m.editRow].WorkDir = value
	case 5: // Description
		m.scripts[m.editRow].Description = value
	case 6: // Tags
		var tags []string
		for _, t := range strings.Split(value, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		m.scripts[m.editRow].Tags = tags
	case 7: // Env Vars
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
	m.updateVisibleScripts()
}

func (m *model) cancelEdit() {
	m.mode = modeNormal
	m.editRow = -1
	m.editCol = -1
	m.textInput.Blur()
	m.textInput.SetValue("")
}

// --- Sorting ---

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

// --- Table styling (for output/cron tables) ---

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
