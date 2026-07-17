package ahm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type auditArgs struct {
	agent, model string
	timeout      time.Duration
}
type auditResult struct {
	Findings []auditFinding `json:"findings"`
}
type auditFinding struct {
	Title           string   `json:"title"`
	Problem         string   `json:"problem"`
	RelevantFiles   []string `json:"relevant_files"`
	FixDirection    string   `json:"fix_direction"`
	AcceptanceNotes []string `json:"acceptance_notes"`
	Labels          []string `json:"labels"`
	Priority        string   `json:"priority"`
	Effort          string   `json:"effort"`
}
type auditCreated struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type auditSummary struct {
	Agent   string         `json:"agent"`
	Created []auditCreated `json:"created"`
}

func (s auditSummary) RenderText(w io.Writer) error {
	if len(s.Created) == 0 {
		_, err := fmt.Fprintln(w, "No audit findings.")
		return err
	}
	for _, c := range s.Created {
		fmt.Fprintf(w, "%s: %s\n", c.ID, c.Title)
	}
	return nil
}

const auditResultSchema = `{"type":"object","additionalProperties":false,"required":["findings"],"properties":{"findings":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["title","problem","relevant_files","fix_direction","acceptance_notes","labels","priority","effort"],"properties":{"title":{"type":"string"},"problem":{"type":"string"},"relevant_files":{"type":"array","items":{"type":"string"}},"fix_direction":{"type":"string"},"acceptance_notes":{"type":"array","items":{"type":"string"}},"labels":{"type":"array","items":{"type":"string"}},"priority":{"type":"string","enum":["P0","P1","P2","P3","P4"]},"effort":{"type":"string","enum":["XS","S","M","L","XL"]}}}}}}`
const auditProcedure = `Act as a senior engineering advisor and survey this repository for the highest-value improvement opportunities. Remain strictly read-only: never modify source, configuration, workflow records, or git state. Never reproduce secret values; report only the location and nature of a secret-handling risk. Deduplicate against the active tasks supplied below. Every finding must be independently actionable and fully self-contained, with a concrete problem, relevant files, fix direction, acceptance notes, priority, effort, and existing type/area/risk labels. Return only JSON matching the supplied schema. Do not ask for interactive acceptance: ahm creates findings as Open tasks, which is the acceptance gate.`

func (a *app) auditCommand() *cobra.Command {
	parsed := auditArgs{timeout: taskWorkDefaultTimeout}
	cmd := &cobra.Command{Use: "audit", Short: "Delegate a read-only codebase improvement audit", Long: `Delegate a read-only improvement audit to the configured coding agent.

Ahm validates the complete structured result, then creates one Open task per
finding with source:audit provenance. --dry-run prints the prompt and schema
without delegation or writes.`, Args: noArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.detectRoot(); err != nil {
			return err
		}
		if parsed.timeout <= 0 {
			return usageError("audit --timeout must be greater than 0 (e.g. 30m, 2h, 90s)\n  ahm audit --timeout 2h")
		}
		return a.audit(parsed)
	}}
	cmd.Flags().StringVar(&parsed.agent, "agent", "", "Coding-agent CLI (cake, claude, codex, cursor)")
	cmd.Flags().StringVar(&parsed.model, "model", "", "Model override for the delegated agent")
	cmd.Flags().DurationVar(&parsed.timeout, "timeout", taskWorkDefaultTimeout, "Maximum delegation duration")
	return cmd
}

