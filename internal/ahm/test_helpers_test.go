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
