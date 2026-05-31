package ahm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/travisennis/ahm/internal/templates"
)

type options struct {
	root    string
	json    bool
	plain   bool
	quiet   bool
	verbose bool
	dryRun  bool
	force   bool
}

type app struct {
	opts options
	out  io.Writer
	err  io.Writer
}

// Main runs the CLI and returns a process exit code.
func Main(argv []string, stdout io.Writer, stderr io.Writer) int {
	a := app{out: stdout, err: stderr}
	if err := a.run(argv); err != nil {
		var usage usageError
		if errors.As(err, &usage) {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if errors.Is(err, errValidationFailed) {
			return 1
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

var errValidationFailed = errors.New("workflow has validation errors")

type usageError string

func (e usageError) Error() string {
	return string(e)
}

// noArgs is like cobra.NoArgs but wraps the error as a usageError so that
// Main can distinguish usage errors from runtime errors by type assertion.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()))
	}
	return nil
}

func (a *app) run(argv []string) error {
	cmd := a.command()
	cmd.SetArgs(argv)
	return cmd.Execute()
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use:           "ahm",
		Short:         "Manage repo-local .agents workflows",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       templates.Version,
		Args:          noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.status()
		},
	}
	root.SetOut(a.out)
	root.SetErr(a.err)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(err.Error())
	})
	root.PersistentFlags().StringVar(&a.opts.root, "root", "", "Target repository root")
	root.PersistentFlags().BoolVar(&a.opts.json, "json", false, "Print JSON")
	root.PersistentFlags().BoolVar(&a.opts.plain, "plain", false, "Print stable plain output")
	root.PersistentFlags().BoolVar(&a.opts.quiet, "quiet", false, "Suppress nonessential output")
	root.PersistentFlags().BoolVar(&a.opts.verbose, "verbose", false, "Print verbose output")
	root.PersistentFlags().BoolVar(&a.opts.dryRun, "dry-run", false, "Preview supported writes")
	root.PersistentFlags().BoolVar(&a.opts.force, "force", false, "Force supported overwrites")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(a.out, templates.Version)
			return nil
		},
	})
	root.AddCommand(a.lenientCommand("init", "Install workflow files", func() error {
		return a.install(false)
	}))
	root.AddCommand(a.lenientCommand("upgrade", "Update managed workflow files", func() error {
		return a.install(true)
	}))
	root.AddCommand(a.simpleCommand("status", "Show workflow health", func() error {
		return a.status()
	}))
	root.AddCommand(a.simpleCommand("doctor", "Show environment checks", func() error {
		return a.doctor()
	}))
	root.AddCommand(a.simpleCommand("index", "Regenerate task indexes", func() error {
		return a.writeIndexes()
	}))
	root.AddCommand(a.agentsCommand())
	root.AddCommand(a.taskCommand())
	return root
}

func (a *app) agentsCommand() *cobra.Command {
	var showAll bool
	agents := &cobra.Command{
		Use:   "agents",
		Short: "Show AGENTS.md guidance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("agents requires a subcommand")
		},
	}
	suggestions := &cobra.Command{
		Use:   "suggestions",
		Short: "Print suggested AGENTS.md additions",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRootOrCWD(); err != nil {
				return err
			}
			return a.agentsSuggestions(showAll)
		},
	}
	suggestions.Flags().BoolVar(&showAll, "all", false, "Print all suggestions, including ones that appear present")
	agents.AddCommand(suggestions)
	return agents
}

func (a *app) simpleCommand(use string, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return run()
		},
	}
}

func (a *app) lenientCommand(use string, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRootOrCWD(); err != nil {
				return err
			}
			return run()
		},
	}
}

func (a *app) detectRoot() error {
	if a.opts.root != "" {
		return nil
	}
	root, err := detectManagedRoot()
	if err != nil {
		return err
	}
	a.opts.root = root
	return nil
}

func (a *app) detectRootOrCWD() error {
	if a.opts.root != "" {
		return nil
	}
	root, err := detectManagedRoot()
	if err != nil {
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	a.opts.root = root
	return nil
}

func detectManagedRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if stat, err := os.Stat(filepath.Join(dir, ".git")); err == nil && stat.IsDir() {
			return dir, nil
		}
		if stat, err := os.Stat(filepath.Join(dir, ".agents", "ahm.json")); err == nil && !stat.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a managed repository (no .git or .agents/ahm.json found); use --root to specify a directory or run 'ahm init' to create a workflow")
		}
		dir = parent
	}
}

type metadata struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

func (a *app) install(upgrade bool) error {
	root := a.opts.root
	meta, _ := readMetadata(root)
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	for _, target := range generatedIndexTargets() {
		delete(meta.Files, target)
	}
	result := map[string][]string{
		"created":   {},
		"updated":   {},
		"skipped":   {},
		"conflicts": {},
	}
	for _, item := range templates.Files() {
		var content []byte
		if item.Target == "AGENTS.md" {
			content = []byte(templates.RenderAgentsMarkdown())
		} else {
			var err error
			content, err = fs.ReadFile(templates.FS, item.Source)
			if err != nil {
				return err
			}
		}
		target := filepath.Join(root, item.Target)
		hash := hashBytes(content)
		existing, readErr := readWorkflowFile(target)
		switch {
		case errors.Is(readErr, os.ErrNotExist):
			result["created"] = append(result["created"], item.Target)
			if !a.opts.dryRun {
				if err := writeFileAtomic(target, content, 0o644); err != nil {
					return err
				}
			}
			if item.CreateOnly {
				delete(meta.Files, item.Target)
			} else {
				meta.Files[item.Target] = hash
			}
		case readErr != nil:
			return readErr
		case item.CreateOnly:
			result["skipped"] = append(result["skipped"], item.Target)
			delete(meta.Files, item.Target)
		case !upgrade && !a.opts.force:
			result["skipped"] = append(result["skipped"], item.Target)
		case a.opts.force || meta.Files[item.Target] == hashBytes(existing):
			if string(existing) != string(content) {
				result["updated"] = append(result["updated"], item.Target)
				if !a.opts.dryRun {
					if err := writeFileAtomic(target, content, 0o644); err != nil {
						return err
					}
				}
			} else {
				result["skipped"] = append(result["skipped"], item.Target)
			}
			meta.Files[item.Target] = hash
		default:
			result["conflicts"] = append(result["conflicts"], item.Target)
		}
	}
	dirs, err := a.ensureWorkflowDirs()
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		result["directories"] = dirs
	}
	meta.Version = templates.Version
	if a.opts.dryRun {
		result["metadata"] = []string{".agents/ahm.json"}
		indexes, err := a.indexWriteTargets()
		if err != nil {
			return err
		}
		result["indexes"] = indexes
	} else {
		if err := writeMetadata(root, meta); err != nil {
			return err
		}
		if err := a.writeIndexes(); err != nil {
			return err
		}
	}
	return a.emit(result)
}

