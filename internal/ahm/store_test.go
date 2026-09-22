package ahm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithinTempDir(t *testing.T) {
	tmp := filepath.Clean(os.TempDir())
	cases := []struct {
		path string
		want bool
	}{
		{path: tmp, want: true},
		{path: tmp + string(filepath.Separator), want: true},
		{path: filepath.Join(tmp, "ahm-home"), want: true},
		{path: filepath.Join(tmp, "nested", "store"), want: true},
		{path: filepath.Join(filepath.Dir(tmp), "outside"), want: false},
		{path: filepath.Join(string(filepath.Separator), "usr", "local", "ahm"), want: false},
	}
	for _, tc := range cases {
		if got := withinTempDir(tc.path); got != tc.want {
			t.Errorf("withinTempDir(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestStoreRootFromAHMHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(storeHomeEnvVar, home)
	got, err := storeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Fatalf("storeRoot() = %q, want %q", got, home)
	}

	// A trailing separator names the same directory, so the store must not
	// resolve to two roots.
	t.Setenv(storeHomeEnvVar, home+string(filepath.Separator))
	again, err := storeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if again != home {
		t.Errorf("storeRoot() = %q for a spelling with a trailing separator, want %q", again, home)
	}
}

func TestStoreRootRejectsRelativeAHMHome(t *testing.T) {
	for _, value := range []string{"ahm-home", "./ahm-home", "../ahm-home"} {
		t.Setenv(storeHomeEnvVar, value)
		_, err := storeRoot()
		var usage usageError
		if !errors.As(err, &usage) {
			t.Fatalf("storeRoot() error = %v for %q, want a usage error", err, value)
		}
		assertContainsAll(t, err.Error(), storeHomeEnvVar, "absolute")
	}
}

func TestStoreRootDefaultsToUserHome(t *testing.T) {
	t.Setenv(storeHomeEnvVar, "")
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("user home unavailable: %v", err)
	}
	got, err := storeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(userHome, toolRecordsDirName); got != want {
		t.Errorf("storeRoot() = %q, want %q", got, want)
	}
}

func TestResolveStoreUsesRemoteIdentity(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != storeHome {
		t.Errorf("resolveStore() root = %q, want %q", paths.Root, storeHome)
	}
	if paths.Key != "github.com/travisennis/ahm" {
		t.Errorf("resolveStore() key = %q, want the canonical remote key", paths.Key)
	}
	if paths.Kind != storeKindRemote {
		t.Errorf("resolveStore() kind = %q, want %q", paths.Kind, storeKindRemote)
	}
	wantDir := filepath.Join(storeHome, storeProjectsDirName, storeDirName(paths.Key))
	if paths.ProjectDir != wantDir {
		t.Errorf("resolveStore() project dir = %q, want %q", paths.ProjectDir, wantDir)
	}
	if wantRecords := filepath.Join(wantDir, storeRecordsDirName); paths.recordsDir() != wantRecords {
		t.Errorf("recordsDir() = %q, want %q", paths.recordsDir(), wantRecords)
	}

	// Resolution reads: it must not create the store or record anything.
	if _, err := os.Stat(storeHome); err == nil {
		entries, readErr := os.ReadDir(storeHome)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Errorf("resolveStore() wrote %d entries into the store root", len(entries))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestResolveStoreHonorsRegistryDirectory(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	// The registry is the authority for the key-to-directory mapping, so a
	// directory recorded by an earlier naming rule still wins.
	writeRegistryFile(t, storeHome, registry{
		Version: storeFormatVersion,
		Projects: map[string]projectEntry{
			"github.com/travisennis/ahm": {
				Key:     "github.com/travisennis/ahm",
				Kind:    storeKindRemote,
				Dir:     "ahm-legacy",
				Created: "2026-09-21T10:00:00-04:00",
			},
		},
	})

	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(storeHome, storeProjectsDirName, "ahm-legacy"); paths.ProjectDir != want {
		t.Errorf("resolveStore() project dir = %q, want the recorded %q", paths.ProjectDir, want)
	}
}

func TestResolveStoreRegistryFailures(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "corrupt registry",
			content: "{not json",
			want:    "corrupt store registry",
		},
		{
			name:    "newer store version",
			content: `{"version": 99, "projects": {}}`,
			want:    "version 99",
		},
		{
			name:    "directory outside the store",
			content: `{"version": 1, "projects": {"github.com/travisennis/ahm": {"key": "github.com/travisennis/ahm", "kind": "remote", "dir": "../escape", "created": "2026-09-21T10:00:00-04:00"}}}`,
			want:    `"../escape"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			storeHome := setStoreHome(t)
			root := newGitRepo(t)
			git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")
			writeFile(t, filepath.Join(storeHome, storeRegistryFileName), tc.content)

			_, err := resolveStore(root)
			if err == nil {
				t.Fatal("resolveStore() succeeded, want an error")
			}
			assertContainsAll(t, err.Error(), tc.want)
		})
	}
}

func writeRegistryFile(t *testing.T, storeHome string, reg registry) {
	t.Helper()
	data, err := marshalStoreJSON(reg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(storeHome, storeRegistryFileName), string(data))
}

func TestStorePathCommandReportsRemoteIdentity(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	stdout, stderr, code := runCLIFromDir(t, root, "store", "path")
	if code != 0 {
		t.Fatalf("ahm store path exited %d: %s", code, stderr)
	}
	wantRecords := filepath.Join(storeHome, storeProjectsDirName, storeDirName("github.com/travisennis/ahm"), storeRecordsDirName)
	assertContainsAll(t, stdout,
		"root:    "+storeHome,
		"key:     github.com/travisennis/ahm",
		"records: "+wantRecords,
	)

	stdout, stderr, code = runCLIFromDir(t, root, "--json", "store", "path")
	if code != 0 {
		t.Fatalf("ahm --json store path exited %d: %s", code, stderr)
	}
	var report storePathReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal %q: %v", stdout, err)
	}
	want := storePathReport{
		Root:    storeHome,
		Key:     "github.com/travisennis/ahm",
		Kind:    storeKindRemote,
		Records: wantRecords,
	}
	if report != want {
		t.Errorf("ahm --json store path = %+v, want %+v", report, want)
	}
}

func TestStorePathCommandAgreesAcrossClones(t *testing.T) {
	const remote = "git@github.com:travisennis/ahm.git"
	setStoreHome(t)
	first := newGitRepo(t)
	second := newGitRepo(t)
	for _, root := range []string{first, second} {
		git(t, root, "remote", "add", "origin", remote)
	}

	firstOut, firstErr, firstCode := runCLIFromDir(t, first, "store", "path")
	secondOut, secondErr, secondCode := runCLIFromDir(t, second, "store", "path")
	if firstCode != 0 || secondCode != 0 {
		t.Fatalf("ahm store path exited %d and %d: %s%s", firstCode, secondCode, firstErr, secondErr)
	}
	if firstOut != secondOut {
		t.Errorf("two clones reported different store locations:\nclone one:\n%sclone two:\n%s", firstOut, secondOut)
	}
}

func TestStorePathCommandFallsBackToPathKey(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)

	stdout, stderr, code := runCLIFromDir(t, root, "--json", "store", "path")
	if code != 0 {
		t.Fatalf("ahm --json store path exited %d: %s", code, stderr)
	}
	var report storePathReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal %q: %v", stdout, err)
	}
	if report.Kind != storeKindPath {
		t.Errorf("kind = %q, want %q in a repository with no remote", report.Kind, storeKindPath)
	}
	wantKey, err := canonicalPathKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Key != wantKey {
		t.Errorf("key = %q, want the path key %q", report.Key, wantKey)
	}
	if want := filepath.Join(storeHome, storeProjectsDirName, storeDirName(wantKey), storeRecordsDirName); report.Records != want {
		t.Errorf("records = %q, want %q", report.Records, want)
	}
}

// TestRecordingAStoreProjectRefusesAnUnresolvedStore keeps the containment rule
// structural: the registry lives at the store root, so a zero store location
// must fail instead of naming the process's working directory.
func TestRecordingAStoreProjectRefusesAnUnresolvedStore(t *testing.T) {
	for _, store := range []storePaths{
		{},
		{Root: t.TempDir()},
		{ProjectDir: filepath.Join(t.TempDir(), "projects", "repo-3f9ac4d1")},
	} {
		if err := recordStoreProject(store); err == nil {
			t.Errorf("recordStoreProject(%+v) succeeded, want an error", store)
		}
	}
}

func TestStorePathCommandRecordsRegistryWithoutCredentials(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "https://travis:secret@github.com/travisennis/ahm.git")

	_, stderr, code := runCLIFromDir(t, root, "store", "path")
	if code != 0 {
		t.Fatalf("ahm store path exited %d: %s", code, stderr)
	}

	registryData := mustRead(t, filepath.Join(storeHome, storeRegistryFileName))
	if strings.Contains(registryData, "secret") {
		t.Errorf("the registry persisted a credential:\n%s", registryData)
	}
	var reg registry
	if err := json.Unmarshal([]byte(registryData), &reg); err != nil {
		t.Fatalf("unmarshal registry %q: %v", registryData, err)
	}
	if reg.Version != storeFormatVersion {
		t.Errorf("registry version = %d, want %d", reg.Version, storeFormatVersion)
	}
	entry, ok := reg.Projects["github.com/travisennis/ahm"]
	if !ok {
		t.Fatalf("registry has no entry for the project key:\n%s", registryData)
	}
	if entry.Kind != storeKindRemote {
		t.Errorf("entry kind = %q, want %q", entry.Kind, storeKindRemote)
	}
	if entry.Dir != storeDirName("github.com/travisennis/ahm") {
		t.Errorf("entry dir = %q, want %q", entry.Dir, storeDirName("github.com/travisennis/ahm"))
	}
	if len(entry.Remotes) != 1 || entry.Remotes[0] != "https://github.com/travisennis/ahm.git" {
		t.Errorf("entry remotes = %v, want the redacted remote spelling", entry.Remotes)
	}
	resolved, err := resolveProjectPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Paths) != 1 || entry.Paths[0] != resolved {
		t.Errorf("entry paths = %v, want %q", entry.Paths, resolved)
	}
	if _, err := time.Parse(time.RFC3339, entry.Created); err != nil {
		t.Errorf("entry created = %q: %v", entry.Created, err)
	}

	var state projectState
	statePath := filepath.Join(storeHome, storeProjectsDirName, entry.Dir, storeStateFileName)
	if err := json.Unmarshal([]byte(mustRead(t, statePath)), &state); err != nil {
		t.Fatal(err)
	}
	if state.Version != storeFormatVersion {
		t.Errorf("project state version = %d, want %d", state.Version, storeFormatVersion)
	}
}

func TestStorePathCommandIsIdempotent(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("ahm store path exited %d: %s", code, stderr)
	}
	before := snapshotTree(t, storeHome)
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("second ahm store path exited %d: %s", code, stderr)
	}
	assertTreeUnchanged(t, storeHome, before)
}

func TestStorePathCommandDryRunWritesNothing(t *testing.T) {
	storeHome := filepath.Join(t.TempDir(), "store")
	t.Setenv(storeHomeEnvVar, storeHome)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	stdout, stderr, code := runCLIFromDir(t, root, "--dry-run", "store", "path")
	if code != 0 {
		t.Fatalf("ahm --dry-run store path exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "root:    "+storeHome, "key:     github.com/travisennis/ahm")
	if _, err := os.Stat(storeHome); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--dry-run created %s", storeHome)
	}
}

func TestStorePathCommandRejectsRelativeAHMHome(t *testing.T) {
	t.Setenv(storeHomeEnvVar, "relative-store")
	root := newGitRepo(t)

	// The scratch store root the CLI helpers install would replace the value
	// under test, so this command runs with the environment as set.
	_, stderr, code := runCLIFromDirKeepingEnv(t, root, "store", "path")
	if code != 2 {
		t.Fatalf("ahm store path exited %d, want 2 for a relative %s", code, storeHomeEnvVar)
	}
	assertContainsAll(t, stderr, storeHomeEnvVar, "absolute")
}

func TestStorePathCommandRejectsNewerStoreState(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")

	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("ahm store path exited %d: %s", code, stderr)
	}
	statePath := filepath.Join(storeHome, storeProjectsDirName, storeDirName("github.com/travisennis/ahm"), storeStateFileName)
	writeFile(t, statePath, `{"version": 99}`)

	stdout, stderr, code := runCLIFromDir(t, root, "store", "path")
	if code != 1 {
		t.Fatalf("ahm store path exited %d for an unknown store version, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "version 99")
}

func TestStorePathCommandRequiresAManagedRoot(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()

	stdout, stderr, code := runCLIFromDir(t, root, "store", "path")
	if code != 1 {
		t.Fatalf("ahm store path exited %d outside a managed repository, want 1\nstdout:\n%s", code, stdout)
	}
	assertContainsAll(t, stderr, "not in a managed repository")
}

func TestStoreCommandRequiresASubcommand(t *testing.T) {
	root := newGitRepo(t)
	for _, args := range [][]string{{"store"}, {"store", "bogus"}} {
		_, stderr, code := runCLIFromDir(t, root, args...)
		if code != 2 {
			t.Errorf("ahm %s exited %d, want 2\nstderr:\n%s", strings.Join(args, " "), code, stderr)
		}
	}
}

func TestStorePathCommandRejectsUnreadableGitRepository(t *testing.T) {
	setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")
	// A repository Git cannot read must fail instead of quietly resolving a
	// path key, which would point the project at a different store directory.
	gitDir := filepath.Join(root, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, gitDir, "gitdir: /nonexistent/ahm-test-git-dir\n")

	stdout, stderr, code := runCLIFromDir(t, root, "store", "path")
	if code != 1 {
		t.Fatalf("ahm store path exited %d for an unreadable repository, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "reading Git remotes")
}

func TestStorePathCommandRejectsAStoreRootThatIsNotADirectory(t *testing.T) {
	storeHome := filepath.Join(t.TempDir(), "store")
	writeFile(t, storeHome, "not a directory\n")
	t.Setenv(storeHomeEnvVar, storeHome)
	root := newGitRepo(t)

	stdout, stderr, code := runCLIFromDirKeepingEnv(t, root, "store", "path")
	if code != 1 {
		t.Fatalf("ahm store path exited %d for a store root that is a file, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "store root", storeHome, "is not a directory")
}

func TestStorePathCommandRecordsAnUnusableRemote(t *testing.T) {
	storeHome := setStoreHome(t)
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "file:///tmp/ahm-origin.git")

	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("ahm store path exited %d: %s", code, stderr)
	}
	var reg registry
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(storeHome, storeRegistryFileName))), &reg); err != nil {
		t.Fatal(err)
	}
	for key, entry := range reg.Projects {
		if entry.Kind != storeKindPath {
			t.Errorf("entry %s kind = %q, want %q", key, entry.Kind, storeKindPath)
		}
		// The remote that could not key the project is still an observation, so a
		// later adoption can recognize a re-pointed remote.
		if len(entry.Remotes) != 1 || entry.Remotes[0] != "file:///tmp/ahm-origin.git" {
			t.Errorf("entry %s remotes = %v, want the observed remote spelling", key, entry.Remotes)
		}
	}
	if len(reg.Projects) != 1 {
		t.Errorf("registry has %d projects, want 1", len(reg.Projects))
	}
}

func TestStorePathReportAbbreviatesHome(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("user home unavailable: %v", err)
	}
	report := storePathReport{
		Root:    filepath.Join(userHome, ".ahm"),
		Key:     "github.com/travisennis/ahm",
		Kind:    storeKindRemote,
		Records: filepath.Join(userHome, ".ahm", "projects", "ahm-12345678", "tasks"),
	}
	var out strings.Builder
	if err := report.RenderText(&out); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		"root:    ~"+string(filepath.Separator)+".ahm",
		"key:     github.com/travisennis/ahm",
		"records: ~"+filepath.Join(string(filepath.Separator)+".ahm", "projects", "ahm-12345678", "tasks"),
	)
	assertNotContains(t, out.String(), userHome)

	// A store outside the home directory keeps its absolute path.
	elsewhere := filepath.Join(t.TempDir(), "store")
	var other strings.Builder
	otherReport := storePathReport{Root: elsewhere, Key: "k", Kind: storeKindPath, Records: fmt.Sprintf("%s/tasks", elsewhere)}
	if err := otherReport.RenderText(&other); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, other.String(), "root:    "+elsewhere)
}
