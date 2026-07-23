package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *app) writeIndexes() error {
	defer a.emitWarnings()
	a.invalidateTasks()
	tasks, err := a.getTasks()
	if err != nil {
		if tasks == nil {
			return err
		}
		a.addWarning("some task files could not be parsed and were skipped: %s", err)
	}
	return a.writeIndexesForTasks(tasks, err == nil)
}

func (a *app) writeIndexesForTasks(tasks []Task, completeState bool) error {
	writes, err := indexWritesForPaths(a.opts.root, tasks, a.workflowPaths())
	if err != nil {
		if writes == nil {
			return err
		}
		a.addWarning("%s", err)
	}
	paths := sortedKeys(writes)
	// Index writes are sequential and best-effort. If a mid-batch write
	// fails (e.g., disk full, permissions), earlier files have already
	// been updated, the failed file remains stale, and later files are
	// skipped. This leaves a temporarily inconsistent index state that
	// self-heals on the next successful ahm index run. There is no
	// rollback — the alternative (cross-file atomic commit or transaction
	// semantics) is overengineered for regenerated output.
	for _, path := range paths {
		if a.opts.dryRun {
			if isStaleIndex(path, writes[path]) {
				fmt.Fprintln(a.out, relPath(a.opts.root, path))
			}
			continue
		}
		if !isStaleIndex(path, writes[path]) {
			continue
		}
		if err := writeFileAtomic(path, []byte(writes[path]), 0o644); err != nil {
			return err
		}
	}
	// Run post-mutation workflow validation to surface findings the
	// mutation just created or left behind. This uses only the workflow
	// scope so that markdown-link false positives do not drown out core
	// workflow drift.
	if completeState {
		a.tasksCache = tasks
	}
	a.emitPostMutationFindings(tasks, writes, completeState)
	return nil
}

// isStaleIndex returns true when the file at path is missing or its content
// differs from want. It is used by writeIndexes to skip unchanged writes
// and by dry-run mode to report only indexes that would change.
func isStaleIndex(path string, want string) bool {
	data, err := readWorkflowFile(path)
	if err != nil {
		// File missing or unreadable — stale.
		return true
	}
	return string(data) != want
}

func (a *app) indexWrites() (map[string]string, error) {
	tasks, err := a.getTasks()
	if err != nil {
		if tasks == nil {
			return nil, err
		}
		a.addWarning("some task files could not be parsed and were skipped: %s", err)
	}
	writes, err := indexWritesForPaths(a.opts.root, tasks, a.workflowPaths())
	if err != nil {
		if writes == nil {
			return nil, err
		}
		a.addWarning("%s", err)
	}
	return writes, nil
}

func (a *app) indexWriteTargetsFor(paths workflowPaths) ([]string, error) {
	tasks, err := collectTasksForPaths(a.opts.root, paths)
	if err != nil {
		if tasks == nil {
			return nil, err
		}
		a.addWarning("some task files could not be parsed and were skipped: %s", err)
	}
	writes, err := indexWritesForPaths(a.opts.root, tasks, paths)
	if err != nil {
		if writes == nil {
			return nil, err
		}
		a.addWarning("%s", err)
	}
	targets := make([]string, 0, len(writes))
	for _, path := range sortedKeys(writes) {
		targets = append(targets, relPath(a.opts.root, path))
	}
	return targets, nil
}

