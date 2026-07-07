package ahm

import (
	"fmt"
	"io"

	"github.com/travisennis/ahm/internal/templates"
)

// primeReport is the structured data for the ahm prime session briefing.
// It implements textRenderer for text output.
type primeReport struct {
	Root     string          `json:"root"`
	Workflow contextWorkflow `json:"workflow"`
	Git      contextGit      `json:"git"`
	Tasks    primeTasks      `json:"tasks"`
	Commands []string        `json:"commands"`
}

type primeTasks struct {
	InProgress []taskSummary `json:"in_progress"`
	Ready      []taskSummary `json:"ready"`
	ReadyTotal int           `json:"ready_total"`
	Blocked    int           `json:"blocked"`
	Open       int           `json:"open"`
}

func (a *app) prime() error {
	defer a.emitWarnings()
	report := a.buildPrimeReport()
	return a.emit(report)
}

func (a *app) buildPrimeReport() primeReport {
	validation, tasks := validateWorkflow(a.opts.root)
	meta, metaErr := readMetadata(a.opts.root)
	if metaErr != nil {
		var err error
		tasks, err = a.getTasks()
		if err != nil {
			a.addWarning("some task files could not be parsed and were skipped")
			if tasks == nil {
				tasks = []Task{}
			}
		}
	}
	var installedVersion string
	if metaErr == nil {
		installedVersion = meta.Version
	}
	taskInfo := a.primeTaskSummary(tasks)
	gitInfo := readGitContext(a.opts.root)
	return primeReport{
		Root: a.opts.root,
		Workflow: contextWorkflow{
			Installed:        metaErr == nil,
			InstalledVersion: installedVersion,
			TemplateVersion:  templates.Version,
			ValidationOK:     validation.OK && len(validation.Warnings) == 0,
			Errors:           len(validation.Errors),
			Warnings:         len(validation.Warnings),
			Findings:         contextFindings(validation, 5),
		},
		Git:      gitInfo,
		Tasks:    taskInfo,
		Commands: contextCommands(""),
	}
}

func (a *app) primeTaskSummary(tasks []Task) primeTasks {
	if tasks == nil {
		var err error
		tasks, err = a.getTasks()
		if err != nil {
			a.addWarning("some task files could not be parsed and were skipped")
		}
	}
	inProgress := filterTasksByStatus(tasks, map[string]bool{"In Progress": true})
	ready := filterTasks(tasks, "ready")
	blocked := filterTasks(tasks, "blocked")
	counts := taskCounts(tasks)

	return primeTasks{
		InProgress: taskSummaries(inProgress, 5, a.opts.root),
		Ready:      taskSummaries(ready, 5, a.opts.root),
		ReadyTotal: len(ready),
		Blocked:    len(blocked),
		Open:       counts["Open"],
	}
}

// RenderText implements the textRenderer interface for primeReport.
func (r primeReport) RenderText(w io.Writer) error {
	// Section 1: Dirty-worktree warning
	if r.Git.Dirty {
		fmt.Fprintf(w, "# Dirty Worktree\n")
		fmt.Fprintf(w, "The working directory has uncommitted changes (%d files modified/untracked). Resolve them before starting new work.\n", r.Git.Changes)
	}

	// Section 2: Root, workflow, validation
	fmt.Fprintf(w, "root: %s\n", r.Root)
	if r.Workflow.Installed {
		fmt.Fprintf(w, "workflow: installed %s (templates %s)\n", r.Workflow.InstalledVersion, r.Workflow.TemplateVersion)
	} else {
		fmt.Fprintf(w, "workflow: not installed (templates %s)\n", r.Workflow.TemplateVersion)
	}
	switch {
	case r.Workflow.Errors == 0 && r.Workflow.Warnings == 0:
		fmt.Fprintln(w, "validation: ok")
	case r.Workflow.Errors > 0:
		fmt.Fprintf(w, "validation: %d errors, %d warnings; run `ahm doctor`\n", r.Workflow.Errors, r.Workflow.Warnings)
	default:
		fmt.Fprintf(w, "validation: %d warnings; run `ahm doctor`\n", r.Workflow.Warnings)
	}
	for _, finding := range r.Workflow.Findings {
		if finding.Path != "" {
			fmt.Fprintf(w, "- %s %s %s: %s\n", finding.Severity, finding.Code, finding.Path, finding.Message)
		} else {
			fmt.Fprintf(w, "- %s %s: %s\n", finding.Severity, finding.Code, finding.Message)
		}
	}

	// Section 3: In Progress
	if len(r.Tasks.InProgress) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## In Progress")
		for _, task := range r.Tasks.InProgress {
			fmt.Fprintf(w, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
		}
	}

	// Section 4: Ready (capped at 5 with overflow pointer)
	if len(r.Tasks.Ready) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Ready")
		for _, task := range r.Tasks.Ready {
			fmt.Fprintf(w, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
		}
		overflow := r.Tasks.ReadyTotal - len(r.Tasks.Ready)
		if overflow > 0 {
			fmt.Fprintf(w, "run `ahm task ready` for %d more\n", overflow)
		}
	}

	// Section 5: Blocked and Open counts
	fmt.Fprintln(w)
	if r.Tasks.Blocked > 0 {
		fmt.Fprintf(w, "Blocked: %d (run `ahm task blocked`)\n", r.Tasks.Blocked)
	} else {
		fmt.Fprintln(w, "Blocked: 0")
	}
	if r.Tasks.Open > 0 {
		fmt.Fprintf(w, "Open: %d (run `ahm task list --status Open`)\n", r.Tasks.Open)
	} else {
		fmt.Fprintln(w, "Open: 0")
	}

	// Section 6: Managed Work Intake
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Managed Work Intake")
	fmt.Fprintln(w, "- Work a task → `ahm context task`, then `ahm task show <id>`")
	fmt.Fprintln(w, "- ExecPlan work → `ahm context plan`")
	fmt.Fprintln(w, "- ADR work → `ahm context adr`")
	fmt.Fprintln(w, "- Research notes → `ahm context research`")
	fmt.Fprintln(w, "- Documentation work → `ahm context docs`")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ahm manages work records, not implementation; after intake, classify the implementation under the project's own workflow routing (AGENTS.md).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Before executing a multi-step plan, materialize it as ahm tasks (or an ExecPlan) — plans in context die at compaction; records survive.")

	// Section 7: Useful commands
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Useful Commands")
	for _, cmd := range r.Commands {
		fmt.Fprintf(w, "- `%s`\n", cmd)
	}

	return nil
}
