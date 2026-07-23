package ahm

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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

// RenderText implements the textRenderer interface for validationReport.
func (r validationReport) RenderText(w io.Writer) error {
	if r.OK && len(r.Errors) == 0 && len(r.Warnings) == 0 && len(r.Info) == 0 {
		_, err := fmt.Fprintln(w, "ok")
		return err
	}
	if _, err := fmt.Fprintf(w, "ok: %v\n", r.OK); err != nil {
		return err
	}
	if len(r.Errors) > 0 {
		if _, err := fmt.Fprintln(w, "errors:"); err != nil {
			return err
		}
		for _, e := range r.Errors {
			if e.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", e.Code, e.Message, e.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", e.Code, e.Message); err != nil {
					return err
				}
			}
		}
	}
	if len(r.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "warnings:"); err != nil {
			return err
		}
		for _, wrn := range r.Warnings {
			if wrn.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", wrn.Code, wrn.Message, wrn.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", wrn.Code, wrn.Message); err != nil {
					return err
				}
			}
		}
	}
	if len(r.Info) > 0 {
		if _, err := fmt.Fprintln(w, "info:"); err != nil {
			return err
		}
		for _, i := range r.Info {
			if i.Path != "" {
				if _, err := fmt.Fprintf(w, "  - %s: %s (%s)\n", i.Code, i.Message, i.Path); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "  - %s: %s\n", i.Code, i.Message); err != nil {
					return err
				}
			}
		}
	}
	return nil
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

func (a *app) validateWorkflow(scopes []string) (validationReport, []Task) {
	return validateWorkflowScopedForPaths(a.opts.root, scopes, a.workflowPaths())
}

func validateWorkflowScopedForPaths(root string, scopes []string, paths workflowPaths) (validationReport, []Task) {
	report := newValidationReport()

	all := len(scopes) == 0
	want := func(s string) bool { return all || containsScope(scopes, s) }

	var tasks []Task
	if want(CheckScopeWorkflow) {
		tasks = validateManagedFiles(root, paths, &report)
		validateTaskDependencies(root, tasks, &report)
		validateBlockedDepsComplete(root, tasks, &report)
		validateTaskBuckets(root, paths, tasks, &report)
		validateTaskExecPlans(root, paths, tasks, &report)
		validateExecPlans(root, paths, tasks, &report)
		validateResearchInbox(root, paths, &report, time.Now())
		validateADRs(root, &report)
		validateGeneratedIndexes(root, paths, tasks, &report)
	}
	if want(CheckScopeLinks) {
		validateMarkdownLinks(root, paths, &report)
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

func newValidationReport() validationReport {
	return validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}}
}

// validateWorkflowStateForPaths validates a complete, already-parsed task set
// and already-rendered generated indexes. Mutation paths use it only when the
// task parse had no errors; standalone status and doctor keep the independent
// disk-reading path above.
func validateWorkflowStateForPaths(root string, paths workflowPaths, tasks []Task, writes map[string]string) validationReport {
	report := newValidationReport()
	validateMetadata(root, &report)
	validateTaskDuplicateIDs(root, tasks, &report)
	for _, task := range tasks {
		validateTaskFrontMatterMeta(task.meta, relPath(root, task.Path), &report)
		validateTaskAcceptance(root, task, &report)
	}
	validateTaskDependencies(root, tasks, &report)
	validateBlockedDepsComplete(root, tasks, &report)
	validateTaskBuckets(root, paths, tasks, &report)
	validateTaskExecPlans(root, paths, tasks, &report)
	validateExecPlans(root, paths, tasks, &report)
	validateResearchInbox(root, paths, &report, time.Now())
	validateADRs(root, &report)
	if validateGeneratedIndexMetadata(root, &report) {
		validateGeneratedIndexWrites(root, writes, &report)
	}
	report.OK = len(report.Errors) == 0
	return report
}

func validateResearchInbox(root string, paths workflowPaths, report *validationReport, now time.Time) {
	meta, err := readMetadata(root)
	if err != nil {
		return
	}
	threshold, enabled := meta.researchInboxStaleThreshold()
	if !enabled {
		return
	}
	dir := filepath.Join(root, filepath.FromSlash(paths.researchRel()), "inbox")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ageDays, err := researchNoteAgeDays(path, now)
		if err != nil || ageDays < threshold {
			continue
		}
		report.addWarning(
			"research_inbox_stale",
			relPath(root, path),
			fmt.Sprintf("research inbox note is %d days old (threshold %d); promote it to research/topics, convert it to a task, or delete it if it has no continuing value", ageDays, threshold),
		)
	}
}

func containsScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// emitPostMutationFindings runs workflow-scope validation after a successful
// non-dry-run mutation and emits findings as warnings. It uses only the
// workflow scope so that markdown-link false positives do not drown out core
// workflow drift. The validation runs on every writeIndexes call, which
// covers task create, lifecycle commands, dep updates, comments, ADR lifecycle
// commands, and explicit ahm index.
func (a *app) emitPostMutationFindings(tasks []Task, writes map[string]string, reuseState bool) {
	if a.opts.dryRun {
		return
	}
	var report validationReport
	if reuseState {
		report = validateWorkflowStateForPaths(a.opts.root, a.workflowPaths(), tasks, writes)
	} else {
		report, _ = a.validateWorkflow([]string{CheckScopeWorkflow})
	}
	for _, finding := range report.Errors {
		a.addWarning("%s", finding.Message)
	}
	for _, finding := range report.Warnings {
		a.addWarning("%s", finding.Message)
	}
}

func validateManagedFiles(root string, paths workflowPaths, report *validationReport) []Task {
	validateMetadata(root, report)
	tasks := validateTaskFiles(root, paths, report)
	validateTaskDuplicateIDs(root, tasks, report)
	return tasks
}

func validateMetadata(root string, report *validationReport) {
	_, metaErr := readMetadata(root)
	if metaErr != nil {
		if errors.Is(metaErr, os.ErrNotExist) {
			report.addError("metadata_missing", metadataErrorPath(metaErr), "workflow metadata is missing")
		} else {
			report.addError("metadata_corrupt", metadataErrorPath(metaErr), fmt.Sprintf("workflow metadata is corrupt: %v", metaErr))
		}
	}
}

func validateTaskFiles(root string, paths workflowPaths, report *validationReport) []Task {
	var tasks []Task
	files, err := taskFilePathsFor(paths)
	if err != nil {
		report.addError("task_dir_unreadable", paths.tasksRel(), err.Error())
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
	validateTaskFrontMatterMeta(meta, relPath, report)
}

func validateTaskFrontMatterMeta(meta map[string]string, relPath string, report *validationReport) {
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

func validateBlockedDepsComplete(root string, tasks []Task, report *validationReport) {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	for _, task := range tasks {
		if task.Status != "Blocked" || task.Bucket != "active" || len(task.DependsOn) == 0 {
			continue
		}
		depsAllComplete := true
		for _, dep := range task.DependsOn {
			if !completed[dep] {
				depsAllComplete = false
				break
			}
		}
		if depsAllComplete {
			report.addWarning("task_blocked_deps_complete", relPath(root, task.Path), fmt.Sprintf("task %s is Blocked but all its dependencies are Completed", task.ID))
		}
	}
}

func validateTaskBuckets(root string, paths workflowPaths, tasks []Task, report *validationReport) {
	tasksRel := paths.tasksRel()
	for _, task := range tasks {
		switch {
		case task.Status == "Completed" && task.Bucket != "completed":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "completed task should be in "+tasksRel+"/completed")
		case task.Status == "Cancelled" && task.Bucket != "cancelled":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "cancelled task should be in "+tasksRel+"/cancelled")
		case task.Status != "Completed" && task.Status != "Cancelled" && task.Bucket != "active":
			report.addWarning("task_bucket_mismatch", relPath(root, task.Path), "active task status should be in "+tasksRel+"/active")
		}
	}
}

func validateTaskDuplicateIDs(root string, tasks []Task, report *validationReport) {
	byID := map[string][]Task{}
	for _, task := range tasks {
		byID[task.ID] = append(byID[task.ID], task)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		matches := byID[id]
		if len(matches) < 2 {
			continue
		}
		paths := make([]string, 0, len(matches))
		for _, task := range matches {
			paths = append(paths, relPath(root, task.Path))
		}
		sort.Strings(paths)
		report.addError("task_duplicate_id", "", fmt.Sprintf("task ID %s is used by multiple files: %s; resolve the duplicate manually (remove or rename one file)", id, strings.Join(paths, ", ")))
	}
}

