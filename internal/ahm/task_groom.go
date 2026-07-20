package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type taskGroomArgs struct {
	id      string
	agent   string
	model   string
	timeout time.Duration
}

type groomResult struct {
	Verdicts []groomVerdict `json:"verdicts"`
}

type groomVerdict struct {
	Task       string         `json:"task"`
	Action     string         `json:"action"`
	Comment    string         `json:"comment"`
	AddDeps    []string       `json:"add_deps"`
	RemoveDeps []string       `json:"remove_deps"`
	Labels     []string       `json:"labels"`
	Revision   *groomRevision `json:"revision,omitempty"`
}

type groomRevision struct {
	Priority string                 `json:"priority"`
	Effort   string                 `json:"effort"`
	Sections []groomSectionRevision `json:"sections"`
}

type groomSectionRevision struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groomChange struct {
	Task        string               `json:"task"`
	Action      string               `json:"action"`
	Commented   bool                 `json:"commented"`
	AddedDeps   []string             `json:"added_deps,omitempty"`
	RemovedDeps []string             `json:"removed_deps,omitempty"`
	Labels      []string             `json:"labels,omitempty"`
	Priority    *groomValueChange    `json:"priority,omitempty"`
	Effort      *groomValueChange    `json:"effort,omitempty"`
	Sections    []groomSectionChange `json:"sections,omitempty"`
}

type groomValueChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type groomSectionChange struct {
	Role     string `json:"role"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Inserted bool   `json:"inserted"`
}

type groomSummary struct {
	Agent      string           `json:"agent"`
	Correction *groomCorrection `json:"correction,omitempty"`
	Changes    []groomChange    `json:"changes"`
}

type groomCorrection struct {
	Attempted        bool     `json:"attempted"`
	Succeeded        bool     `json:"succeeded"`
	ValidationErrors []string `json:"validation_errors"`
}

type groomValidationErrors struct {
	messages []string
}

func (e *groomValidationErrors) Error() string {
	return strings.Join(e.messages, "; ")
}

func (s groomSummary) RenderText(w io.Writer) error {
	if s.Correction != nil {
		fmt.Fprintln(w, "Correction retry succeeded after semantic validation failed:")
		for _, message := range s.Correction.ValidationErrors {
			fmt.Fprintf(w, "  - %s\n", message)
		}
	}
	if len(s.Changes) == 0 {
		fmt.Fprintln(w, "No grooming changes.")
		return nil
	}
	for _, change := range s.Changes {
		fmt.Fprintf(w, "%s: %s", change.Task, change.Action)
		if change.Commented {
			fmt.Fprint(w, ", commented")
		}
		if len(change.AddedDeps) > 0 {
			fmt.Fprintf(w, ", added deps %s", strings.Join(change.AddedDeps, ","))
		}
		if len(change.RemovedDeps) > 0 {
			fmt.Fprintf(w, ", removed deps %s", strings.Join(change.RemovedDeps, ","))
		}
		if len(change.Labels) > 0 {
			fmt.Fprintf(w, ", labels %s", strings.Join(change.Labels, ","))
		}
		if change.Priority != nil {
			fmt.Fprintf(w, ", priority %s", change.Priority.After)
		}
		if change.Effort != nil {
			fmt.Fprintf(w, ", effort %s", change.Effort.After)
		}
		if len(change.Sections) > 0 {
			roles := make([]string, len(change.Sections))
			for i := range change.Sections {
				roles[i] = change.Sections[i].Role
			}
			fmt.Fprintf(w, ", revised %s", strings.Join(roles, ","))
		}
		fmt.Fprintln(w)
	}
	return nil
}

const groomResultSchema = `{"type":"object","additionalProperties":false,"required":["verdicts"],"properties":{"verdicts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["task","action","comment","add_deps","remove_deps","labels","revision"],"properties":{"task":{"type":"string"},"action":{"type":"string","enum":["accept","comment","revise"]},"comment":{"type":"string"},"add_deps":{"type":"array","items":{"type":"string"}},"remove_deps":{"type":"array","items":{"type":"string"}},"labels":{"type":"array","items":{"type":"string"}},"revision":{"type":["object","null"],"additionalProperties":false,"required":["priority","effort","sections"],"properties":{"priority":{"type":"string","enum":["","P0","P1","P2","P3","P4"]},"effort":{"type":"string","enum":["","XS","S","M","L","XL"]},"sections":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["role","content"],"properties":{"role":{"type":"string","enum":["problem","relevant_files","fix_direction","acceptance_notes"]},"content":{"type":"string"}}}}}}}}}}}`

const groomProcedure = `Act as a backlog groomer. Inspect the listed task files and repository code as needed before judging readiness. Every Open task must either be ready to accept, receive an objective structured revision, or receive a precise comment explaining what remains. Review Blocked tasks for stale or incorrect dependencies. Verify priority, effort, type and area labels, dependency accuracy, decision completeness, relevant paths, and actionable acceptance notes. Use action=accept when the task is ready after applying the optional revision; if the revision resolves the final gap, you must accept rather than revise. Use action=revise only when a concrete blocker or human decision remains after the revision, and name that remaining issue in the required comment. Set revision=null when no revision is needed. Revisions may change only priority, effort, and the closed section roles problem, relevant_files, fix_direction, and acceptance_notes. Supply complete replacement section content. Never edit files, change protected metadata, cancel tasks, or run ahm mutation commands; ahm alone validates and applies your structured verdicts. A blocked task, cancel recommendation, or question requiring human judgment must use action=comment. Return only JSON matching the supplied schema.`

func (a *app) taskGroom(parsed taskGroomArgs) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		return fmt.Errorf("cannot groom with malformed task files: %w", err)
	}
	targets, err := groomTargets(tasks, parsed.id)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return a.emit(groomSummary{})
	}
	prompt := buildGroomPrompt(targets, summarizeTaskLabels(tasks))
	roles, err := a.resolveTaskWorkRoles(parsed.agent, parsed.model)
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		return a.emit(map[string]any{
			"agent": roles.implAgent.name, "prompt": prompt, "schema": groomResultSchema, "tasks": taskIDs(targets),
			"correction_retry": map[string]any{"available": true, "max_attempts": 1, "trigger": "semantic_validation_failure"},
		})
	}
	executable, err := taskWorkLookPath(roles.implAgent.executable)
	if err != nil {
		return fmt.Errorf("cannot groom with %s: executable %q not found on PATH", roles.implAgent.name, roles.implAgent.executable)
	}
	result, raw, err := a.runGroomDelegation(parsed, roles, executable, prompt)
	if err != nil {
		return fmt.Errorf("%w; no changes applied (raw output preserved below):\n%s", err, raw)
	}
	_, err = validateGroomResult(result, targets, tasks)
	var correction *groomCorrection
	if err != nil {
		validationErrors := groomValidationMessages(err)
		fmt.Fprintf(a.err, "groom correction retry: attempting after %d semantic validation error(s)\n", len(validationErrors))
		correctionPrompt, promptErr := buildGroomCorrectionPrompt(prompt, result, validationErrors)
		if promptErr != nil {
			return promptErr
		}
		corrected, correctedRaw, correctionErr := a.runGroomDelegation(parsed, roles, executable, correctionPrompt)
		if correctionErr != nil {
			return groomCorrectionFailure(result, validationErrors, nil, nil, fmt.Errorf("correction attempt failed: %w", correctionErr), correctedRaw)
		}
		if _, correctionErr = validateGroomResult(corrected, targets, tasks); correctionErr != nil {
			correctedErrors := groomValidationMessages(correctionErr)
			return groomCorrectionFailure(result, validationErrors, &corrected, correctedErrors, fmt.Errorf("corrected groom result is invalid"), nil)
		}
		result = corrected
		correction = &groomCorrection{Attempted: true, Succeeded: true, ValidationErrors: validationErrors}
	}
	return a.withWorkflowRecordLock(true, func() error {
		a.invalidateTasks()
		current, err := a.getTasks()
		if err != nil {
			return fmt.Errorf("cannot apply groom result after task state changed: %w", err)
		}
		currentTargets, err := groomTargets(current, parsed.id)
		if err != nil {
			return err
		}
		validated, err := validateGroomResult(result, currentTargets, current)
		if err != nil {
			return fmt.Errorf("groom targets changed before apply; no changes applied: %w", err)
		}
		summary, err := a.applyGroomVerdicts(validated, current, roles.implAgent.name)
		if err != nil {
			return err
		}
		summary.Correction = correction
		return a.emit(summary)
	})
}

