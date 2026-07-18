package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	adrKindMADR      = "madr"
	adrKindLegacy    = "legacy"
	adrKindMalformed = "malformed"
)

var adrSupersededStatusPattern = regexp.MustCompile(`^superseded by ADR-[0-9]{3}$`)

// ADR is the parsed representation of an Architecture Decision Record file.
type ADR struct {
	ID             string
	Slug           string
	Title          string
	Status         string
	Date           string
	DecisionMakers string
	Consulted      string
	Informed       string
	Extra          map[string]string
	Path           string
	Body           string
	Kind           string
	ParseError     string
}

func adrFilePaths(root string) ([]string, error) {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if strings.EqualFold(entry.Name(), "README.md") || strings.EqualFold(entry.Name(), "index.md") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(files, func(i, j int) bool {
		leftID, leftSlug, leftOK := splitADRFileName(files[i])
		rightID, rightSlug, rightOK := splitADRFileName(files[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return files[i] < files[j]
		}
		if leftID != rightID {
			return leftID < rightID
		}
		return leftSlug < rightSlug
	})
	return files, nil
}

func collectADRs(root string) ([]ADR, error) {
	files, err := adrFilePaths(root)
	if err != nil {
		return nil, err
	}
	var adrs []ADR
	var errs []error
	for _, path := range files {
		adr, err := parseADR(path)
		if err != nil {
			adr = malformedADR(path, err)
			errs = append(errs, fmt.Errorf("%s: %w", relPath(root, path), err))
		}
		adrs = append(adrs, adr)
	}
	sort.Slice(adrs, func(i, j int) bool {
		if adrs[i].ID != adrs[j].ID {
			return adrIDLess(adrs[i].ID, adrs[j].ID)
		}
		return adrs[i].Slug < adrs[j].Slug
	})
	if len(errs) > 0 {
		return adrs, errors.Join(errs...)
	}
	return adrs, nil
}

func parseADR(path string) (ADR, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return ADR{}, err
	}
	return parseADRFromData(data, path)
}

func parseADRFromData(data []byte, path string) (ADR, error) {
	id, slug, ok := splitADRFileName(path)
	if !ok {
		return ADR{}, fmt.Errorf("ADR filename must match NNN-kebab-slug.md")
	}
	paddedID := fmt.Sprintf("%03d", id)
	text := string(data)
	_, _, hasFrontMatter, err := splitFrontMatter(text)
	if err != nil {
		return ADR{}, fmt.Errorf("ADR front matter is not closed")
	}
	if !hasFrontMatter {
		return parseLegacyADR(text, path, paddedID, slug), nil
	}
	meta, body, err := parseFrontMatter(text)
	if err != nil {
		return ADR{}, err
	}
	if metaID := strings.TrimSpace(meta["id"]); metaID != "" && metaID != paddedID {
		return ADR{}, fmt.Errorf("ADR id %q does not match filename id %s", metaID, paddedID)
	}
	title := headingTitle(body, paddedID)
	body = stripHeading(body, title)
	adr := ADR{
		ID:             paddedID,
		Slug:           slug,
		Title:          title,
		Status:         meta["status"],
		Date:           meta["date"],
		DecisionMakers: meta["decision-makers"],
		Consulted:      meta["consulted"],
		Informed:       meta["informed"],
		Extra:          adrMetaExtra(meta),
		Path:           path,
		Body:           body,
		Kind:           adrKindMADR,
	}
	return adr, nil
}

func parseLegacyADR(text string, path string, id string, slug string) ADR {
	title := headingTitle(text, id)
	if strings.HasPrefix(title, "ADR ") {
		prefix := "ADR " + id + ":"
		if strings.HasPrefix(title, prefix) {
			title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	body := stripHeading(text, headingTitle(text, id))
	return ADR{
		ID:     id,
		Slug:   slug,
		Title:  title,
		Path:   path,
		Body:   body,
		Kind:   adrKindLegacy,
		Extra:  map[string]string{},
		Status: legacyBoldValue(text, "Status"),
		Date:   legacyBoldValue(text, "Date"),
	}
}

func malformedADR(path string, err error) ADR {
	id, slug, ok := splitADRFileName(path)
	adr := ADR{
		Path:       path,
		Kind:       adrKindMalformed,
		ParseError: err.Error(),
		Extra:      map[string]string{},
	}
	if ok {
		adr.ID = fmt.Sprintf("%03d", id)
		adr.Slug = slug
	}
	return adr
}

func renderADR(adr ADR) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "status: %s\n", adr.Status)
	fmt.Fprintf(&b, "date: %s\n", adr.Date)
	if adr.DecisionMakers != "" {
		fmt.Fprintf(&b, "decision-makers: %s\n", strings.ReplaceAll(strings.ReplaceAll(adr.DecisionMakers, "\r\n", " "), "\n", " "))
	}
	if adr.Consulted != "" {
		fmt.Fprintf(&b, "consulted: %s\n", adr.Consulted)
	}
	if adr.Informed != "" {
		fmt.Fprintf(&b, "informed: %s\n", adr.Informed)
	}
	for _, k := range sortedKeys(adr.Extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, adr.Extra[k])
	}
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "# %s\n\n", strings.ReplaceAll(strings.ReplaceAll(adr.Title, "\r\n", " "), "\n", " "))
	body := strings.TrimSpace(adr.Body)
	if body != "" {
		fmt.Fprintln(&b, body)
		fmt.Fprintln(&b)
	}
	return b.String()
}