func (a *app) audit(parsed auditArgs) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		return fmt.Errorf("cannot audit with malformed task files: %w", err)
	}
	validation, _ := validateWorkflow(a.opts.root)
	prompt := buildAuditPrompt(tasks, summarizeTaskLabels(tasks), validation)
	roles, err := a.resolveTaskWorkRoles(parsed.agent, parsed.model)
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		return a.emit(map[string]any{"agent": roles.implAgent.name, "prompt": prompt, "schema": auditResultSchema})
	}
	executable, err := taskWorkLookPath(roles.implAgent.executable)
	if err != nil {
		return fmt.Errorf("cannot audit with %s: executable %q not found on PATH", roles.implAgent.name, roles.implAgent.executable)
	}
	args, cleanup, err := delegatedResultArgs(roles.implAgent, prompt, roles.implModel, auditResultSchema)
	if err != nil {
		return err
	}
	defer cleanup()
	var out bytes.Buffer
	if err := taskWorkRunCommand(taskWorkRunContext(parsed.timeout, roles.implAgent.envFilter(os.Environ())), a.opts.root, executable, args, nil, &out, a.err); err != nil {
		return fmt.Errorf("audit delegation failed (raw output preserved below): %w\n%s", err, out.String())
	}
	if roles.implAgent.parseSessionID != nil {
		if id, e := roles.implAgent.parseSessionID(out.Bytes()); e == nil && id != "" {
			fmt.Fprintf(a.err, "%s session started: %.8s\n", roles.implAgent.name, id)
		}
	}
	result, err := parseAuditResult(out.Bytes())
	if err != nil {
		return fmt.Errorf("invalid audit result; no changes applied (raw output preserved below): %w\n%s", err, out.String())
	}
	if err := validateAuditResult(result, tasks); err != nil {
		return fmt.Errorf("invalid audit result; no changes applied (raw output preserved below): %w\n%s", err, out.String())
	}
	return a.createAuditTasks(result.Findings, roles.implAgent.name)
}

func buildAuditPrompt(tasks []Task, labels []taskLabelSummary, validation validationReport) string {
	var b strings.Builder
	b.WriteString(auditProcedure)
	b.WriteString("\n\nResult schema:\n")
	b.WriteString(auditResultSchema)
	b.WriteString("\n\nExisting active tasks (dedupe list):\n")
	active := make([]Task, 0)
	for _, t := range tasks {
		if t.Status != "Completed" && t.Status != "Cancelled" {
			active = append(active, t)
		}
	}
	sort.Slice(active, func(i, j int) bool { return taskLess(active[i].ID, active[j].ID) })
	const capTasks = 100
	shown := active
	if len(shown) > capTasks {
		shown = shown[:capTasks]
	}
	for _, t := range shown {
		fmt.Fprintf(&b, "- %s [%s] %s labels=%s\n", t.ID, t.Status, t.Title, t.Labels)
	}
	if len(active) > len(shown) {
		fmt.Fprintf(&b, "- %d more; inspect with `ahm task list` before reporting a possible duplicate\n", len(active)-len(shown))
	}
	b.WriteString("\nExisting label vocabulary:\n")
	for _, l := range labels {
		fmt.Fprintf(&b, "- %s\n", l.Label)
	}
	b.WriteString("\nKnown validation findings (do not re-report):\n")
	for _, group := range []struct {
		severity string
		items    []validationFinding
	}{{"error", validation.Errors}, {"warning", validation.Warnings}, {"info", validation.Info}} {
		for _, f := range group.items {
			fmt.Fprintf(&b, "- %s %s: %s\n", group.severity, f.Code, f.Message)
		}
	}
	return b.String()
}

func parseAuditResult(raw []byte) (auditResult, error) {
	if r, e := decodeAuditJSON(bytes.TrimSpace(raw)); e == nil {
		return r, nil
	}
	lines := bytes.Split(raw, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var v any
		if json.Unmarshal(lines[i], &v) == nil {
			if r, ok := findAuditJSON(v); ok {
				return r, nil
			}
		}
	}
	return auditResult{}, fmt.Errorf("no schema-valid JSON findings object found")
}
func findAuditJSON(v any) (auditResult, bool) {
	data, _ := json.Marshal(v)
	if r, e := decodeAuditJSON(data); e == nil {
		return r, true
	}
	switch x := v.(type) {
	case map[string]any:
		for _, c := range x {
			if r, ok := findAuditJSON(c); ok {
				return r, true
			}
		}
	case []any:
		for _, c := range x {
			if r, ok := findAuditJSON(c); ok {
				return r, true
			}
		}
	case string:
		if r, e := decodeAuditJSON([]byte(x)); e == nil {
			return r, true
		}
	}
	return auditResult{}, false
}
func decodeAuditJSON(data []byte) (auditResult, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var r auditResult
	if err := d.Decode(&r); err != nil {
		return auditResult{}, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return auditResult{}, fmt.Errorf("invalid trailing JSON content")
	}
	if r.Findings == nil {
		return auditResult{}, fmt.Errorf("missing findings")
	}
	var raw struct {
		Findings []map[string]json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return auditResult{}, err
	}
	required := []string{"title", "problem", "relevant_files", "fix_direction", "acceptance_notes", "labels", "priority", "effort"}
	for i, f := range raw.Findings {
		for _, field := range required {
			if _, ok := f[field]; !ok {
				return auditResult{}, fmt.Errorf("finding %d missing required field %s", i+1, field)
			}
		}
	}
	return r, nil
}

