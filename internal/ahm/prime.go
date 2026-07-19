package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/travisennis/ahm/internal/templates"
)

// primeReport is the structured data for the ahm prime session briefing.
// It implements textRenderer for text output.
type primeReport struct {
	Root     string                 `json:"root"`
	Workflow contextWorkflow        `json:"workflow"`
	Git      contextGit             `json:"git"`
	Tasks    primeTasks             `json:"tasks"`
	Plans    []primePlanSummary     `json:"plans,omitempty"`
	Research []primeResearchNote    `json:"research,omitempty"`
	Commands []string               `json:"commands"`
	Paths    instructionRenderPaths `json:"-"`
}

type primePlanSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Link  string `json:"link"`
}

type primeResearchNote struct {
	Bucket  string `json:"bucket"`
	Link    string `json:"link"`
	Title   string `json:"title"`
	AgeDays *int   `json:"age_days,omitempty"`
	Stale   bool   `json:"stale,omitempty"`
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
			paths := a.workflowPaths()
			if _, err := a.ensureWorkflowDirs(paths.recordsDir); err != nil {
				return err
			}
			if err := a.ensureWorkflowGitignore(paths.recordsDir); err != nil {
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
		if !isStaleIndex(path, writes[path]) {
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
	plans := a.primeActivePlans()
	research := a.primeRecentResearch()

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
		Plans:    plans,
		Research: research,
		Commands: contextCommands(""),
		Paths:    pathsForWorkflowPaths(a.workflowPaths()),
	}
}

// primeActivePlans collects active ExecPlans in the current record layout.
func (a *app) primeActivePlans() []primePlanSummary {
	paths := a.workflowPaths()
	dir := paths.execPlansDir("active")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var plans []primePlanSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
			continue
		}
		fpath := filepath.Join(dir, entry.Name())
		title, err := markdownTitle(fpath)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		plans = append(plans, primePlanSummary{
			ID:    id,
			Title: title,
			Link:  filepath.ToSlash(filepath.Join(paths.execPlansRel("active"), entry.Name())),
		})
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].ID < plans[j].ID
	})
	if len(plans) > 5 {
		plans = plans[:5]
	}
	return plans
}

// primeRecentResearch collects recent research notes (up to 5, newest by
// filename sort) in the current record layout.
func (a *app) primeRecentResearch() []primeResearchNote {
	return a.primeRecentResearchAt(time.Now())
}

func (a *app) primeRecentResearchAt(now time.Time) []primeResearchNote {
	paths := a.workflowPaths()
	meta, metaErr := readMetadata(a.opts.root)
	threshold, staleEnabled := meta.researchInboxStaleThreshold()
	if metaErr != nil {
		staleEnabled = false
	}
	buckets := []string{"inbox", "topics", "investigations", "sources"}
	var notes []primeResearchNote
	for _, bucket := range buckets {
		dir := filepath.Join(a.opts.root, filepath.FromSlash(paths.researchRel()), bucket)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			fpath := filepath.Join(dir, entry.Name())
			title, err := markdownTitle(fpath)
			if err != nil {
				continue
			}
			note := primeResearchNote{
				Bucket: bucket,
				Link:   filepath.ToSlash(filepath.Join(paths.researchRel(), bucket, entry.Name())),
				Title:  title,
			}
			if bucket == "inbox" {
				if ageDays, err := researchNoteAgeDays(fpath, now); err == nil {
					note.AgeDays = &ageDays
					note.Stale = staleEnabled && ageDays >= threshold
				}
			}
			notes = append(notes, note)
		}
	}
	sort.Slice(notes, func(i, j int) bool {
		iName := filepath.Base(notes[i].Link)
		jName := filepath.Base(notes[j].Link)
		if iName != jName {
			return iName > jName
		}
		return notes[i].Link < notes[j].Link
	})
	if len(notes) > 5 {
		notes = notes[:5]
	}
	return notes
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

	// Section 5: Ready (capped at 5 with overflow pointer)
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

	// Section 6: Blocked and Open counts
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

	// Section 7: Active ExecPlans
	if len(r.Plans) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Active ExecPlans")
		for _, plan := range r.Plans {
			fmt.Fprintf(w, "- %s %s\n", plan.ID, plan.Title)
		}
	}

	// Section 8: Recent Research
	if len(r.Research) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Recent Research")
		for _, note := range r.Research {
			fmt.Fprintf(w, "- [%s](%s) %s", note.Bucket, note.Link, note.Title)
			if note.AgeDays != nil {
				fmt.Fprintf(w, " (%d days old", *note.AgeDays)
				if note.Stale {
					fmt.Fprint(w, ", STALE")
				}
				fmt.Fprint(w, ")")
			}
			fmt.Fprintln(w)
		}
	}

	// Section 9: Managed Work Intake
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Managed Work Intake")
	fmt.Fprintln(w, "- Work a task → `ahm context task`, then `ahm task show <id>`")
	fmt.Fprintln(w, "- ExecPlan work → `ahm context plan`")
	fmt.Fprintln(w, "- ADR work → `ahm context adr`")
	fmt.Fprintln(w, "- Research notes → `ahm context research`")
	fmt.Fprintln(w, "- Documentation work → `ahm context docs`")
	fmt.Fprintln(w, "- Groom the backlog → `ahm task groom`")
	fmt.Fprintln(w, "- Audit for improvements → `ahm audit`")
	fmt.Fprintf(w, "- Workflow records: tasks `%s`, research `%s`, ExecPlans `%s`\n", r.Paths.TasksDir, r.Paths.ResearchDir, r.Paths.ExecPlansDir)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ahm manages work records, not implementation; after intake, classify the implementation under the project's own workflow routing (AGENTS.md).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Before executing a multi-step plan, materialize it as ahm tasks (or an ExecPlan) — plans in context die at compaction; records survive.")

	// Section 10: Useful commands
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Useful Commands")
	for _, cmd := range r.Commands {
		fmt.Fprintf(w, "- `%s`\n", cmd)
	}

	return nil
}
