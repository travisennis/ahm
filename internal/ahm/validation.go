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
