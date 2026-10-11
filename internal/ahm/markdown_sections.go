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
// at the end of lines. Fenced code blocks are opaque: a heading-looking line
// inside a fence does not open or close a section. Callers choose their own
// missing- and repeated-match policies.
func locateHeadingSections(lines []string, headings []string) []markdownHeadingSection {
	fences := markdownFences(lines)
	var sections []markdownHeadingSection
	for i, line := range lines {
		if fences.inFence(i) {
			continue
		}
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
			if fences.inFence(j) {
				continue
			}
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

// markdownFenceMap records, per line index, whether a line belongs to a fenced
// code block. The fence delimiters themselves count as fenced so that no scan
// mistakes them for content. Heading scans consult inFence so a `##` line inside
// a fence is never treated as Markdown structure.
type markdownFenceMap struct {
	fenced []bool
}

// markdownFences classifies lines once per body so every heading scan shares a
// single fence model and cannot drift. It recognizes backtick and tilde fences,
// up to three leading spaces of indentation, an info string after the opening
// fence, and a closing fence of the same character at least as long as the
// opener.
func markdownFences(lines []string) markdownFenceMap {
	fenced := make([]bool, len(lines))
	inFence := false
	var (
		fenceChar byte
		fenceLen  int
	)
	for i, line := range lines {
		indent, fence, rest, ok := parseFenceLine(line)
		if !inFence {
			if !ok || indent > 3 {
				continue
			}
			inFence = true
			fenceChar = fence[0]
			fenceLen = len(fence)
			fenced[i] = true
			continue
		}
		fenced[i] = true
		if ok && indent <= 3 && fence[0] == fenceChar && len(fence) >= fenceLen && strings.TrimSpace(rest) == "" {
			inFence = false
		}
	}
	return markdownFenceMap{fenced: fenced}
}

// inFence reports whether line i lies inside (or delimits) a fenced code block.
// Out-of-range indexes are treated as unfenced.
func (m markdownFenceMap) inFence(i int) bool {
	return i >= 0 && i < len(m.fenced) && m.fenced[i]
}

// parseFenceLine reports the leading space indentation and, when the line opens
// with a run of three or more backticks or tildes, that run plus the rest of the
// line after it (a fence's info string or trailing content).
func parseFenceLine(line string) (indent int, fence string, rest string, ok bool) {
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent >= len(line) {
		return indent, "", "", false
	}
	char := line[indent]
	if char != '`' && char != '~' {
		return indent, "", "", false
	}
	end := indent
	for end < len(line) && line[end] == char {
		end++
	}
	if end-indent < 3 {
		return indent, "", "", false
	}
	return indent, line[indent:end], line[end:], true
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
