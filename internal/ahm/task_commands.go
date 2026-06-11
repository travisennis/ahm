package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	taskWorkLookPath   = exec.LookPath
	taskWorkRunCommand = runTaskWorkCommand
)

func (a *app) taskCommand() *cobra.Command {
	task := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("task requires a subcommand")
		},
	}

	createArgs := taskCreateArgs{
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:unknown",
		status:   "Open",
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
	create.Flags().StringVar(&createArgs.bodyFile, "body-file", "", "Full Markdown body from a file (or - for stdin); ahm handles ID, front matter, and indexes")
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

	workArgs := taskWorkArgs{}
	work := &cobra.Command{
		Use:   "work <id>",
		Short: "Hand a task to a coding-agent CLI",
		Args:  exactArgs(1, "task work requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			workArgs.id = args[0]
			return a.taskWork(workArgs)
		},
	}
	work.Flags().StringVar(&workArgs.agent, "agent", "", "Agent to run: cake, codex, or cursor")
	task.AddCommand(work)

	for _, spec := range []struct {
		use        string
		aliases    []string
		short      string
		status     string
		withReason bool
	}{
		{use: "accept <id>", short: "Accept a task into the ready queue", status: "Pending"},
		{use: "start <id>", short: "Mark a task in progress", status: "In Progress"},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", status: "Completed"},
		{use: "cancel <id>", short: "Mark a task cancelled", status: "Cancelled", withReason: true},
		{use: "reopen <id>", short: "Reopen a task", status: "Pending"},
	} {
		status := spec.status
		reason := ""
		cmd := &cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Args:    exactArgs(1, "task status command requires an id"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.taskStatusWithArgs(taskStatusArgs{
					ids:    args,
					status: status,
					reason: reason,
				})
			},
		}
		if spec.withReason {
			cmd.Flags().StringVar(&reason, "reason", "", "Reason for cancelling the task")
		}
		task.AddCommand(cmd)
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
	bodyFile    string
}

type taskWorkArgs struct {
	id    string
	agent string
}

type taskStatusArgs struct {
	ids    []string
	status string
	reason string
}

type taskWorkAgent struct {
	name       string
	executable string
	args       func(string) []string
}

func (a *app) taskWork(parsed taskWorkArgs) error {
	task, err := a.resolveTask(parsed.id)
	if err != nil {
		return err
	}
	if task.Status == "Completed" || task.Status == "Cancelled" {
		return fmt.Errorf("cannot work task %s: status is %s", task.ID, task.Status)
	}
	if err := a.ensureTaskDependenciesComplete(task); err != nil {
		return err
	}
	agent, err := a.selectTaskWorkAgent(parsed.agent)
	if err != nil {
		return err
	}
	prompt := a.buildTaskWorkPrompt(task)
	executable, err := taskWorkLookPath(agent.executable)
	if err != nil {
		return fmt.Errorf("cannot work task %s with %s: executable %q not found on PATH", task.ID, agent.name, agent.executable)
	}
	args := agent.args(prompt)
	if a.opts.dryRun {
		return a.emit(map[string]any{
			"task":       task.ID,
			"agent":      agent.name,
			"executable": executable,
			"args":       args,
			"status":     taskWorkDryRunStatus(task.Status),
		})
	}
	if task.Status == "Pending" {
		if err := a.markTaskInProgress(task); err != nil {
			return err
		}
	}
	return taskWorkRunCommand(a.opts.root, executable, args, a.in, a.out, a.err)
}

func taskWorkDryRunStatus(status string) string {
	if status == "Pending" {
		return "In Progress"
	}
	return status
}

func (a *app) selectTaskWorkAgent(flagValue string) (taskWorkAgent, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		meta, err := readMetadata(a.opts.root)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// No metadata yet, no default agent configured.
		case err != nil:
			fmt.Fprintln(a.err, "warning: corrupt workflow metadata .agents/ahm.json, using default agent")
		default:
			value = meta.DefaultWorkAgent
		}
	}
	if value == "" {
		value = "cake"
	}
	return parseTaskWorkAgent(value)
}

