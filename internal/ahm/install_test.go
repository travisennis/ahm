package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
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

func TestReadMetadataUsesLegacyPathWhenConfigMissing(t *testing.T) {
	root := t.TempDir()
	if err := writeMetadata(root, metadata{
		Version:          "0.1.0",
		DefaultWorkAgent: "codex",
		Files:            map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}

	meta, source, err := readMetadataWithSource(root)
	if err != nil {
		t.Fatal(err)
	}
	if source != legacyMetadataRelPath {
		t.Fatalf("source = %q, want %q", source, legacyMetadataRelPath)
	}
	if meta.DefaultWorkAgent != "codex" {
		t.Errorf("default agent = %q, want codex", meta.DefaultWorkAgent)
	}
}

func TestReadMetadataPrefersAhmConfig(t *testing.T) {
	root := t.TempDir()
	if err := writeMetadata(root, metadata{
		Version:          "0.1.0",
		DefaultWorkAgent: "cake",
		Files:            map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigMetadata(root, metadata{
		Version:          "0.2.0",
		DefaultWorkAgent: "cursor",
		StoreMode:        "ref",
		RecordsRef:       "refs/ahm/records",
		RecordsRemote:    "origin",
		RecordsLastSync:  "2026-07-06T12:00:00Z",
		Files:            map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}

	meta, source, err := readMetadataWithSource(root)
	if err != nil {
		t.Fatal(err)
	}
	if source != configMetadataRelPath {
		t.Fatalf("source = %q, want %q", source, configMetadataRelPath)
	}
	if meta.Version != "0.2.0" || meta.DefaultWorkAgent != "cursor" {
		t.Errorf("read wrong metadata: version=%q default=%q", meta.Version, meta.DefaultWorkAgent)
	}
	storage := meta.recordsStorage()
	if storage.Mode != recordStoreModeRef || storage.Ref != defaultRecordsRef || storage.Remote != defaultRecordsRemote || storage.LastSync != "2026-07-06T12:00:00Z" {
		t.Errorf("storage = %#v", storage)
	}
}

func TestWriteMetadataUsesAhmConfigWhenPresent(t *testing.T) {
	root := t.TempDir()
	if err := writeMetadata(root, metadata{
		Version:          "0.1.0",
		DefaultWorkAgent: "cake",
		Files:            map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigMetadata(root, metadata{
		Version:          "0.2.0",
		DefaultWorkAgent: "codex",
		Files:            map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.DefaultWorkAgent = "cursor"
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"default_work_agent": "cursor"`)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "ahm.json"), `"default_work_agent": "cake"`)
}

func TestMetadataRoundTripPreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "0.1.0",
  "strict_acceptance": true,
  "store_mode": "ref",
  "records_ref": "refs/ahm/custom",
  "future_object": {
    "enabled": true
  },
  "future_string": "kept",
  "files": {
    ".agents/skills/preflight/SKILL.md": "abc"
  }
}`)

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.DefaultWorkAgent = "codex"
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	got := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertContainsAll(t, got,
		`"future_object": {`,
		`"enabled": true`,
		`"future_string": "kept"`,
		`"default_work_agent": "codex"`,
		`"store_mode": "ref"`,
		`"records_ref": "refs/ahm/custom"`,
	)
}

func TestTaskWorkRoleConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "0.2.0",
  "default_work_agent": "codex",
  "taskWork": {
    "promptFile": ".agents/prompt.md",
    "implementation": {
      "agent": "codex",
      "model": "gpt-5-codex"
    },
    "review": {
      "agent": "claude",
      "model": "sonnet"
    }
  },
  "files": {}
}`)

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if meta.DefaultWorkAgent != "codex" {
		t.Errorf("DefaultWorkAgent = %q, want codex", meta.DefaultWorkAgent)
	}
	if meta.TaskWork == nil {
		t.Fatal("TaskWork is nil")
	}
	if meta.TaskWork.PromptFile != ".agents/prompt.md" {
		t.Errorf("PromptFile = %q, want .agents/prompt.md", meta.TaskWork.PromptFile)
	}
	if meta.TaskWork.Implementation == nil {
		t.Fatal("TaskWork.Implementation is nil")
	}
	if meta.TaskWork.Implementation.Agent != "codex" {
		t.Errorf("Implementation.Agent = %q, want codex", meta.TaskWork.Implementation.Agent)
	}
	if meta.TaskWork.Implementation.Model != "gpt-5-codex" {
		t.Errorf("Implementation.Model = %q, want gpt-5-codex", meta.TaskWork.Implementation.Model)
	}
	if meta.TaskWork.Review == nil {
		t.Fatal("TaskWork.Review is nil")
	}
	if meta.TaskWork.Review.Agent != "claude" {
		t.Errorf("Review.Agent = %q, want claude", meta.TaskWork.Review.Agent)
	}
	if meta.TaskWork.Review.Model != "sonnet" {
		t.Errorf("Review.Model = %q, want sonnet", meta.TaskWork.Review.Model)
	}

	// Round-trip: write back and verify.
	meta.DefaultWorkAgent = "cursor"
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	got := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertContainsAll(t, got,
		`"default_work_agent": "cursor"`,
		`"implementation":`,
		`"agent": "codex"`,
		`"model": "gpt-5-codex"`,
		`"review":`,
		`"agent": "claude"`,
		`"model": "sonnet"`,
		`"promptFile": ".agents/prompt.md"`,
	)
}

func TestTaskWorkRoleConfigRoundTripPreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "0.2.0",
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
  "future_field": "preserved",
  "files": {}
}`)

	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if meta.TaskWork == nil || meta.TaskWork.Implementation == nil {
		t.Fatal("TaskWork.Implementation is nil after read")
	}

	// Write back and verify unknown top-level field is preserved.
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, filepath.Join(root, ".ahm", "config.json"))
	assertContainsAll(t, got,
		`"future_field": "preserved"`,
		`"implementation":`,
		`"agent": "codex"`,
		`"model": "gpt-5-codex"`,
	)
	assertContainsAll(t, got, `"review":`)
}

func TestRecordsStorageDefaults(t *testing.T) {
	meta := metadata{}
	storage := meta.recordsStorage()
	if storage.Mode != recordStoreModeCommitted {
		t.Errorf("mode = %q, want %q", storage.Mode, recordStoreModeCommitted)
	}
	if storage.Ref != defaultRecordsRef {
		t.Errorf("ref = %q, want %q", storage.Ref, defaultRecordsRef)
	}
	if storage.Remote != defaultRecordsRemote {
		t.Errorf("remote = %q, want %q", storage.Remote, defaultRecordsRemote)
	}
}

func TestInstallDryRunPreviewsAllWrites(t *testing.T) {
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.install(false); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"directories:",
		"  .agents/.tasks/active",
		"metadata:",
		"  .agents/ahm.json",
		"indexes:",
		"  .agents/.tasks/active/index.md",
		"  .agents/.tasks/cancelled/index.md",
		"  .agents/.tasks/completed/index.md",
		"  .agents/.tasks/index.md",
		"  .agents/.research/index.md",
		"  .agents/exec-plans/active/index.md",
		"  .agents/exec-plans/completed/index.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
	assertNotContains(t, got, "AGENTS.md", ".agents/TASKS.md", "docs/adr/README.md")
	if _, err := os.Stat(filepath.Join(root, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run wrote .agents directory, err = %v", err)
	}
}

func TestInstallDryRunDoesNotMutateMetadata(t *testing.T) {
	root := t.TempDir()
	oldVersion := "0.0.1"
	initialMeta := metadata{
		Version: oldVersion,
		Files: map[string]string{
			".agents/TASKS.md": "abc123",
			".agents/PLANS.md": "def456",
		},
	}
	if err := writeMetadata(root, initialMeta); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, ".agents", "ahm.json")
	before, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.install(false); err != nil {
		t.Fatal(err)
	}

	// Metadata on disk must be unchanged.
	after, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("metadata was modified by dry-run:\nbefore: %s\nafter:  %s", before, after)
	}

	// Dry-run output must still contain the expected sections.
	got := out.String()
	for _, want := range []string{
		"metadata:",
		"  .agents/ahm.json",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestInstallWritesExpectedScaffoldOutput(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"metadata:",
		"  .agents/ahm.json",
		"indexes:",
		"  .agents/.tasks/index.md",
		"  docs/adr/index.md",
	)
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md", "docs/adr/README.md")

	assertFileContainsAll(t, filepath.Join(root, ".agents", "ahm.json"),
		`"version": "`+templates.Version+`"`,
	)
	assertNotContains(t, mustRead(t, filepath.Join(root, ".agents", "ahm.json")), ".agents/TASKS.md", "docs/adr/README.md")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "index.md"),
		"# Task Index",
		"- Pending: 0",
		"## Next Ready Queue",
		"None.",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".research", "index.md"),
		"# Research Index",
		"This file is generated by `ahm index`.",
		"No inbox research documents yet.",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", "exec-plans", "active", "index.md"),
		"# Active ExecPlans",
		"This file is generated by `ahm index`.",
		"No active ExecPlans yet.",
	)
	for _, target := range []string{
		"AGENTS.md",
		".agents/TASKS.md",
		".agents/PLANS.md",
		".agents/RESEARCH.md",
		".agents/DOCS.md",
		".agents/.tasks/README.md",
		".agents/.research/README.md",
		"docs/adr/README.md",
		".agents/skills",
	} {
		if _, err := os.Stat(filepath.Join(root, target)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s should not be installed, err = %v", target, err)
		}
	}
}

