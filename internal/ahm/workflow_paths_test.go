package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimeMemoizesWorkflowPathsPerCommand(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	loads := 0
	var out, errOut strings.Builder
	a := app{
		opts: options{root: root}, out: &out, err: &errOut,
		pathsLoad: func(root string) workflowPaths {
			loads++
			return workflowPathsFor(root)
		},
	}

	if err := a.prime(); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatalf("workflow path resolver calls = %d, want 1", loads)
	}
}

func TestAppInvalidatesWorkflowPathsAfterConfigAnchorChange(t *testing.T) {
	root := t.TempDir()
	loads := 0
	a := app{
		opts: options{root: root},
		pathsLoad: func(root string) workflowPaths {
			loads++
			return workflowPathsFor(root)
		},
	}

	if got := a.workflowPaths().recordsDir; got != legacyRecordsDirName {
		t.Fatalf("initial records dir = %q, want %q", got, legacyRecordsDirName)
	}
	writeAHMConfig(t, root)
	if got := a.workflowPaths().recordsDir; got != legacyRecordsDirName {
		t.Fatalf("cached records dir = %q, want %q", got, legacyRecordsDirName)
	}
	a.invalidateWorkflowPaths()
	if got := a.workflowPaths().recordsDir; got != toolRecordsDirName {
		t.Fatalf("records dir after invalidation = %q, want %q", got, toolRecordsDirName)
	}
	if loads != 2 {
		t.Fatalf("workflow path resolver calls = %d, want 2", loads)
	}
}

func TestAppWorkflowPathsPreserveCorruptConfigLayout(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ahm", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := app{opts: options{root: root}}
	if got := a.workflowPaths().recordsDir; got != toolRecordsDirName {
		t.Fatalf("records dir = %q, want %q for corrupt .ahm config", got, toolRecordsDirName)
	}
}
