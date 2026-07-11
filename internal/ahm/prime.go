package ahm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

// primeReport is the structured data for the ahm prime session briefing.
// It implements textRenderer for text output.
type primeReport struct {
	Root     string                 `json:"root"`
	Workflow contextWorkflow        `json:"workflow"`
	Git      contextGit             `json:"git"`
	Tasks    primeTasks             `json:"tasks"`
	Records  primeRecords           `json:"records"`
	Plans    []primePlanSummary     `json:"plans,omitempty"`
	Research []primeResearchNote    `json:"research,omitempty"`
	Commands []string               `json:"commands"`
	Paths    instructionRenderPaths `json:"-"`
}

type primeRecords struct {
	Mode     string `json:"mode"`
	Synced   bool   `json:"synced"`
	Stale    bool   `json:"stale"`
	LastSync string `json:"last_sync,omitempty"`
	Message  string `json:"message,omitempty"`
}

type primePlanSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Link  string `json:"link"`
}

type primeResearchNote struct {
	Bucket string `json:"bucket"`
	Link   string `json:"link"`
	Title  string `json:"title"`
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

	// 1. Sync records in ref mode (skip with --no-sync)
	if !a.opts.noSync {
		a.primeSyncRecords()
	}

	// 2. Build and emit the report
	report := a.buildPrimeReport()
	return a.emit(report)
}

// primeSyncRecords synchronizes workflow records in ref mode.
// Failures degrade to warnings so the briefing is always shown.
func (a *app) primeSyncRecords() {
	meta, err := readMetadata(a.opts.root)
	if err != nil {
		return // legacy mode, nothing to sync
	}
	cfg := meta.recordsStorage()
	if cfg.Mode != recordStoreModeRef {
		return // not ref mode
	}

	ctx := context.Background()

	// Try to fetch remote and pull if ahead
	remoteCommit, remoteErr := lsRemoteRecordsRef(ctx, a.opts.root, cfg)
	if remoteErr != nil && !errors.Is(remoteErr, errGitRefMissing) {
		a.addWarning("prime: could not check remote records: %v", remoteErr)
	}

	if remoteErr == nil {
		localCommit, localErr := resolveGitRef(ctx, a.opts.root, cfg.Ref)
		if localErr != nil && !errors.Is(localErr, errGitRefMissing) {
			a.addWarning("prime: could not read local records ref: %v", localErr)
			return
		}

		// Pull when remote is ahead or local ref is missing
		if errors.Is(localErr, errGitRefMissing) || localCommit != remoteCommit {
			working, workErr := recordsWorkingStatus(ctx, a.opts.root, cfg.Ref)
			if workErr == nil && working.Clean {
				trackingRef, fetchErr := fetchRecordsRef(ctx, a.opts.root, cfg)
				if fetchErr != nil {
					a.addWarning("prime: fetch failed: %v", fetchErr)
				} else {
					cmp, cmpErr := compareRecordsRefs(ctx, a.opts.root, cfg.Ref, trackingRef)
					if cmpErr == nil && (cmp == recordsRefBehind || cmp == recordsRefEqual) {
						if err := updateRecordsRef(ctx, a.opts.root, cfg.Ref, remoteCommit); err != nil {
							a.addWarning("prime: update records ref failed: %v", err)
						} else if _, err := materializeRecordsRef(ctx, a.opts.root, cfg.Ref); err != nil {
							a.addWarning("prime: materialization failed: %v", err)
						}
					}
				}
			}
		}
	}

	// Snapshot local changes
	if _, err := snapshotRecordsRef(ctx, a.opts.root, cfg, "Snapshot ahm workflow records before session"); err != nil {
		a.addWarning("prime: records snapshot failed: %v; run 'ahm records status' to inspect", err)
	}

	// Push local snapshot when remote is reachable
	if remoteErr == nil {
		if err := pushRecordsRef(ctx, a.opts.root, cfg); err != nil {
			a.addWarning("prime: push failed: %v; local records are safe, run 'ahm records push' later", err)
		} else if err := a.markRecordsSynced(meta); err != nil {
			a.addWarning("prime: could not update sync timestamp: %v", err)
		}
	}

	// Regenerate indexes after sync
	if err := a.writeIndexes(); err != nil {
		a.addWarning("prime: index regeneration failed: %v", err)
	}
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
	recordsInfo := a.buildPrimeRecords(meta, metaErr)
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
		Records:  recordsInfo,
		Plans:    plans,
		Research: research,
		Commands: contextCommands(""),
		Paths:    instructionPathsFor(a.opts.root),
	}
}

func (a *app) buildPrimeRecords(meta metadata, metaErr error) primeRecords {
	if metaErr != nil {
		return primeRecords{Mode: "committed", Synced: true}
	}
	cfg := meta.recordsStorage()
	info := primeRecords{
		Mode:     string(cfg.Mode),
		LastSync: meta.RecordsLastSync,
	}
	if cfg.Mode != recordStoreModeRef {
		info.Synced = true
		return info
	}
	ref := cfg.Ref
	if ref == "" {
		ref = defaultRecordsRef
	}
	if _, err := resolveGitRef(context.Background(), a.opts.root, ref); err != nil {
		info.Stale = true
		info.Message = "local records ref is missing; run 'ahm records status'"
		return info
	}
	working, err := recordsWorkingStatus(context.Background(), a.opts.root, ref)
	if err != nil {
		info.Stale = true
		info.Message = fmt.Sprintf("could not check working records: %v", err)
		return info
	}
	if !working.Clean {
		info.Stale = true
		info.Message = "local records have unsnapshotted changes; run 'ahm records push'"
		return info
	}
	if info.LastSync == "" {
		info.Stale = true
		info.Message = "records have never been synced to remote"
		return info
	}
	info.Synced = true
	return info
}

// primeActivePlans collects active ExecPlans in the current storage mode.
func (a *app) primeActivePlans() []primePlanSummary {
	paths := workflowPathsFor(a.opts.root)
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
// filename sort) in the current storage mode.
func (a *app) primeRecentResearch() []primeResearchNote {
	paths := workflowPathsFor(a.opts.root)
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
			notes = append(notes, primeResearchNote{
				Bucket: bucket,
				Link:   filepath.ToSlash(filepath.Join(paths.researchRel(), bucket, entry.Name())),
				Title:  title,
			})
		}
	}
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

	// Section 3: Stale/unsynced records state
	if r.Records.Stale {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "# Records: %s\n", r.Records.Message)
	}
	if !r.Records.Synced && !r.Records.Stale && r.Records.Mode != "committed" {
		if r.Records.Message != "" {
			fmt.Fprintln(w)
			fmt.Fprintf(w, "# Records: %s\n", r.Records.Message)
		}
	}

	// Section 4: In Progress
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
			fmt.Fprintf(w, "- [%s](%s) %s\n", note.Bucket, note.Link, note.Title)
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
