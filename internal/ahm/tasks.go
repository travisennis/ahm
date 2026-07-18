package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// taskFileInfo holds the path and bucket for a task markdown file.
type taskFileInfo struct {
	Path   string
	Bucket string
}

// taskFilePaths collects all task markdown file paths across the
// active, completed, and cancelled buckets. It skips index.md and
// non-.md entries. Directories that do not exist are silently skipped.
func taskFilePaths(root string) ([]taskFileInfo, error) {
	paths := workflowPathsFor(root)
	var files []taskFileInfo
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := paths.tasksBucketDir(bucket)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			files = append(files, taskFileInfo{
				Path:   filepath.Join(dir, entry.Name()),
				Bucket: bucket,
			})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// Task is the parsed representation of a workflow task file.
type Task struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	Effort      string            `json:"effort"`
	Labels      string            `json:"labels"`
	ExecPlan    string            `json:"exec_plan"`
	DependsOn   []string          `json:"depends_on"`
	Created     string            `json:"created"`
	Updated     string            `json:"updated"`
	Parent      string            `json:"parent"`
	ExternalRef string            `json:"external_ref"`
	Extra       map[string]string `json:"extra"` // unknown front matter fields preserved from the original file
	Path        string            `json:"path"`
	Bucket      string            `json:"bucket"`
	Body        string            `json:"body"`
}

func collectTasks(root string) ([]Task, error) {
	files, err := taskFilePaths(root)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	var errs []error
	for _, f := range files {
		task, err := parseTask(f.Path, f.Bucket)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", relPath(root, f.Path), err))
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	if len(errs) > 0 {
		return tasks, errors.Join(errs...)
	}
	return tasks, nil
}

func parseTask(path string, bucket string) (Task, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return Task{}, err
	}
	return parseTaskFromData(data, path, bucket)
}

// parseTaskFromData is like parseTask but takes pre-read file data.
func parseTaskFromData(data []byte, path string, bucket string) (Task, error) {
	text := string(data)
	meta, body, err := parseFrontMatter(text)
	if err != nil {
		return Task{}, err
	}
	id := meta["id"]
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	title := meta["title"]
	if title == "" {
		title = headingTitle(body, id)
	}
	body = stripHeading(body, title)
	task := Task{
		ID:          id,
		Title:       title,
		Status:      defaultDash(meta["status"]),
		Priority:    defaultDash(meta["priority"]),
		Effort:      defaultDash(meta["effort"]),
		Labels:      defaultDash(meta["labels"]),
		ExecPlan:    defaultDash(meta["exec_plan"]),
		DependsOn:   parseList(meta["depends_on"]),
		Created:     meta["created"],
		Updated:     meta["updated"],
		Parent:      meta["parent"],
		ExternalRef: meta["external_ref"],
		Extra:       metaExtra(meta),
		Path:        path,
		Bucket:      bucket,
		Body:        body,
	}
	if err := validateTaskEnums(task, path); err != nil {
		return Task{}, err
	}
	return task, nil
}

func parseFrontMatter(text string) (map[string]string, string, error) {
	meta := map[string]string{}
	raw, body, ok, err := splitFrontMatter(text)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return meta, body, nil
	}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok, err := parseFrontMatterLine(line)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			continue
		}
		meta[key] = value
	}
	return meta, body, nil
}

func splitFrontMatter(text string) (string, string, bool, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text != "---" && !strings.HasPrefix(text, "---\n") {
		return "", text, false, nil
	}
	// A lone opening delimiter without a close is malformed front matter.
	if text == "---" {
		return "", text, false, fmt.Errorf("front matter is not closed")
	}
	rest := text[4:]
	// Empty front matter: closing delimiter immediately after opening.
	if rest == "---" {
		return "", "", true, nil
	}
	if strings.HasPrefix(rest, "---\n") {
		return "", rest[4:], true, nil
	}
	// Non-empty front matter: look for closing delimiter on its own line.
	end := strings.Index(rest, "\n---\n")
	if end >= 0 {
		return rest[:end], rest[end+5:], true, nil
	}
	// Closing delimiter at end of file (no trailing newline).
	if strings.HasSuffix(rest, "\n---") {
		return rest[:len(rest)-4], "", true, nil
	}
	return "", text, false, fmt.Errorf("front matter is not closed")
}

func parseFrontMatterLine(line string) (string, string, bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] == '#' {
		return "", "", false, nil
	}
	if strings.HasPrefix(trimmed, "- ") {
		return "", "", false, fmt.Errorf("unsupported block list syntax in front matter")
	}
	key, value, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", "", false, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false, nil
	}
	if strings.ContainsAny(key, " :") {
		return "", "", false, fmt.Errorf("invalid front matter key %q", key)
	}
	value = strings.TrimSpace(value)
	// Only unquoted values that look like block scalars are rejected; a quoted
	// value such as "| block" is intentionally escaped and must round-trip.
	if !isDoubleQuoted(value) {
		if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
			return "", "", false, fmt.Errorf("unsupported block scalar in front matter field %q", key)
		}
	}
	value = unquoteFrontMatterScalar(value)
	return key, value, true, nil
}

func isDoubleQuoted(value string) bool {
	return strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2
}

func frontMatterValue(line string) string {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return unquoteFrontMatterScalar(value)
}

