package ahm

import (
	"strings"
	"testing"
)

func TestValidateGeneratedIndexesContinuesAfterPartialADRParseError(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-valid.md", "---\nstatus: accepted\ndate: 2026-07-01\n---\n# Valid ADR\n\nBody.\n")
	writeADRFile(t, root, "002-bad.md", "---\nkey: >\n---\n# Bad\n")

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, workflowPathsFor(root))
	if !hasFinding(report.Errors, "adr_malformed") {
		t.Errorf("missing adr_malformed error: %#v", report.Errors)
	}
	if !hasFinding(report.Warnings, "generated_index_stale") {
		t.Errorf("missing generated_index_stale warning: %#v", report.Warnings)
	}
	if hasFinding(report.Warnings, "generated_index_check_failed") {
		t.Errorf("unexpected generated_index_check_failed warning: %#v", report.Warnings)
	}
}
