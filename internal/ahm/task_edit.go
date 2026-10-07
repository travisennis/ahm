package ahm

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// protectedTaskSections are the task body sections that a command other than
// `task edit` owns. Both hold machine-generated provenance — a comment log and
// a cancellation reason — so an edit that replaces them would destroy a record
// that `task comment` and `task cancel` are the only writers of. Each entry
// names the command that owns the section so the refusal can point at it.
var protectedTaskSections = []struct {
	Name  string
	Owner string
}{
	{Name: "Comments", Owner: "task comment"},
	{Name: "Cancellation Reason", Owner: "task cancel"},
}

// taskEditArgs carries the parsed `task edit` flags. Every flag has replace or
// additive semantics that only apply when the caller supplied it, so `set`
// records which flags were actually present: `--priority` equal to the current
// value is a no-op, not a write, and an omitted flag is not a request to clear
// a field.
type taskEditArgs struct {
	id                string
	title             string
	priority          string
	effort            string
	externalRef       string
	parent            string
	clearParent       bool
	body              string
	bodyFile          string
	section           string
	set               map[string]bool
	addLabelValues    []string // raw --add-label flag values, split before the lock
	removeLabelValues []string // raw --remove-label flag values, split before the lock
	resolvedParentID  string   // set after parent validation, used inside the locked section

	addLabels    []string // parsed --add-label values, validated before the lock
	removeLabels []string // parsed --remove-label values, validated before the lock
}

// taskEditChange is one changed field, in report order.
type taskEditChange struct {
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// taskEditReport is the structured payload of `task edit` and the source of its
// text output. `Changed` always renders as an array, empty on the unchanged
// path, so an agent can assert on it without a null check.
type taskEditReport struct {
	ID      string           `json:"id"`
	Path    string           `json:"path"`
	DryRun  bool             `json:"dry_run,omitempty"`
	Updated bool             `json:"updated"`
	Changed []string         `json:"changed"`
	Changes []taskEditChange `json:"changes,omitempty"`
	Task    *Task            `json:"task,omitempty"`

	// previewPath is the dry-run text rendering of the record path. It is not a
	// structured field: `Path` stays a recordPath rendering so a store record is
	// never printed as an absolute machine path, while the dry-run line uses
	// the slash-normalized form every other preview uses.
	previewPath string
}

// RenderText implements the textRenderer interface for taskEditReport. A real
// write reports the fields it changed; a dry run reports the record it would
// write and a per-field diff so an agent can review the change before applying
// it.
func (r taskEditReport) RenderText(w io.Writer) error {
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format+"\n", args...)
		return err
	}
	if len(r.Changed) == 0 {
		return write("%s unchanged", r.ID)
	}
	if r.DryRun {
		preview := r.previewPath
		if preview == "" {
			preview = r.Path
		}
		if err := write("%s edit: %s", r.ID, preview); err != nil {
			return err
		}
		for _, change := range r.Changes {
			if change.From == "" && change.To == "" {
				if err := write("%s %s: updated", r.ID, change.Field); err != nil {
					return err
				}
				continue
			}
			if err := write("%s %s: %s -> %s", r.ID, change.Field, change.From, change.To); err != nil {
				return err
			}
		}
		return nil
	}
	return write("%s updated (%s)", r.ID, strings.Join(r.Changed, ", "))
}

