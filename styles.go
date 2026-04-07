package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors — consistent palette
	colorPrimary = lipgloss.Color("#5AF78E") // green
	colorAccent  = lipgloss.Color("#57C7FF") // blue
	colorWarn    = lipgloss.Color("#FF6AC1") // pink
	colorError   = lipgloss.Color("#FF5C57") // red
	colorDim     = lipgloss.Color("#606060")
	colorText    = lipgloss.Color("#EEEEEE")
	colorBg      = lipgloss.Color("#1A1A2E")

	// Header
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	// Page tabs
	activePageStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Underline(true)

	inactivePageStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Keys / actions
	keyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	actionStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	bulletStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	dimTextStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	errorTextStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	successTextStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	warnTextStyle = lipgloss.NewStyle().
			Foreground(colorWarn).
			Bold(true)

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(colorAccent)

	// Dialog overlay
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	// Split panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	panelActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	categoryHeaderStyle = lipgloss.NewStyle().
				Foreground(colorWarn).
				Bold(true)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	// Tabs for running scripts
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Background(lipgloss.Color("#3A3A5C")).
			Padding(0, 1)

	tabStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(0, 1)

	// Edit form fields
	fieldLabelStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Width(14)

	inactiveFieldStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Detail view labels
	detailLabelStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Width(12)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(colorText)
)
