package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
)

// ref is an input-local name, never a requested task ID.
type taskImportRecord struct {
	Ref         string   `json:"ref"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Effort      string   `json:"effort"`
	Labels      string   `json:"labels"`
	Created     string   `json:"created"`
	Parent      string   `json:"parent"`
	DependsOn   []string `json:"depends_on"`
	ExternalRef string   `json:"external_ref"`
}

type taskImportOutcome struct {
	Ref       string   `json:"ref"`
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Parent    string   `json:"parent"`
	DependsOn []string `json:"depends_on"`
	Outcome   string   `json:"outcome"`
	Errors    []string `json:"errors"`
}

type taskImportReport struct {
	DryRun  bool                `json:"dry_run"`
	Records []taskImportOutcome `json:"records"`
}

func (a *app) taskImportCommand() *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use: "import --from-file <path>", Short: "Import a batch of tasks from JSON",
		Long: "Import a JSON array of tasks after validating the entire batch. Use @ref for references within the file.\n\nExamples:\n  ahm --dry-run task import --from-file tasks.json\n  ahm task import --from-file tasks.json",
		Args: noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if source == "" {
				return usageError("task import requires --from-file <path>")
			}
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskImport(source)
		},
	}
	cmd.Flags().StringVar(&source, "from-file", "", "JSON task array from a file (or - for stdin)")
	return cmd
}

func (a *app) taskImport(source string) error {
	var data []byte
	var err error
	if source == "-" {
		if a.in == nil {
			return usageError("task import --from-file - requires stdin")
		}
		data, err = io.ReadAll(a.in)
	} else {
		data, err = os.ReadFile(source) // #nosec G304 // The explicit --from-file flag authorizes reading this input path.
	}
	if err != nil {
		return fmt.Errorf("reading import file: %w", err)
	}
	records, err := decodeTaskImport(data)
	if err != nil {
		return err
	}
	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		defer a.emitWarnings()
		a.invalidateTasks()
		existing, err := a.getTasks()
		if err != nil {
			return fmt.Errorf("reading tasks before import: %w", err)
		}
		tasks, report, err := a.planTaskImport(records, existing)
		if err != nil {
			return err
		}
		refused := false
		for i := range report.Records {
			if len(report.Records[i].Errors) > 0 {
				report.Records[i].Outcome = "refused"
				refused = true
			}
		}
		if refused {
			for i := range report.Records {
				if report.Records[i].Outcome != "refused" {
					report.Records[i].Outcome = "not_imported"
				}
			}
			if err := a.emit(report); err != nil {
				return err
			}
			return errValidationFailed
		}
		if !a.opts.dryRun && len(tasks) > 0 {
			if err := a.writeTaskImport(tasks, existing); err != nil {
				return err
			}
			for i := range report.Records {
				report.Records[i].Outcome = "imported"
			}
		}
		// Report title collisions only for a batch that lands (or, in dry-run,
		// would land); a refused batch introduces no record, so there is nothing
		// to flag. A warning never changes the report or the outcome.
		a.warnImportDuplicateTitles(existing, tasks)
		return a.emit(report)
	})
}

// warnImportDuplicateTitles reports each imported record whose title collides,
// case-insensitively, with an active pre-existing record or with an earlier
// record in the same batch. The batch is bulk input, so a per-record stderr
// warning matches the `task create` rule rather than inventing an aggregate
// one, and the structured report is left untouched. Completed and Cancelled
// records are skipped on both sides, exactly as warnDuplicateTitle does.
func (a *app) warnImportDuplicateTitles(existing []Task, tasks []Task) {
	active := slices.Clone(existing)
	for _, task := range tasks {
		a.warnDuplicateTitle(active, task.ID, task.Title)
		active = append(active, task)
	}
}

// Decode the document shape separately so JSON null cannot silently become a
// zero-valued object, defaulted scalar, or omitted dependency list.
func decodeTaskImport(data []byte) ([]taskImportRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var objects []json.RawMessage
	if err := decoder.Decode(&objects); err != nil {
		return nil, usageError("invalid task import JSON: " + err.Error())
	}
	if objects == nil {
		return nil, usageError("task import requires a JSON array, not null")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, usageError("task import requires exactly one JSON array")
	}
	records := make([]taskImportRecord, len(objects))
	for i, object := range objects {
		fields, err := decodeTaskImportObject(object)
		if err != nil {
			return nil, usageError(fmt.Sprintf("invalid task import record %d: %s", i+1, err))
		}
		for _, key := range sortedStringKeys(fields) {
			if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
				return nil, usageError(fmt.Sprintf("task import record %d field %s must not be null", i+1, key))
			}
		}
		for _, key := range sortedStringKeys(fields) {
			if !strings.EqualFold(key, "depends_on") {
				continue
			}
			dependencies := fields[key]
			var values []json.RawMessage
			if err := json.Unmarshal(dependencies, &values); err != nil {
				return nil, usageError(fmt.Sprintf("task import record %d depends_on must be an array of strings", i+1))
			}
			for _, value := range values {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return nil, usageError(fmt.Sprintf("task import record %d depends_on entries must not be null", i+1))
				}
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(object))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&records[i]); err != nil {
			return nil, usageError(fmt.Sprintf("invalid task import record %d: %s", i+1, err))
		}
	}
	return records, nil
}

// Read keys before building the map: unmarshaling straight into a map would
// discard duplicate fields before validation can refuse them.
func decodeTaskImportObject(object []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(object))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, fmt.Errorf("record must be an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("field name must be a string")
		}
		for previous := range fields {
			if strings.EqualFold(previous, key) {
				return nil, fmt.Errorf("duplicate field %q", key)
			}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return fields, nil
}

func (a *app) planTaskImport(records []taskImportRecord, existing []Task) ([]Task, taskImportReport, error) {
	paths := a.workflowPaths()
	report := taskImportReport{DryRun: a.opts.dryRun, Records: make([]taskImportOutcome, len(records))}
	tasks := make([]Task, len(records))
	refs := map[string]int{}
	now := time.Now().Format(time.RFC3339)
	add := func(i int, message string) { report.Records[i].Errors = append(report.Records[i].Errors, message) }
	for i, record := range records {
		report.Records[i] = taskImportOutcome{Ref: record.Ref, DependsOn: []string{}, Errors: []string{}, Outcome: "planned"}
		if record.Ref != "" {
			if strings.ContainsRune(record.Ref, '@') || strings.ContainsFunc(record.Ref, unicode.IsSpace) {
				add(i, "ref must not contain @ or whitespace")
			}
			if previous, ok := refs[record.Ref]; ok {
				add(i, "duplicate ref "+record.Ref)
				add(previous, "duplicate ref "+record.Ref)
			} else {
				refs[record.Ref] = i
			}
		}
		if record.Status == "" {
			record.Status = "Open"
		}
		if record.Priority == "" {
			record.Priority = "P2"
		}
		if record.Effort == "" {
			record.Effort = "S"
		}
		if record.Labels == "" {
			record.Labels = "type:task, area:unknown"
		}
		if record.Created == "" {
			record.Created = now
		}
		for _, field := range []struct{ name, value string }{{"title", record.Title}, {"labels", record.Labels}, {"external_ref", record.ExternalRef}} {
			if strings.ContainsAny(field.value, "\r\n") {
				add(i, field.name+" must not contain newlines")
			}
			if strings.TrimSpace(field.value) != field.value {
				add(i, field.name+" must not have leading or trailing whitespace")
			}
		}
		if record.Title == "" {
			add(i, "title is required")
		}
		if !validTaskStatus(record.Status) {
			add(i, enumError("status", record.Status, statusOrder()))
		}
		if !validTaskPriority(record.Priority) {
			add(i, enumError("priority", record.Priority, priorityOrder()))
		}
		if !validTaskEffort(record.Effort) {
			add(i, enumError("effort", record.Effort, effortOrder()))
		}
		if _, err := time.Parse(time.RFC3339, record.Created); err != nil {
			add(i, "created must be an RFC3339 timestamp")
		}
		bucket := "active"
		if record.Status == "Completed" {
			bucket = "completed"
		}
		if record.Status == "Cancelled" {
			bucket = "cancelled"
		}
		tasks[i] = Task{Title: record.Title, Body: stripHeading(strings.ReplaceAll(record.Body, "\r\n", "\n"), record.Title), Status: record.Status, Priority: record.Priority, Effort: record.Effort, Labels: record.Labels, Created: record.Created, Updated: now, ExternalRef: record.ExternalRef, Bucket: bucket}
	}
	simulated := append([]Task{}, existing...)
	// Allocate top-level records first, irrespective of where their children occur.
	for i, record := range records {
		if record.Parent != "" {
			continue
		}
		id, err := nextTaskIDForPaths(simulated, paths)
		if err != nil {
			return nil, report, err
		}
		tasks[i].ID = id
		simulated = append(simulated, tasks[i])
	}
	resolve := func(reference string) (Task, error) {
		if strings.HasPrefix(reference, "@") {
			index, ok := refs[strings.TrimPrefix(reference, "@")]
			if !ok {
				return Task{}, fmt.Errorf("unknown batch reference %q", reference)
			}
			if tasks[index].ID == "" {
				return Task{}, fmt.Errorf("batch reference %q has no allocated top-level ID", reference)
			}
			return tasks[index], nil
		}
		if _, _, ok := splitTaskID(reference); !ok {
			return Task{}, fmt.Errorf("invalid task reference %q", reference)
		}
		task, err := resolveTaskFromTasks(reference, existing)
		if err != nil {
			return Task{}, err
		}
		if err := checkDuplicateTaskID(existing, task.ID, paths); err != nil {
			return Task{}, err
		}
		return task, nil
	}
	for i, record := range records {
		if record.Parent == "" {
			continue
		}
		parent, err := resolve(record.Parent)
		if err != nil {
			add(i, "parent: "+err.Error())
			continue
		}
		_, suffix, ok := splitTaskID(parent.ID)
		if !ok || suffix != "" {
			add(i, "parent must be a top-level task")
			continue
		}
		id, err := nextChildTaskIDForPaths(simulated, paths, parent.ID)
		if err != nil {
			add(i, err.Error())
			continue
		}
		tasks[i].ID, tasks[i].Parent = id, parent.ID
		simulated = append(simulated, tasks[i])
	}
	for i, record := range records {
		seen := map[string]bool{}
		for _, reference := range record.DependsOn {
			dep, err := resolve(reference)
			if err != nil {
				add(i, "dependency: "+err.Error())
				continue
			}
			if dep.ID == tasks[i].ID {
				add(i, "task cannot depend on itself")
				continue
			}
			if dep.Status == "Cancelled" && tasks[i].Status != "Completed" && tasks[i].Status != "Cancelled" {
				add(i, "active task cannot depend on cancelled task "+dep.ID)
			}
			if !seen[dep.ID] {
				tasks[i].DependsOn = append(tasks[i].DependsOn, dep.ID)
				seen[dep.ID] = true
			}
		}
		sort.Slice(tasks[i].DependsOn, func(j, k int) bool { return taskLess(tasks[i].DependsOn[j], tasks[i].DependsOn[k]) })
		if tasks[i].ID != "" {
			tasks[i].Path = paths.taskFile(tasks[i].Bucket, tasks[i].ID)
			if _, err := os.Lstat(tasks[i].Path); err == nil {
				add(i, "target already exists")
			} else if !errors.Is(err, os.ErrNotExist) {
				add(i, "checking target: "+err.Error())
			}
		}
		report.Records[i].ID = tasks[i].ID
		if tasks[i].Path != "" {
			report.Records[i].Path = paths.payloadPath(tasks[i].Path)
		}
		report.Records[i].Parent = tasks[i].Parent
		report.Records[i].DependsOn = append(report.Records[i].DependsOn, tasks[i].DependsOn...)
	}
	combined := append(append([]Task{}, existing...), tasks...)
	sort.Slice(combined, func(i, j int) bool { return taskLess(combined[i].ID, combined[j].ID) })
	for _, cycle := range taskDependencyCycles(combined) {
		for i, task := range tasks {
			if containsString(cycle, task.ID) {
				add(i, "dependency cycle: "+strings.Join(cycle, " -> "))
			}
		}
	}
	return tasks, report, nil
}

// The hook injects ordinary write failures in transaction tests.
var taskImportWriteHook = func(string) error { return nil }

type taskImportBackup struct {
	path   string
	data   []byte
	exists bool
}

func (a *app) writeTaskImport(tasks, existing []Task) error {
	paths := a.workflowPaths()
	if paths.inStore() {
		return withStoreStateLock(paths.store, func() error { return a.writeTaskImportLocked(tasks, existing) })
	}
	return a.writeTaskImportLocked(tasks, existing)
}

// Caller holds the record lock and, in home mode, the store-state lock.
func (a *app) writeTaskImportLocked(tasks, existing []Task) (result error) {
	paths := a.workflowPaths()
	for i, task := range tasks {
		parsed, err := parseTaskFromData([]byte(renderTask(task)), task.Path, task.Bucket)
		if err != nil {
			return err
		}
		tasks[i] = parsed
	}
	combined := append(append([]Task{}, existing...), tasks...)
	sort.Slice(combined, func(i, j int) bool { return taskLess(combined[i].ID, combined[j].ID) })
	writes, err := indexWritesForPaths(a.opts.root, combined, paths, newRecordCache())
	if err != nil {
		return err
	}
	var backups []taskImportBackup
	snapshot := func(path string, newRecord bool) error {
		data, err := os.ReadFile(path) // #nosec G304 // Snapshot targets come only from resolved record, index, and counter accessors.
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if newRecord && err == nil {
			return fmt.Errorf("import target already exists: %s", paths.displayPath(path))
		}
		backups = append(backups, taskImportBackup{path: path, data: data, exists: err == nil})
		return nil
	}
	for _, task := range tasks {
		if err := snapshot(task.Path, true); err != nil {
			return err
		}
	}
	for _, path := range sortedKeys(writes) {
		if err := snapshot(path, false); err != nil {
			return err
		}
	}
	if path, ok := paths.storeStatePath(); ok {
		if err := snapshot(path, false); err != nil {
			return err
		}
	}
	// Track attempted writes, including a rename followed by a failed directory sync.
	touched := map[string]bool{}
	defer func() {
		if result == nil {
			a.invalidateTasks()
			return
		}
		var restoration []error
		for i := len(backups) - 1; i >= 0; i-- {
			backup := backups[i]
			if !touched[backup.path] {
				continue
			}
			var err error
			if backup.exists {
				err = writeOwned(paths, backup.path, backup.data)
			} else {
				err = os.Remove(backup.path)
				if errors.Is(err, os.ErrNotExist) {
					err = nil
				}
			}
			if err != nil {
				restoration = append(restoration, fmt.Errorf("restore %s: %w", paths.displayPath(backup.path), err))
			}
		}
		a.invalidateTasks()
		if len(restoration) > 0 {
			result = errors.Join(result, fmt.Errorf("import rollback incomplete; inspect batch paths and run ahm index: %w", errors.Join(restoration...)))
		}
	}()
	write := func(path string, data []byte) error {
		if err := taskImportWriteHook(path); err != nil {
			return err
		}
		touched[path] = true
		return writeOwned(paths, path, data)
	}
	for _, task := range tasks {
		if err := write(task.Path, []byte(renderTask(task))); err != nil {
			return err
		}
	}
	for _, path := range sortedKeys(writes) {
		if err := write(path, []byte(writes[path])); err != nil {
			return err
		}
	}
	if path, ok := paths.storeStatePath(); ok {
		if err := taskImportWriteHook(path); err != nil {
			return err
		}
		touched[path] = true
		marks := childSuffixMarksFromRecords(combined, paths)
		if err := mutateProjectStateLocked(paths, func(state *projectState) {
			state.NextID = higherTaskIDCounter(highestTaskNumber(combined, paths)+1, state.NextID)
			for parent, suffix := range marks {
				raiseChildSuffixMark(state, parent, suffix)
			}
		}); err != nil {
			return err
		}
	}
	a.tasksCache = combined
	a.emitPostMutationFindings(combined, writes, true, newRecordCache())
	return nil
}

func (report taskImportReport) RenderText(w io.Writer) error {
	for i, record := range report.Records {
		if _, err := fmt.Fprintf(w, "record %d: %s\n  ref: %s\n  id: %s\n  path: %s\n  parent: %s\n  depends_on: %s\n", i+1, record.Outcome, defaultDash(record.Ref), defaultDash(record.ID), defaultDash(record.Path), defaultDash(record.Parent), formatList(record.DependsOn)); err != nil {
			return err
		}
		for _, message := range record.Errors {
			if _, err := fmt.Fprintln(w, "  refusal:", message); err != nil {
				return err
			}
		}
	}
	if len(report.Records) == 0 {
		_, err := fmt.Fprintln(w, "records: []")
		return err
	}
	return nil
}
