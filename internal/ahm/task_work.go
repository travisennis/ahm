package ahm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// taskWorkDefaultTimeout is the default maximum time an external agent
// subprocess is allowed to run before being killed. This prevents hung
// agent CLIs from blocking ahm indefinitely. The --timeout flag overrides
// this for a single invocation.
var taskWorkDefaultTimeout = 30 * time.Minute

// Unexported context keys for threading the task work timeout and filtered
// environment through to runTaskWorkCommand without changing the
// taskWorkRunnerFunc signature.
type taskWorkTimeoutKey struct{}
type taskWorkEnvKey struct{}

func withTaskWorkTimeout(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, taskWorkTimeoutKey{}, d)
}

func taskWorkTimeoutFromContext(ctx context.Context) time.Duration {
	if d, ok := ctx.Value(taskWorkTimeoutKey{}).(time.Duration); ok && d > 0 {
		return d
	}
	return taskWorkDefaultTimeout
}

// withTaskWorkEnv threads a filtered environment through to runTaskWorkCommand.
// A nil env is stored as an untyped nil so taskWorkEnvFromContext treats it as
// "not set" and the child process inherits the parent environment.
func withTaskWorkEnv(ctx context.Context, env []string) context.Context {
	return context.WithValue(ctx, taskWorkEnvKey{}, env)
}

// taskWorkEnvFromContext returns the filtered environment from ctx, or nil if
// no filter was provided.
func taskWorkEnvFromContext(ctx context.Context) []string {
	if env, ok := ctx.Value(taskWorkEnvKey{}).([]string); ok {
		return env
	}
	return nil
}

// taskWorkRunContext returns a context carrying both the timeout and the
// filtered environment for a delegated agent run. A nil env means the child
// process should inherit the parent environment unchanged.
func taskWorkRunContext(timeout time.Duration, env []string) context.Context {
	return withTaskWorkEnv(withTaskWorkTimeout(context.Background(), timeout), env)
}

// taskWorkRunnerFunc is the signature for running an external agent command.
// The context carries the deadline, cancellation signal, and optional filtered
// environment.
type taskWorkRunnerFunc func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error

var (
	taskWorkLookPath                      = exec.LookPath
	taskWorkRunCommand taskWorkRunnerFunc = runTaskWorkCommand
)

type taskWorkArgs struct {
	id              string
	agent           string
	model           string
	noReview        bool
	noCommit        bool
	noProjectPrompt bool
	timeout         time.Duration
}

func (a *app) taskWork(parsed taskWorkArgs) error {
	defer a.emitWarnings()
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

	// Resolve role-specific agents and models.
	roles, err := a.resolveTaskWorkRoles(parsed.agent, parsed.model)
	if err != nil {
		return err
	}

	review := !parsed.noReview
	commit := !parsed.noCommit
	timeout := parsed.timeout
	if timeout <= 0 {
		timeout = taskWorkDefaultTimeout
	}

	prompt := a.buildTaskWorkPrompt(task, parsed.noProjectPrompt, review)

	executable, err := taskWorkLookPath(roles.implAgent.executable)
	if err != nil {
		return fmt.Errorf("cannot work task %s with %s: executable %q not found on PATH", task.ID, roles.implAgent.name, roles.implAgent.executable)
	}
	args := roles.implAgent.args(prompt, roles.implModel)

	// Validate review executable when review will run.
	var reviewExecutable string
	if review && roles.reviewAgent.executable != roles.implAgent.executable {
		reviewExecutable, err = taskWorkLookPath(roles.reviewAgent.executable)
		if err != nil {
			return fmt.Errorf("cannot run review for task %s with %s: executable %q not found on PATH", task.ID, roles.reviewAgent.name, roles.reviewAgent.executable)
		}
	}

	if a.opts.dryRun {
		preview := map[string]any{
			"task":       task.ID,
			"agent":      roles.implAgent.name,
			"model":      roles.implModel,
			"executable": executable,
			"args":       args,
			"prompt":     prompt,
			"status":     taskWorkDryRunStatus(task.Status),
			"timeout":    timeout,
		}
		if commit {
			preview["commit"] = true
		}
		if review {
			preview["review"] = true
			preview["review_agent"] = roles.reviewAgent.name
			preview["review_model"] = roles.reviewModel
		}
		return a.emit(preview)
	}
	if task.Status == "Pending" {
		if err := a.withWorkflowRecordLock(true, func() error {
			a.invalidateTasks()
			current, err := a.resolveTask(parsed.id)
			if err != nil {
				return err
			}
			if current.Status != "Pending" {
				return fmt.Errorf("cannot work task %s: status changed to %s", current.ID, current.Status)
			}
			if err := a.ensureTaskDependenciesComplete(current); err != nil {
				return err
			}
			return a.markTaskInProgressLocked(current)
		}); err != nil {
			return err
		}
	}
	return a.taskWorkWithSession(roles, executable, args, reviewExecutable, review, commit, task, timeout)
}