func TestInstallLeavesExistingAgentsEntrypointAlone(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Project Agent Instructions\n\nKeep this.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--force", "init")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
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

func TestUpgradeRemovesOwnedInstructionTemplatesAndPreservesProjectAgents(t *testing.T) {
	root := t.TempDir()
	meta := metadata{
		Version: "0.0.1",
		Files: map[string]string{
			"AGENTS.md":                             hashBytes([]byte("old managed agents\n")),
			".agents/TASKS.md":                      hashBytes([]byte("old managed tasks\n")),
			".agents/PLANS.md":                      hashBytes(templateBytes(t, "workflow/PLANS.md")),
			".agents/RESEARCH.md":                   hashBytes([]byte("locally changed research\n")),
			".agents/DOCS.md":                       hashBytes([]byte("old managed docs\n")),
			".agents/.tasks/README.md":              hashBytes([]byte("old managed tasks readme\n")),
			".agents/.research/README.md":           hashBytes([]byte("old managed research readme\n")),
			".agents/.research/index.md":            hashBytes([]byte("old managed research index\n")),
			".agents/exec-plans/active/index.md":    hashBytes([]byte("old active plan index\n")),
			".agents/exec-plans/completed/index.md": hashBytes([]byte("old completed plan index\n")),
			"docs/adr/README.md":                    hashBytes([]byte("old managed adr\n")),
			".agents/skills/preflight/SKILL.md":     hashBytes([]byte("old managed skill\n")),
		},
	}
	for target := range meta.Files {
		content := "old managed\n"
		switch target {
		case "AGENTS.md":
			content = "old managed agents\n"
		case ".agents/TASKS.md":
			content = "old managed tasks\n"
		case ".agents/PLANS.md":
			content = string(templateBytes(t, "workflow/PLANS.md"))
		case ".agents/RESEARCH.md":
			content = "local edit that should conflict\n"
		case ".agents/DOCS.md":
			content = "old managed docs\n"
		case ".agents/.tasks/README.md":
			content = "old managed tasks readme\n"
		case ".agents/.research/README.md":
			content = "old managed research readme\n"
		case ".agents/.research/index.md":
			content = "old managed research index\n"
		case ".agents/exec-plans/active/index.md":
			content = "locally edited active plan index\n"
		case ".agents/exec-plans/completed/index.md":
			content = "locally edited completed plan index\n"
		case "docs/adr/README.md":
			content = "old managed adr\n"
		case ".agents/skills/preflight/SKILL.md":
			content = "old managed skill\n"
		}
		writeFile(t, filepath.Join(root, target), content)
	}
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		"removed:",
		"  .agents/DOCS.md",
		"  .agents/PLANS.md",
		"  .agents/TASKS.md",
		"  .agents/.tasks/README.md",
		"  .agents/.research/README.md",
		"  docs/adr/README.md",
		"  .agents/skills/preflight/SKILL.md",
		"conflicts:",
		"  .agents/RESEARCH.md",
	)
	assertNotContains(t, got, "  AGENTS.md")

	after, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != templates.Version {
		t.Errorf("metadata version = %q, want %q (version advances despite conflicts)", after.Version, templates.Version)
	}
	for _, target := range []string{
		".agents/TASKS.md",
		".agents/PLANS.md",
		".agents/DOCS.md",
		".agents/.tasks/README.md",
		".agents/.research/README.md",
		".agents/.research/index.md",
		".agents/exec-plans/active/index.md",
		".agents/exec-plans/completed/index.md",
		"docs/adr/README.md",
	} {
		if _, ok := after.Files[target]; ok {
			t.Errorf("%s should not remain in metadata", target)
		}
	}
	for _, target := range []string{
		".agents/TASKS.md",
		".agents/PLANS.md",
		".agents/DOCS.md",
		".agents/.tasks/README.md",
		".agents/.research/README.md",
		"docs/adr/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, target)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s should have been removed, err = %v", target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "preflight", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("obsolete preflight skill should be removed, err = %v", err)
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".research", "index.md"),
		"# Research Index",
		"This file is generated by `ahm index`.",
	)

	var forceOut strings.Builder
	forced := app{opts: options{root: root, force: true}, out: &forceOut}
	if err := forced.install(true); err != nil {
		t.Error(err)
	}
	assertContainsAll(t, forceOut.String(), "removed:", "  .agents/RESEARCH.md")
	assertFileContainsAll(t, filepath.Join(root, "AGENTS.md"), "old managed agents")
	if _, err := os.Stat(filepath.Join(root, ".agents", "RESEARCH.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("forced upgrade should remove modified instruction template, err = %v", err)
	}
	afterForce, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if afterForce.Version != templates.Version {
		t.Errorf("forced metadata version = %q, want %q", afterForce.Version, templates.Version)
	}
}