func (a *app) taskEditCommand() *cobra.Command {
	args := taskEditArgs{}
	cmd := &cobra.Command{
		Use:   "edit <id> [flags]",
		Short: "Edit a task's fields and sections",
		Long: `Edit an existing task's front matter and body sections.

Flags replace a field when supplied and leave it alone when omitted. --add-label
and --remove-label adjust the label set without clobbering it. --add-label
accepts only labels already in use somewhere in the records, which is the
vocabulary "ahm task labels" reports; introduce a new label through
"task create --labels". Status and dependencies belong to task
accept|start|complete|cancel|reopen and task dep add|remove, and this command
refuses to write them.

The ## Comments and ## Cancellation Reason sections are owned by task comment
and task cancel. Use --section to rewrite any other section; a whole-body
replacement that would drop one of those sections is refused unless --force.

Examples:
  ahm task edit 270 --priority P1 --effort S
  ahm task edit 270 --add-label area:docs --remove-label area:cli
  ahm task edit 270 --section "Fix Direction" --body "Change the parser."
  ahm task edit 270 --body-file body.md
  ahm --dry-run task edit 270 --title "New title"`,
		Args: exactArgs(1, "task edit requires an id\n  ahm task edit <id> [flags]"),
		RunE: func(cmd *cobra.Command, positional []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			args.id = positional[0]
			args.set = taskEditSetFlags(cmd)
			return a.taskEdit(args)
		},
	}
	cmd.Flags().StringVarP(&args.title, "title", "t", "", "Replace the task title")
	cmd.Flags().StringVarP(&args.priority, "priority", "p", "", "Replace the task priority")
	cmd.Flags().StringVar(&args.effort, "effort", "", "Replace the task effort")
	cmd.Flags().StringArrayVar(&args.addLabelValues, "add-label", nil, "Label to add; must already be in use (see 'task labels'); comma-separated or repeatable")
	cmd.Flags().StringArrayVar(&args.removeLabelValues, "remove-label", nil, "Label to remove from the set; comma-separated or repeatable")
	cmd.Flags().StringVar(&args.externalRef, "external-ref", "", "Replace the external reference; an empty value clears it")
	cmd.Flags().StringVar(&args.parent, "parent", "", "Replace the parent task ID")
	cmd.Flags().BoolVar(&args.clearParent, "clear-parent", false, "Remove the parent task ID")
	cmd.Flags().StringVarP(&args.body, "body", "b", "", "Replacement body text; exclusive with --body-file")
	cmd.Flags().StringVarP(&args.bodyFile, "body-file", "F", "", "Replacement body from a file (or - for stdin)")
	cmd.Flags().StringVar(&args.section, "section", "", "Scope --body/--body-file to this heading instead of the whole body")
	return cmd
}

// taskEditFlags are the mutation flags `task edit` accepts, in help order.
var taskEditFlags = []string{
	"title", "priority", "effort", "add-label", "remove-label", "external-ref",
	"parent", "clear-parent", "body", "body-file", "section",
}

// taskEditSetFlags records which flags the caller actually supplied, so an
// omitted flag is distinguishable from one set to its zero value.
func taskEditSetFlags(cmd *cobra.Command) map[string]bool {
	set := map[string]bool{}
	for _, name := range taskEditFlags {
		if cmd.Flags().Changed(name) {
			set[name] = true
		}
	}
	return set
}

// taskEditPreLockHook is called in taskEdit after the flags are validated and
// before the workflow record lock is acquired. It exists for tests that need a
// deterministic ordering between a concurrent write and this edit, the same
// seam taskStatusPreLockHook provides for the status commands.
var taskEditPreLockHook = func() {}

func (a *app) taskEdit(args taskEditArgs) error {
	if err := validateTaskEditArgs(args); err != nil {
		return err
	}
	addLabels, removeLabels, err := validateTaskEditLabels(args)
	if err != nil {
		return err
	}
	args.addLabels = addLabels
	args.removeLabels = removeLabels
	body, hasBody, err := a.resolveTaskEditBody(args)
	if err != nil {
		return err
	}
	if args.set["parent"] {
		// Resolve the parent before the lock for fast validation (read-only).
		// Re-resolution inside the locked section uses the stored ID.
		parent, err := a.resolveTaskForMutation(args.parent)
		if err != nil {
			return usageError(fmt.Sprintf("parent task %q: %s", args.parent, err))
		}
		if _, suffix, ok := splitTaskID(parent.ID); ok && suffix != "" {
			return usageError(fmt.Sprintf("parent task %q is a child task; only top-level tasks can be parents", args.parent))
		}
		args.resolvedParentID = parent.ID
	}

	taskEditPreLockHook()

	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		// Re-resolve from fresh on-disk state under the lock so a concurrent
		// update that landed before lock acquisition is preserved instead of
		// being overwritten by a pre-lock Task value.
		a.invalidateTasks()
		task, err := a.resolveTaskForMutation(args.id)
		if err != nil {
			return err
		}
		return a.taskEditLocked(args, task, body, hasBody)
	})
}

