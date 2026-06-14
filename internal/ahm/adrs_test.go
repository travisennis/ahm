package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseADR(t *testing.T) {
	path := filepath.Join(t.TempDir(), "009-madr-adr-management.md")
	input := "---\n" +
		"status: accepted\n" +
		"date: 2026-06-14\n" +
		"decision-makers: Travis Ennis, Codex\n" +
		"consulted: research.md\n" +
		"informed: task 075\n" +
		"source: hand-authored\n" +
		"---\n" +
		"# MADR ADR Management\n\n" +
		"## Context\n\n" +
		"Body.\n"

	adr, err := parseADRFromData([]byte(input), path)
	if err != nil {
		t.Fatalf("parseADRFromData: %v", err)
	}
	if adr.ID != "009" || adr.Slug != "madr-adr-management" {
		t.Fatalf("ADR identity = %s/%s", adr.ID, adr.Slug)
	}
	if adr.Title != "MADR ADR Management" {
		t.Fatalf("Title = %q", adr.Title)
	}
	if adr.Status != "accepted" || adr.Date != "2026-06-14" {
		t.Fatalf("status/date = %q/%q", adr.Status, adr.Date)
	}
	if adr.DecisionMakers != "Travis Ennis, Codex" || adr.Consulted != "research.md" || adr.Informed != "task 075" {
		t.Fatalf("participants = %q/%q/%q", adr.DecisionMakers, adr.Consulted, adr.Informed)
	}
	if adr.Kind != adrKindMADR {
		t.Fatalf("Kind = %q", adr.Kind)
	}
	if adr.Extra["source"] != "hand-authored" {
		t.Fatalf("Extra = %v", adr.Extra)
	}
	if strings.Contains(adr.Body, "# MADR ADR Management") || !strings.Contains(adr.Body, "## Context") {
		t.Fatalf("Body = %q", adr.Body)
	}
}

func TestRenderADRRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "010-round-trip.md")
	input := "---\n" +
		"informed: task 075\n" +
		"zeta: last\n" +
		"date: 2026-06-14\n" +
		"status: proposed\n" +
		"consulted: research.md\n" +
		"alpha: first\n" +
		"decision-makers: Travis Ennis\n" +
		"---\n" +
		"# Round Trip\n\n" +
		"## Context\n\n" +
		"Body.\n"
	want := "---\n" +
		"status: proposed\n" +
		"date: 2026-06-14\n" +
		"decision-makers: Travis Ennis\n" +
		"consulted: research.md\n" +
		"informed: task 075\n" +
		"alpha: first\n" +
		"zeta: last\n" +
		"---\n" +
		"# Round Trip\n\n" +
		"## Context\n\n" +
		"Body.\n\n"

	adr, err := parseADRFromData([]byte(input), path)
	if err != nil {
		t.Fatalf("parseADRFromData: %v", err)
	}
	got := renderADR(adr)
	if got != want {
		t.Fatalf("renderADR mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	again, err := parseADRFromData([]byte(got), path)
	if err != nil {
		t.Fatalf("parse rendered ADR: %v", err)
	}
	if renderADR(again) != got {
		t.Fatal("parse/render round trip is not stable")
	}
}

func TestParseADRDetectsIDMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "009-title.md")
	input := "---\n" +
		"id: 008\n" +
		"status: accepted\n" +
		"date: 2026-06-14\n" +
		"---\n" +
		"# Title\n"
	_, err := parseADRFromData([]byte(input), path)
	if err == nil || !strings.Contains(err.Error(), "does not match filename") {
		t.Fatalf("expected ID mismatch error, got %v", err)
	}
}

func TestParseADRRejectsInvalidFilenameSlug(t *testing.T) {
	input := "---\n" +
		"status: accepted\n" +
		"date: 2026-06-14\n" +
		"---\n" +
		"# Title\n"
	for _, name := range []string{
		"009-.md",
		"009-Title.md",
		"009--leading.md",
		"009-two--dash.md",
		"009-leading-.md",
		"009-under_score.md",
		"009-has space.md",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			_, err := parseADRFromData([]byte(input), path)
			if err == nil || !strings.Contains(err.Error(), "NNN-kebab-slug.md") {
				t.Fatalf("expected filename error, got %v", err)
			}
		})
	}
}

