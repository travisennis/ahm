package ahm

import (
	"bytes"
	"encoding/json"
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

const codexBypassApprovalsAndSandboxFlag = "--dangerously-bypass-approvals-and-sandbox"

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
		Use:   "labels",
		Short: "List task labels",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskLabels()
		},
	})
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
	work.Flags().StringVar(&workArgs.agent, "agent", "", "Agent to run: cake, claude, codex, or cursor")
	work.Flags().BoolVar(&workArgs.review, "review", false, "Run review orchestration after work session")
	work.Flags().BoolVar(&workArgs.complete, "complete", false, "Run completion handoff after work session")
	work.Flags().BoolVar(&workArgs.commit, "commit", false, "Run commit handoff after work session")
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
	var statuses []string
	var labels []string
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskList(mode, statuses, labels)
		},
	}
	if mode == "all" {
		cmd.Flags().StringSliceVar(&statuses, "status", nil, "Filter tasks by status (comma-separated or repeatable)")
	}
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Filter tasks by label; all labels must match (comma-separated or repeatable)")
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
	id       string
	agent    string
	review   bool
	complete bool
	commit   bool
}

type taskStatusArgs struct {
	ids    []string
	status string
	reason string
}

type taskWorkAgent struct {
	name                string
	executable          string
	args                func(string) []string
	supportsSessions    bool
	resumeArgs          func(string, string) []string // sessionID, prompt
	parseSessionID      func([]byte) (string, error)
	supportsReview      bool
	reviewArgs          func(string) []string // prompt
	parseReviewFeedback func([]byte) (string, error)
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
		preview := map[string]any{
			"task":       task.ID,
			"agent":      agent.name,
			"executable": executable,
			"args":       args,
			"status":     taskWorkDryRunStatus(task.Status),
		}
		if parsed.complete {
			preview["complete"] = true
		}
		if parsed.commit {
			preview["commit"] = true
		}
		if parsed.review {
			preview["review"] = true
		}
		return a.emit(preview)
	}
	if task.Status == "Pending" {
		if err := a.markTaskInProgress(task); err != nil {
			return err
		}
	}
	if parsed.review && !agent.supportsReview {
		fmt.Fprintf(a.err, "warning: --review is not supported by agent %s; review will not run\n", agent.name)
	}
	if parsed.complete && !agent.supportsSessions {
		fmt.Fprintf(a.err, "warning: --complete is not supported by agent %s; completion handoff will not run\n", agent.name)
	}
	if parsed.commit && !agent.supportsSessions {
		fmt.Fprintf(a.err, "warning: --commit is not supported by agent %s; commit handoff will not run\n", agent.name)
	}
	if agent.supportsSessions {
		return a.taskWorkWithSession(agent, executable, args, parsed.review, parsed.complete, parsed.commit, task.ID)
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
				return []string{"--output-format", "stream-json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       cakeResumeArgs,
			parseSessionID:   parseCakeSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"--no-session", "--skills", "preflight", "--output-format", "stream-json", prompt}
			},
			parseReviewFeedback: parseCakeReviewFeedback,
		}, nil
	case "codex":
		return taskWorkAgent{
			name:       "codex",
			executable: "codex",
			args: func(prompt string) []string {
				return []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       codexResumeArgs,
			parseSessionID:   parseCodexSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json", prompt}
			},
			parseReviewFeedback: parseCodexReviewFeedback,
		}, nil
	case "cursor", "cursoragent":
		return taskWorkAgent{
			name:       "cursor",
			executable: "cursor-agent",
			args: func(prompt string) []string {
				return []string{"-p", "--output-format", "stream-json", "--trust", prompt}
			},
			supportsSessions: true,
			resumeArgs:       cursorResumeArgs,
			parseSessionID:   parseCursorSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"-p", "--output-format", "stream-json", "--mode", "ask", "--trust", prompt}
			},
			parseReviewFeedback: parseCursorReviewFeedback,
		}, nil
	case "claude", "claudecode":
		return taskWorkAgent{
			name:       "claude",
			executable: "claude",
			args: func(prompt string) []string {
				return []string{"-p", "--verbose", "--output-format", "stream-json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       claudeResumeArgs,
			parseSessionID:   parseClaudeSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"-p", "--verbose", "--output-format", "stream-json", prompt}
			},
			parseReviewFeedback: parseClaudeReviewFeedback,
		}, nil
	default:
		return taskWorkAgent{}, usageError(fmt.Sprintf("unsupported task work agent %q (supported: cake, claude, codex, cursor)", value))
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
	return fmt.Sprintf(`Work on task %s.

Before making changes, read AGENTS.md and .agents/TASKS.md, then run ahm task show %s to inspect the task.

Use the repository task workflow. Keep changes scoped to the task. Fill the task Acceptance Notes when the work is done, run the required verification, and mark the task complete with ahm when acceptance is satisfied. Do not commit or push unless the user explicitly asked for that.
`, task.ID, task.ID)
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

