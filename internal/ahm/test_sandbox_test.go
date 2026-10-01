package ahm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestResolvedRootGuardFailsOutsideTheTempDir pins the guard the test binary
// installs on resolvedRootHook: a root ahm resolves outside the temporary
// directory fails loudly and names the path that escaped, so a failure says
// which test resolved what. TestMain is what puts the guard in place, so this
// calls it the way ahm does.
func TestResolvedRootGuardFailsOutsideTheTempDir(t *testing.T) {
	outside := filepath.Join(filepath.Dir(filepath.Clean(os.TempDir())), "ahm-outside")

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		assertResolvedRootWithinTempDir(outside)
	}()

	if recovered == nil {
		t.Fatalf("the guard accepted %s, want a failure naming it", outside)
	}
	assertContainsAll(t, fmt.Sprint(recovered), outside)
}

// TestResolvedRootGuardAcceptsATempDir pins the other half: a scratch root
// passes, including one that does not exist yet, which is how a store
// directory reaches ahm before the first write creates it.
func TestResolvedRootGuardAcceptsATempDir(t *testing.T) {
	assertResolvedRootWithinTempDir(t.TempDir())
	assertResolvedRootWithinTempDir(filepath.Join(t.TempDir(), "store"))
}

// TestWithinTempDirResolvesAlternativeSpellings pins the canonicalization the
// guard and the store-root pin need. One temporary directory reaches a test
// under several spellings: t.TempDir's own, the spelling os.Getwd resolves
// (macOS makes /var a symlink to /private/var, and Windows resolves a short
// name like RUNNER~1), and any symlink a test adds. Comparing literal paths
// would call each of those external and fail a test that stayed inside its
// sandbox.
func TestWithinTempDirResolvesAlternativeSpellings(t *testing.T) {
	resolved, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !withinTempDir(resolved) {
		t.Errorf("withinTempDir(%q) = false, want true for the resolved spelling of the temporary directory", resolved)
	}
	if !withinTempDir(filepath.Join(resolved, "nested", "store")) {
		t.Errorf("withinTempDir(%q) = false, want true for a path that does not exist yet", filepath.Join(resolved, "nested", "store"))
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(os.TempDir(), link); err != nil {
		t.Skipf("creating a symlink: %v", err)
	}
	if !withinTempDir(filepath.Join(link, "store")) {
		t.Errorf("withinTempDir(%q) = false, want true for a path that reaches the temporary directory through a symlink", filepath.Join(link, "store"))
	}
}
