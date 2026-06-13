package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests run the task work output parsers against golden transcripts
// captured from the real agent CLIs (see testdata/agents/README.md). Unlike
// the inline fixtures, the goldens cannot drift into a parser's invented
// schema: refresh them with `just capture-agent-fixtures`. A missing golden
// is a test failure, not a skip, so the suite stays tied to real output.

var agentGoldenDir = filepath.Join("testdata", "agents")

func readAgentGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentGoldenDir, name))
	if err != nil {
		t.Fatalf("reading golden transcript (refresh with `just capture-agent-fixtures`): %v", err)
	}
	return data
}

func TestGoldenCakeWorkSessionID(t *testing.T) {
	id, err := parseCakeSessionID(readAgentGolden(t, "cake-work.jsonl"))
	if err != nil {
		t.Fatalf("parseCakeSessionID: %v", err)
	}
	if id == "" {
		t.Fatal("no session ID parsed from cake work golden")
	}
}

func TestGoldenCakeReviewFeedback(t *testing.T) {
	feedback, err := parseCakeReviewFeedback(readAgentGolden(t, "cake-review.jsonl"))
	if err != nil {
		t.Fatalf("parseCakeReviewFeedback: %v", err)
	}
	if strings.TrimSpace(feedback) == "" {
		t.Fatal("no review feedback parsed from cake review golden")
	}
}

func TestGoldenCodexReviewFeedback(t *testing.T) {
	feedback, err := parseCodexReviewFeedback(readAgentGolden(t, "codex-review.jsonl"))
	if err != nil {
		t.Fatalf("parseCodexReviewFeedback: %v", err)
	}
	if strings.TrimSpace(feedback) == "" {
		t.Fatal("no review feedback parsed from codex review golden")
	}
}

func TestGoldenCodexExecSessionID(t *testing.T) {
	id, err := parseCodexSessionID(readAgentGolden(t, "codex-exec.jsonl"))
	if err != nil {
		t.Fatalf("parseCodexSessionID: %v", err)
	}
	if id == "" {
		t.Fatal("no session ID parsed from codex exec golden")
	}
}

// The codex resume golden was captured by resuming the session from the exec
// golden, so the parsed IDs must match. This pins both that resume emits
// thread.started and that a captured thread ID is what resume accepts.
func TestGoldenCodexResumeRoundTrip(t *testing.T) {
	execID, err := parseCodexSessionID(readAgentGolden(t, "codex-exec.jsonl"))
	if err != nil {
		t.Fatalf("parseCodexSessionID(exec): %v", err)
	}
	resumeID, err := parseCodexSessionID(readAgentGolden(t, "codex-resume.jsonl"))
	if err != nil {
		t.Fatalf("parseCodexSessionID(resume): %v", err)
	}
	if resumeID == "" {
		t.Fatal("no session ID parsed from codex resume golden")
	}
	if resumeID != execID {
		t.Fatalf("resume thread ID = %q, want exec thread ID %q", resumeID, execID)
	}
}

// The capture script replays the runReview prompt so the review golden
// reflects a real review run; fail when the copies drift apart.
func TestCaptureScriptUsesReviewPrompt(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "capture-agent-fixtures.sh"))
	if err != nil {
		t.Fatalf("reading capture script: %v", err)
	}
	if !strings.Contains(string(script), taskWorkReviewPrompt) {
		t.Fatalf("scripts/capture-agent-fixtures.sh does not use the runReview prompt %q; update its review_prompt", taskWorkReviewPrompt)
	}
}

func TestGoldenProvenanceSidecars(t *testing.T) {
	dir := agentGoldenDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading golden directory: %v", err)
	}
	var goldens int
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		goldens++
		metaName := strings.TrimSuffix(name, ".jsonl") + ".meta"
		meta, err := os.ReadFile(filepath.Join(dir, metaName))
		if err != nil {
			t.Errorf("golden %s has no provenance sidecar: %v", name, err)
			continue
		}
		for _, field := range []string{"agent:", "captured:", "command:"} {
			if !strings.Contains(string(meta), field) {
				t.Errorf("provenance sidecar %s is missing %q", metaName, field)
			}
		}
	}
	if goldens == 0 {
		t.Fatal("no golden transcripts found in testdata/agents")
	}
}