func validateTaskExecPlans(root string, paths workflowPaths, tasks []Task, report *validationReport) {
	for _, task := range tasks {
		if task.ExecPlan == "" || task.ExecPlan == "-" {
			continue
		}
		plan, bucket, ok := resolveExecPlanReference(paths, task.ExecPlan)
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

func validateExecPlans(root string, paths workflowPaths, tasks []Task, report *validationReport) {
	referenced := referencedExecPlans(paths, tasks)
	for _, bucket := range []string{"active", "completed"} {
		dir := paths.execPlansDir(bucket)
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

func referencedExecPlans(paths workflowPaths, tasks []Task) map[string]bool {
	referenced := map[string]bool{}
	for _, task := range tasks {
		if task.ExecPlan == "" || task.ExecPlan == "-" {
			continue
		}
		plan, _, ok := resolveExecPlanReference(paths, task.ExecPlan)
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

func resolveExecPlanReference(paths workflowPaths, ref string) (string, string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "-" {
		return "", "", false
	}
	root := paths.root
	var candidates []string
	if filepath.IsAbs(ref) {
		candidates = append(candidates, ref)
	} else {
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(ref)))
		// Records migrated from .agents/ to .ahm/ may still reference their
		// ExecPlans by the legacy repo-relative path.
		if paths.recordsDir == toolRecordsDirName {
			if legacyRel, ok := strings.CutPrefix(ref, legacyRecordsDirName+"/exec-plans/"); ok {
				candidates = append(candidates, filepath.Join(paths.execPlansDir(""), filepath.FromSlash(legacyRel)))
			}
		}
		for _, bucket := range []string{"active", "completed"} {
			candidates = append(candidates, filepath.Join(paths.execPlansDir(bucket), filepath.FromSlash(ref)))
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
		case strings.HasPrefix(rel, paths.execPlansRel("active")+"/"):
			return candidate, "active", true
		case strings.HasPrefix(rel, paths.execPlansRel("completed")+"/"):
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

func isUncheckedChecklistItem(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "* [ ]")
}

func execPlanSectionHasOpenProgress(section execPlanSection) bool {
	for _, line := range section.Lines {
		if isUncheckedChecklistItem(line) {
			return true
		}
	}
	return false
}

func validateGeneratedIndexes(root string, paths workflowPaths, tasks []Task, report *validationReport) {
	if !validateGeneratedIndexMetadata(root, report) {
		return
	}
	writes, err := indexWritesForPaths(root, tasks, paths)
	if err != nil {
		if writes == nil {
			report.addWarning("generated_index_check_failed", "", err.Error())
			return
		}
		// validateADRs reports each malformed ADR. Keep checking the indexes
		// rendered from the readable records instead of duplicating that finding.
	}
	validateGeneratedIndexWrites(root, writes, report)
}

func validateGeneratedIndexMetadata(root string, report *validationReport) bool {
	if _, err := readMetadata(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", metadataErrorPath(err), fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
		return false
	}
	return true
}

func validateGeneratedIndexWrites(root string, writes map[string]string, report *validationReport) {
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

// inlineCodeSpanPattern matches inline code span content delimited by single
// backticks. Quoted example links written inside backticks (for example a
// span containing a markdown link) are text, not navigation, so the link
// extractor strips them before matching link syntax. Fenced code blocks are
// handled separately by the fence-tracking logic above this call.
var inlineCodeSpanPattern = regexp.MustCompile("`[^`]*`")

// stripInlineCodeSpans removes inline code span content from a line so the
// link extractor does not treat quoted example links inside backticks as
// navigation. Backticks are removed in pairs; an unmatched trailing backtick
// is left untouched.
func stripInlineCodeSpans(line string) string {
	return inlineCodeSpanPattern.ReplaceAllString(line, "")
}

// walkMarkdownLinks calls visit for each raw Markdown link target outside
// fenced and inline code, with the target's one-based source line number.
func walkMarkdownLinks(data []byte, visit func(lineNo int, target string)) {
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
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(stripInlineCodeSpans(line), -1) {
			visit(lineNo+1, match[1])
		}
	}
}

func validateMarkdownLinks(root string, paths workflowPaths, report *validationReport) {
	if _, err := readMetadata(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", metadataErrorPath(err), fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
		return
	}
	for _, path := range workflowMarkdownFilesForPaths(root, paths) {
		validateMarkdownFileLinks(root, path, report)
	}
}

func workflowMarkdownFilesForPaths(root string, resolved workflowPaths) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}
	walkRoots := []string{filepath.Join(root, legacyRecordsDirName)}
	if recordsDir := resolved.recordsDir; recordsDir != legacyRecordsDirName {
		walkRoots = append(walkRoots, filepath.Join(root, recordsDir))
	}
	for _, walkRoot := range walkRoots {
		_ = filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				return nil
			}
			add(path)
			return nil
		})
	}
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
	walkMarkdownLinks(data, func(lineNo int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || shouldSkipMarkdownLink(target) {
			return
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
		if _, err := os.Stat(resolved); errors.Is(err, os.ErrNotExist) {
			report.addWarning(missingCode, fmt.Sprintf("%s:%d", relPath(root, path), lineNo), fmt.Sprintf("relative Markdown link target does not exist: %s", target))
		} else if err != nil {
			report.addWarning(failedCode, fmt.Sprintf("%s:%d", relPath(root, path), lineNo), err.Error())
		}
	})
}

// projectDocPrefixes lists common root-level project documentation filename
// prefixes, matched case-insensitively. These are broad conventions, not this
// repository's specific layout.
var projectDocPrefixes = []string{"AGENTS", "README", "CONTRIBUTING", "CHANGELOG", "ARCHITECTURE", "DESIGN"}

// validateProjectDocs runs opt-in, read-only structural checks over a project's
// own documentation surface. It discovers common documentation files rather
// than requiring any specific layout. It reports broken relative links,
// non-portable link targets, entry-point line budget overages, and generalized
// doc index coverage. It never runs as part of the default validation scope.
func validateProjectDocs(root string, report *validationReport) {
	meta, metaErr := readMetadata(root)
	for _, path := range projectDocFiles(root) {
		validateMarkdownFileLinksWithCodes(root, path, report, "project_doc_link_missing", "project_doc_link_check_failed")
		validateProjectDocLinkPortability(root, path, report)
	}
	validateDesignDocIndex(root, report)
	validateDocIndexCoverage(root, report)
	if metaErr == nil {
		validateEntryPointBudget(root, meta.ProjectDocs, report)
	}
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
	walkMarkdownLinks(indexData, func(_ int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || shouldSkipMarkdownLink(target) {
			return
		}
		resolved := filepath.Clean(filepath.Join(designDir, filepath.FromSlash(target)))
		targets[resolved] = true
	})
	return targets
}

// linkPortabilityProblemPatterns matches link targets that are not portable
// across machines: file:// URIs, absolute filesystem paths (Unix and Windows),
// and home-directory paths (~/).
var linkPortabilityProblemPatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`^file://`), "file:// URI"},
	{regexp.MustCompile(`^~/`), "home-directory path (~/)"},
	{regexp.MustCompile(`^/[^/]`), "absolute filesystem path"},
	{regexp.MustCompile(`^[A-Za-z]:[\\/]`), "absolute Windows path"},
}