func resolveADR(pattern string, adrs []ADR) (ADR, error) {
	normalized, ok := normalizeADRRef(pattern)
	if ok {
		for _, adr := range adrs {
			if adrRef(adr) == normalized || adr.ID == normalized {
				return adr, nil
			}
		}
	}
	for _, adr := range adrs {
		if adr.ID == pattern || adrRef(adr) == pattern {
			return adr, nil
		}
	}
	return ADR{}, fmt.Errorf("ADR %q not found", pattern)
}

func nextADRID(adrs []ADR, root string) string {
	maxID := 0
	for _, adr := range adrs {
		n, err := strconv.Atoi(adr.ID)
		if err == nil && n > maxID {
			maxID = n
		}
	}
	files, err := adrFilePaths(root)
	if err == nil {
		for _, path := range files {
			n, _, ok := splitADRFileName(path)
			if ok && n > maxID {
				maxID = n
			}
		}
	}
	return fmt.Sprintf("%03d", maxID+1)
}

func validADRStatus(status string) bool {
	switch status {
	case "proposed", "accepted", "rejected", "deprecated":
		return true
	default:
		return adrSupersededStatusPattern.MatchString(status)
	}
}

func rewriteADRFrontMatter(path string, fields map[string]string) error {
	return rewriteADR(path, fields, nil)
}

func rewriteADR(path string, fields map[string]string, updateBody func(string) string) error {
	data, err := os.ReadFile(path) // #nosec G304 // path comes from resolved ADR files under docs/adr.
	if err != nil {
		return err
	}
	text := string(data)
	raw, body, newline, ok, err := splitRawFrontMatter(text)
	if err != nil {
		return fmt.Errorf("ADR front matter is missing or not closed")
	}
	if !ok {
		return fmt.Errorf("ADR front matter is missing or not closed")
	}
	if updateBody != nil {
		body = updateBody(body)
	}
	updated := renderRawFrontMatter(updateRawFrontMatter(raw, newline, fields), body, newline)
	if updated == text {
		return nil
	}
	return writeFileAtomic(path, []byte(updated), 0o644)
}

func splitRawFrontMatter(text string) (string, string, string, bool, error) {
	for _, newline := range []string{"\n", "\r\n"} {
		open := "---" + newline
		if !strings.HasPrefix(text, open) {
			// A lone opening delimiter with no newline is malformed regardless of newline style.
			if text == "---" {
				return "", "", newline, false, fmt.Errorf("front matter is not closed")
			}
			continue
		}
		rest := text[len(open):]
		// Empty front matter: closing delimiter immediately after opening.
		closeDelim := "---"
		if rest == closeDelim {
			return "", "", newline, true, nil
		}
		if strings.HasPrefix(rest, closeDelim+newline) {
			return "", rest[len(closeDelim)+len(newline):], newline, true, nil
		}
		// Non-empty front matter: look for closing delimiter on its own line.
		close := newline + "---" + newline
		end := strings.Index(rest, close)
		if end >= 0 {
			rawEnd := end
			bodyStart := end + len(close)
			return rest[:rawEnd], text[len(open)+bodyStart:], newline, true, nil
		}
		// Closing delimiter at end of file (no trailing newline).
		endEOF := newline + "---"
		if strings.HasSuffix(rest, endEOF) {
			rawEnd := len(rest) - len(endEOF)
			return rest[:rawEnd], "", newline, true, nil
		}
		return "", "", newline, false, fmt.Errorf("front matter is not closed")
	}
	return "", text, "\n", false, nil
}

func updateRawFrontMatter(raw string, newline string, fields map[string]string) string {
	if len(fields) == 0 {
		return raw
	}
	lines := strings.Split(raw, newline)
	seen := map[string]bool{}
	for i, line := range lines {
		key, _, ok, err := parseFrontMatterLine(line)
		if err != nil || !ok {
			continue
		}
		value, replace := fields[key]
		if !replace {
			continue
		}
		lines[i] = key + ": " + value
		seen[key] = true
	}
	for _, key := range sortedKeys(fields) {
		if seen[key] {
			continue
		}
		lines = append(lines, key+": "+fields[key])
	}
	return strings.Join(lines, newline)
}

func renderRawFrontMatter(raw string, body string, newline string) string {
	return "---" + newline + raw + newline + "---" + newline + body
}

func upsertADRSupersessionNote(body string, replacement ADR) string {
	return upsertADRSection(body, "Supersession", "Superseded by "+adrMarkdownLink(replacement)+".")
}

