package ahm

import (
	"reflect"
	"strings"
	"testing"
)

func TestLocateHeadingSections(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		headings []string
		want     []markdownHeadingSection
	}{
		{
			name:     "missing section",
			body:     "## Other\n\nBody.\n",
			headings: []string{"Target"},
		},
		{
			name:     "final LF section",
			body:     "## Other\n\nBody.\n\n## Target\n\nFinal.\n",
			headings: []string{"Target"},
			want:     []markdownHeadingSection{{Start: 4, End: 8}},
		},
		{
			name:     "CRLF and case insensitive alias",
			body:     "## Other\r\n\r\nBody.\r\n\r\n### TARGET ALIAS\r\n\r\nContent.\r\n## Next\r\n",
			headings: []string{"target alias"},
			want:     []markdownHeadingSection{{Start: 4, End: 7}},
		},
		{
			name:     "nested heading stays in section",
			body:     "## Target\n\nBody.\n\n### Nested\n\nNested body.\n\n## Next\n",
			headings: []string{"Target"},
			want:     []markdownHeadingSection{{Start: 0, End: 8}},
		},
		{
			name:     "repeated headings return distinct spans",
			body:     "## Target\n\nFirst.\n\n## Other\n\nBody.\n\n### Target\n\nSecond.\n",
			headings: []string{"Target"},
			want: []markdownHeadingSection{
				{Start: 0, End: 4},
				{Start: 8, End: 12},
			},
		},
		{
			name:     "fenced heading is not a section boundary",
			body:     "## Target\n\nBody.\n\n```\n## Target\n```\n\n## Next\n",
			headings: []string{"Target"},
			want:     []markdownHeadingSection{{Start: 0, End: 8}},
		},
		{
			name:     "heading only inside a fence yields no section",
			body:     "## Other\n\n```\n## Target\n```\n",
			headings: []string{"Target"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.body, "\n")
			got := locateHeadingSections(lines, tt.headings)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("locateHeadingSections() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHeadingSectionRepeatedMatchPolicies(t *testing.T) {
	t.Run("cancellation updates first match", func(t *testing.T) {
		body := "## Cancellation Reason\n\nFirst.\n\n## Cancellation Reason\n\nSecond."
		want := "## Cancellation Reason\n\nReplacement.\n\n## Cancellation Reason\n\nSecond."
		if got := upsertCancellationReason(body, "Replacement."); got != want {
			t.Errorf("upsertCancellationReason() = %q, want %q", got, want)
		}
	})

	t.Run("ADR updates first match", func(t *testing.T) {
		body := "## Supersession\n\nFirst.\n\n## Supersession\n\nSecond.\n"
		want := "## Supersession\n\nReplacement.\n\n## Supersession\n\nSecond.\n"
		if got := upsertADRSection(body, "Supersession", "Replacement."); got != want {
			t.Errorf("upsertADRSection() = %q, want %q", got, want)
		}
	})

	t.Run("fenced cancellation heading is ignored", func(t *testing.T) {
		body := "## Summary\n\nText.\n\n```\n## Cancellation Reason\n```\n\n## Cancellation Reason\n\nOld reason."
		want := "## Summary\n\nText.\n\n```\n## Cancellation Reason\n```\n\n## Cancellation Reason\n\nNew reason."
		if got := upsertCancellationReason(body, "New reason."); got != want {
			t.Errorf("upsertCancellationReason() = %q, want %q", got, want)
		}
	})
}

// TestMarkdownFences pins the fence classifier every heading scan shares.
func TestMarkdownFences(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []bool
	}{
		{
			name: "info strings, tilde fence, longer close, indentation, unclosed tail",
			body: "intro\n```go\n## inside\n```\n~~~\ncode\n~~~~\n    ```\n    code\n```\nunclosed",
			want: []bool{false, true, true, true, true, true, true, false, false, true, true},
		},
		{
			name: "mismatched fence character stays inside",
			body: "```\n~~~\ncode\n```\ntail",
			want: []bool{true, true, true, true, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := markdownFences(strings.Split(tt.body, "\n"))
			if !reflect.DeepEqual(got.fenced, tt.want) {
				t.Errorf("markdownFences(%q) = %v, want %v", tt.body, got.fenced, tt.want)
			}
		})
	}
}

// TestUpsertADRMoreInformationLineSkipsFencedHeading proves the ADR scan reuses
// the shared fence model and edits the real section, not a fenced impostor.
func TestUpsertADRMoreInformationLineSkipsFencedHeading(t *testing.T) {
	body := "## Status\n\nAccepted.\n\n```\n## More Information\n```\n\n## More Information\n\n- Supersedes [ADR-001](001-one.md).\n"
	got := upsertADRMoreInformationLine(body, "- Supersedes [ADR-002](002-two.md).", "ADR-002")
	assertContainsAll(t, got,
		"```\n## More Information\n```",
		"- Supersedes [ADR-001](001-one.md).",
		"- Supersedes [ADR-002](002-two.md).",
	)
}
