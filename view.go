package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	if m.running {
		return m.renderRunOutput()
	}
	if m.viewingOutput {
		return m.renderOutputHistory()
	}

	// Header
	header := m.renderHeader()

	// Main content
	var content string
	if len(m.scripts) == 0 {
		content = dimTextStyle.Render("No scripts registered yet. Press 'n' to add one.")
	} else {
		content = m.table.View()
	}

	// Status
	var statusLine string
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		if strings.Contains(m.statusMsg, "Failed") || strings.Contains(m.statusMsg, "Error") {
			statusLine = errorTextStyle.Render("> " + m.statusMsg)
		} else {
			statusLine = statusMsgStyle.Render("> " + m.statusMsg)
		}
	}

	// Footer
	footer := m.renderFooter()

	var parts []string
	parts = append(parts, header, "", content)
	if statusLine != "" {
		parts = append(parts, "", statusLine)
	}
	parts = append(parts, "", footer)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderHeader() string {
	title := titleStyle.Render("Runx")
	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		title,
		"  ",
		dimTextStyle.Render(fmt.Sprintf("%d scripts", len(m.scripts))),
	)
}

func (m model) renderFooter() string {
	if m.editMode {
		colNames := []string{"Name", "Category", "Command", "Work Dir", "Description"}
		colName := colNames[m.editCol]
		return warnTextStyle.Render(fmt.Sprintf("Editing %s: ", colName)) +
			m.textInput.View() +
			"\n" + dimTextStyle.Render("tab: next field  enter: save  esc: cancel")
	}

	if m.confirmDelete {
		return errorTextStyle.Render(fmt.Sprintf("Delete '%s'? ", m.scripts[m.deleteIndex].Name)) +
			dimTextStyle.Render("y: yes  n/esc: no")
	}

	if m.clearMode {
		return warnTextStyle.Render(fmt.Sprintf("Clear outputs older than %d days ", m.clearDays)) +
			dimTextStyle.Render("+/-: adjust  enter: confirm  esc: cancel")
	}

	var parts []string

	add := func(key, action string) {
		if len(parts) > 0 {
			parts = append(parts, bulletStyle.Render(" · "))
		}
		parts = append(parts, keyStyle.Render(key), " ", actionStyle.Render(action))
	}

	add("enter", "run")
	add("e", "edit")
	add("n", "add")
	add("d", "delete")
	add("v", "history")
	add("c", "clear")
	add("r", "refresh")
	add("q", "quit")

	if m.maxCols > len(m.table.Columns()) {
		add("←→", "scroll cols")
	}

	return strings.Join(parts, "")
}

func (m model) renderRunOutput() string {
	title := titleStyle.Render("Script Output")

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

	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDim).
		Padding(0, 1).
		Height(visibleHeight).
		Width(m.width - 4)

	content := contentStyle.Render(visibleContent)

	var status []string
	status = append(status,
		keyStyle.Render("j/k"), " ", actionStyle.Render("scroll"),
		bulletStyle.Render(" · "),
		keyStyle.Render("pgup/pgdn"), " ", actionStyle.Render("page"),
		bulletStyle.Render(" · "),
		keyStyle.Render("esc"), " ", actionStyle.Render("close"),
	)

	return lipgloss.JoinVertical(lipgloss.Left, title, "", content, "", strings.Join(status, ""))
}

func (m model) renderOutputHistory() string {
	title := titleStyle.Render("Output History")

	if len(m.outputFiles) == 0 {
		content := dimTextStyle.Render("No output files found.")
		footer := keyStyle.Render("esc") + " " + actionStyle.Render("back")
		return lipgloss.JoinVertical(lipgloss.Left, title, "", content, "", footer)
	}

	var status []string
	status = append(status,
		keyStyle.Render("enter"), " ", actionStyle.Render("view"),
		bulletStyle.Render(" · "),
		keyStyle.Render("esc"), " ", actionStyle.Render("back"),
	)

	return lipgloss.JoinVertical(lipgloss.Left, title, "", m.outputTable.View(), "", strings.Join(status, ""))
}
