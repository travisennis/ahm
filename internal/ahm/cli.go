package ahm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

type options struct {
	root    string
	json    bool
	plain   bool
	quiet   bool
	verbose bool
	dryRun  bool
	force   bool
}

type app struct {
	opts options
	out  io.Writer
	err  io.Writer
}

func Main(argv []string, stdout io.Writer, stderr io.Writer) int {
	a := app{out: stdout, err: stderr}
	if err := a.run(argv); err != nil {
		var usage usageError
		if errors.As(err, &usage) {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

type usageError string

func (e usageError) Error() string {
	return string(e)
}

func (a *app) run(argv []string) error {
	opts, command, rest, err := parseGlobal(argv)
	if err != nil {
		return err
	}
	a.opts = opts
	if command == "" {
		command = "status"
	}
	if opts.root == "" {
		opts.root, err = detectRoot()
		if err != nil {
			return err
		}
		a.opts.root = opts.root
	}
	switch command {
	case "help", "--help", "-h":
		a.printHelp()
	case "version", "--version":
		fmt.Fprintln(a.out, templates.Version)
	case "init":
		return a.install(false)
	case "upgrade":
		return a.install(true)
	case "status":
		return a.status()
	case "doctor":
		return a.doctor()
	case "index":
		return a.writeIndexes()
	case "task":
		return a.task(rest)
	default:
		return usageError("unknown command: " + command)
	}
	return nil
}

func parseGlobal(argv []string) (options, string, []string, error) {
	var opts options
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if !strings.HasPrefix(arg, "-") {
			return opts, arg, argv[i+1:], nil
		}
		switch arg {
		case "--root":
			i++
			if i >= len(argv) {
				return opts, "", nil, usageError("--root requires a value")
			}
			opts.root = argv[i]
		case "--json":
			opts.json = true
		case "--plain":
			opts.plain = true
		case "--quiet":
			opts.quiet = true
		case "--verbose":
			opts.verbose = true
		case "--dry-run":
			opts.dryRun = true
		case "--force":
			opts.force = true
		case "--help", "-h":
			return opts, "help", nil, nil
		case "--version":
			return opts, "version", nil, nil
		default:
			return opts, "", nil, usageError("unknown global flag: " + arg)
		}
	}
	return opts, "", nil, nil
}

func (a app) printHelp() {
	fmt.Fprintln(a.out, `ahm manages repo-local .agents workflows.

Usage:
  ahm [global flags] <command> [command flags]

Commands:
  init                         Install workflow files
  upgrade                      Update managed workflow files
  status                       Show workflow health
  doctor                       Show environment checks
  index                        Regenerate task indexes
  task <subcommand>            Manage tasks

Task commands:
  create <title> [flags]
  list | ready | blocked
  show <id>
  start|complete|cancel|reopen <id>
  dep add|remove <id> <dependency-id>
  dep tree <id>
  dep cycles

Global flags:
  --root <path>  Target repository root
  --json         Print JSON
  --plain        Print stable plain output
  --dry-run      Preview supported writes
  --force        Force supported overwrites
  --version      Print version`)
}

func detectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if stat, err := os.Stat(filepath.Join(dir, ".git")); err == nil && stat.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd, nil
		}
		dir = parent
	}
}

