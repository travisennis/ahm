package ahm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateReportsCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	// Init first to create valid workflow.
	setupAhmRepo(t, root)

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".ahm", "config.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
	foundCorrupt := false
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" {
			foundCorrupt = true
			break
		}
	}
	if !foundCorrupt {
		t.Errorf("expected metadata_corrupt error, got: %v", report.Errors)
	}
	// Should not produce metadata_missing (which is only for absent file).
	for _, err := range report.Errors {
		if err.Code == "metadata_missing" {
			t.Errorf("unexpected metadata_missing error for corrupt file: %v", err)
		}
	}
}

func TestValidateReportsCorruptAhmConfig(t *testing.T) {
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{Version: "0.1.0", Files: map[string]string{}})
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{invalid json}")

	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
	foundCorrupt := false
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" && err.Path == ".ahm/config.json" {
			foundCorrupt = true
			break
		}
	}
	if !foundCorrupt {
		t.Errorf("expected metadata_corrupt error for .ahm/config.json, got: %v", report.Errors)
	}
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" && err.Path != configMetadataRelPath {
			t.Errorf("metadata_corrupt finding path = %q, want %q: %v", err.Path, configMetadataRelPath, err)
		}
	}
}

func TestValidateReportsMissingMetadata(t *testing.T) {
	root := t.TempDir()
	// No init, no metadata at all.
	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
	foundMissing := false
	for _, err := range report.Errors {
		if err.Code == "metadata_missing" {
			foundMissing = true
			break
		}
	}
	if !foundMissing {
		t.Errorf("expected metadata_missing error, got: %v", report.Errors)
	}
	// Should not produce metadata_corrupt.
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" {
			t.Errorf("unexpected metadata_corrupt error for missing file: %v", err)
		}
	}
}
