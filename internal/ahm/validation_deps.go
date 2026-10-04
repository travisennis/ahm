package ahm

import (
	"fmt"
	"sort"
	"strings"
)

func validateTaskDependencies(paths workflowPaths, tasks []Task, report *validationReport) {
	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	for _, task := range tasks {
		for _, dep := range task.DependsOn {
			if _, ok := byID[dep]; !ok {
				report.addError("task_dependency_missing", paths.displayPath(task.Path), fmt.Sprintf("task %s depends on missing task %s", task.ID, dep))
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
				report.addWarning("task_dependency_cancelled", paths.displayPath(task.Path), fmt.Sprintf("task %s depends on cancelled task %s", task.ID, dep))
			}
		}
	}
	for _, cycle := range taskDependencyCycles(tasks) {
		report.addError("task_dependency_cycle", "", "dependency cycle: "+strings.Join(cycle, " -> "))
	}
}

func validateBlockedDepsComplete(paths workflowPaths, tasks []Task, report *validationReport) {
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
			report.addWarning("task_blocked_deps_complete", paths.displayPath(task.Path), fmt.Sprintf("task %s is Blocked but all its dependencies are Completed", task.ID))
		}
	}
}

// validateBlockedReason warns when a task is Blocked without a recorded
// blocked_reason. Blocking a task requires a reason at the command layer, so a
// Blocked task with none is a record written by hand or by a release that
// predates the field.
func validateBlockedReason(paths workflowPaths, tasks []Task, report *validationReport) {
	for _, task := range tasks {
		if task.Status != "Blocked" {
			continue
		}
		if strings.TrimSpace(task.BlockedReason) == "" {
			report.addWarning("task_blocked_missing_reason", paths.displayPath(task.Path), fmt.Sprintf("task %s is Blocked with no blocked_reason", task.ID))
		}
	}
}

// validateTrackingChildrenComplete warns when an active Tracking task has at
// least one child, every child task is Completed or Cancelled, and the
// tracker's own dependencies are satisfied — leaving only the tracker itself
// to be closed. A Tracking task with no children is a valid intermediate
// state during intake, so it does not warn.
func validateTrackingChildrenComplete(paths workflowPaths, tasks []Task, report *validationReport) {
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
			report.addWarning("task_tracking_children_complete", paths.displayPath(task.Path), fmt.Sprintf("task %s is Tracking but all its child tasks are Completed or Cancelled", task.ID))
		}
	}
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
