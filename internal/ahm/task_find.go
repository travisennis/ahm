package ahm

import (
	"fmt"
	"sort"
	"strings"
)

func resolveTaskFromTasks(pattern string, tasks []Task) (Task, error) {
	// Exact string match returns immediately.
	for _, task := range tasks {
		if task.ID == pattern {
			return task, nil
		}
	}
	// Exact numeric match: parsed numeric value + suffix equal.
	// This resolves "1" to "001" before falling through to prefix matching.
	patNum, patSuffix, patOk := splitTaskID(pattern)
	if patOk {
		for _, task := range tasks {
			taskNum, taskSuffix, taskOk := splitTaskID(task.ID)
			if taskOk && taskNum == patNum && taskSuffix == patSuffix {
				return task, nil
			}
		}
	}
	// Constrained prefix matching: parse the numeric prefix so that "1a"
	// matches "001a" and similar short forms match zero-padded task IDs.
	// If a prefix matches more than one task, the command reports ambiguity.
	var matches []Task
	for _, task := range tasks {
		taskNum, taskSuffix, taskOk := splitTaskID(task.ID)
		if patOk && taskOk && taskNum == patNum && strings.HasPrefix(taskSuffix, patSuffix) {
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

func (a *app) resolveTask(pattern string) (Task, error) {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	return resolveTaskFromTasks(pattern, tasks)
}

// resolveTaskForMutation is like resolveTask but also checks that the
// resolved task ID is not duplicated across buckets. Mutation commands must
// use this instead of resolveTask so that duplicates are caught before
// writing, avoiding the risk of mutating or deleting the wrong record.
func (a *app) resolveTaskForMutation(pattern string) (Task, error) {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	task, err := resolveTaskFromTasks(pattern, tasks)
	if err != nil {
		return Task{}, err
	}
	if err := checkDuplicateTaskID(tasks, task.ID, a.opts.root); err != nil {
		return Task{}, err
	}
	return task, nil
}

// checkDuplicateTaskID checks whether the given task ID appears in more than
// one file. If so, it returns an error listing the conflicting paths and the
// manual recovery action so that mutation commands can refuse to operate on
// a duplicated ID.
func checkDuplicateTaskID(tasks []Task, id string, root string) error {
	var paths []string
	for _, task := range tasks {
		if task.ID == id {
			paths = append(paths, relPath(root, task.Path))
		}
	}
	if len(paths) > 1 {
		sort.Strings(paths)
		return fmt.Errorf("task ID %s is duplicated across %s; resolve the duplicate manually (remove or rename one file) before retrying", id, strings.Join(paths, ", "))
	}
	return nil
}

// checkTaskDepsNotDuplicated checks that none of the given task's dependency
// IDs appear in more than one file. It is called by mutation paths before
// operating on a task whose dependencies must be unambiguous.
func checkTaskDepsNotDuplicated(tasks []Task, task Task, root string) error {
	for _, dep := range task.DependsOn {
		if err := checkDuplicateTaskID(tasks, dep, root); err != nil {
			return err
		}
	}
	return nil
}
