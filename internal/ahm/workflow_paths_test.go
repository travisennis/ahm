package ahm

import (
	"path/filepath"
	"testing"
)

func TestWorkflowPathsResolveTheAhmLayout(t *testing.T) {
	root := t.TempDir()
	paths := workflowPathsFor(root)

	if got, want := paths.tasksRel(), ".ahm/tasks"; got != want {
		t.Errorf("tasksRel() = %q, want %q", got, want)
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
