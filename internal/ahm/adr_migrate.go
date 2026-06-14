package ahm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type adrMigration struct {
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
}

// hasSupersessionHeading checks whether the body already contains a level-2 or
// level-3 heading that contains "supersession" (case-insensitive).
func hasSupersessionHeading(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if level := headingLevel(line); level > 0 {
			if strings.Contains(strings.ToLower(strings.TrimSpace(trimmed[level:])), "supersession") {
				return true
			}
		}
	}
	return false
}

// adrFullSupersessionPattern matches "Superseded" with optional suffix.
var adrFullSupersessionPattern = regexp.MustCompile(`(?i)^Superseded\s*$`)

// adrPartialSupersessionPattern matches "Accepted, superseded in part by ADR NNN".
var adrPartialSupersessionPattern = regexp.MustCompile(`(?i)^Accepted,\s*superseded\s+in\s+part\s+by\s+ADR\s*-?\s*([0-9]{3})\s*$`)

func (a *app) adrMigrate() error {
	paths, err := adrFilePaths(a.opts.root)
	if err != nil {
		return err
	}
	var migrations []adrMigration
	writes := map[string]string{}
	for _, path := range paths {
		data, err := readWorkflowFile(path)
		if err != nil {
			return err
		}
		next, changes := migrateLegacyADR(string(data), path)
		if len(changes) == 0 {
			continue
		}
		rel := relPath(a.opts.root, path)
		migrations = append(migrations, adrMigration{Path: rel, Changes: changes})
		if next != string(data) {
			writes[path] = next
		}
	}
	if a.opts.json || a.opts.plain {
		return a.emit(map[string]any{"migrations": migrations})
	}
	if a.opts.dryRun {
		if len(migrations) == 0 {
			fmt.Fprintln(a.out, "No ADR migrations found")
			return nil
		}
		fmt.Fprintln(a.out, "migrations:")
		for _, migration := range migrations {
			fmt.Fprintf(a.out, "  %s:\n", migration.Path)
			for _, change := range migration.Changes {
				fmt.Fprintf(a.out, "    - %s\n", change)
			}
		}
		return nil
	}
	for _, path := range sortedKeys(writes) {
		if err := writeFileAtomic(path, []byte(writes[path]), 0o644); err != nil {
			return err
		}
	}
	if len(writes) > 0 {
		if err := a.writeIndexes(); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.out, "migrated %d ADR files\n", len(writes))
	return nil
}

// migrateLegacyADR converts a legacy ADR (H1 + bold Status/Date) to MADR front
// matter. Returns the new content and a list of changes. If the ADR already has
// front matter or does not match the legacy format, returns the original content
// with no changes.
func migrateLegacyADR(text string, path string) (string, []string) {
	id, slug, ok := splitADRFileName(path)
	if !ok {
		return text, nil
	}
	paddedID := fmt.Sprintf("%03d", id)

	// Skip files that already have front matter (MADR records).
	if _, _, has := splitFrontMatter(text); has {
		return text, nil
	}

	// Check that the H1 contains ADR NNN: to confirm legacy format.
	heading := headingTitle(text, "")
	if !strings.HasPrefix(heading, "ADR ") {
		return text, nil
	}
	prefix := "ADR " + paddedID + ":"
	if !strings.HasPrefix(heading, prefix) {
		return text, nil
	}

	// Extract status and date from bold lines.
	status := legacyBoldValue(text, "Status")
	date := legacyBoldValue(text, "Date")
	if status == "" || date == "" {
		return text, []string{"missing Status or Date bold lines; fix manually"}
	}

	// Map legacy status to MADR status.
	madrStatus, notes, err := mapLegacyADRStatus(status, text)
	if err != nil {
		return text, []string{err.Error() + "; fix manually"}
	}

	// Build cleaned body: strip H1, strip bold Status/Date lines.
	body := stripHeading(text, heading)
	body = stripBoldLines(body, "Status", "Date")

	// Build new title without the "ADR NNN:" prefix.
	newTitle := strings.TrimSpace(strings.TrimPrefix(heading, prefix))

	// If partial supersession, add or preserve the supersession note.
	// Only add if there isn't already a supersession-related heading.
	if notes != "" && !hasSupersessionHeading(body) {
		note := "Superseded in part by " + notes + "."
		body = upsertADRSection(body, "Supersession", note)
	}

	changes := []string{"convert legacy metadata to MADR front matter"}
	adr := ADR{
		ID:     paddedID,
		Slug:   slug,
		Title:  newTitle,
		Status: madrStatus,
		Date:   date,
		Extra:  map[string]string{},
		Body:   body,
		Kind:   adrKindMADR,
		Path:   path,
	}
	rendered := renderADR(adr)
	return rendered, changes
}

// mapLegacyADRStatus converts a legacy Status bold value to a MADR status.
// For partial supersession, it returns "accepted" and a note string identifying
// the replacement ADR. For full supersession, it resolves the replacement ID
// from the body. Returns an error when the status cannot be mapped or when a
// supersession replacement cannot be resolved unambiguously.
func mapLegacyADRStatus(status string, text string) (string, string, error) {
	trimmed := strings.TrimSpace(status)
	switch {
	case trimmed == "Proposed":
		return "proposed", "", nil
	case trimmed == "Accepted":
		return "accepted", "", nil
	case trimmed == "Deprecated":
		return "deprecated", "", nil
	case adrPartialSupersessionPattern.MatchString(trimmed):
		// "Accepted, superseded in part by ADR NNN" -> status accepted, note with replacement.
		matches := adrPartialSupersessionPattern.FindStringSubmatch(trimmed)
		if len(matches) == 2 {
			return "accepted", "ADR-" + matches[1], nil
		}
		return "", "", fmt.Errorf("unsupported partial supersession status %q", trimmed)
	case adrFullSupersessionPattern.MatchString(trimmed):
		// "Superseded" -> try to resolve replacement ID from body.
		replacement, err := resolveSupersessionReplacement(text)
		if err != nil {
			return "", "", err
		}
		return "superseded by ADR-" + replacement, "", nil
	default:
		return "", "", fmt.Errorf("unrecognized status %q", trimmed)
	}
}

// resolveSupersessionReplacement looks for a supersession reference in the body
// that identifies the replacement ADR. Returns the replacement ID (3 digits) or
// an error if it cannot be resolved unambiguously.
func resolveSupersessionReplacement(text string) (string, error) {
	// Look for "ADR NNN" patterns in the body (after the H1).
	lines := strings.Split(text, "\n")
	inBody := false
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			inBody = true
			continue
		}
		if !inBody {
			continue
		}
		// Match "ADR NNN" references like "ADR 008", "ADR-008" or "ADR 008:".
		ref := extractADRReference(line)
		if ref != "" {
			return ref, nil
		}
	}
	return "", fmt.Errorf("cannot resolve supersession replacement")
}

// adrRefPattern matches ADR references like "ADR 008", "ADR-008" in text.
var adrRefPattern = regexp.MustCompile(`ADR\s*-?\s*([0-9]{3})(?:\.|:|,|\s|$)`)

func extractADRReference(line string) string {
	matches := adrRefPattern.FindStringSubmatch(line)
	if len(matches) == 2 {
		// Verify it looks like a valid ADR ID.
		if _, err := strconv.Atoi(matches[1]); err == nil {
			return matches[1]
		}
	}
	return ""
}

// stripBoldLines removes lines containing **fieldName:** from the body.
func stripBoldLines(body string, fields ...string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		skip := false
		for _, field := range fields {
			prefix := "**" + field + ":**"
			if strings.HasPrefix(trimmed, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
