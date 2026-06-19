package ahm

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func (a *app) taskList(mode string, statuses []string, labels []string) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, mode)
	if len(statuses) > 0 {
		allowed := make(map[string]bool, len(statuses))
		for _, raw := range statuses {
			normalized, err := normalizeTaskStatus(raw)
			if err != nil {
				return err
			}
			allowed[normalized] = true
		}
		filtered = filterTasksByStatus(filtered, allowed)
	}
	if len(labels) > 0 {
		required, err := normalizeTaskLabels(labels)
		if err != nil {
			return err
		}
		filtered = filterTasksByLabels(filtered, required)
	}
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		a.printTaskLine(task)
	}
	return nil
}

type taskLabelSummary struct {
	Label  string `json:"label"`
	Total  int    `json:"total"`
	Active int    `json:"active"`
	Open   int    `json:"open"`
	Ready  int    `json:"ready"`
}

func (a *app) taskLabels() error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	summaries := summarizeTaskLabels(tasks)
	if a.opts.json {
		return a.emit(summaries)
	}
	for _, summary := range summaries {
		fmt.Fprintf(a.out, "%s total=%d active=%d open=%d ready=%d\n", summary.Label, summary.Total, summary.Active, summary.Open, summary.Ready)
	}
	return nil
}

func (a *app) taskNext() error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	ready := filterTasks(tasks, "ready")
	if len(ready) == 0 {
		if a.opts.json {
			return a.emit(nil)
		}
		fmt.Fprintln(a.out, "No ready tasks.")
		return nil
	}
	if a.opts.json {
		return a.emit(ready[0])
	}
	a.printTaskLine(ready[0])
	return nil
}

func (a *app) taskShow(argv []string) error {
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	if a.opts.json {
		return a.emit(task)
	}
	data, err := os.ReadFile(task.Path)
	if err != nil {
		return err
	}
	_, err = a.out.Write(data)
	return err
}

func (a *app) printTaskLine(task Task) {
	fmt.Fprintf(a.out, "%s [%s] %s %s %s\n", task.ID, task.Status, task.Priority, task.Effort, task.Title)
}

func filterTasks(tasks []Task, mode string) []Task {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	var out []Task
	for _, task := range tasks {
		switch mode {
		case "ready":
			if task.Status == "Pending" && depsComplete(task, completed) {
				out = append(out, task)
			}
		case "blocked":
			if task.Status == "Blocked" || (task.Status == "Pending" && !depsComplete(task, completed)) {
				out = append(out, task)
			}
		default:
			out = append(out, task)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := priorityRank(out[i].Priority)
		pj := priorityRank(out[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return taskLess(out[i].ID, out[j].ID)
	})
	return out
}

func filterTasksByStatus(tasks []Task, allowed map[string]bool) []Task {
	var out []Task
	for _, task := range tasks {
		if allowed[task.Status] {
			out = append(out, task)
		}
	}
	return out
}

func filterTasksByLabels(tasks []Task, required []string) []Task {
	var out []Task
	for _, task := range tasks {
		labels := taskLabelSet(task)
		matches := true
		for _, label := range required {
			if !labels[label] {
				matches = false
				break
			}
		}
		if matches {
			out = append(out, task)
		}
	}
	return out
}

func normalizeTaskLabels(rawLabels []string) ([]string, error) {
	seen := make(map[string]bool, len(rawLabels))
	var labels []string
	for _, raw := range rawLabels {
		if strings.TrimSpace(raw) == "" {
			return nil, usageError("task label filter cannot be empty")
		}
		for _, part := range strings.Split(raw, ",") {
			label := strings.TrimSpace(part)
			if label == "" || label == "-" {
				return nil, usageError("task label filter cannot be empty")
			}
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	return labels, nil
}

func taskLabelSet(task Task) map[string]bool {
	labels := map[string]bool{}
	for _, label := range parseList(task.Labels) {
		labels[label] = true
	}
	return labels
}

func summarizeTaskLabels(tasks []Task) []taskLabelSummary {
	ready := map[string]bool{}
	for _, task := range filterTasks(tasks, "ready") {
		ready[task.ID] = true
	}
	byLabel := map[string]*taskLabelSummary{}
	for _, task := range tasks {
		for label := range taskLabelSet(task) {
			summary := byLabel[label]
			if summary == nil {
				summary = &taskLabelSummary{Label: label}
				byLabel[label] = summary
			}
			summary.Total++
			if task.Bucket == "active" {
				summary.Active++
			}
			if task.Status == "Open" {
				summary.Open++
			}
			if ready[task.ID] {
				summary.Ready++
			}
		}
	}
	labels := make([]string, 0, len(byLabel))
	for label := range byLabel {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	summaries := make([]taskLabelSummary, 0, len(labels))
	for _, label := range labels {
		summaries = append(summaries, *byLabel[label])
	}
	return summaries
}

func depsComplete(task Task, completed map[string]bool) bool {
	for _, dep := range task.DependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func priorityRank(priority string) int {
	switch priority {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	case "P4":
		return 4
	default:
		return 99
	}
}