func TestUpgradeRemovesObsoleteManagedSkill(t *testing.T) {
	root := t.TempDir()
	oldTarget := ".agents/skills/deslop/SKILL.md"
	oldContent := []byte("old managed skill\n")
	meta := metadata{
		Version: "0.2.0",
		Files: map[string]string{
			oldTarget: hashBytes(oldContent),
		},
	}
	writeFile(t, filepath.Join(root, oldTarget), string(oldContent))
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(true); err != nil {
		t.Fatal(err)
	}

	assertContainsAll(t, out.String(),
		"removed:",
		"  .agents/skills/deslop/SKILL.md",
	)
	if _, err := os.Stat(filepath.Join(root, oldTarget)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("obsolete skill file should be removed, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "deslop")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("obsolete skill directory should be removed when empty, err = %v", err)
	}

	after, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Files[oldTarget]; ok {
		t.Error("obsolete skill should not remain in metadata")
	}
}

func TestUpgradePreservesModifiedObsoleteManagedSkillAsConflict(t *testing.T) {
	root := t.TempDir()
	oldTarget := ".agents/skills/deslop/SKILL.md"
	oldContent := []byte("old managed skill\n")
	localContent := "locally modified skill\n"
	meta := metadata{
		Version: "0.2.0",
		Files: map[string]string{
			oldTarget: hashBytes(oldContent),
		},
	}
	writeFile(t, filepath.Join(root, oldTarget), localContent)
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(true); err != nil {
		t.Fatal(err)
	}

	assertContainsAll(t, out.String(),
		"conflicts:",
		"  .agents/skills/deslop/SKILL.md",
	)
	assertFileContainsAll(t, filepath.Join(root, oldTarget), localContent)
	after, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Files[oldTarget] != hashBytes(oldContent) {
		t.Error("modified obsolete skill metadata should be preserved for conflict reporting")
	}
}

func TestUpgradeMigratesFormerManagedSkills(t *testing.T) {
	root := t.TempDir()
	pristine := []string{
		".agents/skills/preflight/SKILL.md",
		".agents/skills/grooming-backlog/SKILL.md",
	}
	modified := ".agents/skills/finding-improvements/SKILL.md"
	meta := metadata{Version: "0.4.6", Files: map[string]string{}}
	for _, target := range append(append([]string{}, pristine...), modified) {
		content := []byte("managed " + target + "\n")
		meta.Files[target] = hashBytes(content)
		writeFile(t, filepath.Join(root, target), string(content))
	}
	writeFile(t, filepath.Join(root, modified), "locally edited audit skill\n")
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	var preview strings.Builder
	dry := app{opts: options{root: root, dryRun: true}, out: &preview}
	if err := dry.install(true); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, preview.String(), "removed:", pristine[0], pristine[1], "conflicts:", modified)
	for _, target := range append(pristine, modified) {
		if _, err := os.Stat(filepath.Join(root, target)); err != nil {
			t.Fatalf("dry-run removed %s: %v", target, err)
		}
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(true); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(), "removed:", pristine[0], pristine[1], "conflicts:", modified)
	for _, target := range pristine {
		if _, err := os.Stat(filepath.Join(root, target)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s should be removed, err=%v", target, err)
		}
	}
	assertFileContainsAll(t, filepath.Join(root, modified), "locally edited")

	var forcedOut strings.Builder
	forced := app{opts: options{root: root, force: true}, out: &forcedOut}
	if err := forced.install(true); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, forcedOut.String(), "removed:", modified)
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty .agents/skills should be removed, err=%v", err)
	}
}