type metadata struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func (a app) install(upgrade bool) error {
	root := a.opts.root
	meta, _ := readMetadata(root)
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	result := map[string][]string{
		"created":   {},
		"updated":   {},
		"skipped":   {},
		"conflicts": {},
	}
	for _, item := range templates.Files() {
		content, err := fs.ReadFile(templates.FS, item.Source)
		if err != nil {
			return err
		}
		target := filepath.Join(root, item.Target)
		hash := hashBytes(content)
		existing, readErr := os.ReadFile(target)
		switch {
		case errors.Is(readErr, os.ErrNotExist):
			result["created"] = append(result["created"], item.Target)
			if !a.opts.dryRun {
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(target, content, 0o644); err != nil {
					return err
				}
			}
			meta.Files[item.Target] = hash
		case readErr != nil:
			return readErr
		case !upgrade && !a.opts.force:
			result["skipped"] = append(result["skipped"], item.Target)
		case a.opts.force || meta.Files[item.Target] == hashBytes(existing):
			if string(existing) != string(content) {
				result["updated"] = append(result["updated"], item.Target)
				if !a.opts.dryRun {
					if err := os.WriteFile(target, content, 0o644); err != nil {
						return err
					}
				}
			} else {
				result["skipped"] = append(result["skipped"], item.Target)
			}
			meta.Files[item.Target] = hash
		default:
			result["conflicts"] = append(result["conflicts"], item.Target)
		}
	}
	if err := a.ensureWorkflowDirs(); err != nil {
		return err
	}
	meta.Version = templates.Version
	if !a.opts.dryRun {
		if err := writeMetadata(root, meta); err != nil {
			return err
		}
		if err := a.writeIndexes(); err != nil {
			return err
		}
	}
	return a.emit(result)
}

