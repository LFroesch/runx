package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
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
	modeDeleteConfirm
	modeHelp
	modeSearch
	modeClear
	modeDryRun
	modeParamPrompt
	modeScheduleEdit
)

// --- Sort ---

const (
	sortByName     = 0
	sortByRunCount = 1
	sortByLastRun  = 2
)

// --- Types ---

type ScriptEntry struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
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

type ScriptManager struct {
	Scripts []ScriptEntry `json:"scripts"`
}

type outputLine struct {
	text string
	done bool
	err  error
}

type RunningScript struct {
	ID        int
	Name      string
	WorkDir   string
	Lines     []string
	Done      bool
	Err       error
	Scroll    int
	StartTime time.Time
	ch        <-chan outputLine
}

func (r *RunningScript) Output() string {
	return strings.Join(r.Lines, "\n")
}

type model struct {
	scripts       []ScriptEntry
	table         table.Model
	configFile    string
	width         int
	height        int
	statusMsg     string
	statusExpiry  time.Time
	scrollOffset  int
	maxCols       int
	scriptIndices []int
	allColumns    []table.Column

	// Navigation
	page appPage
	mode appMode

	// Edit
	editRow   int
	editCol   int
	textInput textinput.Model

	// Delete confirmation
	deleteIndex int

	// Search / filter
	searchInput  textinput.Model
	searchFilter string

	// Running scripts (concurrent, streaming)
	runningScripts []RunningScript
	activeRunTab   int
	nextRunID      int

	// Output history
	outputFiles []string
	outputTable table.Model

	// Clear mode
	clearDays int

	// Sort
	sortMode int

	// Parameterized script prompt
	paramScript *ScriptEntry
	paramFields []string
	paramValues []string
	paramCursor int

	// Schedule
	cronTable      table.Model
	schedEditIndex int
	lastCronCheck  time.Time
}

// --- Messages ---

type tickMsg time.Time

type statusMsg struct {
	message string
}

type scriptStartedMsg struct {
	scriptID int
	ch       <-chan outputLine
}

type scriptLineMsg struct {
	scriptID int
	line     string
}

type scriptFinishedMsg struct {
	scriptID int
	err      error
}

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
