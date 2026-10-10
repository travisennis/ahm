package ahm

import (
	"strings"
	"testing"
)

// TestAppendCommentBlankLineBeforeHeading guards against a regression where a
// comment appended to a ## Comments section that is followed by another
// heading was written flush against that heading (no separating blank line).
func TestAppendCommentBlankLineBeforeHeading(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	body := "# My Task\n\n## Comments\n\n**old** — first comment\n\n## Other\n\nOther content.\n"
	got := appendComment(body, ts)
	want := "# My Task\n\n## Comments\n\n**old** — first comment\n\n" + ts + "\n\n## Other\n\nOther content."
	if got != want {
		t.Errorf("appendComment = %q, want %q", got, want)
	}

	// Exactly one blank line must separate the comment from the heading.
	if strings.Contains(got, ts+"\n## Other") {
		t.Errorf("comment runs into following heading in %q", got)
	}
	if strings.Contains(got, ts+"\n\n\n## Other") {
		t.Errorf("more than one blank line before the following heading in %q", got)
	}
}

// TestAppendCommentEmptySectionBeforeHeading covers the empty-section case, which
// must also leave a blank line before a following heading. The comment sitting
// flush under the empty ## Comments heading is pre-existing behavior, unchanged
// here and asserted as-is.
func TestAppendCommentEmptySectionBeforeHeading(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	body := "# My Task\n\n## Comments\n\n## Other\n"
	got := appendComment(body, ts)
	want := "# My Task\n\n## Comments\n" + ts + "\n\n## Other"
	if got != want {
		t.Errorf("appendComment = %q, want %q", got, want)
	}
	if strings.Contains(got, ts+"\n## Other") {
		t.Errorf("comment runs into following heading in %q", got)
	}
}

// TestAppendCommentUnchangedCases pins the two paths task 290 must leave alone:
// a ## Comments section that ends the body, and a body with no such section.
func TestAppendCommentUnchangedCases(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	t.Run("section at end of body", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n\n**old** — first comment\n"
		got := appendComment(body, ts)
		want := "# My Task\n\n## Comments\n\n**old** — first comment\n\n" + ts
		if got != want {
			t.Errorf("appendComment = %q, want %q", got, want)
		}
	})

	t.Run("no existing section", func(t *testing.T) {
		body := "# My Task\n\nJust some text.\n"
		got := appendComment(body, ts)
		want := "# My Task\n\nJust some text.\n\n## Comments\n\n" + ts
		if got != want {
			t.Errorf("appendComment = %q, want %q", got, want)
		}
	})
}