func (a *app) ensureWorkflowDirs() ([]string, error) {
	dirs := []string{
		".agents/.tasks/active",
		".agents/.tasks/completed",
		".agents/.tasks/cancelled",
		".agents/.research/inbox",
		".agents/.research/investigations",
		".agents/.research/sources",
		".agents/.research/topics",
		".agents/.research/archived",
		".agents/exec-plans/active",
		".agents/exec-plans/completed",
		".agents/skills/deslop",
		"docs/adr",
	}
	var created []string
	for _, dir := range dirs {
		path := filepath.Join(a.opts.root, dir)
		if a.opts.dryRun {
			stat, err := os.Stat(path)
			switch {
			case errors.Is(err, os.ErrNotExist):
				created = append(created, dir)
			case err != nil:
				return nil, err
			case !stat.IsDir():
				return nil, fmt.Errorf("%s exists and is not a directory", path)
			}
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, err
		}
	}
	return created, nil
}

func readMetadata(root string) (metadata, error) {
	var meta metadata
	data, err := os.ReadFile(filepath.Join(root, ".agents", "ahm.json"))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}

func writeMetadata(root string, meta metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, ".agents", "ahm.json"), append(data, '\n'), 0o644)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readWorkflowFile reads a file and normalizes CRLF (\r\n) line endings to
// LF (\n) so that downstream parsing functions do not need to handle both.
func readWorkflowFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Strip UTF-8 BOM if present.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return data, nil
}

func (a *app) status() error {
	validation, tasks := validateWorkflow(a.opts.root)
	meta, metaErr := readMetadata(a.opts.root)
	status := map[string]any{
		"root":              a.opts.root,
		"template_version":  templates.Version,
		"installed":         metaErr == nil,
		"installed_version": meta.Version,
		"tasks":             taskCounts(tasks),
		"validation":        validation,
	}
	if err := a.emit(status); err != nil {
		return err
	}
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}

func (a *app) doctor() error {
	_, goErr := exec.LookPath("go")
	_, gitErr := exec.LookPath("git")
	meta, metaErr := readMetadata(a.opts.root)
	validation, _ := validateWorkflow(a.opts.root)
	report := map[string]any{
		"root":               a.opts.root,
		"go_available":       goErr == nil,
		"git_available":      gitErr == nil,
		"workflow_installed": metaErr == nil,
		"installed_version":  meta.Version,
		"template_version":   templates.Version,
		"validation":         validation,
	}
	if err := a.emit(report); err != nil {
		return err
	}
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}

type validationReport struct {
	OK       bool                `json:"ok"`
	Errors   []validationFinding `json:"errors"`
	Warnings []validationFinding `json:"warnings"`
}

type validationFinding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func validateWorkflow(root string) (validationReport, []Task) {
	report := validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}}
	tasks := validateManagedFiles(root, &report)
	validateTaskDependencies(root, tasks, &report)
	validateTaskBuckets(root, tasks, &report)
	validateTaskExecPlans(root, tasks, &report)
	validateGeneratedIndexes(root, &report)
	validateMarkdownLinks(root, &report)
	report.OK = len(report.Errors) == 0
	return report, tasks
}

func validateManagedFiles(root string, report *validationReport) []Task {
	meta, metaErr := readMetadata(root)
	if metaErr != nil {
		report.addError("metadata_missing", ".agents/ahm.json", "workflow metadata is missing or unreadable")
	} else {
		for _, item := range templates.Files() {
			validateManagedFile(root, meta, item, report)
		}
	}
	return validateTaskFiles(root, report)
}

func validateManagedFile(root string, meta metadata, item templates.File, report *validationReport) {
	if item.CreateOnly {
		return
	}
	data, err := readWorkflowFile(filepath.Join(root, item.Target))
	if errors.Is(err, os.ErrNotExist) {
		report.addError("managed_file_missing", item.Target, "managed workflow file is missing")
		return
	}
	if err != nil {
		report.addError("managed_file_unreadable", item.Target, err.Error())
		return
	}
	expected := meta.Files[item.Target]
	if expected == "" {
		report.addWarning("managed_file_untracked", item.Target, "managed workflow file is not recorded in metadata")
		return
	}
	if hashBytes(data) != expected {
		report.addWarning("managed_file_modified", item.Target, "managed workflow file hash differs from metadata")
	}
}

func validateTaskFiles(root string, report *validationReport) []Task {
	var tasks []Task
	files, err := taskFilePaths(root)
	if err != nil {
		report.addError("task_dir_unreadable", ".agents/.tasks", err.Error())
		return nil
	}
	for _, f := range files {
		validateTaskFrontMatter(root, f.Path, report)
		task, err := parseTask(f.Path, f.Bucket)
		if err != nil {
			report.addError("task_malformed", relPath(root, f.Path), err.Error())
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	return tasks
}

func validateTaskFrontMatter(root string, path string, report *validationReport) {
	data, err := readWorkflowFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Task file was already moved or deleted; not an error.
			return
		}
		report.addError("task_unreadable", relPath(root, path), err.Error())
		return
	}
	meta, _, err := parseFrontMatter(string(data))
	if err != nil {
		report.addError("task_malformed", relPath(root, path), err.Error())
		return
	}
	required := []string{"id", "title", "status", "priority", "effort", "labels", "exec_plan", "depends_on"}
	for _, field := range required {
		if strings.TrimSpace(meta[field]) == "" {
			report.addError("task_missing_field", relPath(root, path), "task front matter is missing "+field)
		}
	}
}

func validateTaskDependencies(root string, tasks []Task, report *validationReport) {
	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	for _, task := range tasks {
		for _, dep := range task.DependsOn {
			if _, ok := byID[dep]; !ok {
				report.addError("task_dependency_missing", relPath(root, task.Path), fmt.Sprintf("task %s depends on missing task %s", task.ID, dep))
			}
		}
	}
	for _, cycle := range taskDependencyCycles(tasks) {
		report.addError("task_dependency_cycle", "", "dependency cycle: "+strings.Join(cycle, " -> "))
	}
}

