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
	CheckScopeWorkflow = "workflow"
	CheckScopeLinks    = "links"
)

// validCheckScopes returns the list of recognised check-scope values.
func validCheckScopes() []string {
	return []string{CheckScopeWorkflow, CheckScopeLinks}
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
		validateTrackingChildrenComplete(root, tasks, &report)
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
	validateTrackingChildrenComplete(root, tasks, &report)
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

// validateTrackingChildrenComplete warns when an active Tracking task has at
// least one child, every child task is Completed or Cancelled, and the
// tracker's own dependencies are satisfied — leaving only the tracker itself
// to be closed. A Tracking task with no children is a valid intermediate
// state during intake, so it does not warn.
func validateTrackingChildrenComplete(root string, tasks []Task, report *validationReport) {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	childrenByParent := map[string][]Task{}
	for _, task := range tasks {
		if task.Parent != "" {
			childrenByParent[task.Parent] = append(childrenByParent[task.Parent], task)
		}
	}
	for _, task := range tasks {
		if task.Status != "Tracking" || task.Bucket != "active" || !depsComplete(task, completed) {
			continue
		}
		children := childrenByParent[task.ID]
		if len(children) == 0 {
			continue
		}
		allResolved := true
		for _, child := range children {
			if child.Status != "Completed" && child.Status != "Cancelled" {
				allResolved = false
				break
			}
		}
		if allResolved {
			report.addWarning("task_tracking_children_complete", relPath(root, task.Path), fmt.Sprintf("task %s is Tracking but all its child tasks are Completed or Cancelled", task.ID))
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
	sourceDirs := []string{
		resolved.tasksBucketDir("active"),
		resolved.tasksBucketDir("completed"),
		resolved.tasksBucketDir("cancelled"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "inbox"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "investigations"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "sources"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "topics"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "archived"),
		resolved.execPlansDir("active"),
		resolved.execPlansDir("completed"),
	}
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "index.md" || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			add(filepath.Join(dir, entry.Name()))
		}
	}
	if adrPaths, err := adrFilePaths(root); err == nil {
		for _, path := range adrPaths {
			add(path)
		}
	}
	indexPaths := []string{
		filepath.Join(resolved.tasksBucketDir(""), "index.md"),
		filepath.Join(resolved.tasksBucketDir("active"), "index.md"),
		filepath.Join(resolved.tasksBucketDir("completed"), "index.md"),
		filepath.Join(resolved.tasksBucketDir("cancelled"), "index.md"),
		filepath.Join(root, filepath.FromSlash(resolved.researchRel()), "index.md"),
		filepath.Join(resolved.execPlansDir("active"), "index.md"),
		filepath.Join(resolved.execPlansDir("completed"), "index.md"),
		filepath.Join(root, "docs", "adr", "index.md"),
	}
	for _, path := range indexPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			add(path)
		}
	}
	sort.Strings(paths)
	return paths
}

func validateMarkdownFileLinks(root string, path string, report *validationReport) {
	data, err := readWorkflowFile(path)
	if err != nil {
		report.addWarning("markdown_link_check_failed", relPath(root, path), err.Error())
		return
	}
	walkMarkdownLinks(data, func(lineNo int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || shouldSkipMarkdownLink(target) {
			return
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
		if _, err := os.Stat(resolved); errors.Is(err, os.ErrNotExist) {
			report.addWarning("markdown_link_missing", fmt.Sprintf("%s:%d", relPath(root, path), lineNo), fmt.Sprintf("relative Markdown link target does not exist: %s", target))
		} else if err != nil {
			report.addWarning("markdown_link_check_failed", fmt.Sprintf("%s:%d", relPath(root, path), lineNo), err.Error())
		}
	})
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
