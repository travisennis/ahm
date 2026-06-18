package ahm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

type validationReport struct {
	OK       bool                `json:"ok"`
	Errors   []validationFinding `json:"errors"`
	Warnings []validationFinding `json:"warnings"`
	Info     []validationFinding `json:"info"`

	// execPlanSectionsCache memoizes parsed exec-plan sections per path
	// within a single validation run so that each plan file is parsed at
	// most once. It is scoped to the report so concurrent validation runs
	// and tests do not share state.
	execPlanSectionsCache map[string]map[string]execPlanSection
}

type validationFinding struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// CheckScope values for validation scopes.
const (
	CheckScopeWorkflow    = "workflow"
	CheckScopeLinks       = "links"
	CheckScopeProjectDocs = "project-docs"
)

// validCheckScopes returns the list of recognised check-scope values.
func validCheckScopes() []string {
	return []string{CheckScopeWorkflow, CheckScopeLinks, CheckScopeProjectDocs}
}

func validateWorkflow(root string) (validationReport, []Task) {
	return validateWorkflowScoped(root, nil)
}

// validateWorkflowScoped runs only the validation groups named in scopes.
// When scopes is nil or empty, all validators run (same as validateWorkflow).
func validateWorkflowScoped(root string, scopes []string) (validationReport, []Task) {
	report := validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}}

	all := len(scopes) == 0
	want := func(s string) bool { return all || containsScope(scopes, s) }

	var tasks []Task
	if want(CheckScopeWorkflow) {
		tasks = validateManagedFiles(root, &report)
		validateTaskDependencies(root, tasks, &report)
		validateTaskBuckets(root, tasks, &report)
		validateTaskExecPlans(root, tasks, &report)
		validateExecPlans(root, tasks, &report)
		validateADRs(root, &report)
		validateGeneratedIndexes(root, tasks, &report)
	}
	if want(CheckScopeLinks) {
		validateMarkdownLinks(root, &report)
	}
	// project-docs is opt-in only: it never runs as part of the default
	// (all) scope, so default status/doctor behavior is unchanged. It runs
	// only when --check project-docs is requested explicitly.
	if containsScope(scopes, CheckScopeProjectDocs) {
		validateProjectDocs(root, &report)
	}

	report.OK = len(report.Errors) == 0
	return report, tasks
}

func containsScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