func validateTaskBuckets(root string, tasks []Task, report *validationReport) {
	for _, task := range tasks {
		switch {
		case task.Status == "Completed" && task.Bucket != "completed":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "completed task should be in .agents/.tasks/completed")
		case task.Status == "Cancelled" && task.Bucket != "cancelled":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "cancelled task should be in .agents/.tasks/cancelled")
		case task.Status != "Completed" && task.Status != "Cancelled" && task.Bucket != "active":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "active task status should be in .agents/.tasks/active")
		}
	}
}

func validateTaskExecPlans(root string, tasks []Task, report *validationReport) {
	for _, task := range tasks {
		if task.ExecPlan == "" || task.ExecPlan == "-" {
			continue
		}
		plan, bucket, ok := resolveExecPlanReference(root, task.ExecPlan)
		if !ok {
			report.addWarning("task_exec_plan_missing", relPath(root, task.Path), fmt.Sprintf("task %s references missing ExecPlan %s", task.ID, task.ExecPlan))
			continue
		}
		if task.Status == "Completed" && bucket == "active" {
			report.addWarning("task_completed_exec_plan_active", relPath(root, task.Path), fmt.Sprintf("completed task %s references active ExecPlan %s", task.ID, relPath(root, plan)))
			continue
		}
		if task.Status == "Completed" && bucket == "completed" && !execPlanHasRetrospective(plan) {
			report.addWarning("task_completed_exec_plan_incomplete", relPath(root, task.Path), fmt.Sprintf("completed task %s references ExecPlan without a completed Outcomes & Retrospective section", task.ID))
		}
	}
}

func resolveExecPlanReference(root string, ref string) (string, string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "-" {
		return "", "", false
	}
	var candidates []string
	if filepath.IsAbs(ref) {
		candidates = append(candidates, ref)
	} else {
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(ref)))
		for _, bucket := range []string{"active", "completed"} {
			candidates = append(candidates, filepath.Join(root, ".agents", "exec-plans", bucket, filepath.FromSlash(ref)))
		}
	}
	if filepath.Ext(ref) == "" {
		var withExt []string
		for _, candidate := range candidates {
			withExt = append(withExt, candidate+".md")
		}
		candidates = append(candidates, withExt...)
	}
	for _, candidate := range candidates {
		stat, err := os.Stat(candidate)
		if err != nil || stat.IsDir() {
			continue
		}
		rel := relPath(root, candidate)
		switch {
		case strings.HasPrefix(rel, ".agents/exec-plans/active/"):
			return candidate, "active", true
		case strings.HasPrefix(rel, ".agents/exec-plans/completed/"):
			return candidate, "completed", true
		default:
			return candidate, "", true
		}
	}
	return "", "", false
}

func execPlanHasRetrospective(path string) bool {
	data, err := readWorkflowFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "## Outcomes & Retrospective") {
			for _, following := range lines[i+1:] {
				trimmed := strings.TrimSpace(following)
				if strings.HasPrefix(trimmed, "#") {
					return false
				}
				if trimmed != "" {
					return true
				}
			}
			return false
		}
	}
	return false
}

func validateGeneratedIndexes(root string, report *validationReport) {
	if _, err := readMetadata(root); err != nil {
		return
	}
	indexer := app{opts: options{root: root}}
	writes, err := indexer.indexWrites()
	if err != nil {
		report.addWarning("generated_index_check_failed", "", err.Error())
		return
	}
	for _, path := range sortedKeys(writes) {
		data, err := readWorkflowFile(path)
		if errors.Is(err, os.ErrNotExist) {
			report.addError("generated_index_missing", relPath(root, path), "generated index is missing; run ahm index")
			continue
		}
		if err != nil {
			report.addError("generated_index_unreadable", relPath(root, path), err.Error())
			continue
		}
		if string(data) != writes[path] {
			report.addWarning("generated_index_stale", relPath(root, path), "generated index is stale; run ahm index")
		}
	}
}

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
var markdownLinkSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

func validateMarkdownLinks(root string, report *validationReport) {
	if _, err := readMetadata(root); err != nil {
		return
	}
	for _, path := range workflowMarkdownFiles(root) {
		validateMarkdownFileLinks(root, path, report)
	}
}

func workflowMarkdownFiles(root string) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}
	for _, item := range templates.Files() {
		if item.CreateOnly {
			continue
		}
		target := filepath.Join(root, item.Target)
		if stat, err := os.Stat(target); err == nil && !stat.IsDir() && strings.HasSuffix(target, ".md") {
			add(target)
		}
	}
	agentsRoot := filepath.Join(root, ".agents")
	_ = filepath.WalkDir(agentsRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		add(path)
		return nil
	})
	sort.Strings(paths)
	return paths
}

func validateMarkdownFileLinks(root string, path string, report *validationReport) {
	data, err := readWorkflowFile(path)
	if err != nil {
		report.addWarning("markdown_link_check_failed", relPath(root, path), err.Error())
		return
	}
	inFence := false
	for lineNo, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(line, -1) {
			target := normalizeMarkdownLinkTarget(match[1])
			if target == "" || shouldSkipMarkdownLink(target) {
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
			if _, err := os.Stat(resolved); errors.Is(err, os.ErrNotExist) {
				report.addWarning("markdown_link_missing", fmt.Sprintf("%s:%d", relPath(root, path), lineNo+1), fmt.Sprintf("relative Markdown link target does not exist: %s", target))
			} else if err != nil {
				report.addWarning("markdown_link_check_failed", fmt.Sprintf("%s:%d", relPath(root, path), lineNo+1), err.Error())
			}
		}
	}
}

func normalizeMarkdownLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "<") {
		if end := strings.Index(target, ">"); end >= 0 {
			target = target[1:end]
		}
	} else if fields := strings.Fields(target); len(fields) > 0 {
		target = fields[0]
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func shouldSkipMarkdownLink(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") || filepath.IsAbs(target) {
		return true
	}
	if strings.HasPrefix(target, "mailto:") {
		return true
	}
	return markdownLinkSchemePattern.MatchString(target)
}

