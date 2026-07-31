package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type taskCreateArgs struct {
	title             string
	priority          string
	effort            string
	labels            string
	status            string
	description       string
	bodyFile          string
	parent            string
	dependsOn         string
	resolvedParentID  string   // set after parent validation, used inside locked section
	resolvedDependsOn []string // parsed --depends-on IDs, validated inside locked section
}

func (a *app) taskCreateParsed(parsed taskCreateArgs) error {
	if parsed.title == "" {
		return usageError("task create requires a title\n  ahm task create <title>")
	}
	if strings.TrimSpace(parsed.title) != parsed.title {
		return usageError("task create title must not have leading or trailing whitespace")
	}
	if strings.TrimSpace(parsed.labels) != parsed.labels {
		return usageError("task create labels must not have leading or trailing whitespace")
	}
	if strings.ContainsAny(parsed.title, "\n\r") {
		return usageError("task create title must not contain newlines")
	}
	if strings.ContainsAny(parsed.labels, "\n\r") {
		return usageError("task create labels must not contain newlines")
	}
	if parsed.labels == "" {
		parsed.labels = "-"
	}
	if err := validateTaskCreateEnums(parsed); err != nil {
		return err
	}
	if parsed.dependsOn != "" {
		deps, err := parseTaskDependsOn(parsed.dependsOn)
		if err != nil {
			return usageError(err.Error())
		}
		parsed.resolvedDependsOn = deps
	}
	if parsed.parent != "" {
		// Resolve parent upfront for fast validation (read-only, no lock needed).
		// Re-resolution inside the locked section uses the stored resolved ID.
		parent, err := a.resolveTaskForMutation(parsed.parent)
		if err != nil {
			return usageError(fmt.Sprintf("parent task %q: %s", parsed.parent, err))
		}
		_, suffix, ok := splitTaskID(parent.ID)
		if ok && suffix != "" {
			return usageError(fmt.Sprintf("parent task %q is a child task; only top-level tasks can be parents", parsed.parent))
		}
		parsed.resolvedParentID = parent.ID
	}
	body, err := a.resolveTaskCreateBody(parsed)
	if err != nil {
		return err
	}
	// Strip any H1 matching the task title to avoid duplicates.
	// renderTask always emits the H1 from front matter.
	body = stripHeading(body, parsed.title)
	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		return a.taskCreateParsedLocked(parsed, body)
	})
}

