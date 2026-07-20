package ahm

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func runCLIFromDir(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	if chErr := os.Chdir(origDir); chErr != nil {
		t.Errorf("failed to restore working directory: %v", chErr)
	}
	return stdout.String(), stderr.String(), code
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, item := range unwanted {
		if strings.Contains(got, item) {
			t.Errorf("output unexpectedly contains %q:\n%s", item, got)
		}
	}
}

func assertFileContainsAll(t *testing.T, path string, wants ...string) {
	t.Helper()
	assertContainsAll(t, mustRead(t, path), wants...)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupAhmRepo creates minimal .ahm/ workflow state (the modern layout)
// in root. Creates .ahm/config.json and the full directory structure.
func setupAhmRepo(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{
		".ahm/tasks/active",
		".ahm/tasks/completed",
		".ahm/tasks/cancelled",
		".ahm/research/inbox",
		".ahm/research/investigations",
		".ahm/research/sources",
		".ahm/research/topics",
		".ahm/research/archived",
		".ahm/exec-plans/active",
		".ahm/exec-plans/completed",
		"docs/adr",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".ahm", "config.json"), []byte(`{"version":"0.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initAndCreateLegacyMetadata creates minimal legacy .agents/ workflow state
// so tests expecting the legacy .agents/ layout can function.
func initAndCreateLegacyMetadata(t *testing.T, root string) {
	t.Helper()
	metaDir := filepath.Join(root, ".agents")
	for _, dir := range []string{
		".agents/.tasks",
		".agents/.tasks/active",
		".agents/.tasks/completed",
		".agents/.tasks/cancelled",
		".agents/.research",
		".agents/.research/inbox",
		".agents/.research/investigations",
		".agents/.research/sources",
		".agents/.research/topics",
		".agents/.research/archived",
		".agents/exec-plans",
		".agents/exec-plans/active",
		".agents/exec-plans/completed",
		"docs/adr",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(metaDir, "ahm.json"), []byte(`{"version":"0.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func templateBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fs.ReadFile(templates.FS, path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeTaskFile(t *testing.T, path string, id string, title string, status string, extraFrontMatter string) {
	t.Helper()
	writeTaskFileWithPriority(t, path, id, title, status, "P2", extraFrontMatter)
}

func writeTaskFileWithPriority(t *testing.T, path string, id string, title string, status string, priority string, extraFrontMatter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: " + status + "\n" +
		"priority: " + priority + "\n" +
		"effort: S\n" +
		"labels: type:task\n" +
		"exec_plan: -\n" +
		extraFrontMatter +
		"---\n" +
		"# " + title + "\n\n" +
		"## Summary\n\nTODO.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
