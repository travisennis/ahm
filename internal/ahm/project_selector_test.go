package ahm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// registerStoreProject writes a project's registry entry and state file
// directly, so a test can populate the store without a checkout. recordedPath
// is the project path the entry remembers; it may name a directory that does
// not exist.
func registerStoreProject(t *testing.T, home string, key string, dir string, recordedPath string) storePaths {
	t.Helper()
	s := storePaths{
		Root:         home,
		Key:          key,
		Kind:         storeKindRemote,
		ProjectDir:   filepath.Join(home, storeProjectsDirName, dir),
		resolvedPath: recordedPath,
	}
	if err := recordStoreProject(s); err != nil {
		t.Fatalf("recording store project %s: %v", key, err)
	}
	return s
}

// selectorFixture registers three projects in a scratch store and returns the
// store home.
func selectorFixture(t *testing.T) string {
	t.Helper()
	home := setStoreHome(t)
	registerStoreProject(t, home, "github.com/travisennis/ahm", "ahm-0242d8b9", filepath.Join(home, "checkouts", "ahm"))
	registerStoreProject(t, home, "github.com/travisennis/other", "other-1111aaaa", "")
	registerStoreProject(t, home, "gitlab.com/acme/ahm-tools", "ahm-tools-2222bbbb", "")
	return home
}

func TestResolveProjectSelection(t *testing.T) {
	home := selectorFixture(t)
	cases := []struct {
		name     string
		selector string
		wantKey  string
		wantDir  string
	}{
		{name: "exact key", selector: "github.com/travisennis/ahm", wantKey: "github.com/travisennis/ahm", wantDir: "ahm-0242d8b9"},
		{name: "substring of key", selector: "travisennis/other", wantKey: "github.com/travisennis/other", wantDir: "other-1111aaaa"},
		{name: "substring of directory", selector: "2222bbbb", wantKey: "gitlab.com/acme/ahm-tools", wantDir: "ahm-tools-2222bbbb"},
		{name: "case-insensitive directory substring", selector: "AHM-TOOLS", wantKey: "gitlab.com/acme/ahm-tools", wantDir: "ahm-tools-2222bbbb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveProjectSelection(tc.selector)
			if err != nil {
				t.Fatalf("resolveProjectSelection(%q): %v", tc.selector, err)
			}
			if got.Key != tc.wantKey {
				t.Errorf("Key = %q, want %q", got.Key, tc.wantKey)
			}
			if base := filepath.Base(got.ProjectDir); base != tc.wantDir {
				t.Errorf("ProjectDir base = %q, want %q", base, tc.wantDir)
			}
			if got.Root != home {
				t.Errorf("Root = %q, want %q", got.Root, home)
			}
		})
	}
}

func TestResolveProjectSelectionErrors(t *testing.T) {
	selectorFixture(t)
	cases := []struct {
		name        string
		selector    string
		wantMessage string
	}{
		{
			name:        "ambiguous",
			selector:    "ahm",
			wantMessage: "matches multiple projects: github.com/travisennis/ahm, gitlab.com/acme/ahm-tools",
		},
		{
			name:        "unknown",
			selector:    "nowhere",
			wantMessage: "known projects: github.com/travisennis/ahm, github.com/travisennis/other, gitlab.com/acme/ahm-tools",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveProjectSelection(tc.selector)
			var usage usageError
			if !errors.As(err, &usage) {
				t.Fatalf("resolveProjectSelection(%q) error = %v, want a usage error", tc.selector, err)
			}
			assertContainsAll(t, err.Error(), tc.wantMessage)
		})
	}
}

func TestResolveProjectSelectionEmptyStore(t *testing.T) {
	setStoreHome(t)
	_, err := resolveProjectSelection("ahm")
	var usage usageError
	if !errors.As(err, &usage) {
		t.Fatalf("resolveProjectSelection on an empty store error = %v, want a usage error", err)
	}
	assertContainsAll(t, err.Error(), "no registered projects")
}