// taskWorkWithSession runs a session-capable agent, captures its stdout,
// parses the session ID from the response, and tees the full output to the
// user's terminal. The session ID is kept in memory for the current
// orchestration run and is available for later review, completion, and commit
// handoff steps.
func (a *app) taskWorkWithSession(agent taskWorkAgent, executable string, args []string, review bool, complete bool, commit bool, taskID string) error {
	var stdoutBuf bytes.Buffer
	// Write captured output to both the user's terminal and the buffer.
	out := io.MultiWriter(a.out, &stdoutBuf)
	if err := taskWorkRunCommand(a.opts.root, executable, args, a.in, out, a.err); err != nil {
		return err
	}
	sessionID, parseErr := agent.parseSessionID(stdoutBuf.Bytes())
	if parseErr != nil {
		if commit {
			return fmt.Errorf("cannot run commit handoff: could not capture session ID from %s output: %w", agent.name, parseErr)
		}
		fmt.Fprintf(a.err, "warning: could not capture session ID from %s output: %v\n", agent.name, parseErr)
		return nil
	}
	if sessionID == "" {
		if commit {
			return fmt.Errorf("cannot run commit handoff: no session ID returned by %s", agent.name)
		}
		fmt.Fprintln(a.err, "warning: no session ID returned by", agent.name)
		return nil
	}

	fmt.Fprintf(a.err, "%s session started: %s\n", agent.name, truncatedID(sessionID, 8))

	if review && agent.supportsReview {
		if err := a.runReview(agent, executable, sessionID); err != nil {
			return err
		}
	}

	if complete {
		if err := a.runCompletion(agent, executable, sessionID, taskID); err != nil {
			return err
		}
	}
	if commit {
		return a.runCommit(agent, executable, sessionID, taskID)
	}
	return nil
}

// taskWorkReviewPrompt is the prompt runReview sends to every review-capable
// agent. It asks the agent to run the repo-owned preflight review skill against
// current uncommitted changes. The fixture capture script
// (scripts/capture-agent-fixtures.sh) replays the same prompt so the committed
// review golden reflects a real review run;
// TestCaptureScriptUsesReviewPrompt keeps the two in sync.
const taskWorkReviewPrompt = "Run the preflight skill on the current uncommitted changes."

// runReview runs an independent review pass using the agent's review
// capability, then feeds actionable feedback back into the original work
// session. If the review produces no feedback, the feedback-resume step is
// skipped. If the review command itself fails, the error is surfaced.
func (a *app) runReview(agent taskWorkAgent, executable, sessionID string) error {
	fmt.Fprintln(a.err, "--- Running review ---")

	reviewArgs := agent.reviewArgs(taskWorkReviewPrompt)

	var reviewBuf bytes.Buffer
	reviewOut := io.MultiWriter(a.out, &reviewBuf)

	if err := taskWorkRunCommand(a.opts.root, executable, reviewArgs, nil, reviewOut, a.err); err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	feedback, parseErr := agent.parseReviewFeedback(reviewBuf.Bytes())
	if parseErr != nil {
		fmt.Fprintf(a.err, "warning: could not parse review feedback from %s output: %v\n", agent.name, parseErr)
		return nil
	}

	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		fmt.Fprintln(a.err, "No review feedback to address, skipping feedback step.")
		return nil
	}

	fmt.Fprintf(a.err, "Review produced feedback, applying to session %s...\n", truncatedID(sessionID, 8))
	resumePrompt := fmt.Sprintf("Please address the following review feedback:\n\n%s", feedback)
	resumeArgs := agent.resumeArgs(sessionID, resumePrompt)
	return taskWorkRunCommand(a.opts.root, executable, resumeArgs, a.in, a.out, a.err)
}

// runCompletion resumes the agent session with a completion handoff prompt,
// asking the delegated agent to fill acceptance notes, run verification, and
// mark the task completed through ahm.
func (a *app) runCompletion(agent taskWorkAgent, executable, sessionID, taskID string) error {
	fmt.Fprintln(a.err, "--- Running completion handoff ---")
	prompt := a.buildTaskWorkCompletionPrompt(taskID)
	resumeArgs := agent.resumeArgs(sessionID, prompt)
	if err := taskWorkRunCommand(a.opts.root, executable, resumeArgs, a.in, a.out, a.err); err != nil {
		return fmt.Errorf("completion handoff failed: %w", err)
	}
	return nil
}

