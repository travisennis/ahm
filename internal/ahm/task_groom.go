package ahm

import (
	"bytes"
	"context"
	"encoding/json"
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
	Task       string   `json:"task"`
	Action     string   `json:"action"`
	Comment    string   `json:"comment"`
	AddDeps    []string `json:"add_deps"`
	RemoveDeps []string `json:"remove_deps"`
	Labels     []string `json:"labels"`
}

type groomChange struct {
	Task        string   `json:"task"`
	Action      string   `json:"action"`
	Commented   bool     `json:"commented"`
	AddedDeps   []string `json:"added_deps,omitempty"`
	RemovedDeps []string `json:"removed_deps,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type groomSummary struct {
	Agent   string        `json:"agent"`
	Changes []groomChange `json:"changes"`
}

func (s groomSummary) RenderText(w io.Writer) error {
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
		fmt.Fprintln(w)
	}
	return nil
}

const groomResultSchema = `{"type":"object","additionalProperties":false,"required":["verdicts"],"properties":{"verdicts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["task","action","comment","add_deps","remove_deps","labels"],"properties":{"task":{"type":"string"},"action":{"type":"string","enum":["accept","comment"]},"comment":{"type":"string"},"add_deps":{"type":"array","items":{"type":"string"}},"remove_deps":{"type":"array","items":{"type":"string"}},"labels":{"type":"array","items":{"type":"string"}}}}}}}`

const groomProcedure = `Act as a backlog groomer. Inspect the listed task files and repository code as needed before judging readiness. Every Open task must either be ready to accept or receive a precise comment explaining what remains. Review Blocked tasks for stale or incorrect dependencies. Verify priority, effort, type and area labels, dependency accuracy, decision completeness, relevant paths, and actionable acceptance notes. Never edit files, cancel tasks, or run ahm mutation commands; ahm alone applies your structured verdicts. A cancel recommendation must be an action=comment verdict. Return only JSON matching the supplied schema.`

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
		return a.emit(map[string]any{"agent": roles.implAgent.name, "prompt": prompt, "schema": groomResultSchema, "tasks": taskIDs(targets)})
	}
	executable, err := taskWorkLookPath(roles.implAgent.executable)
	if err != nil {
		return fmt.Errorf("cannot groom with %s: executable %q not found on PATH", roles.implAgent.name, roles.implAgent.executable)
	}
	args, cleanup, err := delegatedResultArgs(roles.implAgent, prompt, roles.implModel, groomResultSchema)
	if err != nil {
		return err
	}
	defer cleanup()
	var out bytes.Buffer
	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), parsed.timeout), a.opts.root, executable, args, nil, &out, a.err); err != nil {
		return fmt.Errorf("groom delegation failed (raw output preserved below): %w\n%s", err, out.String())
	}
	if roles.implAgent.parseSessionID != nil {
		if sessionID, parseErr := roles.implAgent.parseSessionID(out.Bytes()); parseErr == nil && sessionID != "" {
			fmt.Fprintf(a.err, "%s session started: %.8s\n", roles.implAgent.name, sessionID)
		}
	}
	result, err := parseGroomResult(out.Bytes())
	if err != nil {
		return fmt.Errorf("invalid groom result; no changes applied (raw output preserved below): %w\n%s", err, out.String())
	}
	validated, err := validateGroomResult(result, targets, tasks)
	if err != nil {
		return fmt.Errorf("invalid groom result; no changes applied (raw output preserved below): %w\n%s", err, out.String())
	}
	summary, err := a.applyGroomVerdicts(validated, tasks, roles.implAgent.name)
	if err != nil {
		return err
	}
	return a.emit(summary)
}

