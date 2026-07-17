package ahm

import (
	"fmt"
	"regexp"
	"strings"
)

type taskMigration struct {
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
}

func (a *app) taskMigrate() error {
	defer a.emitWarnings()

	if a.opts.dryRun || a.opts.json || a.opts.plain {
		return a.taskMigratePreview()
	}
	return a.withWorkflowRecordLock(true, func() error {
		return a.taskMigrateLocked()
	})
}

func (a *app) taskMigratePreview() error {
	migrations, _, err := a.taskMigrateCompute()
	if err != nil {
		return err
	}
	if a.opts.json || a.opts.plain {
		return a.emit(map[string]any{"migrations": migrations})
	}
	if len(migrations) == 0 {
		fmt.Fprintln(a.out, "No task migrations found")
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

func (a *app) taskMigrateLocked() error {
	_, writes, err := a.taskMigrateCompute()
	if err != nil {
		return err
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
	fmt.Fprintf(a.out, "migrated %d task files\n", len(writes))
	return nil
}

func (a *app) taskMigrateCompute() ([]taskMigration, map[string]string, error) {
	paths, err := taskMarkdownPaths(a.opts.root)
	if err != nil {
		return nil, nil, err
	}
	var migrations []taskMigration
	writes := map[string]string{}
	for _, path := range paths {
		data, err := readWorkflowFile(path)
		if err != nil {
			return nil, nil, err
		}
		next, changes := migrateTaskFrontMatter(string(data))
		if len(changes) == 0 {
			continue
		}
		rel := relPath(a.opts.root, path)
		migrations = append(migrations, taskMigration{Path: rel, Changes: changes})
		if next != string(data) {
			writes[path] = next
		}
	}
	return migrations, writes, nil
}

func taskMarkdownPaths(root string) ([]string, error) {
	files, err := taskFilePaths(root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths, nil
}

func migrateTaskFrontMatter(text string) (string, []string) {
	raw, body, ok := splitFrontMatter(text)
	if !ok {
		return text, nil
	}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		if _, _, _, err := parseFrontMatterLine(line); err != nil {
			return text, []string{err.Error() + "; fix manually"}
		}
	}
	index := frontMatterLineIndex(lines)
	var changes []string

	// Recompute index after mutations that shift line count (insertFrontMatterField)
	// or conditionally after in-place mutations (normalizeEnumField), so that
	// subsequent operations always see an up-to-date view.

	if _, ok := index["labels"]; !ok {
		lines = insertFrontMatterField(lines, index, "labels", "type:task, area:unknown", "effort")
		index = frontMatterLineIndex(lines)
		changes = append(changes, "add labels")
	}
	if c := normalizeEnumField(lines, index, "priority", "P3", priorityOrder()); len(c) > 0 {
		changes = append(changes, c...)
		index = frontMatterLineIndex(lines)
	}
	if c := normalizeEnumField(lines, index, "effort", "M", effortOrder()); len(c) > 0 {
		changes = append(changes, c...)
		index = frontMatterLineIndex(lines)
	}
	if i, ok := index["depends_on"]; ok {
		oldLine := lines[i]
		oldValue := frontMatterValue(lines[i])
		if next, ok := normalizeDependsOnValue(oldValue); ok && oldLine != "depends_on: "+next {
			lines[i] = "depends_on: " + next
			changes = append(changes, "normalize depends_on")
		}
	}
	if len(changes) == 0 {
		return text, nil
	}
	out := "---\n" + strings.Join(lines, "\n") + "\n---\n" + body
	if out == text {
		return text, changes
	}
	return out, changes
}

func frontMatterLineIndex(lines []string) map[string]int {
	index := map[string]int{}
	for i, line := range lines {
		key, _, ok := strings.Cut(line, ":")
		if ok {
			index[strings.TrimSpace(key)] = i
		}
	}
	return index
}

func insertFrontMatterField(lines []string, index map[string]int, field string, value string, after string) []string {
	line := field + ": " + value
	if i, ok := index[after]; ok {
		out := append([]string{}, lines[:i+1]...)
		out = append(out, line)
		return append(out, lines[i+1:]...)
	}
	return append(lines, line)
}

func normalizeEnumField(lines []string, index map[string]int, field string, placeholder string, allowed []string) []string {
	i, ok := index[field]
	if !ok {
		return nil
	}
	value := frontMatterValue(lines[i])
	if value == "-" || value == "[]" {
		lines[i] = field + ": " + placeholder
		return []string{"set " + field + " placeholder to " + placeholder}
	}
	for _, item := range allowed {
		if value == item {
			return nil
		}
		if strings.HasPrefix(value, item+" ") || strings.HasPrefix(value, item+"(") {
			lines[i] = field + ": " + item
			return []string{"normalize " + field + " to " + item}
		}
	}
	return nil
}

var leadingTaskIDPattern = regexp.MustCompile(`^([0-9]+[A-Za-z]?)\b`)
var followsTaskIDPattern = regexp.MustCompile(`(?i)^follows\s+([0-9]+[A-Za-z]?)\.?$`)
var completedByTaskIDPattern = regexp.MustCompile(`(?i)^completed by\s+([0-9]+[A-Za-z]?)\.?$`)

func normalizeDependsOnValue(value string) (string, bool) {
	value = unquoteFrontMatterScalar(value)
	if value == "" || value == "-" || value == "[]" {
		return "-", true
	}
	if match := followsTaskIDPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1], true
	}
	if match := completedByTaskIDPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1], true
	}
	for _, prefix := range []string{"Closed as obsolete:", "From code review", "Resolved in same PR", "Research:"} {
		if strings.HasPrefix(value, prefix) {
			return "-", true
		}
	}
	parts := splitTopLevelCommas(strings.Trim(value, "[]"))
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		match := leadingTaskIDPattern.FindStringSubmatch(item)
		if len(match) != 2 {
			return value, false
		}
		ids = append(ids, match[1])
	}
	if len(ids) == 0 {
		return "-", true
	}
	next := strings.Join(ids, ", ")
	return next, next != value
}

func splitTopLevelCommas(value string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, value[start:])
	return parts
}
