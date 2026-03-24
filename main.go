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
		clearMode:     false,
		clearDays:     7,
	}

	m.allColumns = []table.Column{
		{Title: "Name", Width: 25},
		{Title: "Category", Width: 20},
		{Title: "Command", Width: 40},
		{Title: "Work Dir", Width: 25},
		{Title: "Description", Width: 30},
		{Title: "Last Run", Width: 20},
	}

	m.textInput = textinput.New()
	m.textInput.CharLimit = 300

	t := table.New(
		table.WithColumns(m.allColumns[:4]),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.table = styledTable(t)

	outputTable := table.New(
		table.WithColumns([]table.Column{{Title: "Output File", Width: 50}}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.outputTable = styledTable(outputTable)

	m.adjustLayout()
	m.updateTable()

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
