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
// at the end of lines. Fenced and indented code blocks are opaque: a
// heading-looking line inside code does not open or close a section. Callers
// choose their own missing- and repeated-match policies.
func locateHeadingSections(lines []string, headings []string) []markdownHeadingSection {
	code := markdownCode(lines)
	var sections []markdownHeadingSection
	for i, line := range lines {
		if code.inCode(i) {
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
			if code.inCode(j) {
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

// markdownCodeMap records, per line index, whether a line is code: inside a
// fenced or indented code block, or a fence delimiter. Every section scan and
// link walk consults inCode so a `##` line or a link inside code is never
// treated as Markdown structure or navigation.
type markdownCodeMap struct {
	code []bool
}

// inCode reports whether line i is code. Out-of-range indexes are not code.
func (m markdownCodeMap) inCode(i int) bool {
	return i >= 0 && i < len(m.code) && m.code[i]
}

// markdownCode classifies lines once per body so every scan shares one code
// model and cannot drift.
//
// Fenced code follows the established rule: backtick and tilde fences, an info
// string after the opening fence, and a closing fence of the same character at
// least as long as the opener.
//
// Indented code is a line indented four or more columns. Indentation is
// measured relative to the innermost open list item, which this pass tracks
// from list markers, so a heading nested under a list item stays Markdown while
// the same indentation at top level, or four columns past a list item's content
// column, is code. A tab advances to the next four-column stop, so tab
// indentation is code too. A paragraph continuation that happens to be
// indented is classified as code as well; for the scans that consult the map,
// opacity is the safe direction.
func markdownCode(lines []string) markdownCodeMap {
	code := make([]bool, len(lines))
	var (
		inFence   bool
		fenceChar byte
		fenceLen  int
		lists     []int // content columns of open list items, outermost first
	)
	for i, line := range lines {
		indent, offset, fence, rest, ok := parseFenceLine(line)
		if inFence {
			code[i] = true
			if ok && fence[0] == fenceChar && len(fence) >= fenceLen &&
				indent-listIndent(lists) <= 3 && strings.TrimSpace(rest) == "" {
				inFence = false
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		for len(lists) > 0 && indent < lists[len(lists)-1] {
			lists = lists[:len(lists)-1]
		}
		container := listIndent(lists)
		switch {
		case ok && indent-container <= 3:
			inFence = true
			fenceChar = fence[0]
			fenceLen = len(fence)
			code[i] = true
		case indent-container >= 4:
			code[i] = true
		default:
			if contentIndent, ok := parseListMarker(line, indent, offset); ok {
				lists = append(lists, contentIndent)
			}
		}
	}
	return markdownCodeMap{code: code}
}

// listIndent returns the content indent of the innermost open list item, or 0
// at the document level.
func listIndent(lists []int) int {
	if len(lists) == 0 {
		return 0
	}
	return lists[len(lists)-1]
}

// parseListMarker reports the content column a list item at the line's leading
// indentation establishes, when the line opens with a bullet (`-`, `+`, `*`)
// or ordered (`1.`, `1)`) marker. col is the marker's column and offset its
// byte offset. A bullet run that is a thematic break (`- - -`, `* * *`) is not
// a marker. The caller has already established that the marker sits within the
// current container.
func parseListMarker(line string, col int, offset int) (contentIndent int, ok bool) {
	rest := line[offset:]
	if rest == "" {
		return 0, false
	}
	markerWidth := 0
	switch rest[0] {
	case '-', '+', '*':
		if isThematicBreak(rest) {
			return 0, false
		}
		markerWidth = 1
	default:
		digits := 0
		for digits < len(rest) && digits < 9 && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(rest) || (rest[digits] != '.' && rest[digits] != ')') {
			return 0, false
		}
		markerWidth = digits + 1
	}
	after := rest[markerWidth:]
	if after == "" {
		return col + markerWidth + 1, true
	}
	width, gap := whitespaceWidth(after, col+markerWidth)
	if gap == 0 {
		return 0, false // no whitespace after the marker
	}
	if gap == len(after) {
		// Marker followed only by whitespace: an empty item whose content
		// column is one space past the marker.
		return col + markerWidth + 1, true
	}
	if width > 4 {
		width = 1
	}
	return col + markerWidth + width, true
}

// whitespaceWidth returns the column width of the leading spaces and tabs of s
// when they start at column start, and the number of bytes they occupy. A tab
// advances to the next four-column stop.
func whitespaceWidth(s string, start int) (width int, offset int) {
	col := start
	for offset < len(s) {
		switch s[offset] {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return col - start, offset
		}
		offset++
	}
	return col - start, offset
}

// indentColumns returns line's leading indentation as a column count and as a
// byte offset. A tab advances to the next four-column stop, the CommonMark tab
// stop, so the two differ on tab-indented lines.
func indentColumns(line string) (cols int, offset int) {
	for offset < len(line) {
		switch line[offset] {
		case ' ':
			cols++
		case '\t':
			cols += 4 - cols%4
		default:
			return cols, offset
		}
		offset++
	}
	return cols, offset
}

// isThematicBreak reports whether rest, which starts with a bullet character,
// is a CommonMark thematic break: three or more `-` or `*` characters
// separated only by spaces or tabs. A `+` run is never a break.
func isThematicBreak(rest string) bool {
	char := rest[0]
	if char != '-' && char != '*' {
		return false
	}
	count := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case char:
			count++
		case ' ', '\t':
		default:
			return false
		}
	}
	return count >= 3
}

// parseFenceLine reports the leading indentation as a column count and as a
// byte offset and, when the line opens with a run of three or more backticks or
// tildes, that run plus the rest of the line after it (a fence's info string or
// trailing content).
func parseFenceLine(line string) (indent int, offset int, fence string, rest string, ok bool) {
	indent, offset = indentColumns(line)
	if offset >= len(line) {
		return indent, offset, "", "", false
	}
	char := line[offset]
	if char != '`' && char != '~' {
		return indent, offset, "", "", false
	}
	end := offset
	for end < len(line) && line[end] == char {
		end++
	}
	if end-offset < 3 {
		return indent, offset, "", "", false
	}
	return indent, offset, line[offset:end], line[end:], true
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