// TestProjectSelectorReachesARecordWithoutACheckout is the acceptance case: a
// record create from an unrelated directory lands in the selected store project,
// touches nothing in the working directory's project, and never reads the
// selected project's checkout — which is deleted before the command runs.
func TestProjectSelectorReachesARecordWithoutACheckout(t *testing.T) {
	setStoreHome(t)
	checkout := t.TempDir()
	writeHomeModeConfig(t, checkout)
	if _, stderr, code := runCLI(t, "--root", checkout, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	store, err := resolveStore(checkout)
	if err != nil {
		t.Fatal(err)
	}
	// Removing the checkout proves the selection resolves from the registry
	// alone: any read of the target project would now fail.
	if err := os.RemoveAll(checkout); err != nil {
		t.Fatal(err)
	}

	elsewhere := t.TempDir()
	before := relativeTreePaths(t, elsewhere)
	beforeDotAhm := filepath.Join(elsewhere, toolRecordsDirName)

	stdout, stderr, code := runCLIFromDir(t, elsewhere, "--project", store.Key, "task", "create", "Cross-project task")
	if code != 0 {
		t.Fatalf("task create --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create output = %q, want 001", stdout)
	}
	record := filepath.Join(store.recordsDir(), "active", "001.md")
	assertFileContainsAll(t, record, "title: Cross-project task")

	if _, err := os.Stat(beforeDotAhm); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--project created %s in the working directory", beforeDotAhm)
	}
	if got := relativeTreePaths(t, elsewhere); !slices.Equal(got, before) {
		t.Errorf("the working directory changed: %v, want %v", got, before)
	}
	if _, err := os.Stat(checkout); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a records-only write recreated the selected project's checkout: %v", err)
	}
}