func unquoteFrontMatterScalar(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "\"") || !strings.HasSuffix(value, "\"") || len(value) < 2 {
		return value
	}
	value = value[1 : len(value)-1]
	value = strings.TrimSpace(value)
	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '\\' && i+1 < len(value) && value[i+1] == '"' {
			b.WriteByte('"')
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// renderFrontMatterScalar renders a single-line front-matter value so that
// parseFrontMatterLine can read it back exactly. Newlines are collapsed to
// spaces, leading/trailing whitespace is trimmed, and values that would be
// misinterpreted by the minimal grammar are wrapped in double quotes. Inside
// quoted values, double quotes are escaped as \" so that values beginning with
// a quote can round-trip. Backslashes are left literal except when they precede
// an escaped quote, which the parser unescapes back to a quote.
func renderFrontMatterScalar(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return "\"\""
	}
	if !needsFrontMatterQuoting(value) {
		return value
	}
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for i := 0; i < len(value); i++ {
		if value[i] == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(value[i])
	}
	b.WriteByte('"')
	return b.String()
}

// needsFrontMatterQuoting reports whether a trimmed, non-empty scalar must be
// quoted to survive the minimal front-matter parser unchanged.
func needsFrontMatterQuoting(value string) bool {
	if value == "" {
		return true
	}
	if strings.TrimSpace(value) != value {
		return true
	}
	switch value[0] {
	case '#', '|', '>', '"':
		return true
	case '-':
		if len(value) >= 2 && value[1] == ' ' {
			return true
		}
	}
	return false
}

// metaExtra returns the subset of meta keys that are not known task fields.
func metaExtra(meta map[string]string) map[string]string {
	extra := map[string]string{}
	for k, v := range meta {
		switch k {
		case "id", "title", "status", "priority", "effort", "labels",
			"exec_plan", "depends_on", "created", "updated",
			"parent", "external_ref":
			// known field, skip
		default:
			extra[k] = v
		}
	}
	return extra
}

func headingTitle(body string, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallback
}

func stripHeading(body string, title string) string {
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && headingMatchesTitle(lines[0], title) {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func headingMatchesTitle(line string, title string) bool {
	heading, ok := strings.CutPrefix(strings.TrimSpace(line), "# ")
	if !ok {
		return false
	}
	heading = strings.TrimSpace(heading)
	if heading == title {
		return true
	}
	plainHeading := plainHeadingText(heading)
	return plainHeading != "" && plainHeading == strings.Join(strings.Fields(title), " ")
}

func plainHeadingText(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '`', '*', '~':
			continue
		case '\\':
			if i+1 < len(text) {
				i++
				b.WriteByte(text[i])
			}
		case '[':
			end := strings.IndexByte(text[i+1:], ']')
			if end < 0 {
				b.WriteByte(text[i])
				continue
			}
			label := text[i+1 : i+1+end]
			b.WriteString(plainHeadingText(label))
			i += end + 1
			if i+1 < len(text) && text[i+1] == '(' {
				if close := strings.IndexByte(text[i+2:], ')'); close >= 0 {
					i += close + 2
				}
			}
		default:
			b.WriteByte(text[i])
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || value == "[]" {
		return nil
	}
	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" && item != "-" {
			out = append(out, item)
		}
	}
	return out
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func defaultDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func taskLess(a string, b string) bool {
	an, as, aok := splitTaskID(a)
	bn, bs, bok := splitTaskID(b)
	if aok != bok {
		return aok // numeric IDs sort before non-numeric
	}
	if !aok {
		return a < b
	}
	if an != bn {
		return an < bn
	}
	return as < bs
}

func splitTaskID(id string) (int, string, bool) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, id, false
	}
	n, err := strconv.Atoi(id[:i])
	if err != nil {
		return 0, id, false
	}
	return n, id[i:], true
}

func taskCounts(tasks []Task) map[string]int {
	counts := map[string]int{}
	for _, status := range statusOrder() {
		counts[status] = 0
	}
	for _, task := range tasks {
		counts[task.Status]++
	}
	return counts
}

func renderTask(task Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "id: %s\n", renderFrontMatterScalar(task.ID))
	fmt.Fprintf(&b, "title: %s\n", renderFrontMatterScalar(task.Title))
	fmt.Fprintf(&b, "status: %s\n", renderFrontMatterScalar(task.Status))
	fmt.Fprintf(&b, "priority: %s\n", renderFrontMatterScalar(task.Priority))
	fmt.Fprintf(&b, "effort: %s\n", renderFrontMatterScalar(task.Effort))
	fmt.Fprintf(&b, "labels: %s\n", renderFrontMatterScalar(task.Labels))
	fmt.Fprintf(&b, "exec_plan: %s\n", renderFrontMatterScalar(task.ExecPlan))
	fmt.Fprintf(&b, "depends_on: %s\n", renderFrontMatterScalar(formatList(task.DependsOn)))
	if task.Created != "" {
		fmt.Fprintf(&b, "created: %s\n", renderFrontMatterScalar(task.Created))
	}
	if task.Updated != "" {
		fmt.Fprintf(&b, "updated: %s\n", renderFrontMatterScalar(task.Updated))
	}
	if task.Parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", renderFrontMatterScalar(task.Parent))
	}
	if task.ExternalRef != "" {
		fmt.Fprintf(&b, "external_ref: %s\n", renderFrontMatterScalar(task.ExternalRef))
	}
	for _, k := range sortedKeys(task.Extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, renderFrontMatterScalar(task.Extra[k]))
	}
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "# %s\n\n", task.Title)
	body := strings.TrimSpace(task.Body)
	if body != "" {
		fmt.Fprintln(&b, body)
		fmt.Fprintln(&b)
	}
	return b.String()
}
