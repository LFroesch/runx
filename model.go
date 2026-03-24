package main

import (
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// --- Types ---

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
	clearMode     bool
	clearDays     int
}

// --- Messages ---

type statusMsg struct {
	message string
}

type scriptDoneMsg struct {
	scriptName string
	output     string
	err        error
	saveErr    error
}

func showStatus(msg string) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{message: msg}
	}
}

// --- Init ---

func (m model) Init() tea.Cmd {
	return tea.SetWindowTitle("runx")
}