func (a *app) taskEditLocked(args taskEditArgs, task Task, body string, hasBody bool) error {
	defer a.emitWarnings()
	if err := a.validateTaskEditAddedLabels(args.addLabels); err != nil {
		return err
	}
	paths := a.workflowPaths()

	changes, updated, err := editTaskFields(task, args, body, hasBody, a.opts.force)
	if err != nil {
		return err
	}
	report := taskEditReport{
		ID:      task.ID,
		Path:    paths.recordPath(task.Path),
		DryRun:  a.opts.dryRun,
		Updated: len(changes) > 0 && !a.opts.dryRun,
		Changed: []string{},
		Changes: changes,
	}
	if a.opts.dryRun {
		report.previewPath = paths.payloadPath(task.Path)
	}
	if !a.opts.dryRun && len(changes) > 0 {
		// Stamp before the reported record is rendered, so the payload's
		// `updated` is the value the write actually lands.
		updated.Updated = time.Now().Format(time.RFC3339)
	}
	// The resulting record is reported on every path, including the unchanged
	// one, so an agent can assert on the state it asked for without a second
	// read.
	out := a.taskForOutput(updated)
	report.Task = &out
	for _, change := range changes {
		report.Changed = append(report.Changed, change.Field)
	}
	if len(changes) == 0 {
		return a.emit(report)
	}
	if a.opts.dryRun {
		return a.emit(report)
	}
	// An edit never moves a record between buckets; only status transitions do,
	// so the record is rewritten in place.
	if err := writeOwned(paths, task.Path, []byte(renderTask(updated))); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	a.invalidateTasks()
	return a.emit(report)
}

// validateTaskEditArgs checks everything that does not need the record on disk:
// flag exclusivity, enum values, protected section names, and the requirement
// that some flag was supplied at all.
func validateTaskEditArgs(args taskEditArgs) error {
	if len(args.set) == 0 {
		return usageError(taskEditUsageLine())
	}
	if args.set["body"] && args.set["body-file"] {
		return usageError("task edit supports --body or --body-file, not both")
	}
	if args.set["body-file"] && args.bodyFile == "" {
		return usageError("task edit --body-file cannot be empty")
	}
	if args.set["section"] {
		if err := validateTaskEditSection(args.section); err != nil {
			return err
		}
		if owner, ok := protectedSectionOwner(args.section); ok {
			return usageError(fmt.Sprintf("task edit --section %q is owned by %s; use %s to write it", args.section, owner, owner))
		}
		if !args.set["body"] && !args.set["body-file"] {
			return usageError("task edit --section requires --body or --body-file")
		}
	}
	if args.set["title"] {
		if args.title == "" {
			return usageError("task edit --title cannot be empty")
		}
		if strings.TrimSpace(args.title) != args.title {
			return usageError("task edit title must not have leading or trailing whitespace")
		}
		if strings.ContainsAny(args.title, "\n\r") {
			return usageError("task edit title must not contain newlines")
		}
	}
	if args.clearParent && args.set["parent"] {
		return usageError("task edit supports --parent or --clear-parent, not both")
	}
	if args.set["external-ref"] && strings.ContainsAny(args.externalRef, "\n\r") {
		return usageError("task edit external reference must not contain newlines")
	}
	if err := validateTaskEditEnums(args); err != nil {
		return err
	}
	return nil
}

// validateTaskEditSection rejects a section name that could not address a real
// heading. The name is spliced into the body as a `## <name>` heading, so an
// empty or heading-shaped name would write a heading no later lookup can match.
func validateTaskEditSection(name string) error {
	if strings.TrimSpace(name) == "" {
		return usageError("task edit --section cannot be empty")
	}
	if strings.ContainsAny(name, "\n\r") {
		return usageError("task edit --section must not contain newlines")
	}
	if strings.Contains(name, "#") {
		return usageError(fmt.Sprintf("task edit --section %q must be a heading name without # characters", name))
	}
	return nil
}