// indexWritesForPaths generates the complete set of index file writes for the
// given task set and workflow paths. It is used by both the index-writing and
// validation paths to avoid re-parsing the task tree.
func indexWritesForPaths(root string, tasks []Task, paths workflowPaths) (map[string]string, error) {
	indexWritesForPathsHook()
	research, err := collectMarkdownDocs(root, paths.researchRel(), []string{"inbox", "investigations", "sources", "topics", "archived"})
	if err != nil {
		return nil, err
	}
	activePlans, err := collectMarkdownDocs(root, paths.execPlansRel("active"), []string{""})
	if err != nil {
		return nil, err
	}
	completedPlans, err := collectMarkdownDocs(root, paths.execPlansRel("completed"), []string{""})
	if err != nil {
		return nil, err
	}
	adrs, adrErr := collectADRs(root)
	if adrErr != nil && len(adrs) == 0 {
		return nil, adrErr
	}
	writes := map[string]string{
		filepath.Join(paths.tasksBucketDir(""), "index.md"):                      renderRootIndex(tasks),
		filepath.Join(paths.tasksBucketDir("active"), "index.md"):                renderBucketIndex(tasks, "active"),
		filepath.Join(paths.tasksBucketDir("completed"), "index.md"):             renderBucketIndex(tasks, "completed"),
		filepath.Join(paths.tasksBucketDir("cancelled"), "index.md"):             renderBucketIndex(tasks, "cancelled"),
		filepath.Join(root, filepath.FromSlash(paths.researchRel()), "index.md"): renderResearchIndex(research),
		filepath.Join(paths.execPlansDir("active"), "index.md"):                  renderExecPlanIndex("Active ExecPlans", "No active ExecPlans yet.", activePlans),
		filepath.Join(paths.execPlansDir("completed"), "index.md"):               renderExecPlanIndex("Completed ExecPlans", "No completed ExecPlans yet.", completedPlans),
		filepath.Join(root, "docs", "adr", "index.md"):                           renderADRIndex(adrs),
	}
	if adrErr != nil {
		return writes, fmt.Errorf("some ADR files could not be parsed and were skipped: %w", adrErr)
	}
	return writes, nil
}

// indexWritesForPathsHook supports instrumented tests that count complete
// generated-index renders.
var indexWritesForPathsHook = func() {}

var generatedIndexTargetsList = []string{
	".agents/.tasks/index.md",
	".agents/.tasks/active/index.md",
	".agents/.tasks/completed/index.md",
	".agents/.tasks/cancelled/index.md",
	".agents/.research/index.md",
	".agents/exec-plans/active/index.md",
	".agents/exec-plans/completed/index.md",
	".ahm/.tasks/index.md",
	".ahm/.tasks/active/index.md",
	".ahm/.tasks/completed/index.md",
	".ahm/.tasks/cancelled/index.md",
	".ahm/.research/index.md",
	".ahm/tasks/index.md",
	".ahm/tasks/active/index.md",
	".ahm/tasks/completed/index.md",
	".ahm/tasks/cancelled/index.md",
	".ahm/research/index.md",
	".ahm/exec-plans/active/index.md",
	".ahm/exec-plans/completed/index.md",
	"docs/adr/index.md",
}

func generatedIndexTargets() []string {
	return generatedIndexTargetsList
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderRootIndex(tasks []Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Task Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Status Summary")
	fmt.Fprintln(&b)
	counts := taskCounts(tasks)
	for _, status := range statusOrder() {
		fmt.Fprintf(&b, "- %s: %d\n", status, counts[status])
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Next Ready Queue")
	fmt.Fprintln(&b)
	ready := filterTasks(tasks, "ready")
	if len(ready) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		for i, task := range ready {
			fmt.Fprintf(&b, "%d. [%s](active/%s.md) - %s (%s, %s; %s)\n", i+1, task.ID, task.ID, escapeCell(task.Title), task.Priority, task.Effort, task.Labels)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Blocked Or Needs Triage")
	fmt.Fprintln(&b)
	waiting := filterTasks(tasks, "blocked")
	for _, task := range tasks {
		if task.Status == "Open" {
			waiting = append(waiting, task)
		}
	}
	if len(waiting) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, waiting, ".")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## All Tasks")
	fmt.Fprintln(&b)
	writeTaskTable(&b, tasks, ".")
	return b.String()
}

type markdownDoc struct {
	Link  string
	Title string
}

func collectMarkdownDocs(root string, base string, buckets []string) (map[string][]markdownDoc, error) {
	docs := map[string][]markdownDoc{}
	for _, bucket := range buckets {
		docs[bucket] = nil
		dir := filepath.Join(root, base, bucket)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			title, err := markdownTitle(path)
			if err != nil {
				return nil, err
			}
			link := entry.Name()
			if bucket != "" {
				link = filepath.ToSlash(filepath.Join(bucket, entry.Name()))
			}
			docs[bucket] = append(docs[bucket], markdownDoc{Link: link, Title: title})
		}
		sort.Slice(docs[bucket], func(i, j int) bool {
			return docs[bucket][i].Link < docs[bucket][j].Link
		})
	}
	return docs, nil
}

func markdownTitle(path string) (string, error) {
	data, err := readWorkflowFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			title = strings.TrimSpace(title)
			if title != "" {
				return title, nil
			}
		}
	}
	name := filepath.Base(path)
	return strings.TrimSuffix(name, filepath.Ext(name)), nil
}

