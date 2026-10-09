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

// titleGroup is one case-insensitive title and the active tasks carrying it.
type titleGroup struct {
	title string
	tasks []Task
}

// validateTaskDuplicateTitles reports each title shared by more than one active
// task, one warning-tier finding per duplicated title. Titles are the human
// handle for a task and the only field `task search` matches, so two records
// with the same title split one piece of work across files a reader cannot tell
// apart - but a duplicate is legal and recurring work legitimately reuses a
// title, so the finding warns without changing the exit code and never refuses
// the records.
//
// The comparison is exact and case-insensitive, matching the create, edit, and
// import warning (warnDuplicateTitle): the same title reproduced verbatim is the
// failure that occurs, and grouping is by case-fold equivalence rather than a
// similarity threshold. Completed and Cancelled records are skipped on every
// side, because a recurring task reuses their titles and comparing them would
// drown the useful case.
//
// This runs on the disk-reading path (validateManagedFiles) that status, doctor,
// and prime use. It is deliberately absent from the post-mutation pass that
// validates an already-parsed task set (validateWorkflowStateForPaths): a
// create, edit, or import that introduces a collision already prints
// warnDuplicateTitle, so checking the resulting state there would report the
// same collision twice. Status, doctor, and prime are the surfaces that report
// a pair which already existed on disk, including one a `task reopen`
// reactivates without a command-side check. (The fallback branch of
// emitPostMutationFindings re-reads from disk and so does surface the finding;
// that path reads everything fresh, so it stays consistent rather than a second
// report of a new pair.)
func validateTaskDuplicateTitles(paths workflowPaths, tasks []Task, report *validationReport) {
	// Group with strings.EqualFold so the equivalence matches warnDuplicateTitle
	// exactly; a lowercase map key would not fold every pair EqualFold does.
	var groups []titleGroup
	for _, task := range tasks {
		if task.Status == "Completed" || task.Status == "Cancelled" {
			continue
		}
		grouped := false
		for i := range groups {
			if strings.EqualFold(groups[i].title, task.Title) {
				groups[i].tasks = append(groups[i].tasks, task)
				grouped = true
				break
			}
		}
		if !grouped {
			groups = append(groups, titleGroup{title: task.Title, tasks: []Task{task}})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].title) < strings.ToLower(groups[j].title)
	})
	for _, group := range groups {
		if len(group.tasks) < 2 {
			continue
		}
		found := make([]string, 0, len(group.tasks))
		for _, task := range group.tasks {
			found = append(found, paths.displayPath(task.Path))
		}
		sort.Strings(found)
		report.addWarning("task_duplicate_title", "", fmt.Sprintf("%d active tasks share the title %q: %s; combine the records or make the titles distinct", len(group.tasks), group.title, strings.Join(found, ", ")))
	}
}
