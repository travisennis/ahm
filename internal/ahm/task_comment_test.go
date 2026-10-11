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

// TestAppendCommentEmptySection covers the empty-section case. The new comment
// starts after exactly one blank line, both under the heading and before any
// following heading, matching the create-at-end path.
func TestAppendCommentEmptySection(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	t.Run("heading follows the section", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n\n## Other\n"
		got := appendComment(body, ts)
		want := "# My Task\n\n## Comments\n\n" + ts + "\n\n## Other"
		if got != want {
			t.Errorf("appendComment = %q, want %q", got, want)
		}
		if strings.Contains(got, ts+"\n## Other") {
			t.Errorf("comment runs into following heading in %q", got)
		}
	})

	t.Run("section ends the body", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n"
		got := appendComment(body, ts)
		want := "# My Task\n\n## Comments\n\n" + ts
		if got != want {
			t.Errorf("appendComment = %q, want %q", got, want)
		}
	})

	t.Run("section holds only blank lines", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n\n\n\n## Other\n"
		got := appendComment(body, ts)
		want := "# My Task\n\n## Comments\n\n" + ts + "\n\n## Other"
		if got != want {
			t.Errorf("appendComment = %q, want %q", got, want)
		}
	})
}

// TestAppendCommentSkipsFencedHeading guards against matching a `## Comments`
// line that sits inside a fenced code block, which would split the fence.
func TestAppendCommentSkipsFencedHeading(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	body := "# My Task\n\n## Comments\n\nOld comment.\n\n```\n## Comments\nfenced body\n```\n\n## Other\n\nTail.\n"
	got := appendComment(body, ts)
	assertContainsAll(t, got,
		"```\n## Comments\nfenced body\n```",
		"fenced body\n```\n\n"+ts,
		"## Other\n\nTail.",
	)
	assertNotContains(t, got, "Old comment.\n\n"+ts)
}

// TestAppendCommentSkipsIndentedCodeHeading guards against matching a
// `## Comments` line that sits in an indented code block, which would attach
// the new comment to the wrong section and split the code block.
func TestAppendCommentSkipsIndentedCodeHeading(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	body := "# My Task\n\n## Summary\n\nText.\n\n    ## Comments\n    indented body\n\n## Comments\n\nOld comment.\n"
	got := appendComment(body, ts)
	assertContainsAll(t, got,
		"    ## Comments\n    indented body",
		"## Comments\n\nOld comment.\n\n"+ts,
	)
	assertNotContains(t, got, "indented body\n\n"+ts)
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
