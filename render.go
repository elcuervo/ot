package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

const defaultTheme = "dracula"

var glamourRenderer *glamour.TermRenderer

func init() {
	initRenderer(defaultTheme)
}

func initRenderer(theme string) {
	if theme == "" {
		theme = defaultTheme
	}
	glamourRenderer, _ = glamour.NewTermRenderer(
		glamour.WithStandardStyle(theme),
		glamour.WithWordWrap(0),
	)
}

// renderTask renders a full task line with checkbox using Glamour
func renderTask(done bool, description string) string {
	checkbox := "- [ ]"
	if done {
		checkbox = "- [x]"
	}

	taskLine := fmt.Sprintf("%s %s", checkbox, description)

	if glamourRenderer == nil {
		return taskLine
	}

	rendered, err := glamourRenderer.Render(taskLine)
	if err != nil {
		return taskLine
	}

	// Keep as single line
	rendered = strings.TrimSpace(rendered)
	return rendered
}
