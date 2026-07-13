package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// errReviewOutputUnparseable is returned when review output does not contain
// the provider's terminal completion event. Without that event, empty output,
// invalid JSON, and truncated or unexpected JSON are malformed-provider
// conditions rather than valid no-findings reviews.
var errReviewOutputUnparseable = errors.New("review output contained no recognized completion event")

var errSessionOutputUnparseable = errors.New("session output contained no parseable JSON events")

// cakeStreamEvent represents a single line in cake's stream-json output.
// Records are serde-tagged with a top-level "type" field; task_complete
// flattens the outcome, so "result" appears at the top level too.
type cakeStreamEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseCakeSessionID parses cake's stream-json output and returns the
// session_id from the first task_start event.
func parseCakeSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	parsedAny := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cakeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		parsedAny = true
		if evt.Type == "task_start" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	if len(bytes.TrimSpace(output)) > 0 && !parsedAny {
		return "", errSessionOutputUnparseable
	}
	return "", nil
}

// cakeResumeArgs constructs the arguments to resume a cake session with a
// follow-up prompt.
func cakeResumeArgs(sessionID, prompt string) []string {
	return []string{"--resume", sessionID, "--output-format", "stream-json", prompt}
}

// parseCakeReviewFeedback parses cake's stream-json output and returns the
// result field from the final task_complete event, which contains the review
// feedback from a preflight or other review run. A task_complete event with an
// empty result is a valid no-findings review; output without task_complete is
// malformed.
func parseCakeReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	foundCompletion := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cakeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "task_complete" {
			foundCompletion = true
			lastResult = evt.Result
		}
	}
	if !foundCompletion {
		return "", errReviewOutputUnparseable
	}
	return lastResult, nil
}

// codexStreamEvent represents a single JSONL event in codex's --json output.
type codexStreamEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Item     *codexItemEvent `json:"item,omitempty"`
}

type codexItemEvent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// parseCodexSessionID parses codex JSONL output and returns the thread_id
// from the first thread.started event.
func parseCodexSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	parsedAny := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt codexStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		parsedAny = true
		if evt.Type == "thread.started" && evt.ThreadID != "" {
			return evt.ThreadID, nil
		}
	}
	if len(bytes.TrimSpace(output)) > 0 && !parsedAny {
		return "", errSessionOutputUnparseable
	}
	return "", nil
}

// codexResumeArgs constructs the arguments to resume a codex session with a
// follow-up prompt.
func codexResumeArgs(sessionID, prompt string) []string {
	return []string{"exec", "resume", codexBypassApprovalsAndSandboxFlag, "--json", sessionID, prompt}
}

// parseCodexReviewFeedback parses codex JSONL output and returns the
// concatenated text from all agent_message item.completed events, which
// contains the preflight review feedback. A completed turn with no agent text
// is a valid no-findings review; output without turn.completed is malformed.
func parseCodexReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var texts []string
	foundCompletion := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt codexStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "turn.completed" {
			foundCompletion = true
		}
		if evt.Type == "item.completed" && evt.Item != nil && evt.Item.Text != "" {
			texts = append(texts, evt.Item.Text)
		}
	}
	if !foundCompletion {
		return "", errReviewOutputUnparseable
	}
	return strings.Join(texts, "\n"), nil
}

// cursorStreamEvent represents a single JSONL event in cursor-agent
// stream-json output.
type cursorStreamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseCursorSessionID parses cursor-agent stream-json output and returns the
// session_id from the first system/init event.
func parseCursorSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	parsedAny := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cursorStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		parsedAny = true
		if evt.Type == "system" && evt.Subtype == "init" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	if len(bytes.TrimSpace(output)) > 0 && !parsedAny {
		return "", errSessionOutputUnparseable
	}
	return "", nil
}

// cursorResumeArgs constructs the arguments to resume a cursor-agent session
// with a follow-up prompt.
func cursorResumeArgs(sessionID, prompt string) []string {
	return []string{"-p", "--output-format", "stream-json", "--trust", "--resume", sessionID, prompt}
}

// parseCursorReviewFeedback parses cursor-agent stream-json output and returns
// the result field from the final result event. An empty result is a valid
// no-findings review; output without a result event is malformed.
func parseCursorReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	foundCompletion := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt cursorStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "result" {
			foundCompletion = true
			lastResult = evt.Result
		}
	}
	if !foundCompletion {
		return "", errReviewOutputUnparseable
	}
	return lastResult, nil
}

// claudeStreamEvent represents a single JSONL event in Claude Code's
// stream-json output (claude -p --verbose --output-format stream-json).
type claudeStreamEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"`
}

// parseClaudeSessionID parses Claude Code stream-json output and returns the
// session_id from the first system/init event.
func parseClaudeSessionID(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	parsedAny := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt claudeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		parsedAny = true
		if evt.Type == "system" && evt.Subtype == "init" && evt.SessionID != "" {
			return evt.SessionID, nil
		}
	}
	if len(bytes.TrimSpace(output)) > 0 && !parsedAny {
		return "", errSessionOutputUnparseable
	}
	return "", nil
}

// claudeResumeArgs constructs the arguments to resume a Claude Code session
// with a follow-up prompt.
func claudeResumeArgs(sessionID, prompt string) []string {
	return []string{"-p", "--verbose", "--resume", sessionID, "--output-format", "stream-json", prompt}
}

// parseClaudeReviewFeedback parses Claude Code stream-json output and returns
// the result field from the final result event. An empty result is a valid
// no-findings review; output without a result event is malformed.
func parseClaudeReviewFeedback(output []byte) (string, error) {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	var lastResult string
	foundCompletion := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var evt claudeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		if evt.Type == "result" {
			foundCompletion = true
			lastResult = evt.Result
		}
	}
	if !foundCompletion {
		return "", errReviewOutputUnparseable
	}
	return lastResult, nil
}