func (a app) ensureWorkflowDirs() error {
	dirs := []string{
		".agents/.tasks/active",
		".agents/.tasks/completed",
		".agents/.tasks/cancelled",
		".agents/.research/inbox",
		".agents/.research/investigations",
		".agents/.research/sources",
		".agents/.research/topics",
		".agents/.research/archived",
		".agents/exec-plans/active",
		".agents/exec-plans/completed",
		".agents/skills/deslop",
		"docs/adr",
	}
	for _, dir := range dirs {
		if a.opts.dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Join(a.opts.root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func readMetadata(root string) (metadata, error) {
	var meta metadata
	data, err := os.ReadFile(filepath.Join(root, ".agents", "ahm.json"))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}

func writeMetadata(root string, meta metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".agents", "ahm.json"), append(data, '\n'), 0o644)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (a app) status() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	meta, metaErr := readMetadata(a.opts.root)
	status := map[string]any{
		"root":              a.opts.root,
		"template_version":  templates.Version,
		"installed":         metaErr == nil,
		"installed_version": meta.Version,
		"tasks":             taskCounts(tasks),
	}
	return a.emit(status)
}

func (a app) doctor() error {
	_, goErr := exec.LookPath("go")
	_, gitErr := exec.LookPath("git")
	meta, metaErr := readMetadata(a.opts.root)
	report := map[string]any{
		"root":               a.opts.root,
		"go_available":       goErr == nil,
		"git_available":      gitErr == nil,
		"workflow_installed": metaErr == nil,
		"installed_version":  meta.Version,
		"template_version":   templates.Version,
	}
	return a.emit(report)
}

func (a app) emit(value any) error {
	if a.opts.json {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	if m, ok := value.(map[string][]string); ok {
		for _, key := range []string{"created", "updated", "skipped", "conflicts"} {
			items := m[key]
			if len(items) == 0 {
				continue
			}
			fmt.Fprintf(a.out, "%s:\n", key)
			for _, item := range items {
				fmt.Fprintf(a.out, "  %s\n", item)
			}
		}
		return nil
	}
	if a.opts.plain {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.out, string(data))
	return err
}

type Task struct {
	ID        string
	Title     string
	Status    string
	Priority  string
	Effort    string
	Labels    string
	ExecPlan  string
	DependsOn []string
	Path      string
	Bucket    string
	Body      string
}

func collectTasks(root string) ([]Task, error) {
	var tasks []Task
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := filepath.Join(root, ".agents", ".tasks", bucket)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			task, err := parseTask(filepath.Join(dir, entry.Name()), bucket)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	return tasks, nil
}

func parseTask(path string, bucket string) (Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	text := string(data)
	meta, body := parseFrontMatter(text)
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
		ID:        id,
		Title:     title,
		Status:    defaultString(meta["status"], "-"),
		Priority:  defaultString(meta["priority"], "-"),
		Effort:    defaultString(meta["effort"], "-"),
		Labels:    defaultString(meta["labels"], "-"),
		ExecPlan:  defaultString(meta["exec_plan"], "-"),
		DependsOn: parseList(meta["depends_on"]),
		Path:      path,
		Bucket:    bucket,
		Body:      body,
	}
	if err := validateTaskEnums(task, path); err != nil {
		return Task{}, err
	}
	return task, nil
}

func parseFrontMatter(text string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(text, "---\n") {
		return meta, text
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return meta, text
	}
	raw := text[4 : 4+end]
	body := text[4+end+5:]
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return meta, body
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

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func taskLess(a string, b string) bool {
	an, as := splitTaskID(a)
	bn, bs := splitTaskID(b)
	if an != bn {
		return an < bn
	}
	return as < bs
}

func splitTaskID(id string) (int, string) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 {
		return 999999, id
	}
	n, _ := strconv.Atoi(id[:i])
	return n, id[i:]
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

func (a app) task(argv []string) error {
	if len(argv) == 0 {
		return usageError("task requires a subcommand")
	}
	switch argv[0] {
	case "create":
		return a.taskCreate(argv[1:])
	case "list", "ls":
		return a.taskList("all")
	case "ready":
		return a.taskList("ready")
	case "blocked":
		return a.taskList("blocked")
	case "show":
		return a.taskShow(argv[1:])
	case "start":
		return a.taskStatus(argv[1:], "In Progress")
	case "complete", "close":
		return a.taskStatus(argv[1:], "Completed")
	case "cancel":
		return a.taskStatus(argv[1:], "Cancelled")
	case "reopen":
		return a.taskStatus(argv[1:], "Pending")
	case "dep":
		return a.taskDep(argv[1:])
	default:
		return usageError("unknown task subcommand: " + argv[0])
	}
}

func (a app) taskCreate(argv []string) error {
	parsed, err := parseTaskCreateArgs(argv)
	if err != nil {
		return err
	}
	if parsed.title == "" {
		return usageError("task create requires a title")
	}
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	id := nextTaskID(tasks)
	path := filepath.Join(a.opts.root, ".agents", ".tasks", "active", id+".md")
	body := parsed.description
	if body == "" {
		body = "TODO."
	}
	content := renderTask(Task{
		ID:       id,
		Title:    parsed.title,
		Status:   parsed.status,
		Priority: parsed.priority,
		Effort:   parsed.effort,
		Labels:   parsed.labels,
		ExecPlan: "-",
		Body:     "## Summary\n\n" + body + "\n\n## Acceptance Notes\n\n- [ ] TODO\n",
	})
	if a.opts.dryRun {
		return a.emit(map[string]any{"create": path, "id": id})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, id)
	return nil
}

type taskCreateArgs struct {
	title       string
	priority    string
	effort      string
	labels      string
	status      string
	description string
}

func parseTaskCreateArgs(argv []string) (taskCreateArgs, error) {
	parsed := taskCreateArgs{
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:cli",
		status:   "Pending",
	}
	var title []string
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "--priority", "-p":
			i++
			if i >= len(argv) {
				return parsed, usageError(arg + " requires a value")
			}
			parsed.priority = argv[i]
		case "--effort":
			i++
			if i >= len(argv) {
				return parsed, usageError(arg + " requires a value")
			}
			parsed.effort = argv[i]
		case "--labels":
			i++
			if i >= len(argv) {
				return parsed, usageError(arg + " requires a value")
			}
			parsed.labels = argv[i]
		case "--status":
			i++
			if i >= len(argv) {
				return parsed, usageError(arg + " requires a value")
			}
			parsed.status = argv[i]
		case "--description", "-d":
			i++
			if i >= len(argv) {
				return parsed, usageError(arg + " requires a value")
			}
			parsed.description = argv[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, usageError("unknown task create flag: " + arg)
			}
			title = append(title, arg)
		}
	}
	parsed.title = strings.Join(title, " ")
	if err := validateTaskCreateEnums(parsed); err != nil {
		return parsed, err
	}
	return parsed, nil
}