func groomTargets(tasks []Task, id string) ([]Task, error) {
	if id != "" {
		for _, task := range tasks {
			if task.ID == id {
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
	case "codex":
		file, err := os.CreateTemp("", "ahm-result-schema-*.json")
		if err != nil {
			return nil, func() {}, fmt.Errorf("create groom output schema: %w", err)
		}
		path := file.Name()
		cleanup := func() { _ = os.Remove(path) }
		if _, err := file.WriteString(schema); err != nil {
			_ = file.Close()
			cleanup()
			return nil, func() {}, fmt.Errorf("write groom output schema: %w", err)
		}
		if err := file.Close(); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("close groom output schema: %w", err)
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
	}
	return result, nil
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
	for _, verdict := range result.Verdicts {
		task, ok := targetByID[verdict.Task]
		if !ok {
			return nil, fmt.Errorf("verdict task %s is outside the requested scope", verdict.Task)
		}
		if seen[verdict.Task] {
			return nil, fmt.Errorf("duplicate verdict for task %s", verdict.Task)
		}
		seen[verdict.Task] = true
		if verdict.Action != "accept" && verdict.Action != "comment" {
			return nil, fmt.Errorf("task %s has invalid action %q", verdict.Task, verdict.Action)
		}
		if verdict.Action == "accept" && task.Status != "Open" {
			return nil, fmt.Errorf("task %s cannot be accepted from %s", verdict.Task, task.Status)
		}
		if verdict.Action == "comment" && strings.TrimSpace(verdict.Comment) == "" {
			return nil, fmt.Errorf("task %s comment action requires a comment", verdict.Task)
		}
		for _, dep := range append(append([]string{}, verdict.AddDeps...), verdict.RemoveDeps...) {
			if !allIDs[dep] || dep == verdict.Task {
				return nil, fmt.Errorf("task %s has invalid dependency %s", verdict.Task, dep)
			}
		}
		for _, label := range verdict.Labels {
			if !vocabulary[label] {
				return nil, fmt.Errorf("task %s uses unknown label %s", verdict.Task, label)
			}
		}
	}
	for _, task := range targets {
		if !seen[task.ID] {
			return nil, fmt.Errorf("missing verdict for task %s", task.ID)
		}
	}
	modified := append([]Task(nil), all...)
	for i := range modified {
		for _, verdict := range result.Verdicts {
			if modified[i].ID != verdict.Task {
				continue
			}
			deps := map[string]bool{}
			for _, dep := range modified[i].DependsOn {
				deps[dep] = true
			}
			for _, dep := range verdict.RemoveDeps {
				delete(deps, dep)
			}
			for _, dep := range verdict.AddDeps {
				deps[dep] = true
			}
			modified[i].DependsOn = modified[i].DependsOn[:0]
			for dep := range deps {
				modified[i].DependsOn = append(modified[i].DependsOn, dep)
			}
		}
	}
	if cycles := taskDependencyCycles(modified); len(cycles) > 0 {
		return nil, fmt.Errorf("dependency corrections would create a cycle: %s", strings.Join(cycles[0], " -> "))
	}
	return result.Verdicts, nil
}

func (a *app) applyGroomVerdicts(verdicts []groomVerdict, all []Task, agent string) (groomSummary, error) {
	byID := map[string]Task{}
	for _, task := range all {
		byID[task.ID] = task
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
		task.DependsOn = task.DependsOn[:0]
		for dep := range deps {
			task.DependsOn = append(task.DependsOn, dep)
		}
		sort.Slice(task.DependsOn, func(i, j int) bool { return taskLess(task.DependsOn[i], task.DependsOn[j]) })
		if len(verdict.Labels) > 0 {
			task.Labels = strings.Join(verdict.Labels, ", ")
			change.Labels = verdict.Labels
		}
		if strings.TrimSpace(verdict.Comment) != "" {
			task.Body = appendComment(task.Body, formatComment(now, "", strings.TrimSpace(verdict.Comment)))
			change.Commented = true
		}
		if verdict.Action == "accept" {
			task.Status = "Pending"
		}
		task.Updated = now
		target := workflowPathsFor(a.opts.root).taskFile("active", task.ID)
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

func taskIDs(tasks []Task) []string {
	ids := make([]string, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}
	return ids
}