func taskDependencyCycles(tasks []Task) [][]string {
	byID := map[string]Task{}
	for _, task := range tasks {
		if task.Status != "Completed" && task.Status != "Cancelled" {
			byID[task.ID] = task
		}
	}
	var cycles [][]string
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var dfs func(string, []string)
	dfs = func(id string, path []string) {
		if visiting[id] {
			start := 0
			for i, item := range path {
				if item == id {
					start = i
					break
				}
			}
			cycles = append(cycles, append(path[start:], id))
			return
		}
		if visited[id] {
			return
		}
		task, ok := byID[id]
		if !ok {
			return
		}
		visiting[id] = true
		for _, dep := range task.DependsOn {
			dfs(dep, append(path, id))
		}
		visiting[id] = false
		visited[id] = true
	}
	// Sort keys for deterministic traversal order.
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		dfs(id, nil)
	}
	return cycles
}

func (r *validationReport) addError(code string, path string, message string) {
	r.Errors = append(r.Errors, validationFinding{Code: code, Path: path, Message: message})
}

func (r *validationReport) addWarning(code string, path string, message string) {
	r.Warnings = append(r.Warnings, validationFinding{Code: code, Path: path, Message: message})
}

type agentsSuggestionsReport struct {
	Target      string                   `json:"target"`
	Exists      bool                     `json:"exists"`
	Suggestions []agentSuggestionWithHit `json:"suggestions"`
}

type agentSuggestionWithHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Present bool   `json:"present"`
}

func (a *app) agentsSuggestions(showAll bool) error {
	report, err := a.collectAgentSuggestions()
	if err != nil {
		return err
	}
	if a.opts.json {
		return a.emit(report)
	}

	selected := report.Suggestions
	if !showAll {
		selected = nil
		for _, suggestion := range report.Suggestions {
			if !suggestion.Present {
				selected = append(selected, suggestion)
			}
		}
	}

	fmt.Fprintln(a.out, "# Suggested AGENTS.md Additions")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "These are advisory snippets from `ahm`. Review and adapt them before adding them")
	fmt.Fprintln(a.out, "to an existing project-owned AGENTS.md.")
	if len(selected) == 0 {
		fmt.Fprintln(a.out)
		fmt.Fprintln(a.out, "No missing suggestions detected.")
		return nil
	}
	for _, suggestion := range selected {
		fmt.Fprintln(a.out)
		fmt.Fprintf(a.out, "## %s\n\n", suggestion.Title)
		if showAll && suggestion.Present {
			fmt.Fprintln(a.out, "_Already appears present in AGENTS.md._")
			fmt.Fprintln(a.out)
		}
		fmt.Fprintln(a.out, suggestion.Body)
	}
	return nil
}

func (a *app) collectAgentSuggestions() (agentsSuggestionsReport, error) {
	target := filepath.Join(a.opts.root, "AGENTS.md")
	existing, err := os.ReadFile(target)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return agentsSuggestionsReport{}, err
	}

	content := strings.ReplaceAll(string(existing), "\r\n", "\n")
	report := agentsSuggestionsReport{
		Target: "AGENTS.md",
		Exists: exists,
	}
	for _, suggestion := range templates.AgentSuggestions() {
		body := strings.ReplaceAll(suggestion.Body, "\r\n", "\n")
		report.Suggestions = append(report.Suggestions, agentSuggestionWithHit{
			ID:      suggestion.ID,
			Title:   suggestion.Title,
			Body:    suggestion.Body,
			Present: exists && strings.Contains(content, body),
		})
	}
	return report, nil
}

func relPath(root string, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (a *app) emit(value any) error {
	if a.opts.json {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	if m, ok := value.(map[string][]string); ok {
		for _, key := range []string{"created", "updated", "skipped", "conflicts", "directories", "metadata", "indexes"} {
			items := m[key]
			if len(items) == 0 {
				continue
			}
			fmt.Fprintf(a.out, "%s:\n", key)
			for _, item := range items {
				fmt.Fprintf(a.out, "  %s\n", item)
			}
		}
		return nil
	}
	if a.opts.plain {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, string(data))
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.out, string(data))
	return err
}

// taskFileInfo holds the path and bucket for a task markdown file.
type taskFileInfo struct {
	Path   string
	Bucket string
}

// taskFilePaths collects all task markdown file paths across the
// active, completed, and cancelled buckets. It skips index.md and
// non-.md entries. Directories that do not exist are silently skipped.
func taskFilePaths(root string) ([]taskFileInfo, error) {
	var files []taskFileInfo
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := filepath.Join(root, ".agents", ".tasks", bucket)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			files = append(files, taskFileInfo{
				Path:   filepath.Join(dir, entry.Name()),
				Bucket: bucket,
			})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// Task is the parsed representation of a workflow task file.
type Task struct {
	ID          string
	Title       string
	Status      string
	Priority    string
	Effort      string
	Labels      string
	ExecPlan    string
	DependsOn   []string
	Created     string
	Updated     string
	Parent      string
	ExternalRef string
	Extra       map[string]string // unknown front matter fields preserved from the original file
	Path        string
	Bucket      string
	Body        string
}

func collectTasks(root string) ([]Task, error) {
	files, err := taskFilePaths(root)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	for _, f := range files {
		task, err := parseTask(f.Path, f.Bucket)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	return tasks, nil
}

func parseTask(path string, bucket string) (Task, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return Task{}, err
	}
	text := string(data)
	meta, body, err := parseFrontMatter(text)
	if err != nil {
		return Task{}, err
	}
	id := meta["id"]
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	title := meta["title"]
	if title == "" {
		title = headingTitle(body, id)
	}
	body = stripHeading(body, title)
	task := Task{
		ID:          id,
		Title:       title,
		Status:      defaultDash(meta["status"]),
		Priority:    defaultDash(meta["priority"]),
		Effort:      defaultDash(meta["effort"]),
		Labels:      defaultDash(meta["labels"]),
		ExecPlan:    defaultDash(meta["exec_plan"]),
		DependsOn:   parseList(meta["depends_on"]),
		Created:     meta["created"],
		Updated:     meta["updated"],
		Parent:      meta["parent"],
		ExternalRef: meta["external_ref"],
		Extra:       metaExtra(meta),
		Path:        path,
		Bucket:      bucket,
		Body:        body,
	}
	if err := validateTaskEnums(task, path); err != nil {
		return Task{}, err
	}
	return task, nil
}

func parseFrontMatter(text string) (map[string]string, string, error) {
	meta := map[string]string{}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return meta, text, nil
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return meta, text, nil
	}
	raw := text[4 : 4+end]
	body := text[4+end+5:]
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.ContainsAny(key, " :") {
			return nil, "", fmt.Errorf("invalid front matter key %q", key)
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
			value = value[1 : len(value)-1]
			value = strings.TrimSpace(value)
		}
		if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
			return nil, "", fmt.Errorf("unsupported block scalar in front matter field %q", key)
		}
		meta[key] = value
	}
	return meta, body, nil
}