func parseTaskWorkAgent(value string) (taskWorkAgent, error) {
	switch enumKey(value) {
	case "cake":
		return taskWorkAgent{
			name:       "cake",
			executable: "cake",
			args: func(prompt string) []string {
				return []string{"--output-format", "text", prompt}
			},
		}, nil
	case "codex":
		return taskWorkAgent{
			name:       "codex",
			executable: "codex",
			args: func(prompt string) []string {
				return []string{"exec", prompt}
			},
		}, nil
	case "cursor", "cursoragent":
		return taskWorkAgent{
			name:       "cursor",
			executable: "cursor-agent",
			args: func(prompt string) []string {
				return []string{"-p", "--output-format", "text", prompt}
			},
		}, nil
	default:
		return taskWorkAgent{}, usageError(fmt.Sprintf("unsupported task work agent %q (supported: cake, codex, cursor)", value))
	}
}

func (a *app) ensureTaskDependenciesComplete(task Task) error {
	if len(task.DependsOn) == 0 {
		return nil
	}
	allTasks, err := a.getTasks()
	if err != nil {
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
		return fmt.Errorf("cannot work task %s: incomplete dependencies: %s", task.ID, strings.Join(incomplete, ", "))
	}
	return nil
}

func (a *app) buildTaskWorkPrompt(task Task) string {
	taskPath := relPath(a.opts.root, task.Path)
	return fmt.Sprintf(`Work on task %s.

Before making changes, read AGENTS.md, .agents/TASKS.md, .agents/.tasks/index.md, and %s.

Use the repository task workflow. Keep changes scoped to the task. Fill the task Acceptance Notes when the work is done, run the required verification, and mark the task complete with ahm when acceptance is satisfied. Do not commit or push unless the user explicitly asked for that.
`, task.ID, taskPath)
}

