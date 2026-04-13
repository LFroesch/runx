package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	configPath := flag.String("config", "", "Path to config file")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "runx — TUI script runner and manager\n\n")
		fmt.Fprintf(os.Stderr, "Usage: runx [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("runx " + version)
		os.Exit(0)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	configFile := *configPath
	if configFile == "" {
		configFile = filepath.Join(homeDir, ".config", "runx", "runx-scripts.json")
	}

	ti := textinput.New()
	ti.CharLimit = 2000

	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 100

	stdinTi := textinput.New()
	stdinTi.Placeholder = "stdin input..."
	stdinTi.CharLimit = 2000

	ea := textarea.New()
	ea.CharLimit = 0
	ea.ShowLineNumbers = false

	scripts := loadScripts(configFile)
	if ensureScriptIDs(scripts) {
		// Persist newly assigned IDs
		manager := ScriptManager{Scripts: scripts}
		if data, err := json.MarshalIndent(manager, "", "  "); err == nil {
			os.MkdirAll(filepath.Dir(configFile), 0755)
			os.WriteFile(configFile, data, 0644)
		}
	}

	m := model{
		scripts:        scripts,
		configFile:     configFile,
		width:          100,
		height:         24,
		page:           pageScripts,
		mode:           modeNormal,
		editRow:        -1,
		editCol:        -1,
		deleteIndex:    -1,
		scriptEditIdx:  -1,
		textInput:      ti,
		searchInput:    si,
		stdinInput:     stdinTi,
		clearDays:      7,
		scriptEditArea: ea,
	}

	outputTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Script", Width: 30},
			{Title: "Date", Width: 12},
			{Title: "Time", Width: 10},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.outputTable = styledTable(outputTable)

	cronTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 22},
			{Title: "Category", Width: 12},
			{Title: "Schedule", Width: 10},
			{Title: "Status", Width: 8},
			{Title: "Last Run", Width: 16},
			{Title: "Next Run", Width: 18},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.cronTable = styledTable(cronTable)

	m.updateVisibleScripts()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
