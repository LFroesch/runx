package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configFile := filepath.Join(homeDir, ".config", "runx", "runx-scripts.json")

	ti := textinput.New()
	ti.CharLimit = 2000

	si := textinput.New()
	si.Placeholder = "search..."
	si.CharLimit = 100

	m := model{
		scripts:     loadScripts(configFile),
		configFile:  configFile,
		width:       100,
		height:      24,
		page:        pageScripts,
		mode:        modeNormal,
		editRow:     -1,
		editCol:     -1,
		deleteIndex: -1,
		textInput:   ti,
		searchInput: si,
		clearDays:   7,
	}

	m.allColumns = []table.Column{
		{Title: "Name", Width: 25},
		{Title: "Command", Width: 45},
		{Title: "Runs", Width: 6},
		{Title: "Work Dir", Width: 20},
		{Title: "Description", Width: 25},
		{Title: "Last Run", Width: 18},
	}

	t := table.New(
		table.WithColumns(m.allColumns[:3]),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.table = styledTable(t)

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

	m.adjustLayout()
	m.updateTable()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
