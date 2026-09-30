package ahm

import "strings"

type markdownHeadingSection struct {
	Start int
	End   int
	// Content is the section's lines between the heading and End, joined with
	// newlines. Callers that only need the line range leave it unset.
	Content string
	// Level is the heading depth that opened the section (2 for `##`, 3 for
	// `###`). Callers that only need the line range leave it unset.
	Level int
}

// locateHeadingSections returns every matching Markdown section in source
// order. A section ends at the next heading of the same or a higher level, or
// at the end of lines. Callers choose their own missing- and repeated-match
// policies.
func locateHeadingSections(lines []string, headings []string) []markdownHeadingSection {
	var sections []markdownHeadingSection
	for i, line := range lines {
		level := headingLevel(line)
		if level != 2 && level != 3 {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !containsStringFold(headings, strings.TrimSpace(trimmed[level:])) {
			continue
		}

		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			nextLevel := headingLevel(lines[j])
			if nextLevel > 0 && nextLevel <= level {
				end = j
				break
			}
		}
		sections = append(sections, markdownHeadingSection{Start: i, End: end})
	}
	return sections
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
