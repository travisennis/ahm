package ahm

import (
	"os"
	"sort"
	"strings"
)

func validateTaskFiles(paths workflowPaths, report *validationReport) []Task {
	var tasks []Task
	files, err := taskFilePathsFor(paths)
	if err != nil {
		report.addError("task_dir_unreadable", paths.displayPath(paths.tasksBucketDir("")), err.Error())
		return nil
	}
	for _, f := range files {
		data, err := readWorkflowFile(f.Path)
		if err != nil {
			if os.IsNotExist(err) {
				// Task file was already moved or deleted; not an error.
				continue
			}
			report.addError("task_unreadable", paths.displayPath(f.Path), err.Error())
			continue
		}
		validateTaskFrontMatter(data, paths.displayPath(f.Path), report)
		task, err := parseTaskFromData(data, f.Path, f.Bucket)
		if err != nil {
			report.addError("task_malformed", paths.displayPath(f.Path), err.Error())
			continue
		}
		validateTaskAcceptance(paths, task, report)
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return taskLess(tasks[i].ID, tasks[j].ID)
	})
	return tasks
}

func validateTaskAcceptance(paths workflowPaths, task Task, report *validationReport) {
	if task.Status != "Completed" {
		return
	}
	for _, finding := range parseAcceptanceNotes([]byte(task.Body)) {
		report.addWarning(finding.validationCode(), paths.displayPath(task.Path), finding.message(task.ID))
	}
}

func validateTaskFrontMatter(data []byte, label string, report *validationReport) {
	meta, _, err := parseFrontMatter(string(data))
	if err != nil {
		report.addError("task_malformed", label, err.Error())
		return
	}
	validateTaskFrontMatterMeta(meta, label, report)
}

func validateTaskFrontMatterMeta(meta map[string]string, label string, report *validationReport) {
	required := []string{"id", "title", "status", "priority", "effort", "labels", "depends_on"}
	for _, field := range required {
		if strings.TrimSpace(meta[field]) == "" {
			report.addError("task_missing_field", label, "task front matter is missing "+field)
		}
	}
}

func isUncheckedChecklistItem(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "* [ ]")
}
