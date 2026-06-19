package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type taskCreateArgs struct {
	title       string
	priority    string
	effort      string
	labels      string
	status      string
	description string
	bodyFile    string
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
	// Strip any H1 matching the task title to avoid duplicates.
	// renderTask always emits the H1 from front matter.
	body = stripHeading(body, parsed.title)
	if !a.opts.dryRun {
		release, err := acquireWorkflowLock(a.opts.root, "task-create")
		if err != nil {
			return err
		}
		defer func() {
			if err := release(); err != nil {
				fmt.Fprintln(a.err, err)
			}
		}()
		return a.taskCreateParsedLocked(parsed, body)
	}
	return a.taskCreateParsedLocked(parsed, body)
}

func (a *app) taskCreateParsedLocked(parsed taskCreateArgs, body string) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
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
