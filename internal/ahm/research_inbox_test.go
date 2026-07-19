package ahm

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResearchNoteDatePrecedence(t *testing.T) {
	data := []byte("---\ncreated: 2026-01-01\ndate: 2026-02-01\nupdated: 2026-03-01\n---\n# Note\n\nUpdated: 2026-04-01\n")
	got, ok := researchNoteDate(data)
	if !ok {
		t.Fatal("researchNoteDate did not find a date")
	}
	if want := "2026-03-01"; got.Format(time.DateOnly) != want {
		t.Fatalf("researchNoteDate = %s, want %s", got.Format(time.DateOnly), want)
	}
}

func TestResearchNoteAgeDaysFallsBackToMtime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "undated.md")
	writeFile(t, path, "# Undated\n")
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	mtime := now.Add(-25 * 24 * time.Hour)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	got, err := researchNoteAgeDays(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if got != 25 {
		t.Fatalf("researchNoteAgeDays = %d, want 25", got)
	}
}

func TestValidateResearchInboxFreshStaleDisabledAndLegacy(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	t.Run("empty", func(t *testing.T) {
		root := t.TempDir()
		setupAhmRepo(t, root)
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 0 {
			t.Fatalf("empty warnings = %#v, want none", report.Warnings)
		}
	})

	t.Run("fresh", func(t *testing.T) {
		root := t.TempDir()
		setupAhmRepo(t, root)
		writeFile(t, filepath.Join(root, ".ahm", "research", "inbox", "fresh.md"), "# Fresh\n\nCreated: 2026-07-10\n")
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 0 {
			t.Fatalf("fresh warnings = %#v, want none", report.Warnings)
		}
	})

	t.Run("stale missing date", func(t *testing.T) {
		root := t.TempDir()
		setupAhmRepo(t, root)
		path := filepath.Join(root, ".ahm", "research", "inbox", "stale.md")
		writeFile(t, path, "# Stale\n")
		mtime := now.Add(-22 * 24 * time.Hour)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 1 || report.Warnings[0].Code != "research_inbox_stale" {
			t.Fatalf("stale warnings = %#v", report.Warnings)
		}
		if report.Warnings[0].Path != ".ahm/research/inbox/stale.md" {
			t.Fatalf("stale path = %q", report.Warnings[0].Path)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		root := t.TempDir()
		setupAhmRepo(t, root)
		zero := 0
		meta, err := readMetadata(root)
		if err != nil {
			t.Fatal(err)
		}
		meta.Research = &researchConfig{InboxStaleDays: &zero}
		if err := writeMetadata(root, meta); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, ".ahm", "research", "inbox", "old.md"), "---\ncreated: 2020-01-01\n---\n# Old\n")
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 0 {
			t.Fatalf("disabled warnings = %#v, want none", report.Warnings)
		}
	})

	t.Run("custom threshold", func(t *testing.T) {
		root := t.TempDir()
		setupAhmRepo(t, root)
		days := 10
		meta, err := readMetadata(root)
		if err != nil {
			t.Fatal(err)
		}
		meta.Research = &researchConfig{InboxStaleDays: &days}
		if err := writeMetadata(root, meta); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, ".ahm", "research", "inbox", "custom.md"), "# Custom\n\nCreated: 2026-07-09\n")
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 1 || report.Warnings[0].Code != "research_inbox_stale" {
			t.Fatalf("custom threshold warnings = %#v", report.Warnings)
		}
	})

	t.Run("legacy layout", func(t *testing.T) {
		root := t.TempDir()
		initAndCreateLegacyMetadata(t, root)
		writeFile(t, filepath.Join(root, ".agents", ".research", "inbox", "legacy.md"), "# Legacy\n\nCreated: 2026-01-01\n")
		report := newValidationReport()
		validateResearchInbox(root, workflowPathsFor(root), &report, now)
		if len(report.Warnings) != 1 || report.Warnings[0].Path != ".agents/.research/inbox/legacy.md" {
			t.Fatalf("legacy warnings = %#v", report.Warnings)
		}
	})
}
