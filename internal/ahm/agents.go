package ahm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

type agentsSuggestionsReport struct {
	Target      string                   `json:"target"`
	Exists      bool                     `json:"exists"`
	Suggestions []agentSuggestionWithHit `json:"suggestions"`
}

type agentSuggestionWithHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Present bool   `json:"present"`
}

func (a *app) agentsSuggestions(showAll bool) error {
	report, err := a.collectAgentSuggestions()
	if err != nil {
		return err
	}
	if a.opts.json {
		return a.emit(report)
	}

	selected := report.Suggestions
	if !showAll {
		selected = nil
		for _, suggestion := range report.Suggestions {
			if !suggestion.Present {
				selected = append(selected, suggestion)
			}
		}
	}

	fmt.Fprintln(a.out, "# Suggested AGENTS.md Integration")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Advisory instructions from `ahm`. Use these to update the project-owned")
	fmt.Fprintln(a.out, "AGENTS.md without replacing project-specific guidance.")
	if len(selected) == 0 {
		fmt.Fprintln(a.out)
		fmt.Fprintln(a.out, "No missing suggestions detected.")
		return nil
	}
	for _, suggestion := range selected {
		fmt.Fprintln(a.out)
		fmt.Fprintf(a.out, "## %s\n\n", suggestion.Title)
		if showAll && suggestion.Present {
			fmt.Fprintln(a.out, "_Already appears present in AGENTS.md._")
			fmt.Fprintln(a.out)
		}
		fmt.Fprintln(a.out, suggestion.Body)
	}
	return nil
}

func (a *app) collectAgentSuggestions() (agentsSuggestionsReport, error) {
	target := filepath.Join(a.opts.root, "AGENTS.md")
	existing, err := os.ReadFile(target) // #nosec G304 // target is AGENTS.md within project root only
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return agentsSuggestionsReport{}, err
	}

	content := strings.ReplaceAll(string(existing), "\r\n", "\n")
	report := agentsSuggestionsReport{
		Target: "AGENTS.md",
		Exists: exists,
	}
	for _, suggestion := range templates.AgentSuggestions() {
		body := strings.ReplaceAll(suggestion.Body, "\r\n", "\n")
		report.Suggestions = append(report.Suggestions, agentSuggestionWithHit{
			ID:      suggestion.ID,
			Title:   suggestion.Title,
			Body:    suggestion.Body,
			Present: exists && strings.Contains(content, body),
		})
	}
	return report, nil
}