func TestInstallIgnoresUntrackedFormerInstructionFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "TASKS.md"), "# Local project instructions\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.install(false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertNotContains(t, got, ".agents/TASKS.md")
	assertNotContains(t, got, "adopted:")
	assertNotContains(t, got, "conflicts:")
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.Files[".agents/TASKS.md"]; ok {
		t.Error("former instruction file should not be recorded in metadata")
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", "TASKS.md"), "# Local project instructions")
}

func TestMainUpgradeIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "upgrade")
	if code != 0 {
		t.Errorf("upgrade exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"metadata:",
		"  .agents/ahm.json",
		"indexes:",
	)
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")
	assertFileContainsAll(t, filepath.Join(root, ".agents", "ahm.json"), `"version": "`+templates.Version+`"`)
}

func TestInstallFailsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	// Init first to create valid metadata.
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".agents", "ahm.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// init should fail with a clear error about corrupt metadata.
	stdout, stderr, code = runCLI(t, "--root", root, "init")
	if code == 0 {
		t.Errorf("expected init to fail on corrupt metadata, stdout = %s", stdout)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .agents/ahm.json")
}

func TestUpgradeFailsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	// Init first to create valid metadata.
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".agents", "ahm.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// upgrade should fail with a clear error about corrupt metadata.
	stdout, stderr, code = runCLI(t, "--root", root, "upgrade")
	if code == 0 {
		t.Errorf("expected upgrade to fail on corrupt metadata, stdout = %s", stdout)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .agents/ahm.json")
}

func TestInstallSucceedsWithMissingMetadata(t *testing.T) {
	root := t.TempDir()
	// No prior init, no metadata. Should succeed as a fresh install.
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "metadata:", "indexes:")
}