// metaExtra returns the subset of meta keys that are not known task fields.
func metaExtra(meta map[string]string) map[string]string {
	extra := map[string]string{}
	for k, v := range meta {
		switch k {
		case "id", "title", "status", "priority", "effort", "labels",
			"exec_plan", "depends_on", "created", "updated",
			"parent", "external_ref":
			// known field, skip
		default:
			extra[k] = v
		}
	}
	return extra
}

func headingTitle(body string, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallback
}

func stripHeading(body string, title string) string {
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "# "+title {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || value == "[]" {
		return nil
	}
	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" && item != "-" {
			out = append(out, item)
		}
	}
	return out
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

func defaultDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func taskLess(a string, b string) bool {
	an, as := splitTaskID(a)
	bn, bs := splitTaskID(b)
	if an != bn {
		return an < bn
	}
	return as < bs
}

func splitTaskID(id string) (int, string) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 {
		return 999999, id
	}
	n, _ := strconv.Atoi(id[:i])
	return n, id[i:]
}

func taskCounts(tasks []Task) map[string]int {
	counts := map[string]int{}
	for _, status := range statusOrder() {
		counts[status] = 0
	}
	for _, task := range tasks {
		counts[task.Status]++
	}
	return counts
}

func (a *app) taskCommand() *cobra.Command {
	task := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("task requires a subcommand")
		},
	}

	createArgs := taskCreateArgs{
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:cli",
		status:   "Pending",
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
	task.AddCommand(create)

	task.AddCommand(a.taskListCommand("list", []string{"ls"}, "List tasks", "all"))
	task.AddCommand(a.taskListCommand("ready", nil, "List ready tasks", "ready"))
	task.AddCommand(a.taskListCommand("blocked", nil, "List blocked tasks", "blocked"))
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

	for _, spec := range []struct {
		use     string
		aliases []string
		short   string
		status  string
	}{
		{use: "start <id>", short: "Mark a task in progress", status: "In Progress"},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", status: "Completed"},
		{use: "cancel <id>", short: "Mark a task cancelled", status: "Cancelled"},
		{use: "reopen <id>", short: "Reopen a task", status: "Pending"},
	} {
		status := spec.status
		task.AddCommand(&cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Args:    exactArgs(1, "task status command requires an id"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.taskStatus(args, status)
			},
		})
	}

	task.AddCommand(a.taskDepCommand())
	return task
}

func (a *app) taskListCommand(use string, aliases []string, short string, mode string) *cobra.Command {
	status := ""
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskList(mode, status)
		},
	}
	if mode == "all" {
		cmd.Flags().StringVar(&status, "status", "", "Filter tasks by status")
	}
	return cmd
}

func (a *app) taskDepCommand() *cobra.Command {
	dep := &cobra.Command{
		Use:   "dep",
		Short: "Manage task dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("task dep requires a subcommand")
		},
	}
	dep.AddCommand(a.taskDepUpdateCommand("add", nil, "Add a task dependency", true))
	dep.AddCommand(a.taskDepUpdateCommand("remove", []string{"rm"}, "Remove a task dependency", false))
	dep.AddCommand(&cobra.Command{
		Use:   "tree <id>",
		Short: "Print a task dependency tree",
		Args:  exactArgs(1, "task dep tree requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepTree(args)
		},
	})
	dep.AddCommand(&cobra.Command{
		Use:   "cycles",
		Short: "Print dependency cycles",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepCycles()
		},
	})
	return dep
}

func (a *app) taskDepUpdateCommand(use string, aliases []string, short string, add bool) *cobra.Command {
	return &cobra.Command{
		Use:     use + " <id> <dependency-id>",
		Aliases: aliases,
		Short:   short,
		Args:    exactArgs(2, "task dep add/remove requires task id and dependency id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepUpdate(args, add)
		},
	}
}

func exactArgs(count int, message string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return usageError(message)
		}
		return nil
	}
}