func renderResearchIndex(docs map[string][]markdownDoc) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Research Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	writeDocSection(&b, "Inbox", "No inbox research documents yet.", docs["inbox"])
	writeDocSection(&b, "Investigations", "No investigation documents yet.", docs["investigations"])
	writeDocSection(&b, "Sources", "No source notes yet.", docs["sources"])
	writeDocSection(&b, "Topics", "No topic documents yet.", docs["topics"])
	writeDocSection(&b, "Archived", "No archived research documents yet.", docs["archived"])
	return b.String()
}

func renderADRIndex(adrs []ADR) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# ADR Index")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| ADR | Title | Status | Date |")
	fmt.Fprintln(&b, "| --- | ----- | ------ | ---- |")
	for _, adr := range adrs {
		if adr.Kind == adrKindMalformed {
			continue
		}
		fmt.Fprintf(&b, "| [ADR-%s](%s-%s.md) | %s | %s | %s |\n",
			adr.ID,
			adr.ID,
			adr.Slug,
			escapeCell(adr.Title),
			escapeCell(adr.Status),
			escapeCell(adr.Date),
		)
	}
	return b.String()
}

func renderExecPlanIndex(title string, empty string, docs map[string][]markdownDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	writeDocList(&b, empty, docs[""])
	return b.String()
}

func writeDocSection(b *strings.Builder, title string, empty string, docs []markdownDoc) {
	fmt.Fprintf(b, "## %s\n\n", title)
	writeDocList(b, empty, docs)
	fmt.Fprintln(b)
}

func writeDocList(b *strings.Builder, empty string, docs []markdownDoc) {
	if len(docs) == 0 {
		fmt.Fprintln(b, empty)
		return
	}
	for _, doc := range docs {
		fmt.Fprintf(b, "- [%s](%s)\n", escapeCell(doc.Title), doc.Link)
	}
}

func renderBucketIndex(tasks []Task, bucket string) string {
	var b strings.Builder
	title := bucketTitle(bucket)
	fmt.Fprintf(&b, "# %s Tasks\n\n", title)
	fmt.Fprintln(&b, "This file is generated by `ahm index`. Do not edit it by hand.")
	fmt.Fprintln(&b)
	var bucketTasks []Task
	for _, task := range tasks {
		if task.Bucket == bucket {
			bucketTasks = append(bucketTasks, task)
		}
	}
	if len(bucketTasks) == 0 {
		fmt.Fprintln(&b, "None.")
	} else {
		writeTaskTable(&b, bucketTasks, bucket)
	}
	return b.String()
}

func writeTaskTable(b *strings.Builder, tasks []Task, from string) {
	fmt.Fprintln(b, "| Task | Title | Status | Priority | Effort | Labels | ExecPlan | Depends on |")
	fmt.Fprintln(b, "| ---- | ----- | ------ | -------- | ------ | ------ | -------- | ---------- |")
	for _, task := range tasks {
		link := task.ID + ".md"
		if from == "." {
			link = filepath.ToSlash(filepath.Join(task.Bucket, task.ID+".md"))
		}
		fmt.Fprintf(
			b,
			"| [%s](%s) | %s | %s | %s | %s | %s | %s | %s |\n",
			escapeCell(task.ID),
			link,
			escapeCell(task.Title),
			escapeCell(task.Status),
			escapeCell(task.Priority),
			escapeCell(task.Effort),
			escapeCell(task.Labels),
			escapeCell(task.ExecPlan),
			escapeCell(formatList(task.DependsOn)),
		)
	}
}

// escapeCell escapes characters that have special meaning in Markdown table
// cells or could be interpreted as HTML. Currently handles pipes (|),
// backticks (`), angle brackets (<, >), square brackets ([, ]), and newlines.
func escapeCell(value string) string {
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "[", "\\[")
	value = strings.ReplaceAll(value, "]", "\\]")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func bucketTitle(bucket string) string {
	return strings.ToUpper(bucket[:1]) + bucket[1:]
}

func statusOrder() []string {
	return []string{"Open", "Pending", "In Progress", "Blocked", "Tracking", "Completed", "Cancelled"}
}

func priorityOrder() []string {
	return []string{"P0", "P1", "P2", "P3", "P4"}
}

func effortOrder() []string {
	return []string{"XS", "S", "M", "L", "XL"}
}
