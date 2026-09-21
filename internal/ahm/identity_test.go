package ahm

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCanonicalRemoteKey(t *testing.T) {
	const ahm = "github.com/travisennis/ahm"
	cases := []struct {
		name   string
		remote string
		want   string
		ok     bool
	}{
		{name: "scp-like", remote: "git@github.com:travisennis/ahm.git", want: ahm, ok: true},
		{name: "scp-like without user", remote: "github.com:travisennis/ahm.git", want: ahm, ok: true},
		{name: "scp-like without .git", remote: "git@github.com:travisennis/ahm", want: ahm, ok: true},
		{name: "https URL", remote: "https://github.com/travisennis/ahm.git", want: ahm, ok: true},
		{name: "https URL without .git", remote: "https://github.com/travisennis/ahm", want: ahm, ok: true},
		{name: "trailing slash", remote: "https://github.com/travisennis/ahm/", want: ahm, ok: true},
		{name: "trailing slash after .git", remote: "https://github.com/travisennis/ahm.git/", want: ahm, ok: true},
		{name: "ssh scheme", remote: "ssh://git@github.com/travisennis/ahm.git", want: ahm, ok: true},
		{name: "git scheme", remote: "git://github.com/travisennis/ahm.git", want: ahm, ok: true},
		{name: "uppercase", remote: "https://GitHub.COM/TravisEnnis/AHM.git", want: ahm, ok: true},
		{name: "uppercase .git suffix", remote: "https://github.com/travisennis/ahm.GIT", want: ahm, ok: true},
		{name: "credentials", remote: "https://travis:secret@github.com/travisennis/ahm.git", want: ahm, ok: true},
		{name: "surrounding whitespace", remote: "  git@github.com:travisennis/ahm.git\n", want: ahm, ok: true},
		{name: "scp-like default port", remote: "ssh://git@github.com:22/travisennis/ahm.git", want: ahm, ok: true},
		{name: "https default port", remote: "https://github.com:443/travisennis/ahm.git", want: ahm, ok: true},
		{name: "http default port", remote: "http://github.com:80/travisennis/ahm.git", want: ahm, ok: true},
		{name: "git default port", remote: "git://github.com:9418/travisennis/ahm.git", want: ahm, ok: true},
		{name: "non-default ssh port", remote: "ssh://git@github.com:2222/travisennis/ahm.git", want: "github.com:2222/travisennis/ahm", ok: true},
		{name: "non-default https port", remote: "https://git.example.com:8443/team/repo.git", want: "git.example.com:8443/team/repo", ok: true},
		{name: "nested group path", remote: "git@gitlab.example.com:group/subgroup/repo.git", want: "gitlab.example.com/group/subgroup/repo", ok: true},
		{name: "file URL", remote: "file:///Users/travis/Projects/ahm", ok: false},
		{name: "file URL with host", remote: "file://server/share/ahm.git", ok: false},
		{name: "absolute path", remote: "/Users/travis/Projects/ahm", ok: false},
		{name: "relative path", remote: "../ahm", ok: false},
		{name: "dot path", remote: "./ahm", ok: false},
		{name: "bare relative path", remote: "ahm", ok: false},
		{name: "windows path", remote: `C:\Projects\ahm`, ok: false},
		{name: "windows drive with slashes", remote: "C:/Projects/ahm", ok: false},
		{name: "URL without path", remote: "https://github.com", ok: false},
		{name: "URL without path with slash", remote: "https://github.com/", ok: false},
		{name: "scp-like without path", remote: "git@github.com:", ok: false},
		{name: "empty", remote: "", ok: false},
		{name: "whitespace only", remote: "   ", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := canonicalRemoteKey(tc.remote)
			if ok != tc.ok {
				t.Fatalf("canonicalRemoteKey(%q) ok = %v (key %q), want %v", tc.remote, ok, key, tc.ok)
			}
			if key != tc.want {
				t.Errorf("canonicalRemoteKey(%q) = %q, want %q", tc.remote, key, tc.want)
			}
		})
	}
}

func TestCanonicalRemoteKeyAgreesAcrossSpellings(t *testing.T) {
	spellings := []string{
		"git@github.com:travisennis/ahm.git",
		"ssh://git@github.com/travisennis/ahm.git",
		"https://github.com/travisennis/ahm.git",
		"https://github.com/travisennis/ahm",
		"https://github.com/travisennis/ahm/",
		"https://github.com:443/travisennis/ahm.git",
		"https://TOKEN@github.com/travisennis/ahm.git",
	}
	const want = "github.com/travisennis/ahm"
	for _, remote := range spellings {
		key, ok := canonicalRemoteKey(remote)
		if !ok || key != want {
			t.Errorf("canonicalRemoteKey(%q) = %q, %v; want %q, true", remote, key, ok, want)
		}
	}
}