func (a *app) taskCreateParsed(parsed taskCreateArgs) error {
	if parsed.title == "" {
		return usageError("task create requires a title")
	}
	if err := validateTaskCreateEnums(parsed); err != nil {
		return err
	}
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	id := nextTaskID(tasks)
	path := filepath.Join(a.opts.root, ".agents", ".tasks", "active", id+".md")
	body := parsed.description
	if body == "" {
		body = "TODO."
	}
	content := renderTask(Task{
		ID:       id,
		Title:    parsed.title,
		Status:   parsed.status,
		Priority: parsed.priority,
		Effort:   parsed.effort,
		Labels:   parsed.labels,
		ExecPlan: "-",
		Body:     "## Summary\n\n" + body + "\n\n## Acceptance Notes\n\n- [ ] TODO\n",
	})
	if a.opts.dryRun {
		return a.emit(map[string]any{"create": path, "id": id})
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

type taskCreateArgs struct {
	title       string
	priority    string
	effort      string
	labels      string
	status      string
	description string
}

type taskMigration struct {
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
}

func (a *app) taskMigrate() error {
	paths, err := taskMarkdownPaths(a.opts.root)
	if err != nil {
		return err
	}
	var migrations []taskMigration
	writes := map[string]string{}
	for _, path := range paths {
		data, err := readWorkflowFile(path)
		if err != nil {
			return err
		}
		next, changes := migrateTaskFrontMatter(string(data))
		if len(changes) == 0 {
			continue
		}
		rel := relPath(a.opts.root, path)
		migrations = append(migrations, taskMigration{Path: rel, Changes: changes})
		writes[path] = next
	}
	if a.opts.json || a.opts.plain {
		return a.emit(map[string]any{"migrations": migrations})
	}
	if a.opts.dryRun {
		if len(migrations) == 0 {
			fmt.Fprintln(a.out, "No task migrations found")
			return nil
		}
		fmt.Fprintln(a.out, "migrations:")
		for _, migration := range migrations {
			fmt.Fprintf(a.out, "  %s:\n", migration.Path)
			for _, change := range migration.Changes {
				fmt.Fprintf(a.out, "    - %s\n", change)
			}
		}
		return nil
	}
	for _, path := range sortedKeys(writes) {
		if err := writeFileAtomic(path, []byte(writes[path]), 0o644); err != nil {
			return err
		}
	}
	if len(writes) > 0 {
		if err := a.writeIndexes(); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.out, "migrated %d task files\n", len(writes))
	return nil
}

func taskMarkdownPaths(root string) ([]string, error) {
	files, err := taskFilePaths(root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths, nil
}

func migrateTaskFrontMatter(text string) (string, []string) {
	raw, body, ok := splitFrontMatter(text)
	if !ok {
		return text, nil
	}
	lines := strings.Split(raw, "\n")
	index := frontMatterLineIndex(lines)
	var changes []string
	if _, ok := index["labels"]; !ok {
		lines = insertFrontMatterField(lines, index, "labels", "type:task, area:unknown", "effort")
		index = frontMatterLineIndex(lines)
		changes = append(changes, "add labels")
	}
	changes = append(changes, normalizeEnumField(lines, index, "priority", "P3", priorityOrder())...)
	changes = append(changes, normalizeEnumField(lines, index, "effort", "M", effortOrder())...)
	if i, ok := index["depends_on"]; ok {
		oldLine := lines[i]
		oldValue := frontMatterValue(lines[i])
		if next, ok := normalizeDependsOnValue(oldValue); ok && oldLine != "depends_on: "+next {
			lines[i] = "depends_on: " + next
			changes = append(changes, "normalize depends_on")
		}
	}
	if len(changes) == 0 {
		return text, nil
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + body, changes
}

func splitFrontMatter(text string) (string, string, bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", text, false
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", text, false
	}
	return text[4 : 4+end], text[4+end+5:], true
}

func frontMatterLineIndex(lines []string) map[string]int {
	index := map[string]int{}
	for i, line := range lines {
		key, _, ok := strings.Cut(line, ":")
		if ok {
			index[strings.TrimSpace(key)] = i
		}
	}
	return index
}

func insertFrontMatterField(lines []string, index map[string]int, field string, value string, after string) []string {
	line := field + ": " + value
	if i, ok := index[after]; ok {
		out := append([]string{}, lines[:i+1]...)
		out = append(out, line)
		return append(out, lines[i+1:]...)
	}
	return append(lines, line)
}

func normalizeEnumField(lines []string, index map[string]int, field string, placeholder string, allowed []string) []string {
	i, ok := index[field]
	if !ok {
		return nil
	}
	value := frontMatterValue(lines[i])
	if value == "-" || value == "[]" {
		lines[i] = field + ": " + placeholder
		return []string{"set " + field + " placeholder to " + placeholder}
	}
	for _, item := range allowed {
		if value == item {
			return nil
		}
		if strings.HasPrefix(value, item+" ") || strings.HasPrefix(value, item+"(") {
			lines[i] = field + ": " + item
			return []string{"normalize " + field + " to " + item}
		}
	}
	return nil
}

func frontMatterValue(line string) string {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
		value = value[1 : len(value)-1]
		value = strings.TrimSpace(value)
	}
	return value
}

var leadingTaskIDPattern = regexp.MustCompile(`^([0-9]+[A-Za-z]?)\b`)
var followsTaskIDPattern = regexp.MustCompile(`(?i)^follows\s+([0-9]+[A-Za-z]?)\.?$`)
var completedByTaskIDPattern = regexp.MustCompile(`(?i)^completed by\s+([0-9]+[A-Za-z]?)\.?$`)

func normalizeDependsOnValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
		value = value[1 : len(value)-1]
		value = strings.TrimSpace(value)
	}
	if value == "" || value == "-" || value == "[]" {
		return "-", true
	}
	if match := followsTaskIDPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1], true
	}
	if match := completedByTaskIDPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1], true
	}
	for _, prefix := range []string{"Closed as obsolete:", "From code review", "Resolved in same PR", "Research:"} {
		if strings.HasPrefix(value, prefix) {
			return "-", true
		}
	}
	parts := splitTopLevelCommas(strings.Trim(value, "[]"))
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		match := leadingTaskIDPattern.FindStringSubmatch(item)
		if len(match) != 2 {
			return value, false
		}
		ids = append(ids, match[1])
	}
	if len(ids) == 0 {
		return "-", true
	}
	next := strings.Join(ids, ", ")
	return next, next != value
}

func splitTopLevelCommas(value string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, value[start:])
	return parts
}

func nextTaskID(tasks []Task) string {
	maxID := 0
	for _, task := range tasks {
		n, suffix := splitTaskID(task.ID)
		if suffix == "" && n < 999999 && n > maxID {
			maxID = n
		}
	}
	return fmt.Sprintf("%03d", maxID+1)
}

func renderTask(task Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "id: %s\n", task.ID)
	fmt.Fprintf(&b, "title: %s\n", task.Title)
	fmt.Fprintf(&b, "status: %s\n", task.Status)
	fmt.Fprintf(&b, "priority: %s\n", task.Priority)
	fmt.Fprintf(&b, "effort: %s\n", task.Effort)
	fmt.Fprintf(&b, "labels: %s\n", task.Labels)
	fmt.Fprintf(&b, "exec_plan: %s\n", defaultDash(task.ExecPlan))
	fmt.Fprintf(&b, "depends_on: %s\n", formatList(task.DependsOn))
	if task.Created != "" {
		fmt.Fprintf(&b, "created: %s\n", task.Created)
	}
	if task.Updated != "" {
		fmt.Fprintf(&b, "updated: %s\n", task.Updated)
	}
	if task.Parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", task.Parent)
	}
	if task.ExternalRef != "" {
		fmt.Fprintf(&b, "external_ref: %s\n", task.ExternalRef)
	}
	for _, k := range sortedKeys(task.Extra) {
		fmt.Fprintf(&b, "%s: %s\n", k, task.Extra[k])
	}
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "# %s\n\n", task.Title)
	body := strings.TrimSpace(task.Body)
	if body != "" {
		fmt.Fprintln(&b, body)
		fmt.Fprintln(&b)
	}
	return b.String()
}

