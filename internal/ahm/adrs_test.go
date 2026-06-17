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
		t.Errorf("parseADRFromData: %v", err)
	}
	if adr.ID != "009" || adr.Slug != "madr-adr-management" {
		t.Errorf("ADR identity = %s/%s", adr.ID, adr.Slug)
	}
	if adr.Title != "MADR ADR Management" {
		t.Errorf("Title = %q", adr.Title)
	}
	if adr.Status != "accepted" || adr.Date != "2026-06-14" {
		t.Errorf("status/date = %q/%q", adr.Status, adr.Date)
	}
	if adr.DecisionMakers != "Travis Ennis, Codex" || adr.Consulted != "research.md" || adr.Informed != "task 075" {
		t.Errorf("participants = %q/%q/%q", adr.DecisionMakers, adr.Consulted, adr.Informed)
	}
	if adr.Kind != adrKindMADR {
		t.Errorf("Kind = %q", adr.Kind)
	}
	if adr.Extra["source"] != "hand-authored" {
		t.Errorf("Extra = %v", adr.Extra)
	}
	if strings.Contains(adr.Body, "# MADR ADR Management") || !strings.Contains(adr.Body, "## Context") {
		t.Errorf("Body = %q", adr.Body)
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
		t.Errorf("parseADRFromData: %v", err)
	}
	got := renderADR(adr)
	if got != want {
		t.Errorf("renderADR mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	again, err := parseADRFromData([]byte(got), path)
	if err != nil {
		t.Errorf("parse rendered ADR: %v", err)
	}
	if renderADR(again) != got {
		t.Error("parse/render round trip is not stable")
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
		t.Errorf("expected ID mismatch error, got %v", err)
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
				t.Errorf("expected filename error, got %v", err)
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
		t.Errorf("expected malformed ADR collection error, got %v", err)
	}
	if len(adrs) != 3 {
		t.Errorf("len(adrs) = %d, want 3", len(adrs))
	}
	if adrs[0].ID != "001" || adrs[0].Kind != adrKindLegacy || adrs[0].Title != "Legacy Decision" {
		t.Errorf("legacy ADR = %#v", adrs[0])
	}
	if adrs[1].ID != "002" || adrs[1].Kind != adrKindMADR {
		t.Errorf("MADR ADR = %#v", adrs[1])
	}
	if adrs[2].ID != "003" || adrs[2].Kind != adrKindMalformed || adrs[2].ParseError == "" {
		t.Errorf("malformed ADR = %#v", adrs[2])
	}
}

func TestAdrFilePathsExcludesCaseVariants(t *testing.T) {
	root := t.TempDir()
	// Create a real ADR and various case variants of README.md and index.md
	writeADRFile(t, root, "001-real-decision.md", "# ADR 001: Real\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nBody.\n")
	writeADRFile(t, root, "readme.md", "# readme\n")
	writeADRFile(t, root, "README.MD", "# README uppercase extension\n")
	writeADRFile(t, root, "ReadMe.md", "# ReadMe mixed\n")
	writeADRFile(t, root, "index.md", "# index\n")
	writeADRFile(t, root, "INDEX.md", "# INDEX\n")
	writeADRFile(t, root, "IndEx.md", "# IndEx mixed\n")

	files, err := adrFilePaths(root)
	if err != nil {
		t.Errorf("adrFilePaths: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("len(files) = %d, want 1 (got %v)", len(files), files)
	}
	if !strings.HasSuffix(files[0], "001-real-decision.md") {
		t.Errorf("unexpected file: %s", files[0])
	}
}

func TestCollectADRsCurrentRepoRecords(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	adrs, err := collectADRs(root)
	if err != nil {
		t.Errorf("collectADRs current repo: %v", err)
	}
	byID := map[string]ADR{}
	for _, adr := range adrs {
		byID[adr.ID] = adr
	}
	for _, id := range []string{"001", "002", "003", "004", "005", "006", "007", "008"} {
		adr, ok := byID[id]
		if !ok {
			t.Errorf("current repo ADR %s was not collected", id)
		}
		if adr.Kind == adrKindMalformed {
			t.Errorf("current repo ADR %s is malformed: %s", id, adr.ParseError)
		}
	}
	if byID["009"].Kind != adrKindMADR {
		t.Error("ADR 009 was not classified as MADR")
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
				t.Errorf("resolveADR: %v", err)
			}
			if adr.ID != tt.want {
				t.Errorf("ID = %q, want %q", adr.ID, tt.want)
			}
		})
	}
	t.Run("no substring false positive", func(t *testing.T) {
		_, err := resolveADR("009-madr", adrs)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not found, got %v", err)
		}
	})
}

func TestNextADRID(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-existing.md", "not parseable but filename counts\n")
	writeADRFile(t, root, "015-skipped-malformed.md", "---\nstatus: accepted\n")
	got := nextADRID([]ADR{{ID: "009"}, {ID: "010"}}, root)
	if got != "016" {
		t.Errorf("nextADRID = %q, want 016", got)
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
				t.Errorf("validADRStatus(%q) = %v, want %v", tt.status, got, tt.want)
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
