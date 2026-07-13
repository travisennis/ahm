package ahm

import (
	"bytes"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"text/template"

	"github.com/travisennis/ahm/internal/templates"
)

type contextReport struct {
	Root     string          `json:"root"`
	Workflow contextWorkflow `json:"workflow"`
	Git      contextGit      `json:"git"`
	Tasks    contextTasks    `json:"tasks"`
	Commands []string        `json:"commands"`
}

type contextScopedReport struct {
	Scope        string               `json:"scope"`
	Instructions []contextInstruction `json:"instructions"`
	Commands     []string             `json:"commands,omitempty"`
}

type contextWorkflow struct {
	Installed        bool             `json:"installed"`
	InstalledVersion string           `json:"installed_version,omitempty"`
	TemplateVersion  string           `json:"template_version"`
	ValidationOK     bool             `json:"validation_ok"`
	Errors           int              `json:"errors"`
	Warnings         int              `json:"warnings"`
	Findings         []contextFinding `json:"findings,omitempty"`
}

type contextFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type contextGit struct {
	Available bool   `json:"available"`
	Branch    string `json:"branch,omitempty"`
	Dirty     bool   `json:"dirty"`
	Changes   int    `json:"changes"`
	Error     string `json:"error,omitempty"`
}

type contextTasks struct {
	Counts     map[string]int `json:"counts"`
	InProgress []taskSummary  `json:"in_progress"`
	NextReady  *taskSummary   `json:"next_ready,omitempty"`
	ReadyTotal int            `json:"ready_total"`
	Blocked    int            `json:"blocked"`
	Open       int            `json:"open"`
	ParseError string         `json:"parse_error,omitempty"`
}

type taskSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Effort   string `json:"effort"`
	Path     string `json:"path"`
}

type contextInstruction struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (a *app) context(scope string) error {
	defer a.emitWarnings()
	if a.opts.json || a.opts.plain {
		if scope != "" {
			instruction, err := scopedContextInstruction(scope, a.opts.root)
			if err != nil {
				return err
			}
			scoped := contextScopedReport{
				Scope:        scope,
				Instructions: []contextInstruction{instruction},
				Commands:     contextCommands(scope),
			}
			return a.emit(scoped)
		}
		report := a.contextReport()
		return a.emit(report)
	}
	if scope != "" {
		instruction, err := scopedContextInstruction(scope, a.opts.root)
		if err != nil {
			return err
		}
		a.emitScopedInstructionText(instruction)
		return nil
	}
	a.emitContextText(a.contextReport())
	return nil
}

