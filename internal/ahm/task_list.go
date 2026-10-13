package ahm

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
)

func (a *app) taskListSorted(mode string, statuses []string, labels []string, priorities []string, efforts []string, sortField string, reverse bool) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, mode)
	filtered, err = filterTasksByEnumValues(filtered, statuses, normalizeTaskStatus, filterTasksByStatus)
	if err != nil {
		return err
	}
	if len(labels) > 0 {
		required, err := normalizeTaskLabels(labels)
		if err != nil {
			return err
		}
		filtered = filterTasksByLabels(filtered, required)
	}
	filtered, err = filterTasksByEnumValues(filtered, priorities, normalizeTaskPriority, filterTasksByPriority)
	if err != nil {
		return err
	}
	filtered, err = filterTasksByEnumValues(filtered, efforts, normalizeTaskEffort, filterTasksByEffort)
	if err != nil {
		return err
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
		return a.emit(a.tasksForOutput(filtered))
	}
	isBlocked := mode == "blocked"
	completed := completedTaskIDs(tasks)
	for _, task := range filtered {
		a.printTaskLine(task)
		if isBlocked {
			if reason := taskBlockedReason(task, completed); reason != "" {
				fmt.Fprintf(a.out, "  reason: %s\n", reason)
			}
		}
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

// taskSearchUsage is the usage error for a missing or whitespace-only
// search query; the command's Args validator and the query parser both report
// it.
const taskSearchUsage = "task search requires a query\n  ahm task search <query>"

// parseTaskSearchQuery splits a query into the lowercase terms that every
// match must contain. Whitespace separates terms, except inside a
// double-quoted span, which is one term matched as an adjacent literal
// substring. A double quote must delimit a whole term: it may open only at the
// start of the query or after whitespace, and its closing partner must be
// followed by whitespace or the end of the query. There is no escape sequence;
// a query that breaks these rules is a usage error (exit 2).
func parseTaskSearchQuery(query string) ([]string, error) {
	if strings.TrimSpace(query) == "" {
		return nil, usageError(taskSearchUsage)
	}
	runes := []rune(query)
	var terms []string
	for i := 0; i < len(runes); {
		switch {
		case unicode.IsSpace(runes[i]):
			i++
		case runes[i] == '"':
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '"' {
					end = j
					break
				}
			}
			if end < 0 {
				return nil, usageError(fmt.Sprintf("invalid task search query %q: unterminated double quote", query))
			}
			span := string(runes[i+1 : end])
			i = end + 1
			if i < len(runes) && !unicode.IsSpace(runes[i]) {
				return nil, usageError(fmt.Sprintf("invalid task search query %q: a double quote must delimit a whole term", query))
			}
			if span == "" {
				return nil, usageError(fmt.Sprintf("invalid task search query %q: empty quoted term", query))
			}
			terms = append(terms, strings.ToLower(span))
		default:
			start := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) {
				if runes[i] == '"' {
					return nil, usageError(fmt.Sprintf("invalid task search query %q: a double quote must delimit a whole term", query))
				}
				i++
			}
			terms = append(terms, strings.ToLower(string(runes[start:i])))
		}
	}
	return terms, nil
}

// taskSearchMatch reports whether every term matches the task and whether all
// of them match the title. A task that matches but is not a title match is a
// body-only match.
func taskSearchMatch(task Task, terms []string) (titleMatch, match bool) {
	title := strings.ToLower(task.Title)
	body := strings.ToLower(task.Body)
	titleMatch = true
	for _, term := range terms {
		if strings.Contains(title, term) {
			continue
		}
		titleMatch = false
		if !strings.Contains(body, term) {
			return false, false
		}
	}
	return titleMatch, true
}

