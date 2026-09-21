package ahm

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestWorkflowPathsResolveTheAhmLayout(t *testing.T) {
	root := t.TempDir()
	paths := workflowPathsFor(root)

	if got, want := paths.recordsRel(), ".ahm/tasks"; got != want {
		t.Errorf("recordsRel() = %q, want %q", got, want)
	}
	if got, want := paths.tasksBucketDir("active"), filepath.Join(root, ".ahm", "tasks", "active"); got != want {
		t.Errorf("tasksBucketDir(active) = %q, want %q", got, want)
	}
	if got, want := paths.taskFile("active", "001"), filepath.Join(root, ".ahm", "tasks", "active", "001.md"); got != want {
		t.Errorf("taskFile() = %q, want %q", got, want)
	}
	// The root bucket directory is the tasks directory itself.
	if got, want := paths.tasksBucketDir(""), filepath.Join(root, ".ahm", "tasks"); got != want {
		t.Errorf("tasksBucketDir(\"\") = %q, want %q", got, want)
	}
}

// TestWorkflowPathsIgnoreMetadata pins the collapse of the dual layout: record
// paths no longer depend on which metadata file anchors the repository, so a
// legacy .agents/ahm.json config cannot move them.
func TestWorkflowPathsIgnoreMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), `{"version":"0.6.4"}`+"\n")

	withMetadata := workflowPathsFor(root).taskFile("active", "001")
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{"strict_acceptance":false,"files":{}}`+"\n")
	withConfig := workflowPathsFor(root).taskFile("active", "001")

	if withMetadata != withConfig {
		t.Errorf("task path changed with metadata location: %q vs %q", withMetadata, withConfig)
	}
	if want := filepath.Join(root, ".ahm", "tasks", "active", "001.md"); withConfig != want {
		t.Errorf("task path = %q, want %q", withConfig, want)
	}
}

// TestWorkflowPathsProjectModeAccessors pins the project-mode layout every
// existing repository uses. The two-root split must not move a single path
// while records stay in the project.
func TestWorkflowPathsProjectModeAccessors(t *testing.T) {
	root := t.TempDir()
	paths := workflowPathsFor(root)

	if got, want := paths.configPath(), filepath.Join(root, ".ahm", "config.json"); got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
	if got, want := paths.adrDir(), filepath.Join(root, "docs", "adr"); got != want {
		t.Errorf("adrDir() = %q, want %q", got, want)
	}
	if got, want := paths.adrIndexPath(), filepath.Join(root, "docs", "adr", "index.md"); got != want {
		t.Errorf("adrIndexPath() = %q, want %q", got, want)
	}
	if got, want := paths.ownedRoots(), []string{root}; !slices.Equal(got, want) {
		t.Errorf("ownedRoots() = %v, want %v", got, want)
	}
	// The lock stays at .ahm/.lock, the path every existing repository and
	// concurrent invocation already uses.
	if got, want := paths.lockDir(), filepath.Join(root, ".ahm", ".lock"); got != want {
		t.Errorf("lockDir() = %q, want %q", got, want)
	}
	// A project path is displayed relative to the project root.
	if got, want := paths.displayPath(paths.taskFile("active", "001")), ".ahm/tasks/active/001.md"; got != want {
		t.Errorf("displayPath() = %q, want %q", got, want)
	}
}

// TestWorkflowPathsHomeModeAccessors pins the home store layout: configuration
// and ADRs stay in the project, records and their indexes move to the store,
// and the lock moves with the records so two clones sharing a store serialize
// on one lock.
func TestWorkflowPathsHomeModeAccessors(t *testing.T) {
	root := t.TempDir()
	store := testStorePaths(t)
	paths := workflowPathsForStore(root, store)

	if got, want := paths.recordsRoot, filepath.Join(store.ProjectDir, "tasks"); got != want {
		t.Errorf("recordsRoot = %q, want %q", got, want)
	}
	if got, want := paths.recordsRel(), "tasks"; got != want {
		t.Errorf("recordsRel() = %q, want %q", got, want)
	}
	if got, want := paths.taskFile("active", "267"), filepath.Join(store.ProjectDir, "tasks", "active", "267.md"); got != want {
		t.Errorf("taskFile() = %q, want %q", got, want)
	}
	// Configuration and ADRs do not move with the records.
	if got, want := paths.configPath(), filepath.Join(root, ".ahm", "config.json"); got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
	if got, want := paths.adrIndexPath(), filepath.Join(root, "docs", "adr", "index.md"); got != want {
		t.Errorf("adrIndexPath() = %q, want %q", got, want)
	}
	if got, want := paths.ownedRoots(), []string{root, store.ProjectDir}; !slices.Equal(got, want) {
		t.Errorf("ownedRoots() = %v, want %v", got, want)
	}
	if got, want := paths.lockDir(), filepath.Join(store.ProjectDir, ".lock"); got != want {
		t.Errorf("lockDir() = %q, want %q", got, want)
	}
	if got, want := paths.displayPath(paths.taskFile("active", "267")), "store:tasks/active/267.md"; got != want {
		t.Errorf("displayPath(record) = %q, want %q", got, want)
	}
	if got, want := paths.displayPath(paths.adrIndexPath()), "docs/adr/index.md"; got != want {
		t.Errorf("displayPath(adr index) = %q, want %q", got, want)
	}
}
