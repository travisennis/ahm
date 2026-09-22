package ahm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWorkflowFile_CRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	// Write a file with CRLF line endings.
	content := "---\r\nid: 001\r\ntitle: CRLF Task\r\n---\r\n# CRLF Task\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readWorkflowFile(path)
	if err != nil {
		t.Errorf("readWorkflowFile: %v", err)
	}

	// Verify CRLF was normalized to LF.
	if strings.Contains(string(data), "\r\n") {
		t.Errorf("readWorkflowFile did not normalize CRLF: %q", data)
	}
	if !strings.HasPrefix(string(data), "---\n") {
		t.Errorf("expected LF front matter marker, got: %q", data)
	}
	if !strings.Contains(string(data), "\n---\n") {
		t.Errorf("expected LF front matter end marker, got: %q", data)
	}
}

func TestReadWorkflowFile_BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")

	// Write a file with UTF-8 BOM and CRLF line endings.
	// BOM = 0xEF 0xBB 0xBF = "\xef\xbb\xbf"
	bom := "\xef\xbb\xbf"
	content := bom + "---\r\nid: 001\r\ntitle: BOM Task\r\n---\r\n# BOM Task\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readWorkflowFile(path)
	if err != nil {
		t.Errorf("readWorkflowFile: %v", err)
	}

	// Verify BOM was stripped and CRLF was normalized to LF.
	if strings.Contains(string(data), "\r\n") {
		t.Errorf("readWorkflowFile did not normalize CRLF: %q", data)
	}
	if !strings.HasPrefix(string(data), "---\n") {
		t.Errorf("expected LF front matter marker after BOM strip, got: %q", data)
	}
	if strings.HasPrefix(string(data), "\xef\xbb\xbf") {
		t.Errorf("BOM was not stripped: %q", data)
	}
}

// TestFreshInitDefaultsToTheHomeStore is the milestone's headline behavior: a
// repository with no configuration is a new project, and a new project keeps
// its task records in the user-level store.
func TestFreshInitDefaultsToTheHomeStore(t *testing.T) {
	home := setStoreHome(t)
	root := newGitRepo(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"created:",
		"  .ahm/.gitignore",
		"  .ahm/config.json",
		"  store:.gitignore",
		"directories:",
		"  store:tasks/active",
		"  store:tasks/completed",
		"  store:tasks/cancelled",
		"  docs/adr",
		"indexes:",
		"  store:tasks/index.md",
		"  store:tasks/active/index.md",
		"  docs/adr/index.md",
	)
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")
	assertNotContains(t, stdout, ".ahm/.tasks", ".ahm/.research", ".ahm/exec-plans")

	config := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertContainsAll(t, config, `"tasks_location": "home"`, `"strict_acceptance": false`, `"files": {}`)
	assertNotContains(t, config, `"version":`, "taskWork", "projectDocs", "research")
	// The committed .gitignore keeps only the temp-file pattern: the generated
	// task indexes and the records lock moved into the store with the records.
	gitignore := mustRead(t, filepath.Join(root, ".ahm", ".gitignore"))
	if want := homeRecordsGitignoreHeader + gitignoreTempPattern + "\n"; gitignore != want {
		t.Errorf(".ahm/.gitignore = %q, want %q", gitignore, want)
	}
	projectRecords := filepath.Join(root, ".ahm", "tasks")
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("home-mode init created %s: %v", projectRecords, err)
	}
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName)); err != nil {
		t.Errorf("home-mode init did not create the store project directory: %v", err)
	}
	assertFileContainsAll(t, storeTaskFile(t, root, "", "index"),
		"# Task Index",
		"- Pending: 0",
		"## Next Ready Queue",
		"None.",
	)

	// A task created after this init lands in the store and never in the project.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Fresh task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: Fresh task")
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("task create wrote under the project records directory: %v", err)
	}

	for _, target := range []string{
		"AGENTS.md",
		".agents/TASKS.md",
		".agents/PLANS.md",
		".agents/RESEARCH.md",
		".agents/DOCS.md",
		".agents/.tasks/README.md",
		".agents/.research/README.md",
		".agents/skills",
		".ahm/tasks/README.md",
		"docs/adr/README.md",
		".ahm/research/index.md",
		".ahm/exec-plans/active/index.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s should not be installed, err = %v", target, err)
		}
	}
}