// parseTaskEditLabels splits every --add-label or --remove-label value into
// labels. The values are collected by a repeatable array flag and split here,
// rather than by pflag's comma-separated slice type, because that type silently
// truncates a value at a newline and drops an empty value, so a malformed label
// would reach the record unvalidated.
func parseTaskEditLabels(flag string, values []string) ([]string, error) {
	var labels []string
	for _, value := range values {
		if value == "" {
			return nil, usageError(fmt.Sprintf("task edit --%s cannot be empty", flag))
		}
		if strings.TrimSpace(value) != value {
			return nil, usageError(fmt.Sprintf("task edit --%s value %q must not have leading or trailing whitespace", flag, value))
		}
		if strings.ContainsAny(value, "\n\r") {
			return nil, usageError(fmt.Sprintf("task edit --%s value must not contain newlines", flag))
		}
		for _, part := range strings.Split(value, ",") {
			label := strings.TrimSpace(part)
			if label == "" {
				return nil, usageError(fmt.Sprintf("task edit --%s must be a comma-separated list of labels", flag))
			}
			// `-` and `[]` are the empty-list sentinels the front-matter format
			// uses; as a label value they would be written into the record and
			// then dropped again on the next read.
			if label == "-" || label == "[]" {
				return nil, usageError(fmt.Sprintf("task edit --%s value %q is the empty-list sentinel, not a label", flag, label))
			}
			labels = append(labels, label)
		}
	}
	return labels, nil
}

func validateTaskEditEnums(args taskEditArgs) error {
	if args.set["priority"] && !validTaskPriority(args.priority) {
		return usageError(enumError("priority", args.priority, priorityOrder()))
	}
	if args.set["effort"] && !validTaskEffort(args.effort) {
		return usageError(enumError("effort", args.effort, effortOrder()))
	}
	return nil
}

// validateTaskEditAddedLabels rejects an --add-label value the task corpus has
// never used. The vocabulary is the label set `ahm task labels` reports: every
// label carried by every parsed record. An agent that mistypes a label would
// otherwise write a label no other task shares and only find out later; a
// caller with a genuinely new label introduces it through `task create
// --labels`, which is the only path that extends the vocabulary.
//
// `--remove-label` is deliberately not validated. Removing a label the record
// does not carry is already a no-op, so there is no misspelling to catch, and
// rejecting an unknown removal would turn an idempotent cleanup into an error.
//
// A corpus that carries no labels at all has an empty vocabulary, so there is
// nothing to check an addition against and the label is accepted; a default
// `task create` writes `type:task, area:unknown`, so this is rare.
func (a *app) validateTaskEditAddedLabels(add []string) error {
	if len(add) == 0 {
		return nil
	}
	tasks, err := a.getTasks()
	if err != nil && len(tasks) == 0 {
		// The corpus could not be read at all, so there is no vocabulary to
		// check against; fail closed rather than accept an arbitrary label.
		return err
	}
	// A record that failed to parse is skipped here the same way every other
	// task command skips it, and resolveTaskForMutation already reported the
	// skip for this command, so this adds no second warning.
	vocabulary := map[string]bool{}
	for _, task := range tasks {
		for label := range taskLabelSet(task) {
			vocabulary[label] = true
		}
	}
	if len(vocabulary) == 0 {
		return nil
	}
	for _, label := range add {
		if !vocabulary[label] {
			return usageError(fmt.Sprintf(
				"task edit --add-label %q: label is not in use by any task; run `ahm task labels` to see the vocabulary, or introduce it with `task create --labels`",
				label))
		}
	}
	return nil
}

func validateTaskEditLabels(args taskEditArgs) ([]string, []string, error) {
	var add, remove []string
	for _, group := range []struct {
		flag   string
		values []string
		parsed *[]string
	}{
		{"add-label", args.addLabelValues, &add},
		{"remove-label", args.removeLabelValues, &remove},
	} {
		if !args.set[group.flag] {
			continue
		}
		labels, err := parseTaskEditLabels(group.flag, group.values)
		if err != nil {
			return nil, nil, err
		}
		*group.parsed = labels
	}
	return add, remove, nil
}

