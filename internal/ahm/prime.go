package ahm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/travisennis/ahm/internal/version"
)

// primeReport is the structured data for the ahm prime session briefing.
// It implements textRenderer for text output.
type primeReport struct {
	Root     string        `json:"root"`
	Workflow primeWorkflow `json:"workflow"`
	Git      primeGit      `json:"git"`
	Tasks    primeTasks    `json:"tasks"`
}

type primeWorkflow struct {
	Installed        bool           `json:"installed"`
	InstalledVersion string         `json:"installed_version,omitempty"`
	ValidationOK     bool           `json:"validation_ok"`
	Errors           int            `json:"errors"`
	Warnings         int            `json:"warnings"`
	Findings         []primeFinding `json:"findings,omitempty"`
}

type primeFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type primeGit struct {
	Available bool   `json:"available"`
	Branch    string `json:"branch,omitempty"`
	Dirty     bool   `json:"dirty"`
	Changes   int    `json:"changes"`
	Error     string `json:"error,omitempty"`
}

type primeTasks struct {
	InProgress []taskSummary `json:"in_progress"`
	Ready      []taskSummary `json:"ready"`
	ReadyTotal int           `json:"ready_total"`
	Blocked    int           `json:"blocked"`
	Open       int           `json:"open"`
}

type taskSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Effort   string `json:"effort"`
	Path     string `json:"path"`
}

func (a *app) prime() error {
	defer a.emitWarnings()

	// When workflow metadata is present, prepare the worktree:
	// ensure directories exist, create the managed gitignore in
	// migrated layout, and regenerate indexes from source records
	// so the briefing reflects the current branch state even when
	// gitignored index files are stale from a previous checkout.
	// These preparations are skipped when no workflow is installed,
	// which keeps prime usable for bare git checkouts without
	// creating untracked files.
	if _, err := readMetadata(a.opts.root); err == nil {
		if !a.opts.dryRun {
			if _, err := a.ensureWorkflowDirs(); err != nil {
				return err
			}
			if err := a.ensureWorkflowGitignore(); err != nil {
				return err
			}
			if err := a.regenerateIndexes(); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		a.addWarning("unreadable workflow metadata: %v", err)
	}

	// Build and emit the report
	report := a.buildPrimeReport()
	return a.emit(report)
}

// regenerateIndexes recomputes all generated indexes from source records and
// writes only those that are stale. It is like writeIndexes but does not emit
// post-mutation findings or trigger its own warning emission, making it safe
// to call from prime before buildPrimeReport accumulates its own warnings.
func (a *app) regenerateIndexes() error {
	a.invalidateTasks()
	writes, err := a.indexWrites()
	if err != nil {
		if writes == nil {
			return fmt.Errorf("regenerating indexes: %w", err)
		}
		// Partial results with errors; use what we got.
	}
	for _, path := range sortedKeys(writes) {
		if !isStaleIndex(nil, path, writes[path]) {
			continue
		}
		if err := writeFileAtomic(path, []byte(writes[path]), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) buildPrimeReport() primeReport {
	validation, tasks := a.validateWorkflow(nil)
	_, metaErr := readMetadata(a.opts.root)
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
		installedVersion = version.Binary
	}
	taskInfo := a.primeTaskSummary(tasks)
	gitInfo := readGitContext(a.opts.root)

	return primeReport{
		Root: a.opts.root,
		Workflow: primeWorkflow{
			Installed:        metaErr == nil,
			InstalledVersion: installedVersion,
			ValidationOK:     validation.OK && len(validation.Warnings) == 0,
			Errors:           len(validation.Errors),
			Warnings:         len(validation.Warnings),
			Findings:         primeFindings(validation, 5),
		},
		Git:   gitInfo,
		Tasks: taskInfo,
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

// primeFindings collects up to limit validation findings for the briefing,
// errors first.
func primeFindings(report validationReport, limit int) []primeFinding {
	var findings []primeFinding
	add := func(severity string, values []validationFinding) {
		for _, finding := range values {
			if len(findings) >= limit {
				return
			}
			findings = append(findings, primeFinding{
				Severity: severity,
				Code:     finding.Code,
				Path:     finding.Path,
				Message:  finding.Message,
			})
		}
	}
	add("error", report.Errors)
	add("warning", report.Warnings)
	return findings
}

func taskSummaries(tasks []Task, limit int, root string) []taskSummary {
	if len(tasks) > limit {
		tasks = tasks[:limit]
	}
	summaries := make([]taskSummary, 0, len(tasks))
	for _, task := range tasks {
		summaries = append(summaries, taskSummaryFor(task, root))
	}
	return summaries
}

func taskSummaryFor(task Task, root string) taskSummary {
	return taskSummary{
		ID:       task.ID,
		Title:    task.Title,
		Status:   task.Status,
		Priority: task.Priority,
		Effort:   task.Effort,
		Path:     relPath(root, task.Path),
	}
}

// readGitContext reports the repository's branch and dirty state, if Git is
// available. It runs only the read-only `git status` command, scoped to root.
func readGitContext(root string) primeGit {
	if _, err := exec.LookPath("git"); err != nil {
		return primeGit{Available: false, Error: "git executable not found"}
	}
	cmd := exec.Command("git", "-C", root, "status", "--short", "--branch") // #nosec G204 // read-only git status scoped to the detected repository root
	cmd.Env = cleanGitEnvironment()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return primeGit{Available: true, Error: msg}
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	info := primeGit{Available: true}
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i == 0 && strings.HasPrefix(line, "## ") {
			info.Branch = strings.TrimPrefix(line, "## ")
			continue
		}
		info.Changes++
	}
	info.Dirty = info.Changes > 0
	return info
}

// RenderText implements the textRenderer interface for primeReport.
func (r primeReport) RenderText(w io.Writer) error {
	// Section 1: Dirty-worktree warning
	if r.Git.Dirty {
		fmt.Fprintf(w, "# Dirty Worktree\n")
		fmt.Fprintf(w, "The working directory has uncommitted changes (%d files modified/untracked).\n", r.Git.Changes)
	}

	// Section 2: Root, workflow, validation
	fmt.Fprintf(w, "root: %s\n", r.Root)
	if r.Workflow.Installed {
		fmt.Fprintf(w, "workflow: installed %s\n", r.Workflow.InstalledVersion)
	} else {
		fmt.Fprintln(w, "workflow: not installed")
	}
	switch {
	case r.Workflow.Errors == 0 && r.Workflow.Warnings == 0:
		fmt.Fprintln(w, "validation: ok")
	case r.Workflow.Errors > 0:
		fmt.Fprintf(w, "validation: %d errors, %d warnings\n", r.Workflow.Errors, r.Workflow.Warnings)
	default:
		fmt.Fprintf(w, "validation: %d warnings\n", r.Workflow.Warnings)
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

	// Section 4: Ready (capped at 5 with an overflow count)
	if len(r.Tasks.Ready) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Ready")
		for _, task := range r.Tasks.Ready {
			fmt.Fprintf(w, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
		}
		if overflow := r.Tasks.ReadyTotal - len(r.Tasks.Ready); overflow > 0 {
			fmt.Fprintf(w, "%d more ready\n", overflow)
		}
	}

	// Section 5: Blocked and Open counts
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Blocked: %d\n", r.Tasks.Blocked)
	fmt.Fprintf(w, "Open: %d\n", r.Tasks.Open)

	return nil
}
