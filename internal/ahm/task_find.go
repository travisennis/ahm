package ahm

import (
	"fmt"
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
	tasks, err := a.getTasks()
	if err != nil {
		fmt.Fprintln(a.err, "warning: some task files could not be parsed and were skipped")
	}
	return resolveTaskFromTasks(pattern, tasks)
}