// taskEditUsageLine lists the mutation flags when `task edit` is invoked with
// none of them. The interactive editor this command used to launch is gone, so
// the message has to say what replaces it.
func taskEditUsageLine() string {
	return strings.Join([]string{
		"task edit requires at least one field to change",
		"  ahm task edit <id> --title <text> | --priority <p> | --effort <e> | --external-ref <ref>",
		"  ahm task edit <id> --add-label <labels> | --remove-label <labels>",
		"  ahm task edit <id> --parent <id> | --clear-parent",
		"  ahm task edit <id> [--section <name>] --body <text> | --body-file <file>",
	}, "\n")
}

// protectedSectionOwner reports whether name is a section owned by another
// command, and which one. Matching is case-insensitive because the section
// lookup that finds the section in a body is too.
func protectedSectionOwner(name string) (string, bool) {
	for _, section := range protectedTaskSections {
		if strings.EqualFold(strings.TrimSpace(name), section.Name) {
			return section.Owner, true
		}
	}
	return "", false
}

// resolveTaskEditBody resolves the replacement body from --body or --body-file.
// It reports whether a body was requested at all, so a field-only edit is not
// mistaken for a body replacement.
func (a *app) resolveTaskEditBody(args taskEditArgs) (string, bool, error) {
	if !args.set["body"] && !args.set["body-file"] {
		return "", false, nil
	}
	if args.set["body"] {
		body := strings.TrimSpace(strings.ReplaceAll(args.body, "\r\n", "\n"))
		if body == "" {
			return "", false, usageError("task edit --body cannot be empty")
		}
		return body, true, nil
	}
	var (
		data   []byte
		err    error
		source string
	)
	if args.bodyFile == "-" {
		source = "stdin"
		if a.in == nil {
			return "", false, usageError("task edit --body-file - requires stdin")
		}
		data, err = io.ReadAll(a.in)
	} else {
		source = args.bodyFile
		data, err = os.ReadFile(args.bodyFile)
	}
	if err != nil {
		return "", false, fmt.Errorf("reading task body from %s: %w", source, err)
	}
	body := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	if body == "" {
		return "", false, usageError(fmt.Sprintf("task body from %s is empty", source))
	}
	return body, true, nil
}