// TestInitOnExistingConfigurationKeepsTheProjectLayout is the guard for
// existing repositories: a configuration written before the home store existed
// has no tasks_location key, so it keeps its records in the project and never
// touches the store.
func TestInitOnExistingConfigurationKeepsTheProjectLayout(t *testing.T) {
	home := setStoreHome(t)
	root := projectRoot(t)
	configPath := filepath.Join(root, ".ahm", "config.json")
	before := mustRead(t, configPath)

	// A dry run previews the project layout and writes nothing, the store
	// included.
	var preview strings.Builder
	dry := app{opts: options{root: root, dryRun: true}, out: &preview}
	if err := dry.install(); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, preview.String(),
		"created:",
		"directories:",
		"  .ahm/tasks/active",
		"indexes:",
		"  .ahm/tasks/index.md",
	)
	assertNotContains(t, preview.String(), "store:", ".ahm/config.json")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run init created the project records directory: %v", err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"directories:",
		"  .ahm/tasks/active",
		"  docs/adr",
		"indexes:",
		"  .ahm/tasks/index.md",
		"  .ahm/tasks/active/index.md",
		"  docs/adr/index.md",
	)
	assertNotContains(t, stdout, "store:")
	if got := mustRead(t, configPath); got != before {
		t.Errorf("init rewrote a configuration that predates the store:\nbefore: %s\nafter:  %s", before, got)
	}
	gitignore := mustRead(t, filepath.Join(root, ".ahm", ".gitignore"))
	if gitignore != string(recordsGitignoreContent()) {
		t.Errorf(".ahm/.gitignore = %q, want the managed content", gitignore)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Project task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Errorf("the record is not in the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a project-mode repository touched the store: %v", err)
	}
	if got := relativeTreePaths(t, home); len(got) != 0 {
		t.Errorf("a project-mode repository left %v in the store, want nothing", got)
	}
}

// TestInitOnHomeRepositoryNeverCreatesProjectRecords covers the init side of the
// new default: a directory with no Git metadata is keyed by its path, every run
// of init leaves .ahm/tasks/ absent - including a repeated run and one with
// --force - and the store it lays down is left untouched by those runs.
func TestInitOnHomeRepositoryNeverCreatesProjectRecords(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	projectRecords := filepath.Join(root, ".ahm", "tasks")
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init created the project records directory: %v", err)
	}

	storeBefore := snapshotTree(t, home)
	projectBefore := snapshotTree(t, root)
	for _, args := range [][]string{{"init"}, {"--force", "init"}} {
		stdout, stderr, code := runCLI(t, append([]string{"--root", root}, args...)...)
		if code != 0 {
			t.Fatalf("%v exit code = %d, stdout = %s, stderr = %s", args, code, stdout, stderr)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Errorf("%v reported work on an up-to-date repository:\n%s", args, stdout)
		}
		if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%v created the project records directory: %v", args, err)
		}
		assertTreeUnchanged(t, home, storeBefore)
		assertTreeUnchanged(t, root, projectBefore)
	}

	// A directory with no .git is keyed by its symlink-resolved path, and the
	// store serves the task lifecycle from there.
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Kind != "path" {
		t.Errorf("a directory with no Git metadata resolved key kind %q, want path", store.Kind)
	}
	if _, err := os.Stat(store.recordsDir()); err != nil {
		t.Errorf("the store's records directory is missing: %v", err)
	}
	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Path-keyed task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: Path-keyed task")
}

// TestInitRewritesDriftedManagedGitignoreInHomeMode covers the committed
// .gitignore in the layout a new project gets: home mode keeps the temp-file
// pattern only, so an in-project entry list is drift that init rewrites.
func TestInitRewritesDriftedManagedGitignoreInHomeMode(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	gitignorePath := filepath.Join(root, ".ahm", ".gitignore")
	writeFile(t, gitignorePath, string(recordsGitignoreContent()))

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "updated:", "  .ahm/.gitignore")
	if got, want := mustRead(t, gitignorePath), homeRecordsGitignoreHeader+gitignoreTempPattern+"\n"; got != want {
		t.Errorf(".ahm/.gitignore = %q, want %q", got, want)
	}
}

// TestFreshInitRecordsTheNewProjectInTheStore covers the identity a new project
// leaves behind: the store directory is keyed by the origin remote, and init
// records the observation in the store registry.
func TestFreshInitRecordsTheNewProjectInTheStore(t *testing.T) {
	home := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:example/repo.git")

	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Kind != "remote" || store.Key != "github.com/example/repo" {
		t.Errorf("store identity = kind %q key %q, want the remote key github.com/example/repo", store.Kind, store.Key)
	}
	if _, err := os.Stat(store.recordsDir()); err != nil {
		t.Errorf("the store's records directory is missing: %v", err)
	}
	reg, err := loadRegistry(home)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := reg.Projects[store.Key]
	if !ok {
		t.Fatalf("init recorded no registry entry for %s: %+v", store.Key, reg.Projects)
	}
	if entry.Kind != "remote" || entry.Dir != store.dirName() {
		t.Errorf("registry entry = %+v, want kind remote and dir %q", entry, store.dirName())
	}
	if len(entry.Remotes) != 1 {
		t.Errorf("registry entry recorded remotes %v, want the one origin spelling", entry.Remotes)
	}
}