// buildTaskWorkCompletionPrompt returns the prompt used to ask the delegated
// agent to complete a task. The agent is expected to fill acceptance notes,
// run verification, and run "ahm task complete <id>" when satisfied.
func (a *app) buildTaskWorkCompletionPrompt(taskID string) string {
	return fmt.Sprintf(`Complete task %s.

Fill the task Acceptance Notes, run the required verification (such as "just ci"), and mark the task completed with ahm when acceptance is satisfied:

  ahm task complete %s

If verification fails, address the findings and retry. Do not commit or push unless the user explicitly asked for that.`, taskID, taskID)
}

// runCommit resumes the agent session with a commit handoff prompt. The
// delegated agent owns the actual git operation; ahm only sends the prompt.
func (a *app) runCommit(agent taskWorkAgent, executable, sessionID, taskID string) error {
	fmt.Fprintln(a.err, "--- Running commit handoff ---")
	prompt := a.buildTaskWorkCommitPrompt(taskID)
	resumeArgs := agent.resumeArgs(sessionID, prompt)
	if err := taskWorkRunCommand(a.opts.root, executable, resumeArgs, a.in, a.out, a.err); err != nil {
		return fmt.Errorf("commit handoff failed: %w", err)
	}
	return nil
}

// buildTaskWorkCommitPrompt returns the prompt used to ask the delegated agent
// to commit completed task work. Commit message policy remains project-owned.
func (a *app) buildTaskWorkCommitPrompt(taskID string) string {
	return fmt.Sprintf(`Commit the completed work for task %s.

Make sure the task is marked completed before committing. Include both task files and project source files in a single commit.

Do not push or open a pull request.`, taskID)
}

func truncatedID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// cakeStreamEvent represents a single line in cake's stream-json output.
// Records are serde-tagged with a top-level "type" field; task_complete
// flattens the outcome, so "result" appears at the top level too.
type cakeStreamEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseCakeSessionID parses cake's stream-json output and returns the
// session_id from the first task_start event.
func parseCakeSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cakeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "task_start" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	return "", nil
}

// cakeResumeArgs constructs the arguments to resume a cake session with a
// follow-up prompt.
func cakeResumeArgs(sessionID, prompt string) []string {
	return []string{"--resume", sessionID, "--output-format", "stream-json", prompt}
}

// parseCakeReviewFeedback parses cake's stream-json output and returns the
// result field from the final task_complete event, which contains the review
// feedback from a preflight or other review run.
func parseCakeReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cakeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "task_complete" {
			lastResult = evt.Result
		}
	}
	return lastResult, nil
}

// codexStreamEvent represents a single JSONL event in codex's --json output.
type codexStreamEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Item     *codexItemEvent `json:"item,omitempty"`
}

type codexItemEvent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// parseCodexSessionID parses codex JSONL output and returns the thread_id
// from the first thread.started event.
func parseCodexSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt codexStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "thread.started" && evt.ThreadID != "" {
			return evt.ThreadID, nil
		}
	}
	return "", nil
}

// codexResumeArgs constructs the arguments to resume a codex session with a
// follow-up prompt.
func codexResumeArgs(sessionID, prompt string) []string {
	return []string{"exec", "resume", codexBypassApprovalsAndSandboxFlag, "--json", sessionID, prompt}
}

// parseCodexReviewFeedback parses codex JSONL output and returns the
// concatenated text from all agent_message item.completed events, which
// contains the preflight review feedback.
func parseCodexReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var texts []string
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt codexStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "item.completed" && evt.Item != nil && evt.Item.Text != "" {
			texts = append(texts, evt.Item.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// cursorStreamEvent represents a single JSONL event in cursor-agent
// stream-json output.
type cursorStreamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseCursorSessionID parses cursor-agent stream-json output and returns the
// session_id from the first system/init event.
func parseCursorSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cursorStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "system" && evt.Subtype == "init" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	return "", nil
}

// cursorResumeArgs constructs the arguments to resume a cursor-agent session
// with a follow-up prompt.
func cursorResumeArgs(sessionID, prompt string) []string {
	return []string{"-p", "--output-format", "stream-json", "--trust", "--resume", sessionID, prompt}
}

// parseCursorReviewFeedback parses cursor-agent stream-json output and returns
// the result field from the final result event.
func parseCursorReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cursorStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "result" {
			lastResult = evt.Result
		}
	}
	return lastResult, nil
}

// claudeStreamEvent represents a single JSONL event in Claude Code's
// stream-json output (claude -p --verbose --output-format stream-json).
type claudeStreamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseClaudeSessionID parses Claude Code stream-json output and returns the
// session_id from the first system/init event.
func parseClaudeSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt claudeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "system" && evt.Subtype == "init" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	return "", nil
}