func (a *app) contextReport() contextReport {
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
	taskInfo := a.contextTaskSummary(tasks)
	gitInfo := readGitContext(a.opts.root)
	return contextReport{
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

func (a *app) contextTaskSummary(tasks []Task) contextTasks {
	if tasks == nil {
		var err error
		tasks, err = a.getTasks()
		if err != nil {
			a.addWarning("some task files could not be parsed and were skipped")
		}
	}
	counts := taskCounts(tasks)
	ready := filterTasks(tasks, "ready")
	blocked := filterTasks(tasks, "blocked")
	inProgress := filterTasksByStatus(tasks, map[string]bool{"In Progress": true})
	summary := contextTasks{
		Counts:     counts,
		InProgress: taskSummaries(inProgress, 5, a.opts.root),
		ReadyTotal: len(ready),
		Blocked:    len(blocked),
		Open:       counts["Open"],
	}
	if len(ready) > 0 {
		next := taskSummaryFor(ready[0], a.opts.root)
		summary.NextReady = &next
	}
	return summary
}

func contextFindings(report validationReport, limit int) []contextFinding {
	var findings []contextFinding
	add := func(severity string, values []validationFinding) {
		for _, finding := range values {
			if len(findings) >= limit {
				return
			}
			findings = append(findings, contextFinding{
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

type instructionRenderPaths struct {
	RecordsDir              string
	TasksDir                string
	TasksActiveDir          string
	TasksCompletedDir       string
	TasksCancelledDir       string
	TasksIndex              string
	TasksActiveIndex        string
	TasksCompletedIndex     string
	TasksCancelledIndex     string
	ResearchDir             string
	ResearchIndex           string
	ExecPlansDir            string
	ExecPlansActiveDir      string
	ExecPlansCompletedDir   string
	ExecPlansActiveIndex    string
	ExecPlansCompletedIndex string
	ConfigPath              string
}

func scopedContextInstruction(scope string, root string) (contextInstruction, error) {
	files := map[string]struct {
		id     string
		title  string
		source string
	}{
		"task":     {id: "task-workflow", title: "Task Workflow", source: "workflow/TASKS.md"},
		"adr":      {id: "adr-workflow", title: "ADR Workflow", source: "workflow/adr-README.md"},
		"research": {id: "research-workflow", title: "Research Workflow", source: "workflow/RESEARCH.md"},
		"plan":     {id: "exec-plan-workflow", title: "ExecPlan Workflow", source: "workflow/PLANS.md"},
		"docs":     {id: "docs-workflow", title: "Documentation Workflow", source: "workflow/DOCS.md"},
	}
	file, ok := files[scope]
	if !ok {
		return contextInstruction{}, usageError(fmt.Sprintf("unknown context scope %q (valid: task, adr, research, plan, docs)\n  ahm context <scope>", scope))
	}
	data, err := fs.ReadFile(templates.FS, file.source)
	if err != nil {
		return contextInstruction{}, err
	}
	body, err := renderInstructionTemplate(file.source, string(data), instructionPathsFor(root))
	if err != nil {
		return contextInstruction{}, err
	}
	return contextInstruction{
		ID:    file.id,
		Title: file.title,
		Body:  body,
	}, nil
}

func instructionPathsFor(root string) instructionRenderPaths {
	paths := workflowPathsFor(root)
	return pathsForWorkflowPaths(paths)
}

func instructionPathsForRecordsDir(root string, recordsDir string) instructionRenderPaths {
	wp := workflowPaths{root: root, recordsDir: recordsDir}
	return pathsForWorkflowPaths(wp)
}

func pathsForWorkflowPaths(wp workflowPaths) instructionRenderPaths {
	tasksRel := wp.tasksRel()
	researchRel := wp.researchRel()
	execPlansRel := wp.execPlansRel("")
	configPath := legacyMetadataRelPath
	if wp.recordsDir == toolRecordsDirName {
		configPath = configMetadataRelPath
	}
	return instructionRenderPaths{
		RecordsDir:              slashDir(wp.recordsDir),
		TasksDir:                slashDir(tasksRel),
		TasksActiveDir:          slashDir(tasksRel + "/active"),
		TasksCompletedDir:       slashDir(tasksRel + "/completed"),
		TasksCancelledDir:       slashDir(tasksRel + "/cancelled"),
		TasksIndex:              tasksRel + "/index.md",
		TasksActiveIndex:        tasksRel + "/active/index.md",
		TasksCompletedIndex:     tasksRel + "/completed/index.md",
		TasksCancelledIndex:     tasksRel + "/cancelled/index.md",
		ResearchDir:             slashDir(researchRel),
		ResearchIndex:           researchRel + "/index.md",
		ExecPlansDir:            slashDir(execPlansRel),
		ExecPlansActiveDir:      slashDir(wp.execPlansRel("active")),
		ExecPlansCompletedDir:   slashDir(wp.execPlansRel("completed")),
		ExecPlansActiveIndex:    wp.execPlansRel("active") + "/index.md",
		ExecPlansCompletedIndex: wp.execPlansRel("completed") + "/index.md",
		ConfigPath:              configPath,
	}
}

func slashDir(rel string) string {
	return strings.TrimSuffix(rel, "/") + "/"
}

func renderInstructionTemplate(name string, body string, values instructionRenderPaths) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, values); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

// renderWorkflowTemplateFor is like renderWorkflowTemplate but uses the
// given recordsDir to compute paths instead of detecting from the filesystem.
func renderWorkflowTemplateFor(root string, name string, content []byte, recordsDir string) ([]byte, error) {
	paths := instructionPathsForRecordsDir(root, recordsDir)
	rendered, err := renderInstructionTemplate(name, string(content), paths)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

func contextCommands(scope string) []string {
	common := []string{"ahm status", "ahm doctor"}
	switch scope {
	case "task":
		return append([]string{"ahm task next", "ahm task ready", "ahm task show <id>"}, common...)
	case "adr":
		return append([]string{"ahm adr list", "ahm adr create <title>"}, common...)
	case "research":
		return append([]string{"ahm index", "ahm --dry-run index"}, common...)
	case "plan":
		return append([]string{"ahm index", "ahm --dry-run index"}, common...)
	case "docs":
		return append([]string{"ahm doctor --check project-docs"}, common...)
	default:
		return append([]string{"ahm task next", "ahm task ready", "ahm task blocked", "ahm task show <id>"}, common...)
	}
}

func readGitContext(root string) contextGit {
	if _, err := exec.LookPath("git"); err != nil {
		return contextGit{Available: false, Error: "git executable not found"}
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
		return contextGit{Available: true, Error: msg}
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	info := contextGit{Available: true}
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

func (a *app) emitContextText(report contextReport) {
	fmt.Fprintln(a.out, "# ahm context")
	fmt.Fprintln(a.out)
	fmt.Fprintf(a.out, "root: %s\n", report.Root)
	if report.Workflow.Installed {
		fmt.Fprintf(a.out, "workflow: installed %s (templates %s)\n", report.Workflow.InstalledVersion, report.Workflow.TemplateVersion)
	} else {
		fmt.Fprintf(a.out, "workflow: not installed (templates %s)\n", report.Workflow.TemplateVersion)
	}
	switch {
	case report.Workflow.Errors == 0 && report.Workflow.Warnings == 0:
		fmt.Fprintln(a.out, "validation: ok")
	case report.Workflow.Errors > 0:
		fmt.Fprintf(a.out, "validation: %d errors, %d warnings; run `ahm doctor`\n", report.Workflow.Errors, report.Workflow.Warnings)
	default:
		fmt.Fprintf(a.out, "validation: %d warnings; run `ahm doctor`\n", report.Workflow.Warnings)
	}
	for _, finding := range report.Workflow.Findings {
		if finding.Path != "" {
			fmt.Fprintf(a.out, "- %s %s %s: %s\n", finding.Severity, finding.Code, finding.Path, finding.Message)
		} else {
			fmt.Fprintf(a.out, "- %s %s: %s\n", finding.Severity, finding.Code, finding.Message)
		}
	}
	switch {
	case !report.Git.Available:
		fmt.Fprintf(a.out, "git: unavailable")
		if report.Git.Error != "" {
			fmt.Fprintf(a.out, " (%s)", report.Git.Error)
		}
		fmt.Fprintln(a.out)
	case report.Git.Error != "":
		fmt.Fprintf(a.out, "git: unavailable for repo (%s)\n", report.Git.Error)
	default:
		state := "clean"
		if report.Git.Dirty {
			state = fmt.Sprintf("dirty, %d changes", report.Git.Changes)
		}
		fmt.Fprintf(a.out, "git: %s, %s\n", report.Git.Branch, state)
	}
	fmt.Fprintf(a.out, "tasks: open=%d ready=%d blocked=%d in_progress=%d\n", report.Tasks.Open, report.Tasks.ReadyTotal, report.Tasks.Blocked, len(report.Tasks.InProgress))
	if report.Tasks.NextReady != nil {
		fmt.Fprintf(a.out, "next: %s [%s] %s %s %s\n", report.Tasks.NextReady.ID, report.Tasks.NextReady.Status, report.Tasks.NextReady.Priority, report.Tasks.NextReady.Effort, report.Tasks.NextReady.Title)
	}
	for _, task := range report.Tasks.InProgress {
		fmt.Fprintf(a.out, "in_progress: %s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
	}
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "## Useful Commands")
	for _, command := range report.Commands {
		fmt.Fprintf(a.out, "- `%s`\n", command)
	}
}

func (a *app) emitScopedInstructionText(instruction contextInstruction) {
	fmt.Fprint(a.out, instruction.Body)
	if !strings.HasSuffix(instruction.Body, "\n") {
		fmt.Fprintln(a.out)
	}
}
