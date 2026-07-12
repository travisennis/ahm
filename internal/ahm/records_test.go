package ahm

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newGitRepo creates a new git repository for testing.
func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("git not available")
		}
		t.Fatal(err)
	}
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Test User")
	git(t, root, "config", "user.email", "test@example.com")
	return root
}

// writeAHMConfig writes a minimal .ahm/config.json so workflow paths resolve
// to the migrated .ahm/ layout.
func writeAHMConfig(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "files": {}
}
`)
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...) // #nosec G204 // tests run git with explicit test arguments in a temp repo.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
