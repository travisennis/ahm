package ahm

import (
	"fmt"
	"sort"
	"strings"
)

func validateTaskBuckets(paths workflowPaths, tasks []Task, report *validationReport) {
	for _, task := range tasks {
		switch {
		case task.Status == "Completed" && task.Bucket != "completed":
			report.addWarning("task_bucket_mismatch", paths.displayPath(task.Path), "completed task should be in "+paths.displayPath(paths.tasksBucketDir("completed")))
		case task.Status == "Cancelled" && task.Bucket != "cancelled":
			report.addWarning("task_bucket_mismatch", paths.displayPath(task.Path), "cancelled task should be in "+paths.displayPath(paths.tasksBucketDir("cancelled")))
		case task.Status != "Completed" && task.Status != "Cancelled" && task.Bucket != "active":
			report.addWarning("task_bucket_mismatch", paths.displayPath(task.Path), "active task status should be in "+paths.displayPath(paths.tasksBucketDir("active")))
		}
	}
}

func validateTaskDuplicateIDs(paths workflowPaths, tasks []Task, report *validationReport) {
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
		found := make([]string, 0, len(matches))
		for _, task := range matches {
			found = append(found, paths.displayPath(task.Path))
		}
		sort.Strings(found)
		report.addError("task_duplicate_id", "", fmt.Sprintf("task ID %s is used by multiple files: %s; resolve the duplicate manually (remove or rename one file)", id, strings.Join(found, ", ")))
	}
}
