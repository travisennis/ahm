package ahm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// storeListFixture writes a two-entry store registry with fixed timestamps, one
// live record, one existing and one stale recorded path, and one entry whose
// store directory is gone. It returns the store home.
func storeListFixture(t *testing.T) string {
	t.Helper()
	home := setStoreHome(t)

	liveDir := filepath.Join(home, storeProjectsDirName, "one-0000aaaa")
	writeFile(t, filepath.Join(liveDir, storeRecordsDirName, "active", "001.md"), "record\n")
	liveCheckout := filepath.Join(home, "checkouts", "one")
	if err := os.MkdirAll(liveCheckout, 0o755); err != nil {
		t.Fatal(err)
	}

	writeRegistryFile(t, home, registry{
		Version: storeFormatVersion,
		Projects: map[string]projectEntry{
			"example.com/one": {
				Key:          "example.com/one",
				Kind:         storeKindRemote,
				Dir:          "one-0000aaaa",
				Remotes:      []string{"https://github.com/example/one.git"},
				Paths:        []string{liveCheckout, filepath.Join(home, "stale", "one")},
				Created:      "2026-09-26T10:39:07-04:00",
				MigratedFrom: "project",
			},
			"example.com/two": {
				Key:     "example.com/two",
				Kind:    storeKindPath,
				Dir:     "two-1111bbbb",
				Created: "2026-10-01T09:00:00-04:00",
			},
		},
	})
	return home
}

// TestStoreListReportsEveryEntry covers the text listing: key, kind, store
// directory, creation time, migration source, record count, and the
// exists/missing mark that distinguishes a live path from a stale one. It also
// proves the listing works outside any project, which is the case that surfaced
// the phantom-entry defect.
func TestStoreListReportsEveryEntry(t *testing.T) {
	home := storeListFixture(t)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"root:     "+abbreviateHome(home),
		"projects: 2",
		"example.com/one (remote)",
		"example.com/two (path)",
		"dir:           "+abbreviateHome(filepath.Join(home, storeProjectsDirName, "one-0000aaaa")),
		"created:       2026-09-26T10:39:07-04:00",
		"migrated_from: project",
		"records:       1",
		"remote:        https://github.com/example/one.git",
		"(exists)",
		"(missing)",
	)
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName, "two-1111bbbb")); !os.IsNotExist(err) {
		t.Errorf("the missing store directory was created: %v", err)
	}
}

// TestStoreListMarksAMissingStoreDirectory pins the other half of the stale
// signal: an entry whose store directory is gone is marked, and its record count
// is zero rather than an error.
func TestStoreListMarksAMissingStoreDirectory(t *testing.T) {
	home := storeListFixture(t)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"example.com/two (path)",
		abbreviateHome(filepath.Join(home, storeProjectsDirName, "two-1111bbbb"))+" (missing)",
	)
}

// TestStoreListPlainGolden pins the exact compact JSON a script reads, field by
// field and in order, so a rename or an output-shape change fails here.
func TestStoreListPlainGolden(t *testing.T) {
	home := storeListFixture(t)

	oneDir := filepath.Join(home, storeProjectsDirName, "one-0000aaaa")
	twoDir := filepath.Join(home, storeProjectsDirName, "two-1111bbbb")
	want := fmt.Sprintf(
		`{"root":%q,"entries":[`+
			`{"key":"example.com/one","kind":"remote","dir":%q,"dir_exists":true,`+
			`"remotes":["https://github.com/example/one.git"],`+
			`"paths":[{"path":%q,"exists":true},{"path":%q,"exists":false}],`+
			`"created":"2026-09-26T10:39:07-04:00","migrated_from":"project","records":1},`+
			`{"key":"example.com/two","kind":"path","dir":%q,"dir_exists":false,`+
			`"paths":[],"created":"2026-10-01T09:00:00-04:00","records":0}`+
			`]}`+"\n",
		home,
		oneDir,
		filepath.Join(home, "checkouts", "one"),
		filepath.Join(home, "stale", "one"),
		twoDir,
	)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "--plain", "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	if stdout != want {
		t.Errorf("--plain output:\n got: %s\nwant: %s", stdout, want)
	}
}

// TestStoreListJSONMatchesThePlainPayload pins --json to the same fields the
// plain payload carries.
func TestStoreListJSONMatchesThePlainPayload(t *testing.T) {
	storeListFixture(t)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "--json", "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	var report storeListReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal %q: %v", stdout, err)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("entries = %d, want 2:\n%s", len(report.Entries), stdout)
	}
	one := report.Entries[0]
	if one.Key != "example.com/one" || one.Kind != storeKindRemote || !one.DirExists || one.Records != 1 {
		t.Errorf("entry one = %+v", one)
	}
	if len(one.Paths) != 2 || !one.Paths[0].Exists || one.Paths[1].Exists {
		t.Errorf("entry one paths = %+v", one.Paths)
	}
	two := report.Entries[1]
	if two.Key != "example.com/two" || two.DirExists || two.Records != 0 || len(two.Paths) != 0 {
		t.Errorf("entry two = %+v", two)
	}
}

