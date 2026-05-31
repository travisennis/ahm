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
	var files []taskFileInfo
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := filepath.Join(root, ".agents", ".tasks", bucket)
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
	ID          string
	Title       string
	Status      string
	Priority    string
	Effort      string
	Labels      string
	ExecPlan    string
	DependsOn   []string
	Created     string
	Updated     string
	Parent      string
	ExternalRef string
	Extra       map[string]string // unknown front matter fields preserved from the original file
	Path        string
	Bucket      string
	Body        string
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
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return meta, text, nil
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return meta, text, nil
	}
	raw := text[4 : 4+end]
	body := text[4+end+5:]
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.ContainsAny(key, " :") {
			return nil, "", fmt.Errorf("invalid front matter key %q", key)
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
			value = value[1 : len(value)-1]
			value = strings.TrimSpace(value)
		}
		if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
			return nil, "", fmt.Errorf("unsupported block scalar in front matter field %q", key)
		}
		meta[key] = value
	}
	return meta, body, nil
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
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "# "+title {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
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
	n, _ := strconv.Atoi(id[:i])
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
	fmt.Fprintf(&b, "id: %s\n", task.ID)
	fmt.Fprintf(&b, "title: %s\n", task.Title)
	fmt.Fprintf(&b, "status: %s\n", task.Status)
	fmt.Fprintf(&b, "priority: %s\n", task.Priority)
	fmt.Fprintf(&b, "effort: %s\n", task.Effort)
	fmt.Fprintf(&b, "labels: %s\n", task.Labels)
	fmt.Fprintf(&b, "exec_plan: %s\n", defaultDash(task.ExecPlan))
	fmt.Fprintf(&b, "depends_on: %s\n", formatList(task.DependsOn))
	if task.Created != "" {
		fmt.Fprintf(&b, "created: %s\n", task.Created)
	}
	if task.Updated != "" {
		fmt.Fprintf(&b, "updated: %s\n", task.Updated)
	}
	if task.Parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", task.Parent)
	}
	if task.ExternalRef != "" {
		fmt.Fprintf(&b, "external_ref: %s\n", task.ExternalRef)
	}
	for _, k := range sortedKeys(task.Extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, task.Extra[k])
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