// TestInitFailsWhenGitCannotReadTheRepository pins the consequence of the new
// default: a new project resolves the store, so a root that holds .git but that
// Git cannot read fails before init writes anything instead of deriving a
// different key from the path.
func TestInitFailsWhenGitCannotReadTheRepository(t *testing.T) {
	setStoreHome(t)
	root := newGitRepo(t)
	gitDir := filepath.Join(root, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, gitDir, "gitdir: /nonexistent/ahm-test-git-dir\n")

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 1 {
		t.Fatalf("init exited %d for an unreadable repository, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "reading Git remotes")
	if _, err := os.Stat(filepath.Join(root, ".ahm")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("init wrote into a repository whose identity could not be derived: %v", err)
	}
}

// TestInitIsIdempotent is acceptance criterion one of task 264d: init on an
// up-to-date repository exits 0 and writes nothing. The second run reports no
// work, and every file ahm owns keeps its content and modification time.
func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("first init exit code = %d, stderr = %s", code, stderr)
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Project Agent Instructions\n\nKeep this.\n")
	before := snapshotTree(t, root)

	for _, args := range [][]string{
		{"init"},
		{"--force", "init"},
		{"--dry-run", "init"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, code := runCLI(t, append([]string{"--root", root}, args...)...)
			if code != 0 {
				t.Fatalf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Errorf("reported work on an up-to-date repository:\n%s", stdout)
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

// TestInitDropsObsoleteKeysAndPreservesUnknownMetadata is acceptance criterion
// two of task 264d: the obsolete taskWork key goes, and every unrelated field
// stays.
func TestInitDropsObsoleteKeysAndPreservesUnknownMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "0.2.0",
  "strict_acceptance": true,
  "default_work_agent": "codex",
  "taskWork": {
    "promptFile": ".agents/prompt.md",
    "implementation": {
      "agent": "codex",
      "model": "gpt-5-codex"
    },
    "review": {
      "agent": "claude"
    }
  },
  "projectDocs": {
    "entryPointBudget": 120
  },
  "research": {
    "inboxStaleDays": 9
  },
  "vendorExtension": {
    "enabled": true
  },
  "files": {
    ".agents/skills/preflight/SKILL.md": "abc"
  }
}`)

	before := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	var dryOut strings.Builder
	dry := app{opts: options{root: root, dryRun: true}, out: &dryOut}
	if err := dry.install(); err != nil {
		t.Fatal(err)
	}
	if afterDryRun := mustRead(t, filepath.Join(root, ".ahm", "config.json")); afterDryRun != before {
		t.Fatalf("dry-run modified metadata:\nbefore: %s\nafter: %s", before, afterDryRun)
	}
	assertContainsAll(t, dryOut.String(), "updated:", "  .ahm/config.json")

	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}

	got := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertNotContains(t, got, "taskWork", "default_work_agent", "projectDocs", "research", "promptFile")
	assertContainsAll(t, got, `"version": "0.2.0"`, `"strict_acceptance": true`, `"vendorExtension": {`, `"enabled": true`)
}

func TestInitRewritesDriftedManagedGitignore(t *testing.T) {
	root := projectRoot(t)
	// A repository that ran the retired records migration keeps the broad
	// index.md line that no longer describes ahm-managed state.
	writeFile(t, filepath.Join(root, ".ahm", ".gitignore"), "# Managed by ahm.\nindex.md\n")

	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	got := mustRead(t, filepath.Join(root, ".ahm", ".gitignore"))
	if got != string(recordsGitignoreContent()) {
		t.Errorf(".ahm/.gitignore = %q, want the managed content", got)
	}

	before := snapshotTree(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("second init exit code = %d, stderr = %s", code, stderr)
	}
	assertTreeUnchanged(t, root, before)
}

func TestInitDryRunPreviewsWritesWithoutWriting(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.install(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"created:",
		"  .ahm/.gitignore",
		"  .ahm/config.json",
		"  store:.gitignore",
		"directories:",
		"  store:tasks/active",
		"  docs/adr",
		"indexes:",
		"  store:tasks/index.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
	assertNotContains(t, got, "AGENTS.md", ".agents/TASKS.md")
	for _, dir := range []string{".ahm", ".agents", "docs"} {
		if _, err := os.Stat(filepath.Join(root, dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dry-run created %s, err = %v", dir, err)
		}
	}
	// The store is resolved without being created, so a preview leaves no store
	// directory behind.
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run created the store's project directory: %v", err)
	}
}

func TestInitDryRunReportsNothingWhenUpToDate(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}

	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.install(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("dry-run reported work on an up-to-date repository:\n%s", out.String())
	}
}

// TestInitLeavesProjectOwnedAgentsMdAlone is acceptance criterion three of task
// 264d: no install path creates, replaces, or removes AGENTS.md.
func TestInitLeavesProjectOwnedAgentsMdAlone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Project Agent Instructions\n\nKeep this.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--force", "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	assertNotContains(t, stdout, "AGENTS.md")
	assertFileContainsAll(t, filepath.Join(root, "AGENTS.md"), "Keep this.")

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Files["AGENTS.md"]; ok {
		t.Error("AGENTS.md should not be recorded as a managed file")
	}
}

func TestInitRelinquishesRetiredManagedFileHashes(t *testing.T) {
	root := t.TempDir()
	meta := metadata{Version: "0.4.6", Files: map[string]string{"unrelated/tool.json": "keep"}}
	for _, target := range retiredManagedFiles {
		content := []byte("managed " + target + "\n")
		meta.Files[target] = hashBytes(content)
		writeFile(t, filepath.Join(root, filepath.FromSlash(target)), string(content))
	}
	config, err := marshalMetadata(meta)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), string(config))

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(); err != nil {
		t.Fatal(err)
	}
	for _, target := range retiredManagedFiles {
		assertNotContains(t, out.String(), target)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err != nil {
			t.Errorf("retired managed file %s should remain, err=%v", target, err)
		}
	}

	after, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range retiredManagedFiles {
		if _, ok := after.Files[target]; ok {
			t.Errorf("%s should not remain in metadata", target)
		}
	}
	if after.Files["unrelated/tool.json"] != "keep" {
		t.Errorf("unrelated file hash = %q, want it preserved", after.Files["unrelated/tool.json"])
	}
	if _, ok := after.Files[".ahm/tasks/index.md"]; ok {
		t.Error("generated indexes should not carry ownership hashes")
	}
}

func TestInitJSONResultSchema(t *testing.T) {
	wantKeys := []string{"created", "directories", "indexes", "updated"}

	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "--json", "init")
	if code != 0 {
		t.Fatalf("init --json exit code = %d, stderr = %s", code, stderr)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("init --json returned invalid JSON: %v\n%s", err, stdout)
	}
	if len(result) != len(wantKeys) {
		t.Fatalf("init --json keys = %v, want exactly %v", result, wantKeys)
	}
	for _, key := range wantKeys {
		value, ok := result[key]
		if !ok {
			t.Errorf("init --json missing key %q", key)
			continue
		}
		var items []string
		if err := json.Unmarshal(value, &items); err != nil {
			t.Errorf("init --json key %q is not a string array: %s", key, value)
		}
	}

	// A second init reports no work at all.
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "init")
	if code != 0 {
		t.Fatalf("second init --json exit code = %d, stderr = %s", code, stderr)
	}
	result = nil
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("second init --json returned invalid JSON: %v\n%s", err, stdout)
	}
	for _, key := range wantKeys {
		var items []string
		if err := json.Unmarshal(result[key], &items); err != nil {
			t.Fatalf("second init --json key %q is not a string array: %s", key, result[key])
		}
		if len(items) != 0 {
			t.Errorf("second init reported %s = %v, want none", key, items)
		}
	}
}

func TestInitFailsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}

	metaPath := filepath.Join(root, ".ahm", "config.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code == 0 {
		t.Errorf("expected init to fail on corrupt metadata, stdout = %s", stdout)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .ahm/config.json")
}

// TestInitRefusesLegacyLayout is acceptance criterion five of task 264d: a
// repository still on .agents/ahm.json is reported, with the final v1 release
// named, instead of being initialized over as if it were unmanaged.
func TestInitRefusesLegacyLayout(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), `{"version":"0.6.4"}`+"\n")

	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, filepath.FromSlash(legacyMetadataRelPath), finalV1Release)
	if _, err := os.Stat(filepath.Join(root, ".ahm", "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("init created .ahm/config.json in a legacy repository, err = %v", err)
	}
}

func TestInitRefusesLegacyLayoutForExplicitRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), `{"version":"0.6.4"}`+"\n")

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, filepath.FromSlash(legacyMetadataRelPath), finalV1Release)
	if _, err := os.Stat(filepath.Join(root, ".ahm", "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("init created .ahm/config.json in a legacy repository, err = %v", err)
	}
}