// TestStoreListJSONGolden pins the exact indented JSON a script reads, including
// key order and the omitempty behavior on an entry with no migration source and
// no recorded path.
func TestStoreListJSONGolden(t *testing.T) {
	home := storeListFixture(t)

	oneDir := filepath.Join(home, storeProjectsDirName, "one-0000aaaa")
	twoDir := filepath.Join(home, storeProjectsDirName, "two-1111bbbb")
	want := fmt.Sprintf(`{
  "root": %q,
  "entries": [
    {
      "key": "example.com/one",
      "kind": "remote",
      "dir": %q,
      "dir_exists": true,
      "remotes": [
        "https://github.com/example/one.git"
      ],
      "paths": [
        {
          "path": %q,
          "exists": true
        },
        {
          "path": %q,
          "exists": false
        }
      ],
      "created": "2026-09-26T10:39:07-04:00",
      "migrated_from": "project",
      "records": 1
    },
    {
      "key": "example.com/two",
      "kind": "path",
      "dir": %q,
      "dir_exists": false,
      "paths": [],
      "created": "2026-10-01T09:00:00-04:00",
      "records": 0
    }
  ]
}
`,
		home,
		oneDir,
		filepath.Join(home, "checkouts", "one"),
		filepath.Join(home, "stale", "one"),
		twoDir,
	)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "--json", "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	if stdout != want {
		t.Errorf("--json output:\n got: %s\nwant: %s", stdout, want)
	}
}

// TestStoreListIsReadOnlyAndRootOptional proves the no-write guarantee: run from
// a directory that is not a project, the listing reads the store and leaves it
// byte-identical, creating no state file and no store directory.
func TestStoreListIsReadOnlyAndRootOptional(t *testing.T) {
	home := storeListFixture(t)
	before := snapshotTree(t, home)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "store", "list")
	if code != 0 {
		t.Fatalf("store list outside a project exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "example.com/one")
	assertTreeUnchanged(t, home, before)
}

// TestStoreListEmptyStore covers an absent registry: the listing reports the
// store root and no projects, and creates no store directory.
func TestStoreListEmptyStore(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent-store")
	t.Setenv(storeHomeEnvVar, absent)

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "root:     "+abbreviateHome(absent), "projects: 0")
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Errorf("listing created the store root: %v", err)
	}

	stdout, stderr, code = runCLIFromDir(t, t.TempDir(), "--plain", "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	if want := fmt.Sprintf(`{"root":%q,"entries":[]}`+"\n", absent); stdout != want {
		t.Errorf("empty --plain:\n got: %s\nwant: %s", stdout, want)
	}
}

// TestStoreListRefusesScopedFlags pins the boundary: a store-wide listing rejects
// the single-project selector and the checkout root instead of silently ignoring
// either.
func TestStoreListRefusesScopedFlags(t *testing.T) {
	setStoreHome(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"project selector", []string{"--project", "example.com/one"}, "--project selects one project's records"},
		{"checkout root", []string{"--root", t.TempDir()}, "--root selects a checkout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "store", "list")
			_, stderr, code := runCLIFromDir(t, t.TempDir(), args...)
			if code != 2 {
				t.Fatalf("store list exited %d, want 2\nstderr: %s", code, stderr)
			}
			assertContainsAll(t, stderr, tc.want)
		})
	}
}

// TestStoreListRedactsCredentials extends the `store path` guarantee to the
// listing: a remote spelling a user hand-edited into the registry is redacted
// before it reaches output.
func TestStoreListRedactsCredentials(t *testing.T) {
	home := setStoreHome(t)
	writeRegistryFile(t, home, registry{
		Version: storeFormatVersion,
		Projects: map[string]projectEntry{
			"example.com/one": {
				Key:     "example.com/one",
				Kind:    storeKindRemote,
				Dir:     "one-0000aaaa",
				Remotes: []string{"https://user:secret@github.com/example/one.git"},
				Created: "2026-09-26T10:39:07-04:00",
			},
		},
	})

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "store", "list")
	if code != 0 {
		t.Fatalf("store list exited %d: %s", code, stderr)
	}
	assertNotContains(t, stdout, "secret")
	assertContainsAll(t, stdout, "https://github.com/example/one.git")
}