func upsertADRMoreInformationReference(body string, superseded ADR) string {
	return upsertADRMoreInformationLine(body, "- Supersedes "+adrMarkdownLink(superseded)+".", "ADR-"+superseded.ID)
}

func upsertADRSection(body string, heading string, content string) string {
	newline := dominantNewline(body)
	lines := splitLinesForEdit(body)
	sections := locateHeadingSections(lines, []string{heading})
	if len(sections) > 0 {
		// Preserve the established first-match behavior for repeated headings.
		section := sections[0]
		trimmedLine := strings.TrimSpace(lines[section.Start])
		replacement := []string{trimmedLine, "", content}
		if section.End < len(lines) {
			replacement = append(replacement, "")
		}
		updated := append([]string{}, lines[:section.Start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[section.End:]...)
		return joinLinesForEdit(updated, newline)
	}
	section := "## " + heading + "\n\n" + content
	normalizedBody := strings.ReplaceAll(body, "\r\n", "\n")
	if strings.TrimSpace(normalizedBody) == "" {
		return withNewlineStyle(section+"\n", newline)
	}
	separator := "\n\n"
	if strings.HasSuffix(normalizedBody, "\n\n") {
		separator = ""
	} else if strings.HasSuffix(normalizedBody, "\n") {
		separator = "\n"
	}
	return withNewlineStyle(normalizedBody+separator+section+"\n", newline)
}

func upsertADRMoreInformationLine(body string, line string, match string) string {
	newline := dominantNewline(body)
	lines := splitLinesForEdit(body)
	for i, current := range lines {
		level := headingLevel(current)
		if level != 2 && level != 3 {
			continue
		}
		trimmedLine := strings.TrimSpace(current)
		if !strings.EqualFold(strings.TrimSpace(trimmedLine[level:]), "More Information") {
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
		section := append([]string{}, lines[i+1:end]...)
		filtered := section[:0]
		for _, existing := range section {
			if strings.Contains(existing, match) {
				continue
			}
			filtered = append(filtered, existing)
		}
		for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
			filtered = filtered[:len(filtered)-1]
		}
		if len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) != "" {
			filtered = append(filtered, "")
		}
		filtered = append(filtered, line)
		if end < len(lines) {
			filtered = append(filtered, "")
		}
		updated := append([]string{}, lines[:i+1]...)
		updated = append(updated, filtered...)
		updated = append(updated, lines[end:]...)
		return joinLinesForEdit(updated, newline)
	}
	return upsertADRSection(body, "More Information", line)
}

func dominantNewline(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func splitLinesForEdit(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func joinLinesForEdit(lines []string, newline string) string {
	return withNewlineStyle(strings.Join(lines, "\n"), newline)
}

func withNewlineStyle(text string, newline string) string {
	if newline == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func adrMarkdownLink(adr ADR) string {
	return fmt.Sprintf("[ADR-%s](%s-%s.md)", adr.ID, adr.ID, adr.Slug)
}

func splitADRFileName(path string) (int, string, bool) {
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	idPart, slug, ok := strings.Cut(base, "-")
	if !ok || len(idPart) != 3 || !validADRSlug(slug) {
		return 0, "", false
	}
	for _, r := range idPart {
		if r < '0' || r > '9' {
			return 0, "", false
		}
	}
	n, err := strconv.Atoi(idPart)
	if err != nil {
		return 0, "", false
	}
	return n, slug, true
}

func validADRSlug(slug string) bool {
	if slug == "" || slug[0] == '-' || slug[len(slug)-1] == '-' {
		return false
	}
	previousDash := false
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z':
			previousDash = false
		case r >= '0' && r <= '9':
			previousDash = false
		case r == '-':
			if previousDash {
				return false
			}
			previousDash = true
		default:
			return false
		}
	}
	return true
}

func normalizeADRRef(ref string) (string, bool) {
	n, suffix, ok := splitADRRef(ref)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%03d%s", n, suffix), true
}

func splitADRRef(ref string) (int, string, bool) {
	i := 0
	for i < len(ref) && ref[i] >= '0' && ref[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	if i > 3 {
		return 0, "", false
	}
	suffix := ref[i:]
	if suffix != "" && !strings.HasPrefix(suffix, "-") {
		return 0, "", false
	}
	n, err := strconv.Atoi(ref[:i])
	if err != nil {
		return 0, "", false
	}
	return n, suffix, true
}

func adrRef(adr ADR) string {
	if adr.Slug == "" {
		return adr.ID
	}
	return adr.ID + "-" + adr.Slug
}

func adrIDLess(a string, b string) bool {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil && an != bn {
		return an < bn
	}
	return a < b
}

func adrMetaExtra(meta map[string]string) map[string]string {
	extra := map[string]string{}
	for k, v := range meta {
		switch k {
		case "id", "status", "date", "decision-makers", "consulted", "informed":
		default:
			extra[k] = v
		}
	}
	return extra
}

func legacyBoldValue(text string, field string) string {
	prefix := "**" + field + ":**"
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}
