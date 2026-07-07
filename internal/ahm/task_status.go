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

func (a *app) taskStatus(argv []string, status string) error {
	return a.taskStatusWithArgs(taskStatusArgs{ids: argv, status: status})
}

func (a *app) taskStatusWithArgs(parsed taskStatusArgs) error {
	task, err := a.resolveTask(parsed.ids[0])
	if err != nil {
		return err
	}
	cancelReason := strings.TrimSpace(parsed.reason)
	if parsed.status == "Cancelled" && cancelReason == "" {
		return usageError("task cancel requires --reason\n  ahm task cancel <id> --reason <text>")
	}

	// True no-op: status and bucket both match. Cancellation still rewrites the
	// task so the required reason can be inserted or replaced.
	expectedBucket := bucketForStatus(parsed.status)
	if task.Status == parsed.status && task.Bucket == expectedBucket && parsed.status != "Cancelled" {
		fmt.Fprintf(a.out, "%s already %s\n", task.ID, parsed.status)
		return nil
	}

	if !a.opts.dryRun {
		release, err := acquireWorkflowLock(a.opts.root, "task-mutate")
		if err != nil {
			return err
		}
		defer func() {
			if err := release(); err != nil {
				fmt.Fprintln(a.err, err)
			}
		}()
		return a.taskStatusWithArgsLocked(parsed, task, cancelReason)
	}
	return a.taskStatusWithArgsLocked(parsed, task, cancelReason)
}

func (a *app) taskStatusWithArgsLocked(parsed taskStatusArgs, task Task, cancelReason string) error {
	defer a.emitWarnings()
	// Invalidate the task cache so the locked section reads fresh state from
	// disk. resolveTask outside the lock may have populated the cache before
	// the lock was acquired, and concurrent completions may have changed the
	// on-disk state since then.
	a.invalidateTasks()
	status := parsed.status

	var allTasks []Task
	var allTasksLoaded bool
	loadTasks := func() []Task {
		if allTasksLoaded {
			return allTasks
		}
		allTasksLoaded = true
		tasks, collErr := a.getTasks()
		if collErr != nil {
			a.addWarning("some task files could not be parsed and were skipped")
		}
		allTasks = tasks
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
	}
	if status == "Cancelled" {
		a.warnCancellationAcceptancePlaceholder(task)
		task.Body = upsertCancellationReason(task.Body, cancelReason)
	}

	now := time.Now().Format(time.RFC3339)
	task.Status = status
	task.Updated = now
	bucket := bucketForStatus(status)
	target := workflowPathsFor(a.opts.root).taskFile(bucket, task.ID)
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
	if err := writeFileAtomic(target, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if filepath.Clean(task.Path) != filepath.Clean(target) {
		if err := os.Remove(task.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, utask := range unblocked {
		if err := writeFileAtomic(utask.Path, []byte(renderTask(utask)), 0o644); err != nil {
			return err
		}
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", task.ID, status)
	for _, utask := range unblocked {
		fmt.Fprintf(a.out, "%s -> Pending\n", utask.ID)
	}
	return nil
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
