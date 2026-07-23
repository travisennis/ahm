package ahm

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type taskCommentArgs struct {
	id       string
	text     string
	author   string
	bodyFile string
}

// taskCommentCommand returns the cobra.Command for "task comment".
func (a *app) taskCommentCommand() *cobra.Command {
	flagArgs := taskCommentArgs{}
	cmd := &cobra.Command{
		Use:   "comment <id> <text>",
		Short: "Append a timestamped comment to a task",
		Long: `Append a lightweight timestamped comment to a task's ## Comments section.

Comments are stored in the task Markdown body and appear on all task
outputs including task show. Author is omitted by default and included
only when --author is provided.

Examples:
  ahm task comment 116 "Found the root cause"
  ahm task comment 116 --author "Travis" "Need to revisit this"
  ahm task comment 116 --body-file -`,
		Args: func(cmd *cobra.Command, positional []string) error {
			if len(positional) == 0 {
				return usageError("task comment requires an id and text\n  ahm task comment <id> <text>")
			}
			if len(positional) == 1 && flagArgs.bodyFile == "" {
				return usageError("task comment requires text or --body-file\n  ahm task comment <id> --body-file <file>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, positional []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			parsed := taskCommentArgs{
				id:       positional[0],
				author:   flagArgs.author,
				bodyFile: flagArgs.bodyFile,
			}
			if len(positional) > 1 {
				parsed.text = strings.Join(positional[1:], " ")
			}
			return a.taskComment(parsed)
		},
	}
	cmd.Flags().StringVar(&flagArgs.author, "author", "", "Optional author name for the comment")
	cmd.Flags().StringVar(&flagArgs.bodyFile, "body-file", "", "Read comment text from a file (or - for stdin)")
	return cmd
}

func (a *app) taskComment(parsed taskCommentArgs) error {
	defer a.emitWarnings()

	// Resolve the comment text.
	text, err := a.resolveCommentText(parsed)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return usageError("comment text cannot be empty")
	}

	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		return a.taskCommentLocked(parsed, text)
	})
}

func (a *app) taskCommentLocked(parsed taskCommentArgs, text string) error {
	// Re-resolve the task from fresh on-disk state under the lock so we write
	// to the current bucket/path, not a stale location.
	a.invalidateTasks()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	task, err := resolveTaskFromTasks(parsed.id, tasks)
	if err != nil {
		return err
	}
	if err := checkDuplicateTaskID(tasks, task.ID, a.opts.root); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)

	// Build the formatted comment line.
	comment := formatComment(now, parsed.author, text)

	// Append the comment to the body.
	task.Body = appendComment(task.Body, comment)

	// Update the task's updated timestamp.
	task.Updated = now

	if a.opts.dryRun {
		return a.emit(a.commentRecord(task, comment, parsed))
	}

	if err := writeFileAtomic(task.Path, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	a.invalidateTasks()

	// Text/plain output: concise summary. JSON output: structured record.
	if a.opts.json || a.opts.plain {
		return a.emit(a.commentRecord(task, comment, parsed))
	}
	fmt.Fprintf(a.out, "%s\n", task.ID)
	return nil
}

// resolveCommentText resolves the comment text from positional args or --body-file.
func (a *app) resolveCommentText(parsed taskCommentArgs) (string, error) {
	if parsed.bodyFile == "" {
		return parsed.text, nil
	}
	if parsed.text != "" {
		return "", usageError("task comment accepts text or --body-file, not both")
	}
	var (
		data   []byte
		err    error
		source string
	)
	if parsed.bodyFile == "-" {
		source = "stdin"
		if a.in == nil {
			return "", usageError("task comment --body-file - requires stdin")
		}
		data, err = io.ReadAll(a.in)
	} else {
		source = parsed.bodyFile
		data, err = os.ReadFile(parsed.bodyFile)
	}
	if err != nil {
		return "", fmt.Errorf("reading comment text from %s: %w", source, err)
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")), nil
}

// formatComment returns a single comment line in the stable Markdown format.
func formatComment(timestamp string, author string, text string) string {
	if author != "" {
		return fmt.Sprintf("**%s** — _%s_: %s", timestamp, author, text)
	}
	return fmt.Sprintf("**%s** — %s", timestamp, text)
}

// appendComment appends a comment line to the ## Comments section of a task body.
// If no ## Comments section exists, it creates one at the end.
func appendComment(body string, comment string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		level := headingLevel(trimmed)
		if level == 2 && strings.EqualFold(strings.TrimSpace(trimmed[level:]), "Comments") {
			// Found existing ## Comments section. Find where it ends (next
			// heading at level 2 or above, or end of body).
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				nextLevel := headingLevel(strings.TrimSpace(lines[j]))
				if nextLevel > 0 && nextLevel <= 2 {
					end = j
					break
				}
			}
			// Rebuild: everything before the heading + heading + existing content
			// (trimmed trailing blank lines) + new comment + everything after.
			var updated []string
			updated = append(updated, lines[:i]...)
			updated = append(updated, trimmed)

			// Copy existing content between the heading and end, stripping trailing
			// blank lines from the section content.
			existing := lines[i+1 : end]
			existing = trimTrailingBlankLines(existing)
			updated = append(updated, existing...)

			// Separate existing content from new comment with a blank line.
			if len(existing) > 0 {
				updated = append(updated, "")
			}
			updated = append(updated, comment)
			updated = append(updated, lines[end:]...)
			return strings.TrimSpace(strings.Join(updated, "\n"))
		}
	}

	// No ## Comments section found; create one at the end.
	body = strings.TrimSpace(body)
	if body == "" {
		return "## Comments\n\n" + comment
	}
	return body + "\n\n## Comments\n\n" + comment
}

// trimTrailingBlankLines removes trailing empty or whitespace-only lines.
func trimTrailingBlankLines(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[:end]
}

// commentRecord returns the structured record for JSON output.
func (a *app) commentRecord(task Task, comment string, parsed taskCommentArgs) map[string]any {
	r := map[string]any{
		"id":   task.ID,
		"path": relPath(a.opts.root, task.Path),
		"text": comment,
	}
	if parsed.author != "" {
		r["author"] = parsed.author
	}
	return r
}
