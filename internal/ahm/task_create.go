package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type taskCreateArgs struct {
	title            string
	priority         string
	effort           string
	labels           string
	status           string
	description      string
	bodyFile         string
	parent           string
	resolvedParentID string // set after parent validation, used inside locked section
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
	content := renderTask(task)
	if a.opts.dryRun {
		return a.emit(map[string]any{"create": path, "id": id})
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