func (a *app) runGroomDelegation(parsed taskGroomArgs, roles taskWorkRoles, executable, prompt string) (groomResult, []byte, error) {
	args, cleanup, err := delegatedResultArgs(roles.implAgent, prompt, roles.implModel, groomResultSchema)
	if err != nil {
		return groomResult{}, nil, err
	}
	defer cleanup()
	var out bytes.Buffer
	if err := taskWorkRunCommand(taskWorkRunContext(parsed.timeout, roles.implAgent.envFilter(os.Environ())), a.opts.root, executable, args, nil, &out, a.err); err != nil {
		return groomResult{}, out.Bytes(), fmt.Errorf("groom delegation failed: %w", err)
	}
	if roles.implAgent.parseSessionID != nil {
		if sessionID, parseErr := roles.implAgent.parseSessionID(out.Bytes()); parseErr == nil && sessionID != "" {
			fmt.Fprintf(a.err, "%s session started: %.8s\n", roles.implAgent.name, sessionID)
		}
	}
	result, err := parseGroomResult(out.Bytes())
	if err != nil {
		return groomResult{}, out.Bytes(), fmt.Errorf("invalid groom result: %w", err)
	}
	return result, out.Bytes(), nil
}

func buildGroomCorrectionPrompt(originalPrompt string, result groomResult, validationErrors []string) (string, error) {
	structured, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode invalid groom result for correction: %w", err)
	}
	var b strings.Builder
	b.WriteString("Correct the prior grooming result. Return one complete replacement result matching the schema. Do not omit valid verdicts, coerce actions, edit files, or run mutation commands.\n\nOriginal request and target scope:\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\nOriginal structured result:\n")
	b.Write(structured)
	b.WriteString("\n\nSemantic validation errors:\n")
	for _, message := range validationErrors {
		fmt.Fprintf(&b, "- %s\n", message)
	}
	return b.String(), nil
}

func groomCorrectionFailure(original groomResult, originalErrors []string, corrected *groomResult, correctedErrors []string, cause error, raw []byte) error {
	var b strings.Builder
	fmt.Fprintf(&b, "groom correction retry failed; no changes applied: %v\n", cause)
	b.WriteString("original structured result:\n")
	writeGroomRecoveryResult(&b, original)
	b.WriteString("original validation errors:\n")
	for _, message := range originalErrors {
		fmt.Fprintf(&b, "- %s\n", message)
	}
	if corrected != nil {
		b.WriteString("corrected structured result:\n")
		writeGroomRecoveryResult(&b, *corrected)
		b.WriteString("corrected validation errors:\n")
		for _, message := range correctedErrors {
			fmt.Fprintf(&b, "- %s\n", message)
		}
	} else if len(bytes.TrimSpace(raw)) > 0 {
		fmt.Fprintf(&b, "correction output was not parseable (%d bytes); rerun with --verbose provider logging if manual transport recovery is needed.\n", len(raw))
	}
	return fmt.Errorf("%s", strings.TrimSpace(b.String()))
}

func writeGroomRecoveryResult(b *strings.Builder, result groomResult) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(b, "<cannot encode result: %v>\n", err)
		return
	}
	b.Write(data)
	b.WriteByte('\n')
}

