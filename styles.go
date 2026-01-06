package main

import "github.com/charmbracelet/lipgloss"

// Unified color palette
var (
	primaryColor   = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	accentColor    = lipgloss.AdaptiveColor{Light: "#FF79C6", Dark: "#FF79C6"}
	mutedColor     = lipgloss.Color("241")
	subtleColor    = lipgloss.Color("245")
	warningColor   = lipgloss.Color("214")
	dangerColor    = lipgloss.Color("196")
	successColor   = lipgloss.Color("46")
	highlightColor = lipgloss.Color("212")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)

	titleNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

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
			Foreground(accentColor).
			MarginTop(1)

	countStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	searchStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Bold(true)

	matchStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	searchInputStyle = lipgloss.NewStyle().
				Foreground(accentColor)

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
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(subtleColor)

	tabSeparatorStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
)
