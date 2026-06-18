package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	taskWorkLookPath   = exec.LookPath
	taskWorkRunCommand = runTaskWorkCommand
)

type taskWorkArgs struct {
	id       string
	agent    string
	review   bool
	complete bool
	commit   bool
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

func runTaskWorkCommand(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.Command(executable, args...) //nolint:gosec // executable is selected from the supported task work agent allowlist before LookPath.
	cmd.Dir = root
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