// TestProjectSelectorStatusReportsRecordsAndStore proves status reaches the
// selected project's records and reports the store block without a checkout.
func TestProjectSelectorStatusReportsRecordsAndStore(t *testing.T) {
	home := setStoreHome(t)
	store := registerStoreProject(t, home, "github.com/travisennis/ahm", "ahm-0242d8b9", filepath.Join(home, "checkouts", "ahm"))

	// Create the record through the command so its generated indexes exist and
	// status validates a consistent store.
	elsewhere := t.TempDir()
	if _, stderr, code := runCLIFromDir(t, elsewhere, "--project", "ahm", "task", "create", "Recorded task", "--status", "Pending"); code != 0 {
		t.Fatalf("task create --project: %s", stderr)
	}

	stdout, stderr, code := runCLIFromDir(t, elsewhere, "--project", "ahm", "--json", "status")
	if code != 0 {
		t.Fatalf("status --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	var report struct {
		Root      string            `json:"root"`
		Installed bool              `json:"installed"`
		Tasks     map[string]int    `json:"tasks"`
		Store     map[string]string `json:"store"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal status %q: %v", stdout, err)
	}
	if !report.Installed {
		t.Errorf("installed = false, want true for a selected store project")
	}
	if report.Tasks["Pending"] != 1 {
		t.Errorf("tasks = %v, want one Pending task from the store", report.Tasks)
	}
	want := map[string]string{
		"root":     home,
		"key":      "github.com/travisennis/ahm",
		"kind":     storeKindRemote,
		"location": string(locationHome),
	}
	if len(report.Store) != len(want) {
		t.Fatalf("store = %v, want %v", report.Store, want)
	}
	for key, value := range want {
		if report.Store[key] != value {
			t.Errorf("store[%q] = %q, want %q", key, report.Store[key], value)
		}
	}
	if _, err := os.Stat(filepath.Join(store.recordsDir(), "active", "001.md")); err != nil {
		t.Errorf("record is not in the selected store project: %v", err)
	}
}

func TestProjectSelectorUsageErrors(t *testing.T) {
	selectorFixture(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "mutually exclusive with root",
			args: []string{"--project", "github.com/travisennis/ahm", "--root", "/tmp", "status"},
			want: "mutually exclusive",
		},
		{
			name: "unknown selector",
			args: []string{"--project", "nowhere", "status"},
			want: "known projects:",
		},
		{
			name: "ambiguous selector",
			args: []string{"--project", "ahm", "status"},
			want: "matches multiple projects",
		},
		{
			name: "index needs a checkout",
			args: []string{"--project", "github.com/travisennis/ahm", "index"},
			want: "needs a checkout",
		},
		{
			name: "init needs a checkout",
			args: []string{"--project", "github.com/travisennis/ahm", "init"},
			want: "needs a checkout",
		},
		{
			name: "adr needs a checkout",
			args: []string{"--project", "github.com/travisennis/ahm", "adr", "list"},
			want: "needs a checkout",
		},
		{
			name: "store migrate needs a checkout",
			args: []string{"--project", "github.com/travisennis/ahm", "store", "migrate", "--to", "home"},
			want: "needs a checkout",
		},
		{
			name: "links scope needs a checkout",
			args: []string{"--project", "github.com/travisennis/ahm", "--check", "links", "status"},
			want: "--check links requires a checkout",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, code := runCLIFromDir(t, t.TempDir(), tc.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stderr=%q", code, stderr)
			}
			assertContainsAll(t, stderr, tc.want)
		})
	}
}

// TestProjectSelectorReportsAMissingStoreDirectory documents the consequence of
// selecting a registered project whose records still live in the project: the
// store has no records directory for it, and ahm reports that instead of reading
// the project.
func TestProjectSelectorReportsAMissingStoreDirectory(t *testing.T) {
	setStoreHome(t)
	root := projectRoot(t) // project mode: records live in the project
	if _, stderr, code := runCLI(t, "--root", root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "--project", store.Key, "--json", "status")
	if code != 1 {
		t.Fatalf("status --project: stdout=%q stderr=%q code=%d, want exit 1 for a missing store directory", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "store_dir_unreadable")
}

// TestProjectSelectorPrimeDoctorAndStorePath locks the other commands that
// accept --project: they report the selected project from the store alone.
func TestProjectSelectorPrimeDoctorAndStorePath(t *testing.T) {
	home := setStoreHome(t)
	store := registerStoreProject(t, home, "github.com/travisennis/ahm", "ahm-0242d8b9", "")
	elsewhere := t.TempDir()

	if _, stderr, code := runCLIFromDir(t, elsewhere, "--project", "ahm", "task", "create", "Prime task", "--status", "Pending"); code != 0 {
		t.Fatalf("task create --project: %s", stderr)
	}

	stdout, stderr, code := runCLIFromDir(t, elsewhere, "--project", "ahm", "prime")
	if code != 0 {
		t.Fatalf("prime --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "workflow: installed", "store:", "  key: github.com/travisennis/ahm", "Prime task")

	stdout, stderr, code = runCLIFromDir(t, elsewhere, "--project", "ahm", "--json", "doctor")
	if code != 0 {
		t.Fatalf("doctor --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, `"workflow_installed": true`)

	stdout, stderr, code = runCLIFromDir(t, elsewhere, "--project", "ahm", "store", "path")
	if code != 0 {
		t.Fatalf("store path --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "key:", "github.com/travisennis/ahm", abbreviateHome(store.recordsDir()))

	// No subcommand runs status, which also accepts --project.
	stdout, stderr, code = runCLIFromDir(t, elsewhere, "--project", "ahm")
	if code != 0 {
		t.Fatalf("bare --project: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "installed: true", "  key: github.com/travisennis/ahm")
}

// TestProjectSelectorDoesNotApplyStrictAcceptance pins the documented weakening:
// strict_acceptance lives in the committed configuration, so a records-only
// completion cannot read it and does not apply it.
func TestProjectSelectorDoesNotApplyStrictAcceptance(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	writeMetadataFile(t, root, meta)
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	elsewhere := t.TempDir()
	if _, stderr, code := runCLIFromDir(t, elsewhere, "--project", store.Key, "task", "create", "Needs Acceptance"); code != 0 {
		t.Fatalf("task create --project: %s", stderr)
	}
	stdout, stderr, code := runCLIFromDir(t, elsewhere, "--project", store.Key, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("task complete --project: stdout=%q stderr=%q code=%d; strict acceptance must not apply without a checkout", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
}

// TestProjectSelectorReadsNoGit proves the selection never derives identity from
// the recorded checkout: the entry's key disagrees with the checkout's origin
// remote, and the record lands under the registered key's directory.
func TestProjectSelectorReadsNoGit(t *testing.T) {
	home := setStoreHome(t)
	checkout := newGitRepo(t)
	git(t, checkout, "remote", "add", "origin", "https://github.com/other/project.git")
	registerStoreProject(t, home, "github.com/travisennis/ahm", "ahm-0242d8b9", checkout)

	if _, stderr, code := runCLIFromDir(t, t.TempDir(), "--project", "github.com/travisennis/ahm", "task", "create", "No git read"); code != 0 {
		t.Fatalf("task create --project: %s", stderr)
	}

	entries, err := os.ReadDir(filepath.Join(home, storeProjectsDirName))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		dirs = append(dirs, entry.Name())
	}
	if !slices.Equal(dirs, []string{"ahm-0242d8b9"}) {
		t.Errorf("store projects = %v, want only the registered directory; a directory derived from the checkout's remote means Git was read", dirs)
	}
}

// TestProjectSelectorLeavesCheckoutCommandsWorking keeps --root behavior
// unchanged: the same command without --project still resolves a checkout.
func TestProjectSelectorLeavesCheckoutCommandsWorking(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "index"); code != 0 {
		t.Fatalf("index: %s", stderr)
	}
}