func (a *app) taskList(mode string, status string) error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	filtered := filterTasks(tasks, mode)
	if status != "" {
		normalized, err := normalizeTaskStatus(status)
		if err != nil {
			return err
		}
		filtered = filterTasksByStatus(filtered, normalized)
	}
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		a.printTaskLine(task)
	}
	return nil
}

func (a *app) taskNext() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
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

func filterTasksByStatus(tasks []Task, status string) []Task {
	var out []Task
	for _, task := range tasks {
		if task.Status == status {
			out = append(out, task)
		}
	}
	return out
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
	if len(argv) != 1 {
		return usageError("task show requires an id")
	}
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
	if len(argv) != 1 {
		return usageError("task status command requires an id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	task.Status = status
	bucket := "active"
	if status == "Completed" {
		bucket = "completed"
	}
	if status == "Cancelled" {
		bucket = "cancelled"
	}
	target := filepath.Join(a.opts.root, ".agents", ".tasks", bucket, task.ID+".md")
	if a.opts.dryRun {
		return a.emit(map[string]any{"move": target, "status": status})
	}
	if err := writeFileAtomic(target, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if filepath.Clean(task.Path) != filepath.Clean(target) {
		if err := os.Remove(task.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s -> %s\n", task.ID, status)
	return nil
}

func (a *app) resolveTask(pattern string) (Task, error) {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return Task{}, err
	}
	// Exact match returns immediately.
	for _, task := range tasks {
		if task.ID == pattern {
			return task, nil
		}
	}
	// Constrained prefix matching: parse the numeric prefix so that "1"
	// matches "001", "01" matches "001", and "1a" matches "001a".
	patNum, patSuffix := splitTaskID(pattern)
	var matches []Task
	for _, task := range tasks {
		taskNum, taskSuffix := splitTaskID(task.ID)
		if taskNum == patNum && strings.HasPrefix(taskSuffix, patSuffix) {
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

func (a *app) taskDepUpdate(argv []string, add bool) error {
	if len(argv) != 2 {
		return usageError("task dep add/remove requires task id and dependency id")
	}
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	dep, err := a.resolveTask(argv[1])
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, item := range task.DependsOn {
		set[item] = true
	}
	if add {
		set[dep.ID] = true
	} else {
		delete(set, dep.ID)
	}
	task.DependsOn = nil
	for item := range set {
		task.DependsOn = append(task.DependsOn, item)
	}
	sort.Slice(task.DependsOn, func(i, j int) bool {
		return taskLess(task.DependsOn[i], task.DependsOn[j])
	})
	if a.opts.dryRun {
		return a.emit(map[string]any{"task": task.ID, "depends_on": task.DependsOn})
	}
	if err := writeFileAtomic(task.Path, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s depends_on: %s\n", task.ID, formatList(task.DependsOn))
	return nil
}

func (a *app) taskDepTree(argv []string) error {
	if len(argv) != 1 {
		return usageError("task dep tree requires an id")
	}
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	root, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	var walk func(id string, prefix string, seen map[string]bool)
	walk = func(id string, prefix string, seen map[string]bool) {
		task, ok := byID[id]
		if !ok {
			fmt.Fprintf(a.out, "%s%s [missing]\n", prefix, id)
			return
		}
		fmt.Fprintf(a.out, "%s%s [%s] %s\n", prefix, task.ID, task.Status, task.Title)
		if seen[id] {
			fmt.Fprintf(a.out, "%s  cycle to %s\n", prefix, id)
			return
		}
		nextSeen := map[string]bool{}
		for k, v := range seen {
			nextSeen[k] = v
		}
		nextSeen[id] = true
		for _, dep := range task.DependsOn {
			walk(dep, prefix+"  ", nextSeen)
		}
	}
	walk(root.ID, "", map[string]bool{})
	return nil
}

func (a *app) taskDepCycles() error {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return err
	}
	cycles := taskDependencyCycles(tasks)
	if len(cycles) == 0 {
		fmt.Fprintln(a.out, "No dependency cycles found")
		return nil
	}
	for _, cycle := range cycles {
		fmt.Fprintln(a.out, strings.Join(cycle, " -> "))
	}
	return nil
}

func (a *app) writeIndexes() error {
	if !a.opts.dryRun {
		if err := cleanupStaleTemps(a.opts.root); err != nil {
			return err
		}
	}
	writes, err := a.indexWrites()
	if err != nil {
		return err
	}
	paths := sortedKeys(writes)
	for _, path := range paths {
		if a.opts.dryRun {
			if isStaleIndex(path, writes[path]) {
				fmt.Fprintln(a.out, relPath(a.opts.root, path))
			}
			continue
		}
		if err := writeFileAtomic(path, []byte(writes[path]), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// isStaleIndex returns true when the file at path is missing or its content
// differs from want. It is used by dry-run index to report only generated
// indexes that would actually be rewritten.
func isStaleIndex(path string, want string) bool {
	data, err := readWorkflowFile(path)
	if err != nil {
		// File missing or unreadable — stale.
		return true
	}
	return string(data) != want
}

func (a *app) indexWriteTargets() ([]string, error) {
	writes, err := a.indexWrites()
	if err != nil {
		return nil, err
	}
	paths := sortedKeys(writes)
	targets := make([]string, 0, len(paths))
	for _, path := range paths {
		targets = append(targets, relPath(a.opts.root, path))
	}
	return targets, nil
}

func (a *app) indexWrites() (map[string]string, error) {
	tasks, err := collectTasks(a.opts.root)
	if err != nil {
		return nil, err
	}
	research, err := collectMarkdownDocs(a.opts.root, ".agents/.research", []string{"inbox", "investigations", "sources", "topics", "archived"})
	if err != nil {
		return nil, err
	}
	activePlans, err := collectMarkdownDocs(a.opts.root, ".agents/exec-plans/active", []string{""})
	if err != nil {
		return nil, err
	}
	completedPlans, err := collectMarkdownDocs(a.opts.root, ".agents/exec-plans/completed", []string{""})
	if err != nil {
		return nil, err
	}
	return map[string]string{
		filepath.Join(a.opts.root, ".agents", ".tasks", "index.md"):                  renderRootIndex(tasks),
		filepath.Join(a.opts.root, ".agents", ".tasks", "active", "index.md"):        renderBucketIndex(tasks, "active"),
		filepath.Join(a.opts.root, ".agents", ".tasks", "completed", "index.md"):     renderBucketIndex(tasks, "completed"),
		filepath.Join(a.opts.root, ".agents", ".tasks", "cancelled", "index.md"):     renderBucketIndex(tasks, "cancelled"),
		filepath.Join(a.opts.root, ".agents", ".research", "index.md"):               renderResearchIndex(research),
		filepath.Join(a.opts.root, ".agents", "exec-plans", "active", "index.md"):    renderExecPlanIndex("Active ExecPlans", "No active ExecPlans yet.", activePlans),
		filepath.Join(a.opts.root, ".agents", "exec-plans", "completed", "index.md"): renderExecPlanIndex("Completed ExecPlans", "No completed ExecPlans yet.", completedPlans),
	}, nil
}

func generatedIndexTargets() []string {
	return []string{
		".agents/.tasks/index.md",
		".agents/.tasks/active/index.md",
		".agents/.tasks/completed/index.md",
		".agents/.tasks/cancelled/index.md",
		".agents/.research/index.md",
		".agents/exec-plans/active/index.md",
		".agents/exec-plans/completed/index.md",
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderRootIndex(tasks []Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Task Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Status Summary")
	fmt.Fprintln(&b)
	counts := taskCounts(tasks)
	for _, status := range statusOrder() {
		fmt.Fprintf(&b, "- %s: %d\n", status, counts[status])
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Next Ready Queue")
	fmt.Fprintln(&b)
	ready := filterTasks(tasks, "ready")
	if len(ready) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		for i, task := range ready {
			fmt.Fprintf(&b, "%d. [%s](active/%s.md) - %s (%s, %s; %s)\n", i+1, task.ID, task.ID, task.Title, task.Priority, task.Effort, task.Labels)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Blocked Or Needs Triage")
	fmt.Fprintln(&b)
	waiting := filterTasks(tasks, "blocked")
	for _, task := range tasks {
		if task.Status == "Open" {
			waiting = append(waiting, task)
		}
	}
	if len(waiting) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, waiting, ".")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## All Tasks")
	fmt.Fprintln(&b)
	writeTaskTable(&b, tasks, ".")
	return b.String()
}

type markdownDoc struct {
	Link  string
	Title string
}

func collectMarkdownDocs(root string, base string, buckets []string) (map[string][]markdownDoc, error) {
	docs := map[string][]markdownDoc{}
	for _, bucket := range buckets {
		docs[bucket] = nil
		dir := filepath.Join(root, base, bucket)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			title, err := markdownTitle(path)
			if err != nil {
				return nil, err
			}
			link := entry.Name()
			if bucket != "" {
				link = filepath.ToSlash(filepath.Join(bucket, entry.Name()))
			}
			docs[bucket] = append(docs[bucket], markdownDoc{Link: link, Title: title})
		}
		sort.Slice(docs[bucket], func(i, j int) bool {
			return docs[bucket][i].Link < docs[bucket][j].Link
		})
	}
	return docs, nil
}

func markdownTitle(path string) (string, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			title = strings.TrimSpace(title)
			if title != "" {
				return title, nil
			}
		}
	}
	name := filepath.Base(path)
	return strings.TrimSuffix(name, filepath.Ext(name)), nil
}

func renderResearchIndex(docs map[string][]markdownDoc) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Research Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	writeDocSection(&b, "Inbox", "No inbox research documents yet.", docs["inbox"])
	writeDocSection(&b, "Investigations", "No investigation documents yet.", docs["investigations"])
	writeDocSection(&b, "Sources", "No source notes yet.", docs["sources"])
	writeDocSection(&b, "Topics", "No topic documents yet.", docs["topics"])
	writeDocSection(&b, "Archived", "No archived research documents yet.", docs["archived"])
	return b.String()
}

func renderExecPlanIndex(title string, empty string, docs map[string][]markdownDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	writeDocList(&b, empty, docs[""])
	return b.String()
}

func writeDocSection(b *strings.Builder, title string, empty string, docs []markdownDoc) {
	fmt.Fprintf(b, "## %s\n\n", title)
	writeDocList(b, empty, docs)
	fmt.Fprintln(b)
}

func writeDocList(b *strings.Builder, empty string, docs []markdownDoc) {
	if len(docs) == 0 {
		fmt.Fprintln(b, empty)
		return
	}
	for _, doc := range docs {
		fmt.Fprintf(b, "- [%s](%s)\n", escapeCell(doc.Title), doc.Link)
	}
}

func renderBucketIndex(tasks []Task, bucket string) string {
	var b strings.Builder
	title := bucketTitle(bucket)
	fmt.Fprintf(&b, "# %s Tasks\n\n", title)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	var bucketTasks []Task
	for _, task := range tasks {
		if task.Bucket == bucket {
			bucketTasks = append(bucketTasks, task)
		}
	}
	if len(bucketTasks) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, bucketTasks, bucket)
	}
	return b.String()
}

func writeTaskTable(b *strings.Builder, tasks []Task, from string) {
	fmt.Fprintln(b, "| Task | Title | Status | Priority | Effort | Labels | ExecPlan | Depends on |")
	fmt.Fprintln(b, "| ---- | ----- | ------ | -------- | ------ | ------ | -------- | ---------- |")
	for _, task := range tasks {
		link := task.ID + ".md"
		if from == "." {
			link = filepath.ToSlash(filepath.Join(task.Bucket, task.ID+".md"))
		}
		fmt.Fprintf(
			b,
			"| [%s](%s) | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeCell(task.ID),
			link,
			escapeCell(task.Title),
			escapeCell(task.Status),
			escapeCell(task.Priority),
			escapeCell(task.Effort),
			escapeCell(task.Labels),
			escapeCell(task.ExecPlan),
			escapeCell(formatList(task.DependsOn)),
		)
	}
}

func escapeCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func bucketTitle(bucket string) string {
	if bucket == "" {
		return bucket
	}
	return strings.ToUpper(bucket[:1]) + bucket[1:]
}

func statusOrder() []string {
	return []string{"Open", "Pending", "In Progress", "Blocked", "Tracking", "Completed", "Cancelled"}
}

func priorityOrder() []string {
	return []string{"P0", "P1", "P2", "P3", "P4"}
}

func effortOrder() []string {
	return []string{"XS", "S", "M", "L", "XL"}
}