// claudeResumeArgs constructs the arguments to resume a Claude Code session
// with a follow-up prompt.
func claudeResumeArgs(sessionID, prompt string) []string {
	return []string{"-p", "--verbose", "--resume", sessionID, "--output-format", "stream-json", prompt}
}

// parseClaudeReviewFeedback parses Claude Code stream-json output and returns
// the result field from the final result event.
func parseClaudeReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt claudeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "result" {
			lastResult = evt.Result
		}
	}
	return lastResult, nil
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

func (a *app) taskList(mode string, statuses []string, labels []string) error {
	tasks, err := a.getTasks()
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, mode)
	if len(statuses) > 0 {
		allowed := make(map[string]bool, len(statuses))
		for _, raw := range statuses {
			normalized, err := normalizeTaskStatus(raw)
			if err != nil {
				return err
			}
			allowed[normalized] = true
		}
		filtered = filterTasksByStatus(filtered, allowed)
	}
	if len(labels) > 0 {
		required, err := normalizeTaskLabels(labels)
		if err != nil {
			return err
		}
		filtered = filterTasksByLabels(filtered, required)
	}
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		a.printTaskLine(task)
	}
	return nil
}

type taskLabelSummary struct {
	Label  string `json:"label"`
	Total  int    `json:"total"`
	Active int    `json:"active"`
	Open   int    `json:"open"`
	Ready  int    `json:"ready"`
}

func (a *app) taskLabels() error {
	tasks, err := a.getTasks()
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	summaries := summarizeTaskLabels(tasks)
	if a.opts.json {
		return a.emit(summaries)
	}
	for _, summary := range summaries {
		fmt.Fprintf(a.out, "%s total=%d active=%d open=%d ready=%d\n", summary.Label, summary.Total, summary.Active, summary.Open, summary.Ready)
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

func filterTasksByStatus(tasks []Task, allowed map[string]bool) []Task {
	var out []Task
	for _, task := range tasks {
		if allowed[task.Status] {
			out = append(out, task)
		}
	}
	return out
}

func filterTasksByLabels(tasks []Task, required []string) []Task {
	var out []Task
	for _, task := range tasks {
		labels := taskLabelSet(task)
		matches := true
		for _, label := range required {
			if !labels[label] {
				matches = false
				break
			}
		}
		if matches {
			out = append(out, task)
		}
	}
	return out
}

func normalizeTaskLabels(rawLabels []string) ([]string, error) {
	seen := make(map[string]bool, len(rawLabels))
	var labels []string
	for _, raw := range rawLabels {
		if strings.TrimSpace(raw) == "" {
			return nil, usageError("task label filter cannot be empty")
		}
		for _, part := range strings.Split(raw, ",") {
			label := strings.TrimSpace(part)
			if label == "" || label == "-" {
				return nil, usageError("task label filter cannot be empty")
			}
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	return labels, nil
}

func taskLabelSet(task Task) map[string]bool {
	labels := map[string]bool{}
	for _, label := range parseList(task.Labels) {
		labels[label] = true
	}
	return labels
}

func summarizeTaskLabels(tasks []Task) []taskLabelSummary {
	ready := map[string]bool{}
	for _, task := range filterTasks(tasks, "ready") {
		ready[task.ID] = true
	}
	byLabel := map[string]*taskLabelSummary{}
	for _, task := range tasks {
		for label := range taskLabelSet(task) {
			summary := byLabel[label]
			if summary == nil {
				summary = &taskLabelSummary{Label: label}
				byLabel[label] = summary
			}
			summary.Total++
			if task.Bucket == "active" {
				summary.Active++
			}
			if task.Status == "Open" {
				summary.Open++
			}
			if ready[task.ID] {
				summary.Ready++
			}
		}
	}
	labels := make([]string, 0, len(byLabel))
	for label := range byLabel {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	summaries := make([]taskLabelSummary, 0, len(labels))
	for _, label := range labels {
		summaries = append(summaries, *byLabel[label])
	}
	return summaries
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

	var allTasks []Task
	var allTasksLoaded bool
	loadTasks := func() []Task {
		if allTasksLoaded {
			return allTasks
		}
		allTasksLoaded = true
		tasks, collErr := a.getTasks()
		if collErr != nil {
			fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
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

	now := time.Now().Format(time.RFC3339)
	task.Status = status
	task.Updated = now
	bucket := bucketForStatus(status)
	target := filepath.Join(a.opts.root, ".agents", ".tasks", bucket, task.ID+".md")
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
	for _, task := range unblocked {
		if err := writeFileAtomic(task.Path, []byte(renderTask(task)), 0o644); err != nil {
			return err
		}
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", task.ID, status)
	for _, task := range unblocked {
		fmt.Fprintf(a.out, "%s -> Pending\n", task.ID)
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