// editTaskFields applies the supplied flags to task and returns one entry per
// changed field, in a fixed order, alongside the resulting record. The caller
// writes nothing when the change list is empty.
//
// A whole-body replacement drops the machine-written sections the caller did
// not reproduce; that is refused as a usage error unless --force, because a
// silently deleted comment log or cancellation reason is an audit-trail loss
// the caller almost never intended.
func editTaskFields(task Task, args taskEditArgs, body string, hasBody bool, force bool) ([]taskEditChange, Task, error) {
	updated := task
	var changes []taskEditChange
	set := func(field string, from string, to string) {
		if from == to {
			return
		}
		changes = append(changes, taskEditChange{Field: field, From: from, To: to})
	}

	if args.set["title"] {
		updated.Title = args.title
		set("title", task.Title, updated.Title)
	}
	if args.set["priority"] {
		updated.Priority = args.priority
		set("priority", task.Priority, updated.Priority)
	}
	if args.set["effort"] {
		updated.Effort = args.effort
		set("effort", task.Effort, updated.Effort)
	}
	if args.set["add-label"] || args.set["remove-label"] {
		updated.Labels = formatList(editTaskLabels(parseList(task.Labels), args.removeLabels, args.addLabels))
		// Compare canonical forms: the stored string is whatever `task create`
		// was handed, so `type:task,area:cli` and `type:task, area:cli` are the
		// same label set and a no-op edit must report `unchanged`.
		set("labels", formatList(parseList(task.Labels)), updated.Labels)
	}
	if args.set["external-ref"] {
		updated.ExternalRef = args.externalRef
		set("external_ref", task.ExternalRef, updated.ExternalRef)
	}
	switch {
	case args.clearParent:
		updated.Parent = ""
		set("parent", task.Parent, updated.Parent)
	case args.set["parent"]:
		if args.resolvedParentID == task.ID {
			return nil, Task{}, usageError(fmt.Sprintf("task %s cannot be its own parent", task.ID))
		}
		updated.Parent = args.resolvedParentID
		set("parent", task.Parent, updated.Parent)
	}

	if hasBody {
		// Both body paths build the whole resulting body and then pass it
		// through the same audit-trail guard, so a section replacement cannot
		// delete protected content that a whole-body replacement refuses to
		// touch. A `### Comments` heading nested inside the section being
		// replaced is reachable this way, because a section runs to the next
		// heading of the same or a higher level.
		field := "body"
		var next string
		if args.set["section"] {
			field = "body:" + args.section
			next = replaceTaskSection(task.Body, args.section, body)
		} else {
			// renderTask emits the H1 from the front-matter title, so strip a
			// duplicate H1 from the replacement before it is stored.
			next = stripHeading(body, updated.Title)
		}
		// The comparison is whitespace-normalized because renderTask trims the
		// body it writes, so a replacement differing only in trailing whitespace
		// is a no-op.
		if strings.TrimSpace(next) == strings.TrimSpace(task.Body) {
			return changes, updated, nil
		}
		if dropped := droppedProtectedSections(task.Body, next); len(dropped) > 0 && !force {
			return nil, Task{}, usageError(fmt.Sprintf(
				"task edit %s: this edit does not carry over %s; keep that content, or use --force",
				task.ID, strings.Join(dropped, " or ")))
		}
		updated.Body = next
		// A section change reports no diff: the old and new text are whole
		// sections, and the field name already says which one.
		changes = append(changes, taskEditChange{Field: field})
	}
	return changes, updated, nil
}

// editTaskLabels applies the removals and then the additions to a label set,
// preserving the order of labels that survive. A label that is both removed
// and added ends up present, and a removal of a label the task does not carry
// is a no-op, so the command converges instead of reporting a conflict.
func editTaskLabels(labels []string, remove []string, add []string) []string {
	result := make([]string, 0, len(labels)+len(add))
	present := map[string]bool{}
	removed := map[string]bool{}
	for _, label := range remove {
		removed[label] = true
	}
	for _, label := range labels {
		if removed[label] || present[label] {
			continue
		}
		present[label] = true
		result = append(result, label)
	}
	for _, label := range add {
		if present[label] {
			continue
		}
		present[label] = true
		result = append(result, label)
	}
	return result
}

// droppedProtectedSections returns the protected sections whose existing
// content the replacement body fails to carry over.
//
// The rule is one sentence: the result's protected sections, in document order,
// must carry the current body's one for one, at the same heading depth, as the
// section's leading run of whole tokens. Pairing by position rather than
// searching the whole result is what aims the rule at the section a command will
// write to: `task comment` appends to the first `## Comments` heading it finds,
// so the first one in the result, not any matching copy, is the one that must
// carry the log. It also refuses a result that merges two existing sections into
// one, because the second has nothing to pair with.
//
// Anything else — a missing heading, an emptied heading, rewritten text, a
// `###` demotion or promotion, or content stranded under a second copy of a
// repeated heading — is a drop.
//
// A result may add protected sections after the ones it carries: an added
// section is not a drop, and the guard protects provenance rather than policing
// what a caller writes.
func droppedProtectedSections(current string, replacement string) []string {
	var dropped []string
	for _, section := range protectedTaskSections {
		existing := locateTaskSections(current, section.Name)
		if len(nonEmptySections(existing)) == 0 {
			// The record has no content anywhere in this section, so there is
			// nothing to lose and the replacement is free to omit or add the
			// heading.
			continue
		}
		if allCarried(existing, locateTaskSections(replacement, section.Name)) {
			continue
		}
		dropped = append(dropped, "## "+section.Name)
	}
	return dropped
}

