package main

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/savioxavier/termlink"
)

var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

var glamourRenderer *glamour.TermRenderer

func init() {
	glamourRenderer, _ = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
}

// renderWithLinks converts markdown links [text](url) to clickable terminal hyperlinks
func renderWithLinks(text string) string {
	return mdLinkRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := mdLinkRe.FindStringSubmatch(match)
		if len(parts) == 3 {
			return termlink.Link(parts[1], parts[2])
		}
		return match
	})
}

// renderMarkdown renders task description with Glamour for formatting
// and converts markdown links to terminal hyperlinks
func renderMarkdown(text string) string {
	if glamourRenderer == nil {
		return renderWithLinks(text)
	}

	rendered, err := glamourRenderer.Render(text)
	if err != nil {
		return renderWithLinks(text)
	}

	rendered = strings.TrimSpace(rendered)
	return renderWithLinks(rendered)
}
