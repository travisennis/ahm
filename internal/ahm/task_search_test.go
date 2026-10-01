package ahm

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskSearchMatchesBody(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, ".ahm", "tasks", "active")
	// 001 matches only in the body, 002 matches in both, 003 matches neither,
	// and 004 is a body-only match that outranks 002 on priority.
	writeTaskFileWithBody(t, filepath.Join(dir, "001.md"), "001", "Add timeout handling", "Pending", "P2", "type:feature, area:cli", "The worker can hijack a slot and stall.")
	writeTaskFileWithBody(t, filepath.Join(dir, "002.md"), "002", "Document hijack behavior", "Open", "P2", "type:docs, area:docs", "Explains how a hijack is recorded.")
	writeTaskFileWithBody(t, filepath.Join(dir, "003.md"), "003", "Refresh index", "Pending", "P2", "type:task, area:cli", "Nothing relevant here.")
	writeTaskFileWithBody(t, filepath.Join(dir, "004.md"), "004", "Backfill worker", "Pending", "P0", "type:task, area:cli", "Can also hijack under load.")

	t.Run("returns a body-only hit", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling", "002 [Open] P2 S Document hijack behavior", "004 [Pending] P0 S Backfill worker")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("body match is case-insensitive", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("HIJACK", nil, nil); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending]", "002 [Open]")
	})

	t.Run("lists title matches before body-only matches", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		title := strings.Index(got, "002 [Open]")
		if title < 0 {
			t.Fatalf("expected the title match, got %q", got)
		}
		// Grouping outranks priority: the P2 title match precedes the P0
		// body-only match.
		for _, body := range []string{"001 [Pending]", "004 [Pending]"} {
			at := strings.Index(got, body)
			if at < 0 {
				t.Fatalf("expected %s in output: %q", body, got)
			}
			if title > at {
				t.Errorf("title match should precede body-only match %s: %q", body, got)
			}
		}
	})

	t.Run("a task matching in both fields appears once", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil); err != nil {
			t.Error(err)
		}
		if n := strings.Count(out.String(), "002 [Open]"); n != 1 {
			t.Errorf("expected 002 once, got %d:\n%s", n, out.String())
		}
	})

	t.Run("body hit composes with status filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", []string{"Open"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Open] P2 S Document hijack behavior")
		assertNotContains(t, got, "001 [Pending]", "004 [Pending]")
	})

	t.Run("body hit composes with label filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, []string{"area:docs"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Open] P2 S Document hijack behavior")
		assertNotContains(t, got, "001 [Pending]", "004 [Pending]")
	})

	t.Run("json output carries the matched body", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root, json: true}, out: &out}
		if err := a.taskSearch("hijack", nil, nil); err != nil {
			t.Error(err)
		}
		// 001 matches only through its body, and its title has no "hijack",
		// so this text can only appear if body search fed the JSON output.
		assertContainsAll(t, out.String(), "\"body\":", "The worker can hijack a slot and stall.")
	})

	t.Run("a miss matches no task", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("zebra", nil, nil); err != nil {
			t.Error(err)
		}
		if strings.TrimSpace(out.String()) != "No tasks found." {
			t.Errorf("unexpected output: %q", out.String())
		}
	})
}
