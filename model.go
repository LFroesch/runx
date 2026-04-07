package main

import (
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// --- App State ---

type appPage int

const (
	pageScripts appPage = iota
	pageSchedules
	pageHistory
	pageRunning
)

type appMode int

const (
	modeNormal appMode = iota
	modeEdit
	modeScriptEdit // textarea-based edit (E key)
	modeDeleteConfirm
	modeHelp
	modeSearch
	modeClear
	modeDryRun
	modeParamPrompt
	modeScheduleEdit
)

const (
	sortByName     = 0
	sortByRunCount = 1
	sortByLastRun  = 2
)

// --- Types ---

type ScriptEntry struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Flags       []string          `json:"flags,omitempty"`
	Args        []string          `json:"args"`
	WorkDir     string            `json:"workdir"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	LastRun     string            `json:"last_run"`
	RunCount    int               `json:"run_count"`
	Tags        []string          `json:"tags,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	Schedule    string            `json:"schedule,omitempty"`
	ScheduleOn  bool              `json:"schedule_on,omitempty"`
}

// FullArgs returns flags + args combined for execution.
func (s ScriptEntry) FullArgs() []string {
	all := make([]string, 0, len(s.Flags)+len(s.Args))
	all = append(all, s.Flags...)
	all = append(all, s.Args...)
	return all
}

// FullCommand returns the display string for the full command.
func (s ScriptEntry) FullCommand() string {
	parts := []string{s.Command}
	parts = append(parts, s.Flags...)
	parts = append(parts, s.Args...)
	return strings.Join(parts, " ")
}

type ScriptManager struct {
	Scripts []ScriptEntry `json:"scripts"`
}

type outputLine struct {
	text string
	done bool
	err  error
}

type RunningScript struct {
	ID           int
	Name         string
	WorkDir      string
	Lines        []string
	Done         bool
	Err          error
	Scroll       int
	StartTime    time.Time
	EndTime      time.Time
	ch           <-chan outputLine
	stdin        io.WriteCloser
	stdinVisible bool // set true when password/passphrase prompt detected
}

func (r *RunningScript) Output() string {
	return strings.Join(r.Lines, "\n")
}

func (r *RunningScript) Elapsed() time.Duration {
	if r.Done && !r.EndTime.IsZero() {
		return r.EndTime.Sub(r.StartTime).Truncate(time.Second)
	}
	return time.Since(r.StartTime).Truncate(time.Second)
}

// leftPanelItem is a row in the scripts left panel.
type leftPanelItem struct {
	isHeader bool
	label    string
	scriptIdx int // index into visibleScripts (-1 if header)
}

type model struct {
	scripts    []ScriptEntry
	configFile string
	width      int
	height     int
	statusMsg  string
	statusExpiry time.Time

	// Navigation
	page appPage
	mode appMode

	// Scripts page — split panel
	scriptCursor   int   // index into visibleScripts
	visibleScripts []int // indices into m.scripts, sorted+filtered
	leftScroll     int

	// Edit (inline in right panel)
	editRow   int
	editCol   int
	textInput textinput.Model

	// Delete confirmation
	deleteIndex int

	// Search / filter
	searchInput  textinput.Model
	searchFilter string

	// Running scripts
	runningScripts []RunningScript
	activeRunTab   int
	nextRunID      int
	stdinInput     textinput.Model

	// Output history
	outputFiles []string
	outputTable table.Model

	// Clear mode
	clearDays   int
	clearSizeMB int
	clearBySize bool // false=age mode, true=size mode

	// Sort
	sortMode int

	// Parameterized script prompt
	paramScript *ScriptEntry
	paramFields []string
	paramDescs  []string // description per field (from {{name:Desc=default}})
	paramValues []string
	paramCursor int

	// Schedule
	cronTable      table.Model
	schedEditIndex int
	lastCronCheck  time.Time

	// Script textarea editor (E key)
	scriptEditArea textarea.Model
	scriptEditIdx  int    // index into m.scripts being edited
	scriptEditFile string // path of the file being edited
}

// --- Messages ---

type tickMsg time.Time

type statusMsg struct {
	message string
}

type scriptStartedMsg struct {
	scriptID int
	ch       <-chan outputLine
	stdin    io.WriteCloser
}

type scriptLineMsg struct {
	scriptID int
	line     string
}

type scriptFinishedMsg struct {
	scriptID int
	err      error
}

type editorDoneMsg struct{}

func showStatus(msg string) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{message: msg}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// --- Init ---

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("runx"),
		tickCmd(),
	)
}