func groomTargets(tasks []Task, id string) ([]Task, error) {
	if id != "" {
		for _, task := range tasks {
			if task.ID == id {
				if task.Status != "Open" && task.Status != "Blocked" {
					return nil, fmt.Errorf("task %s is in status %s and cannot be groomed (only Open and Blocked tasks can be groomed)", id, task.Status)
				}
				return []Task{task}, nil
			}
		}
		return nil, fmt.Errorf("task not found: %s", id)
	}
	var targets []Task
	for _, task := range tasks {
		if task.Status == "Open" || task.Status == "Blocked" {
			targets = append(targets, task)
		}
	}
	return targets, nil
}

func buildGroomPrompt(tasks []Task, labels []taskLabelSummary) string {
	var b strings.Builder
	b.WriteString(groomProcedure)
	b.WriteString("\n\nResult schema:\n")
	b.WriteString(groomResultSchema)
	b.WriteString("\n\nTasks:\n")
	for _, task := range tasks {
		fmt.Fprintf(&b, "- %s [%s] %s %s labels=%s depends_on=%s path=%s\n", task.ID, task.Status, task.Priority, task.Effort, task.Labels, strings.Join(task.DependsOn, ","), task.Path)
	}
	b.WriteString("\nExisting label vocabulary:\n")
	for _, label := range labels {
		fmt.Fprintf(&b, "- %s\n", label.Label)
	}
	return b.String()
}

func delegatedResultArgs(agent taskWorkAgent, prompt, model, schema string) ([]string, func(), error) {
	switch agent.name {
	case "cake":
		path, cleanup, err := writeDelegatedResultSchema(schema)
		if err != nil {
			return nil, cleanup, err
		}
		base := []string{"--output-format", "stream-json", "--output-schema", path}
		if model != "" {
			base = append(base, "--model", model)
		}
		return append(base, prompt), cleanup, nil
	case "codex":
		path, cleanup, err := writeDelegatedResultSchema(schema)
		if err != nil {
			return nil, cleanup, err
		}
		base := []string{"exec"}
		if model != "" {
			base = append(base, "--model", model)
		}
		return append(base, codexBypassApprovalsAndSandboxFlag, "--json", "--output-schema", path, prompt), cleanup, nil
	case "claude":
		base := []string{"-p", "--verbose", "--output-format", "stream-json", "--json-schema", schema}
		if model != "" {
			base = append(base, "--model", model)
		}
		return append(base, prompt), func() {}, nil
	default:
		return agent.args(prompt, model), func() {}, nil
	}
}

func writeDelegatedResultSchema(schema string) (string, func(), error) {
	file, err := os.CreateTemp("", "ahm-result-schema-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create delegated output schema: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.WriteString(schema); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write delegated output schema: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close delegated output schema: %w", err)
	}
	return path, cleanup, nil
}

func parseGroomResult(raw []byte) (groomResult, error) {
	if direct, err := decodeGroomJSON(bytes.TrimSpace(raw)); err == nil {
		return direct, nil
	}
	lines := bytes.Split(raw, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var value any
		if json.Unmarshal(lines[i], &value) != nil {
			continue
		}
		if result, ok := findGroomJSON(value); ok {
			return result, nil
		}
	}
	return groomResult{}, fmt.Errorf("no schema-valid JSON verdict object found")
}

func findGroomJSON(value any) (groomResult, bool) {
	data, _ := json.Marshal(value)
	if result, err := decodeGroomJSON(data); err == nil {
		return result, true
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if found, ok := findGroomJSON(child); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range typed {
			if found, ok := findGroomJSON(child); ok {
				return found, true
			}
		}
	case string:
		if result, err := decodeGroomJSON([]byte(typed)); err == nil {
			return result, true
		}
	}
	return groomResult{}, false
}

