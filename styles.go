package main

import "github.com/charmbracelet/lipgloss"

// Theme defines the color scheme for the application
type Theme struct {
	Primary   lipgloss.Color // Main brand color
	Accent    lipgloss.Color // Secondary accent
	Highlight lipgloss.Color // Selection/cursor
	Success   lipgloss.Color // Confirmations
	Warning   lipgloss.Color // Warnings, matches
	Danger    lipgloss.Color // Errors, deletions
	Text      lipgloss.Color // Primary text
	Muted     lipgloss.Color // De-emphasized
	Subtle    lipgloss.Color // Secondary text
	Dim       lipgloss.Color // Very dim text
	Surface   lipgloss.Color // Bars, backgrounds
	Overlay   lipgloss.Color // Elevated surfaces
}

// theme is the active color scheme
var theme = Theme{
	Primary:   lipgloss.Color("#569cd6"), // VS Code blue
	Accent:    lipgloss.Color("#4ec9b0"), // Teal/cyan
	Highlight: lipgloss.Color("#dcdcaa"), // Yellow (functions)
	Success:   lipgloss.Color("#6a9955"), // Green (comments)
	Warning:   lipgloss.Color("#ce9178"), // Orange (strings)
	Danger:    lipgloss.Color("#f14c4c"), // Red (errors)
	Text:      lipgloss.Color("#d4d4d4"), // Light gray text
	Muted:     lipgloss.Color("#6a6a6a"), // Gray
	Subtle:    lipgloss.Color("#808080"), // Medium gray
	Dim:       lipgloss.Color("#4d4d4d"), // Dark gray
	Surface:   lipgloss.Color("#1e1e1e"), // Editor background
	Overlay:   lipgloss.Color("#252526"), // Sidebar background
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Accent).
			Background(theme.Surface)

	titleNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Primary).
			Background(theme.Surface)

	searchModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Text).
			Background(theme.Danger).
			Padding(0, 1)

	resultsModeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Text).
				Background(theme.Warning).
				Padding(0, 1)

	aboutStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Text)

	aboutBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Muted).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(theme.Highlight).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(theme.Muted).
			Strikethrough(true)

	fileStyle = lipgloss.NewStyle().
			Foreground(theme.Subtle)

	helpStyle = lipgloss.NewStyle().
			Foreground(theme.Muted).
			MarginTop(1)

	cursorStyle = lipgloss.NewStyle().
			Foreground(theme.Highlight)

	groupStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Primary)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Accent)

	countStyle = lipgloss.NewStyle().
			Foreground(theme.Subtle)

	searchStyle = lipgloss.NewStyle().
			Foreground(theme.Highlight).
			Bold(true).
			Background(theme.Surface)

	matchStyle = lipgloss.NewStyle().
			Foreground(theme.Warning).
			Bold(true)

	searchInputStyle = lipgloss.NewStyle().
				Foreground(theme.Accent).
				Background(theme.Surface)

	confirmStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Success)

	cancelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Danger)

	dangerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Danger)

	// Tab bar styles
	activeTabStyle = lipgloss.NewStyle().
			Foreground(theme.Primary).
			Background(theme.Overlay).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(theme.Subtle).
				Background(theme.Surface)

	tabSeparatorStyle = lipgloss.NewStyle().
				Foreground(theme.Muted).
				Background(theme.Surface)

	// Help bar styles
	helpBarStyle = lipgloss.NewStyle().
			Foreground(theme.Subtle).
			Background(theme.Surface)

	headerBarStyle = lipgloss.NewStyle().
			Foreground(theme.Primary).
			Background(theme.Surface)

	helpBarKeyStyle = lipgloss.NewStyle().
			Foreground(theme.Primary).
			Bold(true)

	helpBarDescStyle = lipgloss.NewStyle().
				Foreground(theme.Subtle)

	helpBarSeparatorStyle = lipgloss.NewStyle().
				Foreground(theme.Muted)

	helpBarInfoStyle = lipgloss.NewStyle().
				Foreground(theme.Muted)

	// Help dialog styles
	helpDialogKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Accent)

	helpDialogDescStyle = lipgloss.NewStyle().
				Foreground(theme.Subtle)

	helpDialogHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Primary)

	dimTextStyle = lipgloss.NewStyle().
			Foreground(theme.Dim)

	// Button styles
	buttonDangerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Text).
				Background(theme.Danger).
				Padding(0, 2)

	buttonNeutralStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Text).
				Background(theme.Overlay).
				Padding(0, 2)

	dangerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Danger).
			Padding(1, 2)

	// Loader styles
	loaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.Accent)

	loaderCountStyle = lipgloss.NewStyle().
				Foreground(theme.Accent)

	// Utility
	barColor = lipgloss.NewStyle().Background(theme.Surface)
)