func (a *app) taskSearch(query string, statuses []string, labels []string, priorities []string, efforts []string, sortField string, reverse bool) error {
	terms, err := parseTaskSearchQuery(query)
	if err != nil {
		return err
	}
	// Without --sort the title/body grouping is the order, so there is no
	// selected order for --reverse to reverse.
	if reverse && strings.TrimSpace(sortField) == "" {
		return usageError("task search --reverse requires --sort\n  ahm task search <query> --sort <field> --reverse")
	}
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	filtered := filterTasks(tasks, "all")
	// Title matches are the stronger signal, so they are listed before
	// body-only matches. Within each group the existing priority-then-ID
	// order is preserved.
	var matched, bodyMatches []Task
	for _, task := range filtered {
		titleMatch, match := taskSearchMatch(task, terms)
		if !match {
			continue
		}
		if titleMatch {
			matched = append(matched, task)
		} else {
			bodyMatches = append(bodyMatches, task)
		}
	}
	matched = append(matched, bodyMatches...)
	filtered = matched
	filtered, err = filterTasksByEnumValues(filtered, statuses, normalizeTaskStatus, filterTasksByStatus)
	if err != nil {
		return err
	}
	if len(labels) > 0 {
		required, err := normalizeTaskLabels(labels)
		if err != nil {
			return err
		}
		filtered = filterTasksByLabels(filtered, required)
	}
	filtered, err = filterTasksByEnumValues(filtered, priorities, normalizeTaskPriority, filterTasksByPriority)
	if err != nil {
		return err
	}
	filtered, err = filterTasksByEnumValues(filtered, efforts, normalizeTaskEffort, filterTasksByEffort)
	if err != nil {
		return err
	}
	// An explicit sort replaces the title/body grouping and orders the whole
	// result set.
	if strings.TrimSpace(sortField) != "" {
		if err := sortTaskList(filtered, sortField, reverse); err != nil {
			return err
		}
	}
	if len(filtered) == 0 {
		if a.opts.json {
			return a.emit([]Task{})
		}
		fmt.Fprintln(a.out, "No tasks found.")
		return nil
	}
	if a.opts.json {
		return a.emit(a.tasksForOutput(filtered))
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
		return a.emit(a.taskForOutput(ready[0]))
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
			return errors.Join(append([]error{a.emit(a.taskForOutput(tasks[0]))}, errs...)...)
		default:
			return errors.Join(append([]error{a.emit(a.tasksForOutput(tasks))}, errs...)...)
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

// taskForOutput renders one task for structured output: its record path is
// rendered for the layout the records live in, so a store record path is never
// printed as an absolute machine path.
func (a *app) taskForOutput(task Task) Task {
	task.Path = a.workflowPaths().recordPath(task.Path)
	return task
}

// tasksForOutput renders a task list for structured output.
func (a *app) tasksForOutput(tasks []Task) []Task {
	rendered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		rendered = append(rendered, a.taskForOutput(task))
	}
	return rendered
}

// completedTaskIDs returns the set of task IDs whose status is Completed.
func completedTaskIDs(tasks []Task) map[string]bool {
	completed := map[string]bool{}
	for _, task := range tasks {
		if task.Status == "Completed" {
			completed[task.ID] = true
		}
	}
	return completed
}

// taskBlockedReason explains why a task is in the blocked queue. An explicit
// Blocked task reports its recorded blocked_reason (and reference); a Pending
// task reports the dependencies that are still incomplete.
func taskBlockedReason(task Task, completed map[string]bool) string {
	if task.Status == "Blocked" {
		reason := strings.TrimSpace(task.BlockedReason)
		switch {
		case reason != "" && task.BlockedRef != "":
			return reason + " (" + task.BlockedRef + ")"
		case reason != "":
			return reason
		case task.BlockedRef != "":
			return task.BlockedRef
		default:
			return "no reason recorded"
		}
	}
	var missing []string
	for _, dep := range task.DependsOn {
		if !completed[dep] {
			missing = append(missing, dep)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "waiting on " + strings.Join(missing, ", ")
}

func filterTasks(tasks []Task, mode string) []Task {
	completed := completedTaskIDs(tasks)
	var out []Task
	for _, task := range tasks {
		switch mode {
		case "ready":
			if task.Status == "Pending" && depsComplete(task, completed) {
				out = append(out, task)
			}
			if task.Status == "Tracking" && depsComplete(task, completed) && childrenAllResolved(task, tasks) {
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

// filterTasksByEnumValues keeps the tasks whose field value normalizes into
// values, normalizing every requested value first, so every command that
// filters by an enum field validates the values and reports errors the same
// way. An empty values slice keeps the tasks unchanged.
func filterTasksByEnumValues(tasks []Task, values []string, normalize func(string) (string, error), keep func([]Task, map[string]bool) []Task) ([]Task, error) {
	if len(values) == 0 {
		return tasks, nil
	}
	allowed := make(map[string]bool, len(values))
	for _, raw := range values {
		normalized, err := normalize(raw)
		if err != nil {
			return nil, err
		}
		allowed[normalized] = true
	}
	return keep(tasks, allowed), nil
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

// childrenAllResolved reports whether a Tracking task has at least one child
// and every child task is Completed or Cancelled, so the tracker itself is the
// only remaining work. A Tracking task with no children is not ready.
func childrenAllResolved(task Task, tasks []Task) bool {
	hasChild := false
	for _, other := range tasks {
		if other.Parent != task.ID {
			continue
		}
		hasChild = true
		if other.Status != "Completed" && other.Status != "Cancelled" {
			return false
		}
	}
	return hasChild
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