func decodeGroomJSON(data []byte) (groomResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result groomResult
	if err := decoder.Decode(&result); err != nil {
		return groomResult{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return groomResult{}, fmt.Errorf("invalid trailing JSON content")
	}
	if result.Verdicts == nil {
		return groomResult{}, fmt.Errorf("invalid groom result object")
	}
	var raw struct {
		Verdicts []map[string]json.RawMessage `json:"verdicts"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return groomResult{}, err
	}
	required := []string{"task", "action", "comment", "add_deps", "remove_deps", "labels"}
	for i, verdict := range raw.Verdicts {
		for _, field := range required {
			if _, ok := verdict[field]; !ok {
				return groomResult{}, fmt.Errorf("verdict %d missing required field %s", i+1, field)
			}
		}
		if revisionData, ok := verdict["revision"]; ok {
			if bytes.Equal(bytes.TrimSpace(revisionData), []byte("null")) {
				continue
			}
			var revision map[string]json.RawMessage
			if err := json.Unmarshal(revisionData, &revision); err != nil {
				return groomResult{}, fmt.Errorf("verdict %d has invalid revision", i+1)
			}
			for _, field := range []string{"priority", "effort", "sections"} {
				if _, ok := revision[field]; !ok {
					return groomResult{}, fmt.Errorf("verdict %d revision missing required field %s", i+1, field)
				}
			}
		}
	}
	return result, nil
}

// buildGroomDependsOn returns a new dependency slice containing the existing
// dependencies plus addDeps minus removeDeps, sorted deterministically. It
// never mutates the input slice or its backing array.
func buildGroomDependsOn(existing []string, addDeps, removeDeps []string) []string {
	deps := make(map[string]bool, len(existing)+len(addDeps))
	for _, dep := range existing {
		deps[dep] = true
	}
	for _, dep := range removeDeps {
		delete(deps, dep)
	}
	for _, dep := range addDeps {
		deps[dep] = true
	}
	if len(deps) == 0 {
		return nil
	}
	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	sort.Slice(result, func(i, j int) bool { return taskLess(result[i], result[j]) })
	return result
}

func validateGroomResult(result groomResult, targets, all []Task) ([]groomVerdict, error) {
	targetByID := map[string]Task{}
	allIDs, vocabulary := map[string]bool{}, map[string]bool{}
	for _, task := range all {
		allIDs[task.ID] = true
		for label := range taskLabelSet(task) {
			vocabulary[label] = true
		}
	}
	for _, task := range targets {
		targetByID[task.ID] = task
	}
	seen := map[string]bool{}
	var messages []string
	for _, verdict := range result.Verdicts {
		task, ok := targetByID[verdict.Task]
		if !ok {
			messages = append(messages, fmt.Sprintf("verdict task %s is outside the requested scope", verdict.Task))
		}
		if seen[verdict.Task] {
			messages = append(messages, fmt.Sprintf("duplicate verdict for task %s", verdict.Task))
		}
		seen[verdict.Task] = true
		if verdict.Action != "accept" && verdict.Action != "comment" && verdict.Action != "revise" {
			messages = append(messages, fmt.Sprintf("task %s has invalid action %q", verdict.Task, verdict.Action))
		}
		if ok && (task.Status == "Completed" || task.Status == "Cancelled") {
			messages = append(messages, fmt.Sprintf("task %s is %s and cannot be groomed", verdict.Task, task.Status))
		}
		if ok && verdict.Action == "accept" && task.Status != "Open" {
			messages = append(messages, fmt.Sprintf("task %s cannot be accepted from %s", verdict.Task, task.Status))
		}
		if verdict.Action == "comment" && strings.TrimSpace(verdict.Comment) == "" {
			messages = append(messages, fmt.Sprintf("task %s comment action requires a comment", verdict.Task))
		}
		if verdict.Action == "revise" && verdict.Revision == nil {
			messages = append(messages, fmt.Sprintf("task %s revise action requires a revision", verdict.Task))
		}
		if verdict.Action == "revise" && strings.TrimSpace(verdict.Comment) == "" {
			messages = append(messages, fmt.Sprintf("task %s revise action requires a comment explaining what remains", verdict.Task))
		}
		if verdict.Action == "comment" && verdict.Revision != nil {
			messages = append(messages, fmt.Sprintf("task %s comment action cannot include a revision", verdict.Task))
		}
		if err := validateGroomRevision(verdict.Task, verdict.Revision); err != nil {
			messages = append(messages, groomValidationMessages(err)...)
		}
		for _, dep := range append(append([]string{}, verdict.AddDeps...), verdict.RemoveDeps...) {
			if !allIDs[dep] || dep == verdict.Task {
				messages = append(messages, fmt.Sprintf("task %s has invalid dependency %s", verdict.Task, dep))
			}
		}
		for _, label := range verdict.Labels {
			if !vocabulary[label] {
				messages = append(messages, fmt.Sprintf("task %s uses unknown label %s", verdict.Task, label))
			}
		}
	}
	for _, task := range targets {
		if !seen[task.ID] {
			messages = append(messages, fmt.Sprintf("missing verdict for task %s", task.ID))
		}
	}
	if len(messages) > 0 {
		return nil, &groomValidationErrors{messages: messages}
	}
	modified := make([]Task, len(all))
	copy(modified, all)
	for i := range modified {
		if modified[i].DependsOn != nil {
			modified[i].DependsOn = append([]string(nil), modified[i].DependsOn...)
		}
	}
	for i := range modified {
		for _, verdict := range result.Verdicts {
			if modified[i].ID != verdict.Task {
				continue
			}
			modified[i].DependsOn = buildGroomDependsOn(modified[i].DependsOn, verdict.AddDeps, verdict.RemoveDeps)
			if len(verdict.Labels) > 0 {
				modified[i].Labels = strings.Join(verdict.Labels, ", ")
			}
			if err := applyGroomRevision(&modified[i], verdict.Revision); err != nil {
				messages = append(messages, fmt.Sprintf("task %s revision: %v", verdict.Task, err))
				continue
			}
			if verdict.Action == "accept" {
				modified[i].Status = "Pending"
				if verdict.Revision != nil {
					if err := validateRevisedTaskReadiness(modified[i]); err != nil {
						messages = append(messages, fmt.Sprintf("task %s revised task is not ready: %v", verdict.Task, err))
					}
				}
			}
			if verdict.Revision != nil {
				rendered := renderTask(modified[i])
				if _, err := parseTaskFromData([]byte(rendered), modified[i].Path, modified[i].Bucket); err != nil {
					messages = append(messages, fmt.Sprintf("task %s revised task is invalid: %v", verdict.Task, err))
				}
			}
		}
	}
	for _, cycle := range taskDependencyCycles(modified) {
		messages = append(messages, fmt.Sprintf("dependency corrections would create a cycle: %s", strings.Join(cycle, " -> ")))
	}
	if len(messages) > 0 {
		return nil, &groomValidationErrors{messages: messages}
	}
	return result.Verdicts, nil
}

func groomValidationMessages(err error) []string {
	var validationErr *groomValidationErrors
	if errors.As(err, &validationErr) {
		return append([]string(nil), validationErr.messages...)
	}
	return []string{err.Error()}
}

func (a *app) applyGroomVerdicts(verdicts []groomVerdict, all []Task, agent string) (groomSummary, error) {
	byID := map[string]Task{}
	for _, task := range all {
		byID[task.ID] = task
	}
	for _, verdict := range verdicts {
		task := byID[verdict.Task]
		if task.Status == "Completed" || task.Status == "Cancelled" {
			return groomSummary{}, fmt.Errorf("task %s is %s and cannot be relocated by grooming", task.ID, task.Status)
		}
	}
	now := time.Now().Format(time.RFC3339)
	summary := groomSummary{Agent: agent}
	for _, verdict := range verdicts {
		task := byID[verdict.Task]
		change := groomChange{Task: task.ID, Action: verdict.Action}
		deps := map[string]bool{}
		for _, dep := range task.DependsOn {
			deps[dep] = true
		}
		for _, dep := range verdict.RemoveDeps {
			if deps[dep] {
				delete(deps, dep)
				change.RemovedDeps = append(change.RemovedDeps, dep)
			}
		}
		for _, dep := range verdict.AddDeps {
			if !deps[dep] {
				deps[dep] = true
				change.AddedDeps = append(change.AddedDeps, dep)
			}
		}
		task.DependsOn = buildGroomDependsOn(task.DependsOn, verdict.AddDeps, verdict.RemoveDeps)
		if len(verdict.Labels) > 0 {
			task.Labels = strings.Join(verdict.Labels, ", ")
			change.Labels = verdict.Labels
		}
		if verdict.Revision != nil {
			if verdict.Revision.Priority != "" {
				change.Priority = &groomValueChange{Before: task.Priority, After: verdict.Revision.Priority}
				task.Priority = verdict.Revision.Priority
			}
			if verdict.Revision.Effort != "" {
				change.Effort = &groomValueChange{Before: task.Effort, After: verdict.Revision.Effort}
				task.Effort = verdict.Revision.Effort
			}
			for _, section := range verdict.Revision.Sections {
				before, found, err := groomSectionContent(task.Body, section.Role)
				if err != nil {
					return groomSummary{}, err
				}
				change.Sections = append(change.Sections, groomSectionChange{Role: section.Role, Before: before, After: strings.TrimSpace(section.Content), Inserted: !found})
			}
			if err := applyGroomRevision(&task, &groomRevision{Sections: verdict.Revision.Sections}); err != nil {
				return groomSummary{}, err
			}
		}
		if strings.TrimSpace(verdict.Comment) != "" {
			task.Body = appendComment(task.Body, formatComment(now, "", strings.TrimSpace(verdict.Comment)))
			change.Commented = true
		}
		if verdict.Action == "accept" {
			task.Status = "Pending"
		}
		task.Updated = now
		target := a.workflowPaths().taskFile("active", task.ID)
		if err := writeFileAtomic(target, []byte(renderTask(task)), 0o644); err != nil {
			return groomSummary{}, err
		}
		if filepath.Clean(task.Path) != filepath.Clean(target) {
			if err := os.Remove(task.Path); err != nil {
				return groomSummary{}, err
			}
		}
		summary.Changes = append(summary.Changes, change)
	}
	a.invalidateTasks()
	if len(verdicts) > 0 {
		if err := a.writeIndexes(); err != nil {
			return groomSummary{}, err
		}
	}
	return summary, nil
}

var groomSectionHeadings = map[string][]string{
	"problem":          {"problem", "summary", "description"},
	"relevant_files":   {"relevant files", "relevant paths", "files"},
	"fix_direction":    {"fix direction", "implementation notes", "approach"},
	"acceptance_notes": {"acceptance notes", "acceptance criteria", "acceptance"},
}

var groomCanonicalHeadings = map[string]string{
	"problem": "Problem", "relevant_files": "Relevant Files",
	"fix_direction": "Fix Direction", "acceptance_notes": "Acceptance Notes",
}

func validateGroomRevision(taskID string, revision *groomRevision) error {
	if revision == nil {
		return nil
	}
	var messages []string
	if revision.Priority != "" && !containsString(priorityOrder(), revision.Priority) {
		messages = append(messages, fmt.Sprintf("task %s revision has invalid priority %q", taskID, revision.Priority))
	}
	if revision.Effort != "" && !containsString(effortOrder(), revision.Effort) {
		messages = append(messages, fmt.Sprintf("task %s revision has invalid effort %q", taskID, revision.Effort))
	}
	seen := map[string]bool{}
	for _, section := range revision.Sections {
		if _, ok := groomSectionHeadings[section.Role]; !ok {
			messages = append(messages, fmt.Sprintf("task %s revision has invalid section role %q", taskID, section.Role))
		}
		if seen[section.Role] {
			messages = append(messages, fmt.Sprintf("task %s revision duplicates section role %s", taskID, section.Role))
		}
		seen[section.Role] = true
		if strings.TrimSpace(section.Content) == "" {
			messages = append(messages, fmt.Sprintf("task %s revision section %s is empty", taskID, section.Role))
		}
	}
	if revision.Priority == "" && revision.Effort == "" && len(revision.Sections) == 0 {
		messages = append(messages, fmt.Sprintf("task %s revision is empty", taskID))
	}
	if len(messages) > 0 {
		return &groomValidationErrors{messages: messages}
	}
	return nil
}

func applyGroomRevision(task *Task, revision *groomRevision) error {
	if revision == nil {
		return nil
	}
	if revision.Priority != "" {
		task.Priority = revision.Priority
	}
	if revision.Effort != "" {
		task.Effort = revision.Effort
	}
	for _, section := range revision.Sections {
		body, err := replaceGroomSection(task.Body, section.Role, section.Content)
		if err != nil {
			return err
		}
		task.Body = body
	}
	return nil
}

func replaceGroomSection(body, role, content string) (string, error) {
	aliases := groomSectionHeadings[role]
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	sections := locateHeadingSections(lines, aliases)
	if len(sections) > 1 {
		return "", fmt.Errorf("ambiguous %s sections", role)
	}
	replacement := strings.TrimSpace(content)
	if len(sections) == 0 {
		// Section not found; insert before ## Comments if present so Comments
		// always remains the final section.
		if commentsIdx := groomCommentsIndex(lines); commentsIdx >= 0 {
			newLines := append([]string{}, lines[:commentsIdx]...)
			newLines = append(newLines, "", "## "+groomCanonicalHeadings[role], "", replacement, "")
			newLines = append(newLines, lines[commentsIdx:]...)
			return strings.TrimSpace(strings.Join(newLines, "\n")), nil
		}
		return strings.TrimSpace(body) + "\n\n## " + groomCanonicalHeadings[role] + "\n\n" + replacement, nil
	}
	section := sections[0]
	newLines := append([]string{}, lines[:section.Start+1]...)
	newLines = append(newLines, "", replacement, "")
	newLines = append(newLines, lines[section.End:]...)
	return strings.TrimSpace(strings.Join(newLines, "\n")), nil
}

// groomCommentsIndex returns the line index of the ## Comments heading in
// lines, or -1 if not found. The heading must be level 2 and match
// "Comments" case-insensitively.
func groomCommentsIndex(lines []string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		level := headingLevel(trimmed)
		if level == 2 && strings.EqualFold(strings.TrimSpace(trimmed[level:]), "Comments") {
			return i
		}
	}
	return -1
}

func groomSectionContent(body, role string) (string, bool, error) {
	aliases := groomSectionHeadings[role]
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	sections := locateHeadingSections(lines, aliases)
	if len(sections) > 1 {
		return "", false, fmt.Errorf("ambiguous %s sections", role)
	}
	if len(sections) == 0 {
		return "", false, nil
	}
	section := sections[0]
	return strings.TrimSpace(strings.Join(lines[section.Start+1:section.End], "\n")), true, nil
}

func validateRevisedTaskReadiness(task Task) error {
	labels := taskLabelSet(task)
	hasType, hasArea := false, false
	for label := range labels {
		hasType = hasType || strings.HasPrefix(label, "type:")
		hasArea = hasArea || strings.HasPrefix(label, "area:")
	}
	if !hasType || !hasArea {
		return fmt.Errorf("type and area labels are required")
	}
	for _, finding := range parseAcceptanceNotes([]byte(task.Body)) {
		if finding == taskAcceptanceMissing || finding == taskAcceptancePlaceholder {
			return fmt.Errorf("actionable acceptance notes are required")
		}
	}
	if (task.Effort == "L" || task.Effort == "XL") && (task.ExecPlan == "" || task.ExecPlan == "-") {
		return fmt.Errorf("effort %s requires an ExecPlan", task.Effort)
	}
	return nil
}

func taskIDs(tasks []Task) []string {
	ids := make([]string, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	return ids
}