// isNonPortableLinkTarget reports whether a link target is non-portable
// (file:// URI, absolute path, or home-directory path). It returns the
// human-readable description of the problem or an empty string.
func isNonPortableLinkTarget(target string) string {
	for _, p := range linkPortabilityProblemPatterns {
		if p.pattern.MatchString(target) {
			return p.desc
		}
	}
	return ""
}

// shouldSkipMarkdownLinkForPortability returns true for link targets that
// are portable (not file://, not absolute, not home-directory) and should
// be skipped by the standard link check. This is like shouldSkipMarkdownLink
// but does not skip the portability-problem targets.
func shouldSkipMarkdownLinkForPortability(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") {
		return true
	}
	if strings.HasPrefix(target, "mailto:") {
		return true
	}
	// Skip URLs with non-file:// schemes.
	if markdownLinkSchemePattern.MatchString(target) && !strings.HasPrefix(target, "file://") {
		return true
	}
	return false
}

// validateProjectDocLinkPortability checks every Markdown link target in a
// single file for non-portable references: file:// URIs, absolute filesystem
// paths, and home-directory paths.
func validateProjectDocLinkPortability(root string, path string, report *validationReport) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return
	}
	walkMarkdownLinks(data, func(lineNo int, rawTarget string) {
		target := strings.TrimSpace(rawTarget)
		if shouldSkipMarkdownLinkForPortability(target) {
			return
		}
		if desc := isNonPortableLinkTarget(target); desc != "" {
			report.addError("project_doc_link_not_portable", fmt.Sprintf("%s:%d", relPath(root, path), lineNo),
				fmt.Sprintf("non-portable Markdown link target (%s): %s", desc, target))
		}
	})
}

// validateEntryPointBudget checks that the root AGENTS.md does not exceed the
// configured line-count budget. CLAUDE.md is skipped when it is a symlink or
// a bare @AGENTS.md import. The default budget is defaultEntryPointBudget (150).
func validateEntryPointBudget(root string, cfg *projectDocsConfig, report *validationReport) {
	budget := defaultEntryPointBudget
	if cfg != nil && cfg.EntryPointBudget > 0 {
		budget = cfg.EntryPointBudget
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(agentsPath) // #nosec G304 -- project-root path
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addWarning("entry_point_check_failed", "AGENTS.md", err.Error())
		}
		return
	}
	lineCount := len(strings.Split(string(data), "\n"))
	if lineCount > budget {
		report.addWarning("entry_point_over_budget", "AGENTS.md",
			fmt.Sprintf("AGENTS.md is %d lines (budget %d)", lineCount, budget))
	}
}

