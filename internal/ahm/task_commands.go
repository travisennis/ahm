package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) taskCommand() *cobra.Command {
	task := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("task requires a subcommand")
		},
	}

	createArgs := taskCreateArgs{
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:cli",
		status:   "Pending",
	}
	create := &cobra.Command{
		Use:   "create <title> [flags]",
		Short: "Create a task",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("task create requires a title")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			createArgs.title = strings.Join(args, " ")
			return a.taskCreateParsed(createArgs)
		},
	}
	create.Flags().StringVarP(&createArgs.priority, "priority", "p", createArgs.priority, "Set task priority")
	create.Flags().StringVar(&createArgs.effort, "effort", createArgs.effort, "Set task effort")
	create.Flags().StringVar(&createArgs.labels, "labels", createArgs.labels, "Set task labels")
	create.Flags().StringVar(&createArgs.status, "status", createArgs.status, "Set initial task status")
	create.Flags().StringVarP(&createArgs.description, "description", "d", "", "Set task summary text")
	task.AddCommand(create)

	task.AddCommand(a.taskListCommand("list", []string{"ls"}, "List tasks", "all"))
	task.AddCommand(a.taskListCommand("ready", nil, "List ready tasks", "ready"))
	task.AddCommand(a.taskListCommand("blocked", nil, "List blocked tasks", "blocked"))
	task.AddCommand(&cobra.Command{
		Use:   "next",
		Short: "Show the next ready task",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskNext()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Normalize legacy task front matter",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskMigrate()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a task",
		Args:  exactArgs(1, "task show requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskShow(args)
		},
	})

	for _, spec := range []struct {
		use     string
		aliases []string
		short   string
		status  string
	}{
		{use: "start <id>", short: "Mark a task in progress", status: "In Progress"},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", status: "Completed"},
		{use: "cancel <id>", short: "Mark a task cancelled", status: "Cancelled"},
		{use: "reopen <id>", short: "Reopen a task", status: "Pending"},
	} {
		status := spec.status
		task.AddCommand(&cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Args:    exactArgs(1, "task status command requires an id"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.taskStatus(args, status)
			},
		})
	}

	task.AddCommand(a.taskDepCommand())
	return task
}

func (a *app) taskListCommand(use string, aliases []string, short string, mode string) *cobra.Command {
	status := ""
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskList(mode, status)
		},
	}
	if mode == "all" {
		cmd.Flags().StringVar(&status, "status", "", "Filter tasks by status")
	}
	return cmd
}

type taskCreateArgs struct {
	title       string
	priority    string
	effort      string
	labels      string
	status      string
	description string
}

func (a *app) taskCreateParsed(parsed taskCreateArgs) error {
	if parsed.title == "" {
		return usageError("task create requires a title")
	}
	if err := validateTaskCreateEnums(parsed); err != nil {
		return err
	}
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	id := nextTaskID(tasks, a.opts.root)
	path := filepath.Join(a.opts.root, ".agents", ".tasks", "active", id+".md")
	body := parsed.description
	if body == "" {
		body = "TODO."
	}
	now := time.Now().Format(time.RFC3339)
	content := renderTask(Task{
		ID:       id,
		Title:    parsed.title,
		Status:   parsed.status,
		Priority: parsed.priority,
		Effort:   parsed.effort,
		Labels:   parsed.labels,
		ExecPlan: "-",
		Created:  now,
		Body:     "## Summary\n\n" + body + "\n\n## Acceptance Notes\n\n- [ ] TODO\n",
	})
	if a.opts.dryRun {
		return a.emit(map[string]any{"create": path, "id": id})
	}
	if err := writeFileAtomic(path, []byte(content), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, id)
	return nil
}