// allCarried reports whether the replacement carries each existing section at
// the same position and heading depth. Each section's content must survive as
// its leading run of whole tokens, so a replacement may append to the run or
// re-wrap it but may not prepend to it. Every section is paired, not just the
// first: a record with two `## Comments` headings has two logs, and pairing the
// later ones by position is what refuses a result that merges or reorders them.
// An empty section carries nothing, and only an empty section carries it, so a
// result cannot satisfy the pair by filling the heading with new text.
func allCarried(existing []markdownHeadingSection, replacement []markdownHeadingSection) bool {
	for i, want := range existing {
		if i >= len(replacement) {
			return false
		}
		got := replacement[i]
		if got.Level != want.Level {
			return false
		}
		wantText := normalizeSectionText(want.Content)
		gotText := normalizeSectionText(got.Content)
		if wantText == "" {
			if gotText != "" {
				return false
			}
			continue
		}
		if !carriesLeadingRun(wantText, gotText) {
			return false
		}
	}
	return true
}

// nonEmptySections drops the matches that carry no content.
func nonEmptySections(sections []markdownHeadingSection) []markdownHeadingSection {
	kept := make([]markdownHeadingSection, 0, len(sections))
	for _, section := range sections {
		if strings.TrimSpace(section.Content) == "" {
			continue
		}
		kept = append(kept, section)
	}
	return kept
}

// normalizeSectionText collapses every whitespace run to a single space so that
// re-wrapping or re-indenting a preserved section still counts as carrying its
// content, while a changed, truncated, or replaced section does not.
func normalizeSectionText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// carriesLeadingRun reports whether got carries want as the leading run of whole
// tokens in its section. Both are normalized section texts, so one space after
// want is exactly a token boundary, and re-wrapping or re-indenting a preserved
// run still matches because normalizeSectionText has already collapsed their
// whitespace to single spaces.
//
// Requiring the run to lead is what refuses the demonstrated inversion. A
// negating or qualifying word prepended to the run — `Not Obsolete` for
// `Obsolete`, `Not LGTM` for `LGTM` — leaves the original tokens intact, so a
// bare containment check accepts it and records the opposite of the original
// meaning. Only additions after the run are allowed, which is how `task
// comment` grows a log. The guard refuses an accidental rewrite of provenance,
// not tampering with it: a caller can still append text after a preserved run,
// and `--force` remains the deliberate override.
func carriesLeadingRun(want string, got string) bool {
	return got == want || strings.HasPrefix(got, want+" ")
}

// locateTaskSections resolves the named heading section in body and returns
// each match with its content and heading level. The lookup matches level 2
// and level 3 headings case-insensitively, so a record that demotes a protected
// heading is still recognized.
func locateTaskSections(body string, name string) []markdownHeadingSection {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	found := locateHeadingSections(lines, []string{name})
	sections := make([]markdownHeadingSection, 0, len(found))
	for _, section := range found {
		if section.End > section.Start+1 {
			section.Content = strings.Join(lines[section.Start+1:section.End], "\n")
		}
		section.Level = headingLevel(lines[section.Start])
		sections = append(sections, section)
	}
	return sections
}

// replaceTaskSection returns body with the named heading section replaced by
// content, appending the section at the end of the body when it is absent.
// Every other section is preserved byte for byte.
func replaceTaskSection(body string, name string, content string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	heading := "## " + name
	content = strings.TrimSpace(content)

	sections := locateHeadingSections(lines, []string{name})
	if len(sections) == 0 {
		trimmed := strings.TrimSpace(body)
		section := heading + "\n\n" + content
		if trimmed == "" {
			return section
		}
		return trimmed + "\n\n" + section
	}

	// Preserve the established first-match behavior for a repeated heading.
	section := sections[0]
	replacement := []string{strings.TrimRight(lines[section.Start], " \t\r"), "", content}
	if section.End < len(lines) {
		replacement = append(replacement, "")
	}
	updated := append([]string{}, lines[:section.Start]...)
	updated = append(updated, replacement...)
	updated = append(updated, lines[section.End:]...)
	return strings.TrimSpace(strings.Join(updated, "\n"))
}
