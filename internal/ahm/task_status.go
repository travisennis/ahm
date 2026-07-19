package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type taskStatusArgs struct {
	ids    []string
	status string
	reason string
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

// taskStatusPreLockHook is called in taskStatusWithArgs after the initial
// task resolution and before the mutation lock is acquired. It is used by
// tests to establish a deterministic ordering with concurrent updates.
var taskStatusPreLockHook = func() {}

func (a *app) taskStatus(argv []string, status string) error {
	return a.taskStatusWithArgs(taskStatusArgs{ids: argv, status: status})
}

func (a *app) taskStatusWithArgs(parsed taskStatusArgs) error {
	// Resolve the task early so we can report "not found" before trying to
	// acquire the mutation lock. Parse warnings are deferred to the fresh
	// resolution under the lock so they are emitted only once.
	tasks, _ := a.getTasks()
	if _, err := resolveTaskFromTasks(parsed.ids[0], tasks); err != nil {
		return err
	}
	cancelReason := strings.TrimSpace(parsed.reason)
	if parsed.status == "Cancelled" && cancelReason == "" {
		return usageError("task cancel requires --reason\n  ahm task cancel <id> --reason <text>")
	}

	taskStatusPreLockHook()

	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		// Re-resolve from fresh on-disk state (under the lock when mutating) so
		// that any concurrent updates that landed before lock acquisition are
		// preserved instead of overwritten by a pre-lock Task value.
		a.invalidateTasks()
		task, err := a.resolveTask(parsed.ids[0])
		if err != nil {
			return err
		}
		return a.taskStatusWithArgsLocked(parsed, task, cancelReason)
	})
}