func nextTaskID(tasks []Task, root string) string {
	maxID := 0
	for _, task := range tasks {
		n, suffix, ok := splitTaskID(task.ID)
		if ok && suffix == "" && n > maxID {
			maxID = n
		}
	}
	// Also scan the filesystem for task files that may have been skipped
	// due to parse errors, to avoid colliding with them.
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := filepath.Join(root, ".agents", ".tasks", bucket)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			n, suffix, ok := splitTaskID(strings.TrimSuffix(entry.Name(), ".md"))
			if ok && suffix == "" && n > maxID {
				maxID = n
			}
		}
	}
	return fmt.Sprintf("%03d", maxID+1)
}

func (a *app) taskList(mode string, status string) error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, mode)
	if status != "" {
		normalized, err := normalizeTaskStatus(status)
		if err != nil {
			return err
		}
		filtered = filterTasksByStatus(filtered, normalized)
	}
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		a.printTaskLine(task)
	}
	return nil
}

func (a *app) taskNext() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	ready := filterTasks(tasks, "ready")
	if len(ready) == 0 {
		if a.opts.json {
			return a.emit(nil)
		}
		fmt.Fprintln(a.out, "No ready tasks.")
		return nil
	}
	if a.opts.json {
		return a.emit(ready[0])
	}
	a.printTaskLine(ready[0])
	return nil
}

func (a *app) printTaskLine(task Task) {
	fmt.Fprintf(a.out, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
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

func filterTasksByStatus(tasks []Task, status string) []Task {
	var out []Task
	for _, task := range tasks {
		if task.Status == status {
			out = append(out, task)
		}
	}
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

func normalizeTaskStatus(status string) (string, error) {
	key := enumKey(status)
	for _, item := range statusOrder() {
		if enumKey(item) == key {
			return item, nil
		}
	}
	return "", usageError(enumError("status", status, statusOrder()))
}

func enumKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(value)
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

func (a *app) taskShow(argv []string) error {
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

func (a *app) taskStatus(argv []string, status string) error {
	if len(argv) != 1 {
		return usageError("task status command requires an id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}

	// Enforce dependency completion before completing a task.
	if status == "Completed" && len(task.DependsOn) > 0 {
		allTasks, collErr := collectTasks(a.opts.root)
		if collErr != nil {
			fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
		}
		completed := map[string]bool{}
		for _, t := range allTasks {
			if t.Status == "Completed" {
				completed[t.ID] = true
			}
		}
		var incomplete []string
		for _, dep := range task.DependsOn {
			if !completed[dep] {
				incomplete = append(incomplete, dep)
			}
		}
		if len(incomplete) > 0 {
			return fmt.Errorf("cannot complete task %s: incomplete dependencies: %s",
				task.ID, strings.Join(incomplete, ", "))
		}
	}

	task.Status = status
	task.Updated = time.Now().Format(time.RFC3339)
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
	if err := writeFileAtomic(target, []byte(renderTask(task)), 0o644); err != nil {
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

func resolveTaskFromTasks(pattern string, tasks []Task) (Task, error) {
	// Exact match returns immediately.
	for _, task := range tasks {
		if task.ID == pattern {
			return task, nil
		}
	}
	// Constrained prefix matching: parse the numeric prefix so that "1"
	// matches "001", "01" matches "001", and "1a" matches "001a".
	patNum, patSuffix, patOk := splitTaskID(pattern)
	var matches []Task
	for _, task := range tasks {
		taskNum, taskSuffix, taskOk := splitTaskID(task.ID)
		if patOk && taskOk && taskNum == patNum && strings.HasPrefix(taskSuffix, patSuffix) {
			matches = append(matches, task)
		}
	}
	if len(matches) == 0 {
		return Task{}, fmt.Errorf("task %q not found", pattern)
	}
	if len(matches) > 1 {
		var ids []string
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return Task{}, fmt.Errorf("task %q is ambiguous, matches %s", pattern, strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func (a *app) resolveTask(pattern string) (Task, error) {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	return resolveTaskFromTasks(pattern, tasks)
}
