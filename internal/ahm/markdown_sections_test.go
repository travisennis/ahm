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
	t.Run("groom rejects repeated aliases", func(t *testing.T) {
		body := "## Problem\n\nFirst.\n\n## Summary\n\nSecond.\n"
		if _, err := replaceGroomSection(body, "problem", "Replacement."); err == nil || !strings.Contains(err.Error(), "ambiguous problem sections") {
			t.Fatalf("replaceGroomSection() error = %v, want ambiguity error", err)
		}
		if _, _, err := groomSectionContent(body, "problem"); err == nil || !strings.Contains(err.Error(), "ambiguous problem sections") {
			t.Fatalf("groomSectionContent() error = %v, want ambiguity error", err)
		}
	})

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
}
