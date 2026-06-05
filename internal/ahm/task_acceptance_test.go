package ahm

import "testing"

func TestParseAcceptanceNotes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []taskAcceptanceFinding
	}{
		{
			name: "missing",
			body: "## Summary\n\nDone.\n",
			want: []taskAcceptanceFinding{taskAcceptanceMissing},
		},
		{
			name: "todo placeholder",
			body: "## Acceptance Notes\n\n- [ ] TODO\n",
			want: []taskAcceptanceFinding{taskAcceptancePlaceholder},
		},
		{
			name: "unchecked item",
			body: "## Acceptance Criteria\n\n  * [ ] Verify it\n",
			want: []taskAcceptanceFinding{taskAcceptanceUnchecked},
		},
		{
			name: "case insensitive h3 heading",
			body: "### acceptance\n\n- [x] Done\n",
			want: nil,
		},
		{
			name: "indented heading",
			body: "  ## Acceptance Notes\n\n- [x] Done\n",
			want: nil,
		},
		{
			name: "stops at same level heading",
			body: "## Acceptance Notes\n\n- [x] Done\n\n## Later\n\n- [ ] Not acceptance\n",
			want: nil,
		},
		{
			name: "includes deeper subsection",
			body: "## Acceptance Notes\n\n### Follow-up\n\n- [ ] Verify nested item\n",
			want: []taskAcceptanceFinding{taskAcceptanceUnchecked},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAcceptanceNotes([]byte(tt.body))
			if len(got) != len(tt.want) {
				t.Fatalf("findings = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("findings = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}
