package ahm

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func (a *app) taskList(mode string, statuses []string, labels []string, priorities []string, efforts []string) error {
	return a.taskListSorted(mode, statuses, labels, priorities, efforts, "", false)
}

func (a *app) taskListSorted(mode string, statuses []string, labels []string, priorities []string, efforts []string, sortField string, reverse bool) error {
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
	if len(priorities) > 0 {
		allowed := make(map[string]bool, len(priorities))
		for _, raw := range priorities {
			normalized, err := normalizeTaskPriority(raw)
			if err != nil {
				return err
			}
			allowed[normalized] = true
		}
		filtered = filterTasksByPriority(filtered, allowed)
	}
	if len(efforts) > 0 {
		allowed := make(map[string]bool, len(efforts))
		for _, raw := range efforts {
			normalized, err := normalizeTaskEffort(raw)
			if err != nil {
				return err
			}
			allowed[normalized] = true
		}
		filtered = filterTasksByEffort(filtered, allowed)
	}
	if err := sortTaskList(filtered, sortField, reverse); err != nil {
		return err
	}
	if len(filtered) == 0 {
		if a.opts.json {
			return a.emit([]Task{})
		}
		switch mode {
		case "ready":
			fmt.Fprintln(a.out, "No ready tasks.")
		case "blocked":
			fmt.Fprintln(a.out, "No blocked tasks.")
		default:
			fmt.Fprintln(a.out, "No tasks found.")
		}
		return nil
	}
	if a.opts.json {
		return a.emit(filtered)
	}
	for _, task := range filtered {
		a.printTaskLine(task)
	}
	return nil
}

func sortTaskList(tasks []Task, field string, reverse bool) error {
	field = strings.ToLower(strings.TrimSpace(field))
	if field == "" {
		field = "priority"
	}
	supported := taskSortFieldOrder()
	if !containsString(supported, field) {
		return usageError(fmt.Sprintf("unsupported task sort field %q (supported: %s)", field, strings.Join(supported, ", ")))
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		comparison := compareTaskSortField(tasks[i], tasks[j], field)
		if comparison == 0 && field != "id" {
			comparison = compareTaskIDs(tasks[i].ID, tasks[j].ID)
		}
		if reverse {
			return comparison > 0
		}
		return comparison < 0
	})
	return nil
}

func taskSortFieldOrder() []string {
	return []string{"priority", "id", "created", "updated", "effort", "status", "title"}
}

func compareTaskSortField(a, b Task, field string) int {
	switch field {
	case "priority":
		return compareInts(priorityRank(a.Priority), priorityRank(b.Priority))
	case "id":
		return compareTaskIDs(a.ID, b.ID)
	case "created":
		return compareTaskTimestamps(a.Created, b.Created)
	case "updated":
		return compareTaskTimestamps(a.Updated, b.Updated)
	case "effort":
		return compareInts(enumRank(a.Effort, effortOrder()), enumRank(b.Effort, effortOrder()))
	case "status":
		return compareInts(enumRank(a.Status, statusOrder()), enumRank(b.Status, statusOrder()))
	case "title":
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	default:
		return 0
	}
}

func compareTaskIDs(a, b string) int {
	if a == b {
		return 0
	}
	if taskLess(a, b) {
		return -1
	}
	return 1
}

func compareTaskTimestamps(a, b string) int {
	aTime, aErr := time.Parse(time.RFC3339, a)
	bTime, bErr := time.Parse(time.RFC3339, b)
	switch {
	case aErr == nil && bErr == nil:
		return aTime.Compare(bTime)
	case aErr == nil:
		return 1
	case bErr == nil:
		return -1
	default:
		return strings.Compare(a, b)
	}
}

func enumRank(value string, order []string) int {
	for i, candidate := range order {
		if value == candidate {
			return i
		}
	}
	return len(order)
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func (a *app) taskSearch(query string, statuses []string, labels []string) error {
	if strings.TrimSpace(query) == "" {
		return usageError("task search requires a query\n  ahm task search <query>")
	}
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, "all")
	needle := strings.ToLower(query)
	var matched []Task
	for _, task := range filtered {
		if strings.Contains(strings.ToLower(task.Title), needle) {
			matched = append(matched, task)
		}
	}
	filtered = matched
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
	if len(filtered) == 0 {
		if a.opts.json {
			return a.emit([]Task{})
		}
		fmt.Fprintln(a.out, "No tasks found.")
		return nil
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
	var tasks []Task
	var errs []error
	for _, id := range argv {
		task, err := a.resolveTask(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tasks = append(tasks, task)
	}

	if a.opts.json {
		switch len(tasks) {
		case 0:
			return errors.Join(append([]error{a.emit(nil)}, errs...)...)
		case 1:
			return errors.Join(append([]error{a.emit(tasks[0])}, errs...)...)
		default:
			return errors.Join(append([]error{a.emit(tasks)}, errs...)...)
		}
	}

	for i, task := range tasks {
		if i > 0 {
			fmt.Fprint(a.out, "\n---\n")
		}
		data, err := os.ReadFile(task.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = a.out.Write(data)
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}
	return errors.Join(errs...)
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
	return enumRank(priority, priorityOrder())
}

func filterTasksByPriority(tasks []Task, allowed map[string]bool) []Task {
	var out []Task
	for _, task := range tasks {
		if allowed[task.Priority] {
			out = append(out, task)
		}
	}
	return out
}

func filterTasksByEffort(tasks []Task, allowed map[string]bool) []Task {
	var out []Task
	for _, task := range tasks {
		if allowed[task.Effort] {
			out = append(out, task)
		}
	}
	return out
}
