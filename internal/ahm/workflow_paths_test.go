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

// TestWorkflowPathsRecordAndPreviewPaths pins the two output rules that differ
// from displayPath, and why they differ from each other. A structured payload
// carries the record's own absolute path rather than a repository-relative one,
// and a store record renders as store:<store-relative>.
//
// The project case splits by call site. Before the home store, each dry-run
// preview site applied filepath.ToSlash, and nothing touched the JSON `path`
// field, so that field carried the platform's own separator. ADR 023 keeps
// project-mode payloads byte-identical, so recordPath reproduces the field and
// payloadPath reproduces the previews rather than unifying them.
//
// The separator choice is only observable on Windows, where ToSlash is not the
// identity: on a slash-separated platform both assertions below hold for any
// implementation. The windows-latest leg of CI is what discriminates.
func TestWorkflowPathsRecordAndPreviewPaths(t *testing.T) {
	root := t.TempDir()
	project := workflowPathsFor(root)
	native := project.taskFile("active", "001")

	if got := project.recordPath(native); got != native {
		t.Errorf("recordPath(project record) = %q, want the record's own path %q", got, native)
	}
	if got, want := project.payloadPath(native), filepath.ToSlash(native); got != want {
		t.Errorf("payloadPath(project record) = %q, want %q", got, want)
	}

	home := workflowPathsForStore(root, testStorePaths(t))
	storeRecord := home.taskFile("active", "267")
	if got, want := home.recordPath(storeRecord), "store:tasks/active/267.md"; got != want {
		t.Errorf("recordPath(store record) = %q, want %q", got, want)
	}
	if got, want := home.payloadPath(storeRecord), "store:tasks/active/267.md"; got != want {
		t.Errorf("payloadPath(store record) = %q, want %q", got, want)
	}
	// A project path stays an absolute project path even when the records live
	// in the store.
	if got := home.recordPath(project.adrIndexPath()); got != project.adrIndexPath() {
		t.Errorf("recordPath(project path in store mode) = %q, want %q", got, project.adrIndexPath())
	}
	if got, want := home.payloadPath(project.adrIndexPath()), filepath.ToSlash(project.adrIndexPath()); got != want {
		t.Errorf("payloadPath(project path in store mode) = %q, want %q", got, want)
	}
}

// TestWorkflowPathsInProjectRecordPath pins the mapping link validation uses as
// its fallback: a store record resolves links against the in-project path it
// would have if it still lived in the project.
func TestWorkflowPathsInProjectRecordPath(t *testing.T) {
	root := t.TempDir()
	store := testStorePaths(t)
	home := workflowPathsForStore(root, store)

	storeRecord := home.taskFile("active", "267")
	got, ok := home.inProjectRecordPath(storeRecord)
	if !ok {
		t.Fatalf("inProjectRecordPath(%q) reported no mapping", storeRecord)
	}
	if want := filepath.Join(root, ".ahm", "tasks", "active", "267.md"); got != want {
		t.Errorf("inProjectRecordPath() = %q, want %q", got, want)
	}

	project := workflowPathsFor(root)
	if _, ok := project.inProjectRecordPath(project.taskFile("active", "001")); ok {
		t.Error("inProjectRecordPath() mapped a project record")
	}
	if _, ok := home.inProjectRecordPath(project.adrIndexPath()); ok {
		t.Error("inProjectRecordPath() mapped a path outside the records root")
	}
}

// TestWorkflowPathsGitignoreFollowsTheRecords pins where the managed .gitignore
// lives: .ahm/.gitignore in project mode, unchanged, and the store project
// directory's .gitignore in home mode, where the generated indexes and the
// lock live. The store's directory is an owned root, so the write is contained.
func TestWorkflowPathsGitignoreFollowsTheRecords(t *testing.T) {
	root := t.TempDir()
	project := workflowPathsFor(root)
	if got, want := project.workflowGitignorePath(), filepath.Join(root, ".ahm", ".gitignore"); got != want {
		t.Errorf("project gitignore path = %q, want %q", got, want)
	}
	if got, want := string(project.workflowGitignoreContent()), string(recordsGitignoreContent()); got != want {
		t.Errorf("project gitignore content changed:\ngot:\n%s\nwant:\n%s", got, want)
	}

	store := testStorePaths(t)
	home := workflowPathsForStore(root, store)
	gitignorePath := home.workflowGitignorePath()
	if got, want := gitignorePath, filepath.Join(store.ProjectDir, ".gitignore"); got != want {
		t.Errorf("store gitignore path = %q, want %q", got, want)
	}
	if !pathWithin(store.ProjectDir, gitignorePath) {
		t.Errorf("store gitignore %q is outside the owned store project directory", gitignorePath)
	}
	content := string(home.workflowGitignoreContent())
	assertContainsAll(t, content, "tasks/index.md", ".lock/", "*.tmp", storeStateFileName)
	if content == string(recordsGitignoreContent()) {
		t.Error("the store gitignore reused the in-project header and entries")
	}
}

// TestWorkflowPathsRecordsStatus reports the store only when the records live in
// it, which is what keeps project-mode status and prime byte-identical.
func TestWorkflowPathsRecordsStatus(t *testing.T) {
	root := t.TempDir()
	if status, ok := workflowPathsFor(root).recordsStatus(); ok || status != nil {
		t.Errorf("project mode reported a store status: %v, %v", status, ok)
	}

	store := testStorePaths(t)
	status, ok := workflowPathsForStore(root, store).recordsStatus()
	if !ok {
		t.Fatal("home mode reported no store status")
	}
	want := map[string]string{
		"root":     store.Root,
		"key":      store.Key,
		"kind":     store.Kind,
		"location": string(locationHome),
	}
	if len(status) != len(want) {
		t.Fatalf("recordsStatus() = %v, want %v", status, want)
	}
	for key, value := range want {
		if status[key] != value {
			t.Errorf("recordsStatus()[%q] = %q, want %q", key, status[key], value)
		}
	}
}