func (a *app) taskCreateParsedLocked(parsed taskCreateArgs, body string) error {
	defer a.emitWarnings()
	a.invalidateTasks()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	if parsed.resolvedParentID != "" {
		if err := checkDuplicateTaskID(tasks, parsed.resolvedParentID, a.opts.root); err != nil {
			return err
		}
	}
	var id string
	if parsed.resolvedParentID != "" {
		// Re-resolve parent inside the lock for consistency.
		// The parent is known to exist from the pre-lock check, but the ID
		// may have been zero-padded differently; use the resolved ID for child prefix.
		parentID := parsed.resolvedParentID
		id, err = nextChildTaskIDForPaths(tasks, a.workflowPaths(), parentID)
		if err != nil {
			return err
		}
	} else {
		id = nextTaskIDForPaths(tasks, a.workflowPaths())
	}
	path := a.workflowPaths().taskFile("active", id)
	now := time.Now().Format(time.RFC3339)
	task := Task{
		ID:       id,
		Title:    parsed.title,
		Status:   parsed.status,
		Priority: parsed.priority,
		Effort:   parsed.effort,
		Labels:   parsed.labels,
		ExecPlan: "-",
		Created:  now,
		Body:     body,
	}
	if parsed.resolvedParentID != "" {
		task.Parent = parsed.resolvedParentID
	}
	if len(parsed.resolvedDependsOn) > 0 {
		deps, err := a.validateTaskCreateDeps(tasks, task, parsed.resolvedDependsOn)
		if err != nil {
			return err
		}
		task.DependsOn = deps
	}
	content := renderTask(task)
	if a.opts.dryRun {
		payload := map[string]any{"create": filepath.ToSlash(path), "id": id}
		if len(task.DependsOn) > 0 {
			payload["depends_on"] = task.DependsOn
		}
		return a.emit(payload)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("task id %s already exists at %s; retry task create", id, relPath(a.opts.root, path))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking task path %s: %w", relPath(a.opts.root, path), err)
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

// parseTaskDependsOn parses the --depends-on flag value into task IDs. An
// empty value means no dependencies. Comma-separated parts are trimmed; an
// empty part is an error rather than being silently dropped.
func parseTaskDependsOn(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("--depends-on must be a comma-separated list of task IDs")
	}
	parts := strings.Split(value, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, fmt.Errorf("--depends-on must be a comma-separated list of task IDs")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// validateTaskCreateDeps validates the --depends-on patterns against the tasks
// read under the workflow lock, after the new task ID has been allocated. It
// rejects missing, ambiguous, duplicated, Completed, Cancelled, and
// self-referential dependencies, verifies the new dependency set introduces no
// cycle, and returns the canonical (zero-padded) dependency IDs sorted by
// taskLess.
func (a *app) validateTaskCreateDeps(tasks []Task, task Task, patterns []string) ([]string, error) {
	deps := make([]string, 0, len(patterns))
	seen := make(map[string]bool, len(patterns))
	for _, pattern := range patterns {
		dep, err := resolveTaskFromTasks(pattern, tasks)
		if err != nil {
			// A pattern matching the ID about to be allocated is a
			// self-reference: the dependency does not exist yet, so resolution
			// reports it as missing. Surface it as the cycle it actually is. An
			// ambiguity error keeps its own message: the pattern matches
			// existing tasks, not the one being created.
			if sameTaskID(pattern, task.ID) && isTaskNotFoundError(err) {
				return nil, usageError(fmt.Sprintf("task %s cannot depend on itself", task.ID))
			}
			return nil, usageError(fmt.Sprintf("dependency task %q: %s", pattern, err))
		}
		// Defensive: the allocated ID is guaranteed free because ID allocation
		// scans parsed tasks and task files, so resolution cannot return it.
		if dep.ID == task.ID {
			return nil, usageError(fmt.Sprintf("task %s cannot depend on itself", task.ID))
		}
		switch dep.Status {
		case "Completed":
			return nil, usageError(fmt.Sprintf("task %s cannot depend on completed task %s", task.ID, dep.ID))
		case "Cancelled":
			return nil, usageError(fmt.Sprintf("task %s cannot depend on cancelled task %s", task.ID, dep.ID))
		}
		if seen[dep.ID] {
			continue
		}
		seen[dep.ID] = true
		deps = append(deps, dep.ID)
	}
	sort.Slice(deps, func(i, j int) bool { return taskLess(deps[i], deps[j]) })

	canonical := task
	canonical.DependsOn = deps
	if err := checkTaskDepsNotDuplicated(tasks, canonical, a.opts.root); err != nil {
		return nil, err
	}
	// Simulate the new task in the dependency graph. A cycle arises when the
	// new task depends on itself (rejected above) or on a task whose depends_on
	// already references the ID about to be allocated; this guards the same
	// invariant as `task dep add`.
	simulated := make([]Task, 0, len(tasks)+1)
	simulated = append(simulated, tasks...)
	simulated = append(simulated, canonical)
	if cycles := taskDependencyCycles(simulated); len(cycles) > 0 {
		return nil, usageError(fmt.Sprintf("adding dependencies to task %s would create a cycle: %s", task.ID, strings.Join(cycles[0], " -> ")))
	}
	return deps, nil
}

// sameTaskID reports whether the two patterns denote the same task ID,
// comparing numeric value and optional letter suffix.
func sameTaskID(a string, b string) bool {
	an, as, aok := splitTaskID(a)
	bn, bs, bok := splitTaskID(b)
	return aok && bok && an == bn && as == bs
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

func nextTaskIDForPaths(tasks []Task, paths workflowPaths) string {
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
		dir := paths.tasksBucketDir(bucket)
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

func nextChildTaskIDForPaths(tasks []Task, paths workflowPaths, parentID string) (string, error) {
	parentNum, _, ok := splitTaskID(parentID)
	if !ok {
		return "", fmt.Errorf("invalid parent task ID %q", parentID)
	}

	used := map[string]bool{}

	// Check parsed tasks (including active, completed, cancelled).
	for _, task := range tasks {
		n, suffix, ok := splitTaskID(task.ID)
		if ok && n == parentNum && suffix != "" {
			used[suffix] = true
		}
	}

	// Also scan the filesystem for unparsed files that may have been skipped.
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := paths.tasksBucketDir(bucket)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			n, suffix, ok := splitTaskID(strings.TrimSuffix(entry.Name(), ".md"))
			if ok && n == parentNum && suffix != "" {
				used[suffix] = true
			}
		}
	}

	// Find the first unused letter a-z.
	for ch := 'a'; ch <= 'z'; ch++ {
		suffix := string(ch)
		if !used[suffix] {
			prefix := fmt.Sprintf("%03d", parentNum)
			return prefix + suffix, nil
		}
	}

	return "", fmt.Errorf("all 26 child task slots used for parent %q", parentID)
}
