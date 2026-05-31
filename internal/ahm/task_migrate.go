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
	paths, err := taskMarkdownPaths(a.opts.root)
	if err != nil {
		return err
	}
	var migrations []taskMigration
	writes := map[string]string{}
	for _, path := range paths {
		data, err := readWorkflowFile(path)
		if err != nil {
			return err
		}
		next, changes := migrateTaskFrontMatter(string(data))
		if len(changes) == 0 {
			continue
		}
		rel := relPath(a.opts.root, path)
		migrations = append(migrations, taskMigration{Path: rel, Changes: changes})
		writes[path] = next
	}
	if a.opts.json || a.opts.plain {
		return a.emit(map[string]any{"migrations": migrations})
	}
	if a.opts.dryRun {
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
	index := frontMatterLineIndex(lines)
	var changes []string
	if _, ok := index["labels"]; !ok {
		lines = insertFrontMatterField(lines, index, "labels", "type:task, area:unknown", "effort")
		index = frontMatterLineIndex(lines)
		changes = append(changes, "add labels")
	}
	changes = append(changes, normalizeEnumField(lines, index, "priority", "P3", priorityOrder())...)
	changes = append(changes, normalizeEnumField(lines, index, "effort", "M", effortOrder())...)
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
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + body, changes
}

func splitFrontMatter(text string) (string, string, bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", text, false
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", text, false
	}
	return text[4 : 4+end], text[4+end+5:], true
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

func frontMatterValue(line string) string {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
		value = value[1 : len(value)-1]
		value = strings.TrimSpace(value)
	}
	return value
}

var leadingTaskIDPattern = regexp.MustCompile(`^([0-9]+[A-Za-z]?)\b`)
var followsTaskIDPattern = regexp.MustCompile(`(?i)^follows\s+([0-9]+[A-Za-z]?)\.?$`)
var completedByTaskIDPattern = regexp.MustCompile(`(?i)^completed by\s+([0-9]+[A-Za-z]?)\.?$`)

func normalizeDependsOnValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
		value = value[1 : len(value)-1]
		value = strings.TrimSpace(value)
	}
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