func validateAuditResult(result auditResult, tasks []Task) error {
	vocab := map[string]bool{}
	for _, t := range tasks {
		for l := range taskLabelSet(t) {
			vocab[l] = true
		}
	}
	titles := map[string]bool{}
	for _, t := range tasks {
		titles[strings.ToLower(strings.TrimSpace(t.Title))] = true
	}
	for i, f := range result.Findings {
		n := i + 1
		if strings.TrimSpace(f.Title) == "" || strings.ContainsAny(f.Title, "\r\n") || strings.TrimSpace(f.Title) != f.Title {
			return fmt.Errorf("finding %d has invalid title", n)
		}
		if titles[strings.ToLower(strings.TrimSpace(f.Title))] {
			return fmt.Errorf("finding %d duplicates existing task title %q", n, f.Title)
		}
		titles[strings.ToLower(strings.TrimSpace(f.Title))] = true
		if strings.TrimSpace(f.Problem) == "" || strings.TrimSpace(f.FixDirection) == "" || len(f.RelevantFiles) == 0 || len(f.AcceptanceNotes) == 0 {
			return fmt.Errorf("finding %d is not self-contained", n)
		}
		if !validTaskPriority(f.Priority) || !validTaskEffort(f.Effort) {
			return fmt.Errorf("finding %d has invalid priority or effort", n)
		}
		hasType, hasArea := false, false
		for _, l := range f.Labels {
			if !vocab[l] {
				return fmt.Errorf("finding %d uses unknown label %s", n, l)
			}
			hasType = hasType || strings.HasPrefix(l, "type:")
			hasArea = hasArea || strings.HasPrefix(l, "area:")
		}
		if !hasType || !hasArea {
			return fmt.Errorf("finding %d requires type and area labels", n)
		}
	}
	return nil
}

func (a *app) createAuditTasks(findings []auditFinding, agent string) error {
	summary := auditSummary{Agent: agent}
	for _, f := range findings {
		body := renderAuditTaskBody(f)
		var created bytes.Buffer
		old := a.out
		oldIn := a.in
		a.out = &created
		a.in = strings.NewReader(body)
		err := a.taskCreateParsed(taskCreateArgs{title: f.Title, priority: f.Priority, effort: f.Effort, labels: strings.Join(append(append([]string{}, f.Labels...), "source:audit"), ", "), status: "Open", bodyFile: "-"})
		a.out = old
		a.in = oldIn
		if err != nil {
			return err
		}
		a.invalidateTasks()
		summary.Created = append(summary.Created, auditCreated{ID: strings.TrimSpace(created.String()), Title: f.Title})
	}
	return a.emit(summary)
}
func renderAuditTaskBody(f auditFinding) string {
	var b strings.Builder
	b.WriteString("## Problem\n\n")
	b.WriteString(strings.TrimSpace(f.Problem))
	b.WriteString("\n\n## Relevant Files\n\n")
	for _, p := range f.RelevantFiles {
		fmt.Fprintf(&b, "- `%s`\n", strings.TrimSpace(p))
	}
	b.WriteString("\n## Fix Direction\n\n")
	b.WriteString(strings.TrimSpace(f.FixDirection))
	b.WriteString("\n\n## Acceptance Notes\n\n")
	for _, note := range f.AcceptanceNotes {
		fmt.Fprintf(&b, "- [ ] %s\n", strings.TrimSpace(note))
	}
	return b.String()
}