// validateDocIndexCoverage generalizes the design-doc index coverage check to
// any docs/ subdirectory that contains an index.md. Every sibling .md file must
// be represented in the index. design_doc_unindexed keeps its code for
// compatibility; the design-docs directory is excluded from the generalized
// check to avoid duplicate findings.
func validateDocIndexCoverage(root string, report *validationReport) {
	docsDir := filepath.Join(root, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return
	}
	preservedScaffolds := make(map[string]bool, len(preservedScaffoldFiles))
	for _, target := range preservedScaffoldFiles {
		preservedScaffolds[filepath.Clean(filepath.Join(root, target))] = true
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// design-docs already has its own check (design_doc_unindexed).
		if entry.Name() == "design-docs" {
			continue
		}
		subDir := filepath.Join(docsDir, entry.Name())
		indexPath := filepath.Join(subDir, "index.md")
		indexData, err := readWorkflowFile(indexPath)
		if err != nil {
			// No index.md means this directory doesn't use the convention.
			continue
		}
		indexed := docIndexTargets(subDir, indexData)
		cleanIndex := filepath.Clean(indexPath)

		var files []string
		_ = filepath.WalkDir(subDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			clean := filepath.Clean(path)
			if clean == cleanIndex || preservedScaffolds[clean] {
				return nil
			}
			files = append(files, clean)
			return nil
		})
		sort.Strings(files)
		for _, path := range files {
			if !indexed[path] {
				report.addWarning("doc_unindexed", relPath(root, path),
					fmt.Sprintf("Markdown file is not represented in %s/index.md", relPath(root, subDir)))
			}
		}
	}
}

// docIndexTargets collects the cleaned absolute paths of relative Markdown
// links found in a doc index file, resolved relative to the index directory.
func docIndexTargets(dir string, indexData []byte) map[string]bool {
	targets := map[string]bool{}
	walkMarkdownLinks(indexData, func(_ int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || shouldSkipMarkdownLink(target) {
			return
		}
		resolved := filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))
		targets[resolved] = true
	})
	return targets
}

// projectDocNestDepth is the max directory nesting depth (relative to root)
// for nested AGENTS.md discovery. A depth of 3 reaches a/b/c/AGENTS.md but
// skips deeper trees, which avoids scanning generated, vendored, and build
// output directories even when their names are not in the skip list.
const projectDocNestDepth = 3

// projectDocFiles discovers common project documentation Markdown files: root
// level docs matching projectDocPrefixes, CLAUDE.md, every Markdown file under
// docs/ (which covers docs/adr/ and similar), and nested AGENTS.md files (depth-
// limited to projectDocNestDepth and skipping common build-output directories).
// Results are deduplicated and sorted for deterministic output.
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
	// Always include CLAUDE.md if it exists as a regular file (not a symlink
	// or bare import). Symlinks and @AGENTS.md imports are skipped by the
	// entry-point budget check, but link-portability still scans CLAUDE.md
	// when it contains real content.
	claudePath := filepath.Join(root, "CLAUDE.md")
	if stat, err := os.Lstat(claudePath); err == nil && stat.Mode().IsRegular() {
		add(claudePath)
	}
	docsDir := filepath.Join(root, "docs")
	_ = filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		add(path)
		return nil
	})
	// Walk the repo for nested AGENTS.md files, bounded by projectDocNestDepth
	// and skipping dot-directories, vendored deps, and common build-output dirs.
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			name := entry.Name()
			// Never skip the root directory itself — the skip list is for
			// subdirectories only, to avoid a checkout whose basename
			// matches a skipped name (e.g. a CI workspace named "build").
			if path != root && (name == ".git" || name == ".agents" || name == ".ahm" ||
				name == "vendor" || name == "node_modules" ||
				name == "build" || name == "dist" || name == "target" || name == "out") {
				return fs.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			// Stop descending beyond the configured depth.
			rel, err := filepath.Rel(root, path)
			if err == nil && strings.Count(rel, string(filepath.Separator)) >= projectDocNestDepth {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Name() == "AGENTS.md" && path != filepath.Join(root, "AGENTS.md") {
			add(path)
		}
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
			cycles = append(cycles, append(append([]string{}, path[start:]...), id))
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