func (a *app) taskStatusWithArgsLocked(parsed taskStatusArgs, task Task, cancelReason string) error {
	defer a.emitWarnings()
	status := parsed.status

	// True no-op: status and bucket both match. Cancellation still rewrites the
	// task so the required reason can be inserted or replaced.
	expectedBucket := bucketForStatus(parsed.status)
	if task.Status == parsed.status && task.Bucket == expectedBucket && parsed.status != "Cancelled" {
		fmt.Fprintf(a.out, "%s already %s\n", task.ID, parsed.status)
		return nil
	}

	var allTasks []Task
	var allTasksErr error
	var allTasksLoaded bool
	loadTasks := func() []Task {
		if allTasksLoaded {
			return allTasks
		}
		allTasksLoaded = true
		allTasks, allTasksErr = a.getTasks()
		if allTasksErr != nil {
			a.addWarning("some task files could not be parsed and were skipped")
		}
		return allTasks
	}

	// Enforce dependency completion before completing a task,
	// but only when the status is actually changing.
	if task.Status != status && status == "Completed" && len(task.DependsOn) > 0 {
		completed := map[string]bool{}
		for _, t := range loadTasks() {
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

	// Run acceptance notes validation only when actually transitioning to Completed.
	if task.Status != status && status == "Completed" {
		findings := parseAcceptanceNotes([]byte(task.Body))
		for _, finding := range findings {
			a.addWarning("%s", finding.message(task.ID))
		}
		if len(findings) > 0 && !a.opts.force {
			meta, err := readMetadata(a.opts.root)
			switch {
			case errors.Is(err, os.ErrNotExist):
				// No metadata, strict acceptance not configured.
			case err != nil:
				a.addWarning("%s, strict acceptance disabled", metadataCorruptMessage(err))
			case meta.StrictAcceptance:
				return fmt.Errorf("cannot complete task %s: acceptance notes are incomplete; use --force to override", task.ID)
			}
		}

		// Warn when completing a task linked to an active ExecPlan.
		if task.ExecPlan != "" && task.ExecPlan != "-" {
			paths := a.workflowPaths()
			planPath, bucket, ok := resolveExecPlanReference(paths, task.ExecPlan)
			if ok && bucket == "active" {
				relPlan := relPath(a.opts.root, planPath)
				msg := fmt.Sprintf("task %s references active ExecPlan %s; move it to the completed ExecPlan bucket and update the task exec_plan field", task.ID, relPlan)
				if sections, err := parseExecPlanSections(planPath); err == nil {
					outcomes := sections[normalizedExecPlanSection("Outcomes & Retrospective")]
					if execPlanSectionHasBody(outcomes) {
						msg += " (the ExecPlan already has a filled Outcomes & Retrospective section)"
					}
				}
				a.addWarning("%s", msg)
			}
		}
	}
	if status == "Cancelled" {
		a.warnCancellationAcceptancePlaceholder(task)
		task.Body = upsertCancellationReason(task.Body, cancelReason)
	}

	now := time.Now().Format(time.RFC3339)
	task.Status = status
	task.Updated = now
	bucket := bucketForStatus(status)
	target := a.workflowPaths().taskFile(bucket, task.ID)
	var unblocked []Task
	if status == "Completed" {
		unblocked = a.taskUnblockDependents(loadTasks(), task.ID, now)
	}
	if a.opts.dryRun {
		preview := map[string]any{"move": target, "status": status}
		if status == "Cancelled" {
			preview["reason"] = cancelReason
		}
		if len(unblocked) > 0 {
			preview["unblocked"] = taskUnblockPreview(unblocked)
		}
		return a.emit(preview)
	}
	renderedTask := []byte(renderTask(task))
	refreshedTask, err := parseTaskFromData(renderedTask, target, bucket)
	if err != nil {
		return fmt.Errorf("reparse updated task %s: %w", task.ID, err)
	}
	if err := writeFileAtomic(target, renderedTask, 0o644); err != nil {
		return err
	}
	if filepath.Clean(task.Path) != filepath.Clean(target) {
		if err := os.Remove(task.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	refreshedUnblocked := make([]Task, 0, len(unblocked))
	for _, utask := range unblocked {
		rendered := []byte(renderTask(utask))
		refreshed, err := parseTaskFromData(rendered, utask.Path, utask.Bucket)
		if err != nil {
			return fmt.Errorf("reparse unblocked task %s: %w", utask.ID, err)
		}
		if err := writeFileAtomic(utask.Path, rendered, 0o644); err != nil {
			return err
		}
		refreshedUnblocked = append(refreshedUnblocked, refreshed)
	}
	currentTasks := loadTasks()
	if allTasksErr == nil {
		currentTasks = replaceTaskStates(currentTasks, append([]Task{refreshedTask}, refreshedUnblocked...))
		err = a.writeIndexesForTasks(currentTasks, true)
	} else {
		err = a.writeIndexes()
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", task.ID, status)
	for _, utask := range unblocked {
		fmt.Fprintf(a.out, "%s -> Pending\n", utask.ID)
	}
	return nil
}

func replaceTaskStates(tasks, updates []Task) []Task {
	byID := make(map[string]Task, len(updates))
	for _, task := range updates {
		byID[task.ID] = task
	}
	result := make([]Task, len(tasks))
	for i, task := range tasks {
		if updated, ok := byID[task.ID]; ok {
			result[i] = updated
			continue
		}
		result[i] = task
	}
	return result
}

func (a *app) taskUnblockDependents(tasks []Task, completedID string, updated string) []Task {
	completed := map[string]bool{completedID: true}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	var unblocked []Task
	for _, task := range tasks {
		if task.Bucket != "active" || task.Status != "Blocked" || !taskDependsOn(task, completedID) {
			continue
		}
		if !depsComplete(task, completed) {
			continue
		}
		task.Status = "Pending"
		task.Updated = updated
		unblocked = append(unblocked, task)
	}
	return unblocked
}

func taskDependsOn(task Task, depID string) bool {
	for _, dep := range task.DependsOn {
		if dep == depID {
			return true
		}
	}
	return false
}

func taskUnblockPreview(tasks []Task) []map[string]any {
	preview := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		preview = append(preview, map[string]any{
			"id":     task.ID,
			"path":   task.Path,
			"status": "Pending",
		})
	}
	return preview
}

func (a *app) warnCancellationAcceptancePlaceholder(task Task) {
	for _, finding := range parseAcceptanceNotes([]byte(task.Body)) {
		if finding == taskAcceptancePlaceholder {
			a.addWarning("%s", finding.message(task.ID))
		}
	}
}

func upsertCancellationReason(body string, reason string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	sections := locateHeadingSections(lines, []string{"Cancellation Reason"})
	if len(sections) > 0 {
		// Preserve the established first-match behavior for repeated headings.
		section := sections[0]
		trimmedLine := strings.TrimSpace(lines[section.Start])
		replacement := []string{trimmedLine, "", reason}
		if section.End < len(lines) {
			replacement = append(replacement, "")
		}
		updated := append([]string{}, lines[:section.Start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[section.End:]...)
		return strings.TrimSpace(strings.Join(updated, "\n"))
	}
	body = strings.TrimSpace(body)
	section := "## Cancellation Reason\n\n" + reason
	if body == "" {
		return section
	}
	return body + "\n\n" + section
}