func TestCanonicalPathKey(t *testing.T) {
	root := t.TempDir()
	key, err := canonicalPathKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(key) {
		t.Fatalf("canonicalPathKey() = %q, want a lowercase hex SHA-256 digest", key)
	}

	for _, spelling := range []string{
		root + string(filepath.Separator),
		filepath.Join(root, "."),
		filepath.Join(root, "sub", ".."),
	} {
		got, err := canonicalPathKey(spelling)
		if err != nil {
			t.Fatalf("canonicalPathKey(%q): %v", spelling, err)
		}
		if got != key {
			t.Errorf("canonicalPathKey(%q) = %q, want %q", spelling, got, key)
		}
	}

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	aliased, err := canonicalPathKey(alias)
	if err != nil {
		t.Fatal(err)
	}
	if aliased != key {
		t.Errorf("canonicalPathKey(%q) = %q, want the key of its target %q", alias, aliased, key)
	}

	other, err := canonicalPathKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if other == key {
		t.Errorf("distinct project roots share the key %q", key)
	}
}

func TestStoreDirName(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z0-9-]+-[0-9a-f]{8}$`)
	keys := []string{
		"github.com/travisennis/ahm",
		"github.com/one/ahm",
		"example.com/two/ahm",
		"github.com/travisennis/some-very-long-repository-name-here",
		"github.com/travisennis/.github",
		"gitlab.example.com:8443/group/repo",
		strings.Repeat("ab", 32),
	}
	seen := map[string]string{}
	for _, key := range keys {
		name := storeDirName(key)
		if !pattern.MatchString(name) {
			t.Errorf("storeDirName(%q) = %q, want <slug>-<hash8>", key, name)
		}
		if previous, ok := seen[name]; ok {
			t.Errorf("storeDirName(%q) and storeDirName(%q) both = %q", previous, key, name)
		}
		seen[name] = key
		if again := storeDirName(key); again != name {
			t.Errorf("storeDirName(%q) is not deterministic: %q then %q", key, name, again)
		}
	}

	if got := storeDirName("github.com/travisennis/ahm"); !strings.HasPrefix(got, "ahm-") {
		t.Errorf("storeDirName() = %q, want the repository name as its slug", got)
	}
	// A path-rule key is a digest with no repository name to slug.
	if got := storeDirName(strings.Repeat("ab", 32)); !strings.HasPrefix(got, "project-") {
		t.Errorf("storeDirName() = %q, want a project- slug for a path key", got)
	}
	// Slugs are bounded so a long repository name cannot make an unwieldy path.
	if got := storeDirName("github.com/travisennis/some-very-long-repository-name-here"); len(got) > storeDirSlugMax+len("-")+8 {
		t.Errorf("storeDirName() = %q, want a slug of at most %d characters", got, storeDirSlugMax)
	}
}

func TestRedactRemote(t *testing.T) {
	cases := []struct {
		remote string
		want   string
	}{
		{remote: "https://travis:secret@github.com/travisennis/ahm.git", want: "https://github.com/travisennis/ahm.git"},
		{remote: "https://token@github.com/travisennis/ahm.git", want: "https://github.com/travisennis/ahm.git"},
		{remote: "https://github.com/travisennis/ahm.git", want: "https://github.com/travisennis/ahm.git"},
		{remote: "git@github.com:travisennis/ahm.git", want: "git@github.com:travisennis/ahm.git"},
		{remote: "/Users/travis/Projects/ahm", want: "/Users/travis/Projects/ahm"},
		{remote: "ssh://git@github.com:2222/o/r.git", want: "ssh://github.com:2222/o/r.git"},
		{remote: "https://GitHub.com/travisennis/ahm.git", want: "https://GitHub.com/travisennis/ahm.git"},
	}
	for _, tc := range cases {
		if got := redactRemote(tc.remote); got != tc.want {
			t.Errorf("redactRemote(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func TestProjectKeyForRemote(t *testing.T) {
	cases := []struct {
		name    string
		remotes [][2]string
		wantKey string
	}{
		{
			name:    "origin",
			remotes: [][2]string{{"origin", "git@github.com:travisennis/ahm.git"}},
			wantKey: "github.com/travisennis/ahm",
		},
		{
			name: "origin wins over other remotes",
			remotes: [][2]string{
				{"upstream", "https://github.com/other/ahm.git"},
				{"origin", "https://github.com/travisennis/ahm.git"},
			},
			wantKey: "github.com/travisennis/ahm",
		},
		{
			name:    "sole remote that is not origin",
			remotes: [][2]string{{"upstream", "https://github.com/travisennis/ahm.git"}},
			wantKey: "github.com/travisennis/ahm",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newGitRepo(t)
			for _, remote := range tc.remotes {
				git(t, root, "remote", "add", remote[0], remote[1])
			}
			key, kind, rawRemote, err := projectKeyFor(root)
			if err != nil {
				t.Fatal(err)
			}
			if kind != storeKindRemote {
				t.Errorf("projectKeyFor() kind = %q, want %q", kind, storeKindRemote)
			}
			if key != tc.wantKey {
				t.Errorf("projectKeyFor() key = %q, want %q", key, tc.wantKey)
			}
			if rawRemote == "" {
				t.Errorf("projectKeyFor() rawRemote is empty, want the remote spelling the key came from")
			}
		})
	}
}

func TestProjectKeyForFallsBackToPath(t *testing.T) {
	cases := []struct {
		name       string
		remotes    [][2]string
		wantRemote string
	}{
		{name: "no remote"},
		{
			name: "several remotes without origin",
			remotes: [][2]string{
				{"one", "https://github.com/one/ahm.git"},
				{"two", "https://github.com/two/ahm.git"},
			},
		},
		{
			name:       "file remote",
			remotes:    [][2]string{{"origin", "file:///tmp/ahm.git"}},
			wantRemote: "file:///tmp/ahm.git",
		},
		{
			name:       "local path remote",
			remotes:    [][2]string{{"origin", "/tmp/ahm.git"}},
			wantRemote: "/tmp/ahm.git",
		},
		{
			name:    "origin names no URL",
			remotes: [][2]string{{"origin", "   "}},
		},
		{
			name:    "sole remote names no URL",
			remotes: [][2]string{{"upstream", "   "}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newGitRepo(t)
			for _, remote := range tc.remotes {
				git(t, root, "remote", "add", remote[0], remote[1])
			}
			key, kind, rawRemote, err := projectKeyFor(root)
			if err != nil {
				t.Fatal(err)
			}
			if kind != storeKindPath {
				t.Errorf("projectKeyFor() kind = %q, want %q", kind, storeKindPath)
			}
			wantKey, err := canonicalPathKey(root)
			if err != nil {
				t.Fatal(err)
			}
			if key != wantKey {
				t.Errorf("projectKeyFor() key = %q, want the path key %q", key, wantKey)
			}
			// A remote that cannot key the project is still an observation the
			// store records, so a later adoption can recognize a change.
			if rawRemote != tc.wantRemote {
				t.Errorf("projectKeyFor() rawRemote = %q, want %q", rawRemote, tc.wantRemote)
			}
		})
	}
}

func TestProjectKeyForManagedRootWithoutGitMetadata(t *testing.T) {
	// A directory managed by .ahm/config.json alone is a project, and it must
	// not inherit a remote from an enclosing repository.
	outer := newGitRepo(t)
	git(t, outer, "remote", "add", "origin", "git@github.com:travisennis/outer.git")
	inner := filepath.Join(outer, "nested")
	writeFile(t, filepath.Join(inner, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "files": {}
}
`)

	key, kind, rawRemote, err := projectKeyFor(inner)
	if err != nil {
		t.Fatal(err)
	}
	if kind != storeKindPath {
		t.Errorf("projectKeyFor() kind = %q, want %q", kind, storeKindPath)
	}
	wantKey, err := canonicalPathKey(inner)
	if err != nil {
		t.Fatal(err)
	}
	if key != wantKey {
		t.Errorf("projectKeyFor() key = %q, want the path key %q", key, wantKey)
	}
	if rawRemote != "" {
		t.Errorf("projectKeyFor() rawRemote = %q, want empty: the nested root has no .git of its own", rawRemote)
	}
}

