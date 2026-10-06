package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
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
	body              string
	bodySet           bool // --body was supplied, so an empty value is an error rather than an omission
	bodyFile          string
	bodyFileSet       bool // --body-file was supplied, so an empty value is an error rather than an omission
	externalRef       string
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
	if strings.ContainsAny(parsed.externalRef, "\n\r") {
		return usageError("task create external reference must not contain newlines")
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
		if err := checkDuplicateTaskID(tasks, parsed.resolvedParentID, a.workflowPaths()); err != nil {
			return err
		}
	}
	var id string
	paths := a.workflowPaths()
	if parsed.resolvedParentID != "" {
		// Re-resolve parent inside the lock for consistency.
		// The parent is known to exist from the pre-lock check, but the ID
		// may have been zero-padded differently; use the resolved ID for child prefix.
		parentID := parsed.resolvedParentID
		id, err = nextChildTaskIDForPaths(tasks, paths, parentID)
		if err != nil {
			return err
		}
	} else {
		id, err = nextTaskIDForPaths(tasks, paths)
		if err != nil {
			return err
		}
	}
	a.warnDuplicateTitle(tasks, id, parsed.title)
	path := paths.taskFile("active", id)
	now := time.Now().Format(time.RFC3339)
	task := Task{
		ID:          id,
		Title:       parsed.title,
		Status:      parsed.status,
		Priority:    parsed.priority,
		Effort:      parsed.effort,
		Labels:      parsed.labels,
		ExternalRef: parsed.externalRef,
		Created:     now,
		Body:        body,
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
		payload := map[string]any{"create": paths.payloadPath(path), "id": id}
		if len(task.DependsOn) > 0 {
			payload["depends_on"] = task.DependsOn
		}
		return a.emit(payload)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("task id %s already exists at %s; retry task create", id, paths.displayPath(path))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking task path %s: %w", paths.displayPath(path), err)
	}
	if err := writeOwned(paths, path, []byte(content)); err != nil {
		return err
	}
	// The record is what proves the ID was used, so the marks move only once
	// the record exists. A failure between the two writes leaves a mark behind,
	// which the next allocation repairs from the records on disk; the reverse
	// order would burn an ID for a create that never happened.
	if err := persistTaskIDAllocation(paths, id); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, id)
	return nil
}

// warnDuplicateTitle reports each active task that already carries the title
// about to be created. Titles are the human handle for a task and the only
// field `task search` matches, so a duplicate splits one piece of work across
// two records a reader cannot tell apart.
//
// The comparison is exact and case-insensitive. Exact matching has no false
// positives and catches the failure that occurs, which is a title reproduced
// verbatim; a normalized-token similarity would need a threshold and a way to
// explain a near miss, which is a separate change. Completed and Cancelled
// records are skipped, because a recurring task legitimately reuses its title
// and reporting those matches would drown the useful case. Creation is never
// refused: a duplicate title is strong evidence of a mistake but is
// occasionally legitimate, so the warning names the colliding task and leaves
// the decision to the caller.
func (a *app) warnDuplicateTitle(tasks []Task, newID string, title string) {
	for _, task := range tasks {
		if task.Status == "Completed" || task.Status == "Cancelled" {
			continue
		}
		if strings.EqualFold(task.Title, title) {
			a.addWarning("task %s duplicates the title of active task %s [%s]: %q; comment on that task instead or make this title distinct", newID, task.ID, task.Status, task.Title)
		}
	}
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
	if err := checkTaskDepsNotDuplicated(tasks, canonical, a.workflowPaths()); err != nil {
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
// An inline --body or a --body-file provides the full content below the H1
// verbatim; otherwise a default Summary/Acceptance Notes scaffold is generated
// from the optional --description text.
//
// The --body and --body-file branches test presence rather than value, so an
// explicitly empty flag is refused instead of silently falling through to the
// scaffold and skipping the exclusivity checks below. A non-empty value counts
// as present on its own, so a caller that builds taskCreateArgs directly rather
// than through Cobra keeps working.
func (a *app) resolveTaskCreateBody(parsed taskCreateArgs) (string, error) {
	bodySet := parsed.bodySet || parsed.body != ""
	bodyFileSet := parsed.bodyFileSet || parsed.bodyFile != ""
	if bodySet && bodyFileSet {
		return "", usageError("task create supports --body or --body-file, not both")
	}
	if parsed.description != "" {
		if bodySet {
			return "", usageError("task create supports --body or --description, not both")
		}
		if bodyFileSet {
			return "", usageError("task create supports --body-file or --description, not both")
		}
	}
	if bodySet {
		body := strings.TrimSpace(strings.ReplaceAll(parsed.body, "\r\n", "\n"))
		if body == "" {
			return "", usageError("task create --body cannot be empty")
		}
		return body, nil
	}
	if bodyFileSet {
		if parsed.bodyFile == "" {
			return "", usageError("task create --body-file cannot be empty")
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
	body := parsed.description
	if body == "" {
		body = "TODO."
	}
	return "## Summary\n\n" + body + "\n\n## Acceptance Notes\n\n- [ ] TODO\n", nil
}

// nextTaskIDForPaths returns the next top-level task ID: the higher of one past
// the highest number in the records present and the store's persisted counter,
// so a store that predates the counter heals upward instead of reissuing a
// number.
func nextTaskIDForPaths(tasks []Task, paths workflowPaths) (string, error) {
	counter, err := readTaskIDCounter(paths)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%03d", max(counter, highestTaskNumber(tasks, paths)+1)), nil
}

// nextChildTaskIDForPaths returns the next child ID under the parent. Project
// mode keeps the letter scan: Git history there is the evidence that a deleted
// child's letter was spent, not something ahm reads before allocating, so the
// first free letter is the answer. A store has no history, so it follows the
// parent's persisted suffix mark, self-healed from the highest child letter
// present; a letter at or below the mark is spent even when its record is gone.
func nextChildTaskIDForPaths(tasks []Task, paths workflowPaths, parentID string) (string, error) {
	parentNum, _, ok := splitTaskID(parentID)
	if !ok {
		return "", fmt.Errorf("invalid parent task ID %q", parentID)
	}
	prefix := fmt.Sprintf("%03d", parentNum)

	used := map[string]bool{}
	highest := ""
	forEachRecordID(tasks, paths, func(id string) {
		n, suffix, ok := splitTaskID(id)
		if !ok || n != parentNum || suffix == "" {
			return
		}
		used[suffix] = true
		highest = higherChildSuffix(highest, suffix)
	})

	if paths.inStore() {
		mark, err := readChildSuffixMark(paths, prefix)
		if err != nil {
			return "", err
		}
		// Allocation always takes the first free letter, so the letters a
		// store has spent form a prefix a..highest: the letter after the
		// higher of the mark and the records present is both the next free
		// letter and one no record can claim.
		next := childSuffixAfter(higherChildSuffix(mark, highest))
		if next == "" {
			return "", fmt.Errorf("all 26 child task slots used for parent %q", parentID)
		}
		return prefix + next, nil
	}

	// Find the first unused letter a-z.
	for ch := 'a'; ch <= 'z'; ch++ {
		suffix := string(ch)
		if !used[suffix] {
			return prefix + suffix, nil
		}
	}

	return "", fmt.Errorf("all 26 child task slots used for parent %q", parentID)
}