func validateManagedFiles(root string, report *validationReport) []Task {
	meta, metaErr := readMetadata(root)
	if metaErr != nil {
		if errors.Is(metaErr, os.ErrNotExist) {
			report.addError("metadata_missing", ".agents/ahm.json", "workflow metadata is missing")
		} else {
			report.addError("metadata_corrupt", ".agents/ahm.json", fmt.Sprintf("workflow metadata is corrupt: %v", metaErr))
		}
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
		report.addWarning("managed_file_untracked", item.Target, "managed workflow file is not recorded in metadata; run 'ahm init' to adopt")
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
		data, err := readWorkflowFile(f.Path)
		if err != nil {
			if os.IsNotExist(err) {
				// Task file was already moved or deleted; not an error.
				continue
			}
			report.addError("task_unreadable", relPath(root, f.Path), err.Error())
			continue
		}
		validateTaskFrontMatter(data, relPath(root, f.Path), report)
		task, err := parseTaskFromData(data, f.Path, f.Bucket)
		if err != nil {
			report.addError("task_malformed", relPath(root, f.Path), err.Error())
			continue
		}
		validateTaskAcceptance(root, task, report)
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	return tasks
}

func validateTaskAcceptance(root string, task Task, report *validationReport) {
	if task.Status != "Completed" {
		return
	}
	for _, finding := range parseAcceptanceNotes([]byte(task.Body)) {
		report.addWarning(finding.validationCode(), relPath(root, task.Path), finding.message(task.ID))
	}
}

func validateTaskFrontMatter(data []byte, relPath string, report *validationReport) {
	meta, _, err := parseFrontMatter(string(data))
	if err != nil {
		report.addError("task_malformed", relPath, err.Error())
		return
	}
	required := []string{"id", "title", "status", "priority", "effort", "labels", "exec_plan", "depends_on"}
	for _, field := range required {
		if strings.TrimSpace(meta[field]) == "" {
			report.addError("task_missing_field", relPath, "task front matter is missing "+field)
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
	for _, task := range tasks {
		if task.Status == "Completed" || task.Status == "Cancelled" {
			continue
		}
		for _, dep := range task.DependsOn {
			depTask, ok := byID[dep]
			if ok && depTask.Status == "Cancelled" {
				report.addWarning("task_dependency_cancelled", relPath(root, task.Path), fmt.Sprintf("task %s depends on cancelled task %s", task.ID, dep))
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
		if task.Status == "Completed" && bucket == "completed" && !execPlanHasRetrospective(plan, report) {
			report.addWarning("task_completed_exec_plan_incomplete", relPath(root, task.Path), fmt.Sprintf("completed task %s references ExecPlan without a completed Outcomes & Retrospective section", task.ID))
		}
	}
}

var mandatoryExecPlanSections = []string{
	"Progress",
	"Surprises & Discoveries",
	"Decision Log",
	"Outcomes & Retrospective",
}

func validateExecPlans(root string, tasks []Task, report *validationReport) {
	referenced := referencedExecPlans(root, tasks)
	for _, bucket := range []string{"active", "completed"} {
		dir := filepath.Join(root, ".agents", "exec-plans", bucket)
		plans, err := execPlanPaths(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			continue
		}
		for _, path := range plans {
			sections, err := report.getCachedExecPlanSections(path)
			if err != nil {
				continue
			}
			validateExecPlanSections(root, path, bucket, sections, report)
			if !referenced[filepath.Clean(path)] {
				report.addInfo("exec_plan_orphan", relPath(root, path), "ExecPlan is not referenced by any task exec_plan field")
			}
		}
	}
}

func referencedExecPlans(root string, tasks []Task) map[string]bool {
	referenced := map[string]bool{}
	for _, task := range tasks {
		if task.ExecPlan == "" || task.ExecPlan == "-" {
			continue
		}
		plan, _, ok := resolveExecPlanReference(root, task.ExecPlan)
		if ok {
			referenced[filepath.Clean(plan)] = true
		}
	}
	return referenced
}

func execPlanPaths(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

type execPlanSection struct {
	Lines []string
}

func parseExecPlanSections(path string) (map[string]execPlanSection, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return nil, err
	}
	sections := map[string]execPlanSection{}
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		if isExecPlanHeading(line) {
			current = ""
		}
		heading, ok := execPlanSectionHeading(line)
		if ok {
			current = normalizedExecPlanSection(heading)
			sections[current] = execPlanSection{}
			continue
		}
		if current != "" {
			section := sections[current]
			section.Lines = append(section.Lines, line)
			sections[current] = section
		}
	}
	return sections, nil
}

func execPlanSectionHeading(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	marker, heading, ok := strings.Cut(trimmed, " ")
	if !ok || marker != "##" && marker != "###" {
		return "", false
	}
	heading = strings.TrimSpace(heading)
	for _, section := range mandatoryExecPlanSections {
		if strings.EqualFold(heading, section) {
			return section, true
		}
	}
	return "", false
}

func isExecPlanHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "#")
}

func normalizedExecPlanSection(heading string) string {
	return strings.ToLower(heading)
}

func validateExecPlanSections(root string, path string, bucket string, sections map[string]execPlanSection, report *validationReport) {
	rel := relPath(root, path)
	for _, section := range mandatoryExecPlanSections {
		if _, ok := sections[normalizedExecPlanSection(section)]; !ok {
			report.addWarning("exec_plan_missing_section", rel, fmt.Sprintf("ExecPlan is missing mandatory section %q", section))
		}
	}

	outcomes := sections[normalizedExecPlanSection("Outcomes & Retrospective")]
	outcomesFilled := execPlanSectionHasBody(outcomes)
	if bucket == "active" && outcomesFilled {
		report.addWarning("exec_plan_active_with_outcomes", rel, "active ExecPlan has a filled Outcomes & Retrospective section")
	}
	if bucket == "completed" && !outcomesFilled {
		report.addWarning("exec_plan_completed_without_outcomes", rel, "completed ExecPlan has an empty or missing Outcomes & Retrospective section")
	}

	progress := sections[normalizedExecPlanSection("Progress")]
	if bucket == "completed" && execPlanSectionHasOpenProgress(progress) {
		report.addWarning("exec_plan_completed_with_open_progress", rel, "completed ExecPlan still has open progress items")
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

func (r *validationReport) getCachedExecPlanSections(path string) (map[string]execPlanSection, error) {
	if r.execPlanSectionsCache == nil {
		r.execPlanSectionsCache = map[string]map[string]execPlanSection{}
	}
	if sections, ok := r.execPlanSectionsCache[path]; ok {
		return sections, nil
	}
	sections, err := parseExecPlanSections(path)
	if err != nil {
		return nil, err
	}
	r.execPlanSectionsCache[path] = sections
	return sections, nil
}

func execPlanHasRetrospective(path string, report *validationReport) bool {
	sections, err := report.getCachedExecPlanSections(path)
	if err != nil {
		return false
	}
	return execPlanSectionHasBody(sections[normalizedExecPlanSection("Outcomes & Retrospective")])
}

func execPlanSectionHasBody(section execPlanSection) bool {
	for _, line := range section.Lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

func execPlanSectionHasOpenProgress(section execPlanSection) bool {
	for _, line := range section.Lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			return true
		}
	}
	return false
}

func validateGeneratedIndexes(root string, tasks []Task, report *validationReport) {
	if _, err := readMetadata(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", ".agents/ahm.json", fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
		return
	}
	writes, err := indexWritesFor(root, tasks)
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

func validateADRs(root string, report *validationReport) {
	adrs, _ := collectADRs(root)
	byID := map[string][]ADR{}
	for _, adr := range adrs {
		if adr.ID != "" {
			byID[adr.ID] = append(byID[adr.ID], adr)
		}
	}
	for id, matches := range byID {
		if len(matches) < 2 {
			continue
		}
		paths := make([]string, 0, len(matches))
		for _, adr := range matches {
			paths = append(paths, relPath(root, adr.Path))
		}
		sort.Strings(paths)
		report.addError("adr_duplicate_id", "", fmt.Sprintf("ADR ID %s is used by multiple files: %s", id, strings.Join(paths, ", ")))
	}

	for _, adr := range adrs {
		rel := relPath(root, adr.Path)
		switch adr.Kind {
		case adrKindMalformed:
			if strings.Contains(adr.ParseError, "does not match filename id") {
				report.addError("adr_id_mismatch", rel, adr.ParseError)
			} else {
				report.addError("adr_malformed", rel, adr.ParseError)
			}
			continue
		case adrKindLegacy:
			report.addWarning("adr_legacy_format", rel, "legacy ADR format; run ahm adr migrate")
			continue
		}

		status := strings.TrimSpace(adr.Status)
		if !validADRStatus(status) {
			report.addError("adr_invalid_status", rel, fmt.Sprintf("unsupported ADR status %q", adr.Status))
			continue
		}
		replacement, ok := strings.CutPrefix(status, "superseded by ADR-")
		if ok && len(byID[replacement]) == 0 {
			report.addError("adr_supersede_missing", rel, fmt.Sprintf("ADR %s is superseded by missing ADR-%s", adr.ID, replacement))
		}
	}
}

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
var markdownLinkSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

func validateMarkdownLinks(root string, report *validationReport) {
	if _, err := readMetadata(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", ".agents/ahm.json", fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
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
	validateMarkdownFileLinksWithCodes(root, path, report, "markdown_link_missing", "markdown_link_check_failed")
}

// validateMarkdownFileLinksWithCodes checks relative Markdown links in a single
// file and reports missing targets and check failures under the supplied codes.
func validateMarkdownFileLinksWithCodes(root string, path string, report *validationReport, missingCode string, failedCode string) {
	data, err := readWorkflowFile(path)
	if err != nil {
		report.addWarning(failedCode, relPath(root, path), err.Error())
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
				report.addWarning(missingCode, fmt.Sprintf("%s:%d", relPath(root, path), lineNo+1), fmt.Sprintf("relative Markdown link target does not exist: %s", target))
			} else if err != nil {
				report.addWarning(failedCode, fmt.Sprintf("%s:%d", relPath(root, path), lineNo+1), err.Error())
			}
		}
	}
}

// projectDocPrefixes lists common root-level project documentation filename
// prefixes, matched case-insensitively. These are broad conventions, not this
// repository's specific layout.
var projectDocPrefixes = []string{"README", "CONTRIBUTING", "CHANGELOG", "ARCHITECTURE", "DESIGN"}

// validateProjectDocs runs opt-in, read-only structural checks over a project's
// own documentation surface. It discovers common documentation files rather
// than requiring any specific layout and currently reports broken relative
// Markdown links. It never runs as part of the default validation scope.
func validateProjectDocs(root string, report *validationReport) {
	for _, path := range projectDocFiles(root) {
		validateMarkdownFileLinksWithCodes(root, path, report, "project_doc_link_missing", "project_doc_link_check_failed")
	}
	validateDesignDocIndex(root, report)
}

// validateDesignDocIndex runs only when a repository already uses the
// docs/design-docs/ convention with an index.md. It checks the single generic
// invariant that every design-doc Markdown file is represented in the index.
// Broken relative links in the index (including entries that point at missing
// files) and broken links inside design-doc files are reported by the shared
// project-doc relative-link check, so this function does not duplicate that.
// It never creates, rewrites, or formats the index.
func validateDesignDocIndex(root string, report *validationReport) {
	designDir := filepath.Join(root, "docs", "design-docs")
	indexPath := filepath.Join(designDir, "index.md")
	if stat, err := os.Stat(designDir); err != nil || !stat.IsDir() {
		return
	}
	indexData, err := readWorkflowFile(indexPath)
	if err != nil {
		// A missing index means the repository does not use the convention.
		// A genuinely unreadable index that exists is already surfaced by the
		// project-doc relative-link check, so do not double-report here.
		return
	}

	indexed := designDocIndexTargets(designDir, indexData)
	cleanIndex := filepath.Clean(indexPath)

	var files []string
	_ = filepath.WalkDir(designDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		clean := filepath.Clean(path)
		if clean == cleanIndex {
			return nil
		}
		files = append(files, clean)
		return nil
	})
	sort.Strings(files)
	for _, path := range files {
		if !indexed[path] {
			report.addWarning("design_doc_unindexed", relPath(root, path), "design-doc Markdown file is not represented in docs/design-docs/index.md")
		}
	}
}

// designDocIndexTargets collects the cleaned absolute paths of relative
// Markdown links found in the design-doc index, resolved relative to the index
// directory. Links inside fenced code blocks and non-relative links are
// ignored.
func designDocIndexTargets(designDir string, indexData []byte) map[string]bool {
	targets := map[string]bool{}
	inFence := false
	for _, line := range strings.Split(string(indexData), "\n") {
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
			resolved := filepath.Clean(filepath.Join(designDir, filepath.FromSlash(target)))
			targets[resolved] = true
		}
	}
	return targets
}

// projectDocFiles discovers common project documentation Markdown files: root
// level docs matching projectDocPrefixes and every Markdown file under docs/
// (which covers docs/adr/ and similar). Results are deduplicated and sorted for
// deterministic output.
func projectDocFiles(root string) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}
	if entries, err := os.ReadDir(root); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if isProjectDocName(entry.Name()) {
				add(filepath.Join(root, entry.Name()))
			}
		}
	}
	docsDir := filepath.Join(root, "docs")
	_ = filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		add(path)
		return nil
	})
	sort.Strings(paths)
	return paths
}

func isProjectDocName(name string) bool {
	upper := strings.ToUpper(name)
	for _, prefix := range projectDocPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
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

func (r *validationReport) addInfo(code string, path string, message string) {
	r.Info = append(r.Info, validationFinding{Code: code, Path: path, Message: message})
}