func taskWorkDryRunStatus(status string) string {
	if status == "Pending" {
		return "In Progress"
	}
	return status
}

func (a *app) ensureTaskDependenciesComplete(task Task) error {
	if len(task.DependsOn) == 0 {
		return nil
	}
	allTasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
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

func (a *app) buildTaskWorkPrompt(task Task, noProjectPrompt bool, review bool) string {
	var completion string
	if review {
		completion = "Fill the task Acceptance Notes when the work is done and run the required verification, but do NOT mark the task complete yet \u2014 leave it In Progress for independent review. After review resolves, you will receive a finalization prompt to complete the task."
	} else {
		completion = "Fill the task Acceptance Notes when the work is done, run the required verification, and mark the task complete with ahm when acceptance is satisfied."
	}
	prompt := fmt.Sprintf(`Work on task %s.

Before making changes, run ahm context task to load the task workflow reference, then run ahm task show %s to inspect the task.

Use the repository task workflow. Keep changes scoped to the task. %s Do not commit or push unless the user explicitly asked for that.
`, task.ID, task.ID, completion)

	if noProjectPrompt {
		return prompt
	}

	// Resolve the project instructions file path: configured or default.
	promptFile := filepath.Join(a.opts.root, ".agents", "prompt.md")
	meta, err := readMetadata(a.opts.root)
	if err == nil && meta.TaskWork != nil && meta.TaskWork.PromptFile != "" {
		promptFile = meta.TaskWork.PromptFile
		if !filepath.IsAbs(promptFile) {
			promptFile = filepath.Join(a.opts.root, promptFile)
		}
	}

	content, err := os.ReadFile(filepath.Clean(promptFile))
	if err != nil {
		// Missing or unreadable file is not an error — feature is opt-in by presence.
		return prompt
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return prompt
	}

	prompt += "\n\n## Project Instructions\n\n" + trimmed
	return prompt
}

func (a *app) markTaskInProgressLocked(task Task) error {
	task.Status = "In Progress"
	task.Updated = time.Now().Format(time.RFC3339)
	target := workflowPathsFor(a.opts.root).taskFile("active", task.ID)
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

func runTaskWorkCommand(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, taskWorkTimeoutFromContext(ctx))
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is selected from the supported task work agent allowlist before LookPath.
	cmd.Dir = root
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if env := taskWorkEnvFromContext(ctx); env != nil {
		cmd.Env = env
	}
	return cmd.Run()
}

// filterBlockedEnv returns a copy of env with any entries whose name matches
// one of the blocked names removed. The comparison is case-insensitive so it
// catches variables like Anthropic_API_KEY.
func filterBlockedEnv(env, blocked []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if isBlockedEnv(name, blocked) {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}

func isBlockedEnv(name string, blocked []string) bool {
	for _, b := range blocked {
		if strings.EqualFold(name, b) {
			return true
		}
	}
	return false
}