func TestCollectADRsClassifiesLegacyAndMalformed(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-legacy-decision.md", "# ADR 001: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nLegacy body.\n")
	writeADRFile(t, root, "002-madr-decision.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# MADR Decision\n\nBody.\n")
	writeADRFile(t, root, "003-bad-decision.md", "---\nstatus: accepted\n# Missing close\n")
	writeADRFile(t, root, "README.md", "# ADR README\n")
	writeADRFile(t, root, "index.md", "# Generated\n")

	adrs, err := collectADRs(root)
	if err == nil || !strings.Contains(err.Error(), "003-bad-decision.md") {
		t.Fatalf("expected malformed ADR collection error, got %v", err)
	}
	if len(adrs) != 3 {
		t.Fatalf("len(adrs) = %d, want 3", len(adrs))
	}
	if adrs[0].ID != "001" || adrs[0].Kind != adrKindLegacy || adrs[0].Title != "Legacy Decision" {
		t.Fatalf("legacy ADR = %#v", adrs[0])
	}
	if adrs[1].ID != "002" || adrs[1].Kind != adrKindMADR {
		t.Fatalf("MADR ADR = %#v", adrs[1])
	}
	if adrs[2].ID != "003" || adrs[2].Kind != adrKindMalformed || adrs[2].ParseError == "" {
		t.Fatalf("malformed ADR = %#v", adrs[2])
	}
}

func TestCollectADRsCurrentRepoRecords(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	adrs, err := collectADRs(root)
	if err != nil {
		t.Fatalf("collectADRs current repo: %v", err)
	}
	byID := map[string]ADR{}
	for _, adr := range adrs {
		byID[adr.ID] = adr
	}
	for _, id := range []string{"001", "002", "003", "004", "005", "006", "007", "008"} {
		adr, ok := byID[id]
		if !ok {
			t.Fatalf("current repo ADR %s was not collected", id)
		}
		if adr.Kind == adrKindMalformed {
			t.Fatalf("current repo ADR %s is malformed: %s", id, adr.ParseError)
		}
	}
	if byID["009"].Kind != adrKindMADR {
		t.Fatal("ADR 009 was not classified as MADR")
	}
}

func TestResolveADR(t *testing.T) {
	adrs := []ADR{
		{ID: "001", Slug: "alpha"},
		{ID: "009", Slug: "madr-adr-management"},
		{ID: "010", Slug: "madr"},
	}
	tests := []struct {
		pattern string
		want    string
	}{
		{pattern: "9", want: "009"},
		{pattern: "009", want: "009"},
		{pattern: "009-madr-adr-management", want: "009"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			adr, err := resolveADR(tt.pattern, adrs)
			if err != nil {
				t.Fatalf("resolveADR: %v", err)
			}
			if adr.ID != tt.want {
				t.Fatalf("ID = %q, want %q", adr.ID, tt.want)
			}
		})
	}
	t.Run("no substring false positive", func(t *testing.T) {
		_, err := resolveADR("009-madr", adrs)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestNextADRID(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-existing.md", "not parseable but filename counts\n")
	writeADRFile(t, root, "015-skipped-malformed.md", "---\nstatus: accepted\n")
	got := nextADRID([]ADR{{ID: "009"}, {ID: "010"}}, root)
	if got != "016" {
		t.Fatalf("nextADRID = %q, want 016", got)
	}
}

func TestValidADRStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: "proposed", want: true},
		{status: "accepted", want: true},
		{status: "rejected", want: true},
		{status: "deprecated", want: true},
		{status: "superseded by ADR-009", want: true},
		{status: "Superseded by ADR-009", want: false},
		{status: "superseded by 009", want: false},
		{status: "accepted in part", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := validADRStatus(tt.status); got != tt.want {
				t.Fatalf("validADRStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func writeADRFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, "docs", "adr", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
