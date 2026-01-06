package main

import "github.com/charmbracelet/lipgloss"

// Theme color palette - all colors defined in one place
var (
	// Core brand colors
	primaryColor   = lipgloss.Color("109") // Teal/cyan - main accent
	accentColor    = lipgloss.Color("171") // Magenta/purple - secondary accent
	highlightColor = lipgloss.Color("171") // Selection/cursor highlight

	// Semantic colors
	successColor = lipgloss.Color("65")  // Green - confirmations
	warningColor = lipgloss.Color("179") // Yellow/gold - warnings, matches
	dangerColor  = lipgloss.Color("167") // Red - errors, deletions

	// Text colors
	textColor   = lipgloss.Color("white") // Primary text
	mutedColor  = lipgloss.Color("239")   // De-emphasized text
	subtleColor = lipgloss.Color("244")   // Secondary text
	dimColor    = lipgloss.Color("241")   // Very dim text

	// Background colors
	barBackground       = lipgloss.Color("233") // Header/footer bars
	tabBackground       = lipgloss.Color("233") // Inactive tabs
	tabActiveBackground = lipgloss.Color("235") // Active tab
	neutralBackground   = lipgloss.Color("240") // Neutral buttons

	// Utility
	barColor = lipgloss.NewStyle().Background(barBackground)
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
			Foreground(textColor).
			Background(dangerColor).
			Padding(0, 1)

	resultsModeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(textColor).
				Background(warningColor).
				Padding(0, 1)

	aboutStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(textColor)

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
			Foreground(subtleColor)

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

	// Help dialog styles (about screen)
	helpDialogKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accentColor)

	helpDialogDescStyle = lipgloss.NewStyle().
				Foreground(subtleColor)

	helpDialogHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	dimTextStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	// Button styles (delete dialog)
	buttonDangerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(textColor).
				Background(dangerColor).
				Padding(0, 2)

	buttonNeutralStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(textColor).
				Background(neutralBackground).
				Padding(0, 2)

	dangerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dangerColor).
			Padding(1, 2)

	// Loader styles
	loaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accentColor)

	loaderCountStyle = lipgloss.NewStyle().
				Foreground(accentColor)
)
