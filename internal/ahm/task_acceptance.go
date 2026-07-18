package ahm

import (
	"fmt"
	"strings"
)

type taskAcceptanceFinding string

const (
	taskAcceptanceMissing     taskAcceptanceFinding = "missing"
	taskAcceptancePlaceholder taskAcceptanceFinding = "placeholder"
	taskAcceptanceUnchecked   taskAcceptanceFinding = "unchecked"
)

func parseAcceptanceNotes(body []byte) []taskAcceptanceFinding {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	start, level := acceptanceSectionStart(lines)
	if start < 0 {
		return []taskAcceptanceFinding{taskAcceptanceMissing}
	}

	hasPlaceholder := false
	hasUnchecked := false
	for _, line := range lines[start+1:] {
		if headingLevel(line) > 0 && headingLevel(line) <= level {
			break
		}
		if !isUncheckedChecklistItem(line) {
			continue
		}
		if isAcceptanceTODO(line) {
			hasPlaceholder = true
			continue
		}
		hasUnchecked = true
	}

	var findings []taskAcceptanceFinding
	if hasPlaceholder {
		findings = append(findings, taskAcceptancePlaceholder)
	}
	if hasUnchecked {
		findings = append(findings, taskAcceptanceUnchecked)
	}
	return findings
}

func acceptanceSectionStart(lines []string) (int, int) {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		level := headingLevel(trimmed)
		if level != 2 && level != 3 {
			continue
		}
		if isAcceptanceHeading(trimmed[level:]) {
			return i, level
		}
	}
	return -1, 0
}

func headingLevel(line string) int {
	trimmed := strings.TrimSpace(line)
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level == len(trimmed) || trimmed[level] != ' ' {
		return 0
	}
	return level
}

func isAcceptanceHeading(heading string) bool {
	switch strings.ToLower(strings.TrimSpace(heading)) {
	case "acceptance notes", "acceptance criteria", "acceptance":
		return true
	default:
		return false
	}
}

func isAcceptanceTODO(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	trimmed = strings.TrimPrefix(trimmed, "- [ ]")
	trimmed = strings.TrimPrefix(trimmed, "* [ ]")
	return strings.EqualFold(strings.TrimSpace(trimmed), "TODO")
}

func (f taskAcceptanceFinding) validationCode() string {
	switch f {
	case taskAcceptanceMissing:
		return "task_acceptance_missing"
	case taskAcceptancePlaceholder:
		return "task_acceptance_placeholder"
	case taskAcceptanceUnchecked:
		return "task_acceptance_unchecked"
	default:
		return "task_acceptance_unknown"
	}
}

func (f taskAcceptanceFinding) message(taskID string) string {
	switch f {
	case taskAcceptanceMissing:
		return fmt.Sprintf("task %s is completed without an acceptance section", taskID)
	case taskAcceptancePlaceholder:
		return fmt.Sprintf("task %s acceptance notes still contain the TODO placeholder", taskID)
	case taskAcceptanceUnchecked:
		return fmt.Sprintf("task %s acceptance notes contain unchecked items", taskID)
	default:
		return fmt.Sprintf("task %s acceptance notes are incomplete", taskID)
	}
}
