package main

import "github.com/charmbracelet/lipgloss"

// Unified color palette
var (
	primaryColor        = lipgloss.Color("109")
	accentColor         = lipgloss.Color("171")
	barBackground       = lipgloss.Color("233")
	tabBackground       = lipgloss.Color("233")
	tabActiveBackground = lipgloss.Color("235")
	barColor            = lipgloss.NewStyle().Background(barBackground)
	mutedColor          = lipgloss.Color("239")
	subtleColor         = lipgloss.Color("244")
	warningColor        = lipgloss.Color("179")
	dangerColor         = lipgloss.Color("167")
	successColor        = lipgloss.Color("65")
	highlightColor      = lipgloss.Color("171")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor).
			Background(barBackground)

	titleNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(barBackground)

	searchModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(dangerColor).
			Padding(0, 1)

	resultsModeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("231")).
				Background(warningColor).
				Padding(0, 1)

	aboutStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("white"))

	aboutBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Strikethrough(true)

	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1)

	cursorStyle = lipgloss.NewStyle().
			Foreground(highlightColor)

	groupStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)

	countStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	searchStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Bold(true).
			Background(barBackground)

	matchStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	searchInputStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Background(barBackground)

	confirmStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(successColor)

	cancelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(dangerColor)

	dangerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(dangerColor)

	// Tab bar styles - minimal single-line tabs
	activeTabStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Background(tabActiveBackground).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(subtleColor).
				Background(tabBackground)

	tabSeparatorStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Background(tabBackground)

	// Help bar styles - persistent bottom bar
	helpBarStyle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Background(barBackground)

	headerBarStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Background(barBackground)

	helpBarKeyStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	helpBarDescStyle = lipgloss.NewStyle().
				Foreground(subtleColor)

	helpBarSeparatorStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	helpBarInfoStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
)
