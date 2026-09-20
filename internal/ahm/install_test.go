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

func TestFreshInitWritesConfigGitignoreAndIndexes(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"created:",
		"  .ahm/.gitignore",
		"  .ahm/config.json",
		"directories:",
		"  .ahm/tasks/active",
		"  docs/adr",
		"indexes:",
		"  .ahm/tasks/index.md",
		"  .ahm/tasks/active/index.md",
		"  docs/adr/index.md",
	)
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")
	assertNotContains(t, stdout, ".ahm/.tasks", ".ahm/.research", ".ahm/exec-plans")

	config := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertContainsAll(t, config, `"strict_acceptance": false`, `"files": {}`)
	assertNotContains(t, config, `"version":`, "taskWork", "projectDocs", "research")
	gitignore := mustRead(t, filepath.Join(root, ".ahm", ".gitignore"))
	if gitignore != string(recordsGitignoreContent()) {
		t.Errorf(".ahm/.gitignore = %q, want the managed content", gitignore)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "index.md"),
		"# Task Index",
		"- Pending: 0",
		"## Next Ready Queue",
		"None.",
	)
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
	root := t.TempDir()
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
		"directories:",
		"  .ahm/tasks/active",
		"  docs/adr",
		"indexes:",
		"  .ahm/tasks/index.md",
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
	assertContainsAll(t, stderr, ".agents/ahm.json", finalV1Release)
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
	assertContainsAll(t, stderr, ".agents/ahm.json", finalV1Release)
	if _, err := os.Stat(filepath.Join(root, ".ahm", "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("init created .ahm/config.json in a legacy repository, err = %v", err)
	}
}