func (a *app) markTaskInProgress(task Task) error {
	task.Status = "In Progress"
	task.Updated = time.Now().Format(time.RFC3339)
	target := filepath.Join(a.opts.root, ".agents", ".tasks", "active", task.ID+".md")
	if err := writeFileAtomic(target, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if filepath.Clean(task.Path) != filepath.Clean(target) {
		if err := os.Remove(task.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return a.writeIndexes()
}

// bucketForStatus returns the expected bucket directory for a task status.
// Active-status tasks (everything except Completed and Cancelled) live in active/.
func bucketForStatus(status string) string {
	switch status {
	case "Completed":
		return "completed"
	case "Cancelled":
		return "cancelled"
	default:
		return "active"
	}
}

func runTaskWorkCommand(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.Command(executable, args...) //nolint:gosec // executable is selected from the supported task work agent allowlist before LookPath.
	cmd.Dir = root
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (a *app) taskCreateParsed(parsed taskCreateArgs) error {
	if parsed.title == "" {
		return usageError("task create requires a title")
	}
	if err := validateTaskCreateEnums(parsed); err != nil {
		return err
	}
	body, err := a.resolveTaskCreateBody(parsed)
	if err != nil {
		return err
	}
	tasks, err := a.getTasks()
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	id := nextTaskID(tasks, a.opts.root)
	path := filepath.Join(a.opts.root, ".agents", ".tasks", "active", id+".md")
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
		Body:     body,
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

// resolveTaskCreateBody returns the Markdown body to render after the H1 title.
// When --body-file is set, the provided content (everything after the H1) is used
// verbatim; otherwise a default Summary/Acceptance Notes scaffold is generated
// from the optional --description text.
func (a *app) resolveTaskCreateBody(parsed taskCreateArgs) (string, error) {
	if parsed.bodyFile == "" {
		body := parsed.description
		if body == "" {
			body = "TODO."
		}
		return "## Summary\n\n" + body + "\n\n## Acceptance Notes\n\n- [ ] TODO\n", nil
	}
	if parsed.description != "" {
		return "", usageError("task create supports --body-file or --description, not both")
	}
	var (
		data   []byte
		err    error
		source string
	)
	if parsed.bodyFile == "-" {
		source = "stdin"
		if a.in == nil {
			return "", usageError("task create --body-file - requires stdin")
		}
		data, err = io.ReadAll(a.in)
	} else {
		source = parsed.bodyFile
		data, err = os.ReadFile(parsed.bodyFile)
	}
	if err != nil {
		return "", fmt.Errorf("reading task body from %s: %w", source, err)
	}
	body := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	if body == "" {
		return "", usageError(fmt.Sprintf("task body from %s is empty", source))
	}
	return body, nil
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
	tasks, err := a.getTasks()
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
	tasks, err := a.getTasks()
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
	return a.taskStatusWithArgs(taskStatusArgs{ids: argv, status: status})
}

func (a *app) taskStatusWithArgs(parsed taskStatusArgs) error {
	task, err := a.resolveTask(parsed.ids[0])
	if err != nil {
		return err
	}
	status := parsed.status
	cancelReason := strings.TrimSpace(parsed.reason)
	if status == "Cancelled" && cancelReason == "" {
		return usageError("task cancel requires --reason")
	}

	// Enforce dependency completion before completing a task,
	// but only when the status is actually changing.
	if task.Status != status && status == "Completed" && len(task.DependsOn) > 0 {
		allTasks, collErr := a.getTasks()
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

	// True no-op: status and bucket both match. Cancellation still rewrites the
	// task so the required reason can be inserted or replaced.
	expectedBucket := bucketForStatus(status)
	if task.Status == status && task.Bucket == expectedBucket && status != "Cancelled" {
		fmt.Fprintf(a.out, "%s already %s\n", task.ID, status)
		return nil
	}

	// Run acceptance notes validation only when actually transitioning to Completed.
	if task.Status != status && status == "Completed" {
		findings := parseAcceptanceNotes([]byte(task.Body))
		for _, finding := range findings {
			if a.err != nil {
				fmt.Fprintln(a.err, "warning:", finding.message(task.ID))
			}
		}
		if len(findings) > 0 && !a.opts.force {
			meta, err := readMetadata(a.opts.root)
			switch {
			case errors.Is(err, os.ErrNotExist):
				// No metadata, strict acceptance not configured.
			case err != nil:
				fmt.Fprintln(a.err, "warning: corrupt workflow metadata .agents/ahm.json, strict acceptance disabled")
			case meta.StrictAcceptance:
				return fmt.Errorf("cannot complete task %s: acceptance notes are incomplete; use --force to override", task.ID)
			}
		}
	}
	if status == "Cancelled" {
		warnCancellationAcceptancePlaceholder(a.err, task)
		task.Body = upsertCancellationReason(task.Body, cancelReason)
	}

	task.Status = status
	task.Updated = time.Now().Format(time.RFC3339)
	bucket := bucketForStatus(status)
	target := filepath.Join(a.opts.root, ".agents", ".tasks", bucket, task.ID+".md")
	if a.opts.dryRun {
		preview := map[string]any{"move": target, "status": status}
		if status == "Cancelled" {
			preview["reason"] = cancelReason
		}
		return a.emit(preview)
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

func warnCancellationAcceptancePlaceholder(stderr io.Writer, task Task) {
	if stderr == nil {
		return
	}
	for _, finding := range parseAcceptanceNotes([]byte(task.Body)) {
		if finding == taskAcceptancePlaceholder {
			fmt.Fprintln(stderr, "warning:", finding.message(task.ID))
		}
	}
}

func upsertCancellationReason(body string, reason string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		level := headingLevel(line)
		if level != 2 && level != 3 {
			continue
		}
		trimmedLine := strings.TrimSpace(line)
		if !isCancellationReasonHeading(trimmedLine[level:]) {
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
		replacement := []string{trimmedLine, "", reason}
		if end < len(lines) {
			replacement = append(replacement, "")
		}
		updated := append([]string{}, lines[:i]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		return strings.TrimSpace(strings.Join(updated, "\n"))
	}
	body = strings.TrimSpace(body)
	section := "## Cancellation Reason\n\n" + reason
	if body == "" {
		return section
	}
	return body + "\n\n" + section
}

func isCancellationReasonHeading(heading string) bool {
	return strings.EqualFold(strings.TrimSpace(heading), "Cancellation Reason")
}

func resolveTaskFromTasks(pattern string, tasks []Task) (Task, error) {
	// Exact string match returns immediately.
	for _, task := range tasks {
		if task.ID == pattern {
			return task, nil
		}
	}
	// Exact numeric match: parsed numeric value + suffix equal.
	// This resolves "1" to "001" before falling through to prefix matching.
	patNum, patSuffix, patOk := splitTaskID(pattern)
	if patOk {
		for _, task := range tasks {
			taskNum, taskSuffix, taskOk := splitTaskID(task.ID)
			if taskOk && taskNum == patNum && taskSuffix == patSuffix {
				return task, nil
			}
		}
	}
	// Constrained prefix matching: parse the numeric prefix so that "1a"
	// matches "001a" and similar short forms match zero-padded task IDs.
	// If a prefix matches more than one task, the command reports ambiguity.
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
	tasks, err := a.getTasks()
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	return resolveTaskFromTasks(pattern, tasks)
}