func TestProjectKeyForErrorsWhenGitCannotReadTheRepository(t *testing.T) {
	cases := []struct {
		name      string
		breakRepo func(t *testing.T, root string)
	}{
		{
			name: "git is not on PATH",
			breakRepo: func(t *testing.T, root string) {
				t.Setenv("PATH", t.TempDir())
			},
		},
		{
			name: "the .git file points at a missing git directory",
			breakRepo: func(t *testing.T, root string) {
				gitDir := filepath.Join(root, ".git")
				if err := os.RemoveAll(gitDir); err != nil {
					t.Fatal(err)
				}
				writeFile(t, gitDir, "gitdir: /nonexistent/ahm-test-git-dir\n")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newGitRepo(t)
			git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")
			tc.breakRepo(t, root)

			key, kind, rawRemote, err := projectKeyFor(root)
			if err == nil {
				t.Fatalf("projectKeyFor() = %q, %q, %q; want an error rather than a different key", key, kind, rawRemote)
			}
		})
	}
}

func TestProjectKeyForClonesOfOneRepository(t *testing.T) {
	const remote = "git@github.com:travisennis/ahm.git"
	first := newGitRepo(t)
	second := newGitRepo(t)
	for _, root := range []string{first, second} {
		git(t, root, "remote", "add", "origin", remote)
	}
	firstKey, firstKind, _, err := projectKeyFor(first)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, secondKind, _, err := projectKeyFor(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey != secondKey || firstKind != secondKind {
		t.Errorf("clones derived %q (%s) and %q (%s), want one key and kind", firstKey, firstKind, secondKey, secondKind)
	}
}

func TestProjectKeyForSymlinkedProjectRoot(t *testing.T) {
	root := newGitRepo(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	rootKey, rootKind, _, err := projectKeyFor(root)
	if err != nil {
		t.Fatal(err)
	}
	aliasKey, aliasKind, _, err := projectKeyFor(alias)
	if err != nil {
		t.Fatal(err)
	}
	if rootKey != aliasKey || rootKind != aliasKind {
		t.Errorf("a symlinked root derived %q (%s), want the target's %q (%s)", aliasKey, aliasKind, rootKey, rootKind)
	}
}