func nextTaskID(tasks []Task) string {
	maxID := 0
	for _, task := range tasks {
		n, suffix := splitTaskID(task.ID)
		if suffix == "" && n < 999999 && n > maxID {
			maxID = n
		}
	}
	return fmt.Sprintf("%03d", maxID+1)
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
	fmt.Fprintf(&b, "exec_plan: %s\n", defaultString(task.ExecPlan, "-"))
	fmt.Fprintf(&b, "depends_on: %s\n", formatList(task.DependsOn))
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "# %s\n\n", task.Title)
	body := strings.TrimSpace(task.Body)
	if body != "" {
		fmt.Fprintln(&b, body)
		fmt.Fprintln(&b)
	}
	return b.String()
}

func (a app) taskList(mode string) error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	filtered := filterTasks(tasks, mode)
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		fmt.Fprintf(a.out, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
	}
	return nil
}

func filterTasks(tasks []Task, mode string) []Task {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	var out []Task
	for _, task := range tasks {
		switch mode {
		case "ready":
			if task.Status == "Pending" && depsComplete(task, completed) {
				out = append(out, task)
			}
		case "blocked":
			if task.Status == "Blocked" || (task.Status == "Pending" && !depsComplete(task, completed)) {
				out = append(out, task)
			}
		default:
			out = append(out, task)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := priorityRank(out[i].Priority)
		pj := priorityRank(out[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return taskLess(out[i].ID, out[j].ID)
	})
	return out
}

func depsComplete(task Task, completed map[string]bool) bool {
	for _, dep := range task.DependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func priorityRank(priority string) int {
	switch priority {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	case "P4":
		return 4
	default:
		return 99
	}
}

func validateTaskCreateEnums(parsed taskCreateArgs) error {
	if !validTaskStatus(parsed.status) {
		return usageError(enumError("status", parsed.status, statusOrder()))
	}
	if !validTaskPriority(parsed.priority) {
		return usageError(enumError("priority", parsed.priority, priorityOrder()))
	}
	if !validTaskEffort(parsed.effort) {
		return usageError(enumError("effort", parsed.effort, effortOrder()))
	}
	return nil
}

func validateTaskEnums(task Task, source string) error {
	if !validTaskStatus(task.Status) {
		return fmt.Errorf("%s: %s", source, enumError("status", task.Status, statusOrder()))
	}
	if !validTaskPriority(task.Priority) {
		return fmt.Errorf("%s: %s", source, enumError("priority", task.Priority, priorityOrder()))
	}
	if !validTaskEffort(task.Effort) {
		return fmt.Errorf("%s: %s", source, enumError("effort", task.Effort, effortOrder()))
	}
	return nil
}

func enumError(field string, value string, allowed []string) string {
	return fmt.Sprintf("unsupported task %s %q (supported: %s)", field, value, strings.Join(allowed, ", "))
}

func validTaskStatus(status string) bool {
	return containsString(statusOrder(), status)
}

func validTaskPriority(priority string) bool {
	return containsString(priorityOrder(), priority)
}

func validTaskEffort(effort string) bool {
	return containsString(effortOrder(), effort)
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (a app) taskShow(argv []string) error {
	if len(argv) != 1 {
		return usageError("task show requires an id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	if a.opts.json {
		return a.emit(task)
	}
	data, err := os.ReadFile(task.Path)
	if err != nil {
		return err
	}
	_, err = a.out.Write(data)
	return err
}

func (a app) taskStatus(argv []string, status string) error {
	if len(argv) != 1 {
		return usageError("task status command requires an id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	task.Status = status
	bucket := "active"
	if status == "Completed" {
		bucket = "completed"
	}
	if status == "Cancelled" {
		bucket = "cancelled"
	}
	target := filepath.Join(a.opts.root, ".agents", ".tasks", bucket, task.ID+".md")
	if a.opts.dryRun {
		return a.emit(map[string]any{"move": target, "status": status})
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if filepath.Clean(task.Path) != filepath.Clean(target) {
		if err := os.Remove(task.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", task.ID, status)
	return nil
}

func (a app) resolveTask(pattern string) (Task, error) {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return Task{}, err
	}
	var matches []Task
	for _, task := range tasks {
		if task.ID == pattern || strings.Contains(task.ID, pattern) {
			matches = append(matches, task)
		}
	}
	if len(matches) == 0 {
		return Task{}, fmt.Errorf("task %q not found", pattern)
	}
	if len(matches) > 1 {
		return Task{}, fmt.Errorf("task %q is ambiguous", pattern)
	}
	return matches[0], nil
}

func (a app) taskDep(argv []string) error {
	if len(argv) == 0 {
		return usageError("task dep requires a subcommand")
	}
	switch argv[0] {
	case "add":
		return a.taskDepUpdate(argv[1:], true)
	case "remove", "rm":
		return a.taskDepUpdate(argv[1:], false)
	case "tree":
		return a.taskDepTree(argv[1:])
	case "cycles":
		return a.taskDepCycles()
	default:
		return usageError("unknown task dep subcommand: " + argv[0])
	}
}

func (a app) taskDepUpdate(argv []string, add bool) error {
	if len(argv) != 2 {
		return usageError("task dep add/remove requires task id and dependency id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	dep, err := a.resolveTask(argv[1])
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, item := range task.DependsOn {
		set[item] = true
	}
	if add {
		set[dep.ID] = true
	} else {
		delete(set, dep.ID)
	}
	task.DependsOn = nil
	for item := range set {
		task.DependsOn = append(task.DependsOn, item)
	}
	sort.Slice(task.DependsOn, func(i, j int) bool {
		return taskLess(task.DependsOn[i], task.DependsOn[j])
	})
	if a.opts.dryRun {
		return a.emit(map[string]any{"task": task.ID, "depends_on": task.DependsOn})
	}
	if err := os.WriteFile(task.Path, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s depends_on: %s\n", task.ID, formatList(task.DependsOn))
	return nil
}

func (a app) taskDepTree(argv []string) error {
	if len(argv) != 1 {
		return usageError("task dep tree requires an id")
	}
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	root, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	var walk func(id string, prefix string, seen map[string]bool)
	walk = func(id string, prefix string, seen map[string]bool) {
		task, ok := byID[id]
		if !ok {
			fmt.Fprintf(a.out, "%s%s [missing]\n", prefix, id)
			return
		}
		fmt.Fprintf(a.out, "%s%s [%s] %s\n", prefix, task.ID, task.Status, task.Title)
		if seen[id] {
			fmt.Fprintf(a.out, "%s  cycle to %s\n", prefix, id)
			return
		}
		nextSeen := map[string]bool{}
		for k, v := range seen {
			nextSeen[k] = v
		}
		nextSeen[id] = true
		for _, dep := range task.DependsOn {
			walk(dep, prefix+"  ", nextSeen)
		}
	}
	walk(root.ID, "", map[string]bool{})
	return nil
}

func (a app) taskDepCycles() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	byID := map[string]Task{}
	for _, task := range tasks {
		if task.Status != "Completed" && task.Status != "Cancelled" {
			byID[task.ID] = task
		}
	}
	var cycles [][]string
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var dfs func(string, []string)
	dfs = func(id string, path []string) {
		if visiting[id] {
			start := 0
			for i, item := range path {
				if item == id {
					start = i
					break
				}
			}
			cycles = append(cycles, append(path[start:], id))
			return
		}
		if visited[id] {
			return
		}
		task, ok := byID[id]
		if !ok {
			return
		}
		visiting[id] = true
		for _, dep := range task.DependsOn {
			dfs(dep, append(path, id))
		}
		visiting[id] = false
		visited[id] = true
	}
	for id := range byID {
		dfs(id, nil)
	}
	if len(cycles) == 0 {
		fmt.Fprintln(a.out, "No dependency cycles found")
		return nil
	}
	for _, cycle := range cycles {
		fmt.Fprintln(a.out, strings.Join(cycle, " -> "))
	}
	return nil
}

func (a app) writeIndexes() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	writes := map[string]string{
		filepath.Join(a.opts.root, ".agents", ".tasks", "index.md"):              renderRootIndex(tasks),
		filepath.Join(a.opts.root, ".agents", ".tasks", "active", "index.md"):    renderBucketIndex(tasks, "active"),
		filepath.Join(a.opts.root, ".agents", ".tasks", "completed", "index.md"): renderBucketIndex(tasks, "completed"),
		filepath.Join(a.opts.root, ".agents", ".tasks", "cancelled", "index.md"): renderBucketIndex(tasks, "cancelled"),
	}
	for path, content := range writes {
		if a.opts.dryRun {
			fmt.Fprintln(a.out, path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func renderRootIndex(tasks []Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Task Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Status Summary")
	fmt.Fprintln(&b)
	counts := taskCounts(tasks)
	for _, status := range statusOrder() {
		fmt.Fprintf(&b, "- %s: %d\n", status, counts[status])
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Next Ready Queue")
	fmt.Fprintln(&b)
	ready := filterTasks(tasks, "ready")
	if len(ready) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		for i, task := range ready {
			fmt.Fprintf(&b, "%d. [%s](active/%s.md) - %s (%s, %s; %s)\n", i+1, task.ID, task.ID, task.Title, task.Priority, task.Effort, task.Labels)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Blocked Or Needs Triage")
	fmt.Fprintln(&b)
	waiting := filterTasks(tasks, "blocked")
	for _, task := range tasks {
		if task.Status == "Open" {
			waiting = append(waiting, task)
		}
	}
	if len(waiting) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, waiting, ".")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## All Tasks")
	fmt.Fprintln(&b)
	writeTaskTable(&b, tasks, ".")
	return b.String()
}

func renderBucketIndex(tasks []Task, bucket string) string {
	var b strings.Builder
	title := bucketTitle(bucket)
	fmt.Fprintf(&b, "# %s Tasks\n\n", title)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	var bucketTasks []Task
	for _, task := range tasks {
		if task.Bucket == bucket {
			bucketTasks = append(bucketTasks, task)
		}
	}
	if len(bucketTasks) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, bucketTasks, bucket)
	}
	return b.String()
}

func writeTaskTable(b *strings.Builder, tasks []Task, from string) {
	fmt.Fprintln(b, "| Task | Title | Status | Priority | Effort | Labels | ExecPlan | Depends on |")
	fmt.Fprintln(b, "| ---- | ----- | ------ | -------- | ------ | ------ | -------- | ---------- |")
	for _, task := range tasks {
		link := task.ID + ".md"
		if from == "." {
			link = filepath.ToSlash(filepath.Join(task.Bucket, task.ID+".md"))
		}
		fmt.Fprintf(
			b,
			"| [%s](%s) | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeCell(task.ID),
			link,
			escapeCell(task.Title),
			escapeCell(task.Status),
			escapeCell(task.Priority),
			escapeCell(task.Effort),
			escapeCell(task.Labels),
			escapeCell(task.ExecPlan),
			escapeCell(formatList(task.DependsOn)),
		)
	}
}

func escapeCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func bucketTitle(bucket string) string {
	if bucket == "" {
		return bucket
	}
	return strings.ToUpper(bucket[:1]) + bucket[1:]
}

func statusOrder() []string {
	return []string{"Open", "Pending", "In Progress", "Blocked", "Tracking", "Completed", "Cancelled"}
}

func priorityOrder() []string {
	return []string{"P0", "P1", "P2", "P3", "P4"}
}

func effortOrder() []string {
	return []string{"XS", "S", "M", "L", "XL"}
}
