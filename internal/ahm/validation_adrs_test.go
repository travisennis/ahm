package ahm

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateADRsReportsFindings(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Good Decision\n\nBody.\n")
	writeADRFile(t, root, "002-invalid-status.md", "---\nstatus: doing\ndate: 2026-06-02\n---\n# Invalid Status\n\nBody.\n")
	writeADRFile(t, root, "003-missing-replacement.md", "---\nstatus: superseded by ADR-999\ndate: 2026-06-03\n---\n# Missing Replacement\n\nBody.\n")
	writeADRFile(t, root, "004-legacy-decision.md", "# ADR 004: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-04\n\n## Context\n\nBody.\n")
	writeADRFile(t, root, "005-broken-front-matter.md", "---\nstatus: accepted\n# Missing close\n")
	writeADRFile(t, root, "006-id-mismatch.md", "---\nid: 007\nstatus: accepted\ndate: 2026-06-06\n---\n# ID Mismatch\n\nBody.\n")
	writeADRFile(t, root, "008-duplicate-a.md", "---\nstatus: accepted\ndate: 2026-06-08\n---\n# Duplicate A\n\nBody.\n")
	writeADRFile(t, root, "008-duplicate-b.md", "---\nstatus: accepted\ndate: 2026-06-08\n---\n# Duplicate B\n\nBody.\n")

	report := validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}}
	validateADRs(root, &report)

	for _, code := range []string{
		"adr_invalid_status",
		"adr_supersede_missing",
		"adr_malformed",
		"adr_id_mismatch",
		"adr_duplicate_id",
	} {
		if !hasFinding(report.Errors, code) {
			t.Errorf("missing ADR error %q: %#v", code, report.Errors)
		}
	}
	// Verify duplicate ID error has an empty path (no single file blamed).
	for _, f := range report.Errors {
		if f.Code == "adr_duplicate_id" && f.Path != "" {
			t.Errorf("adr_duplicate_id should have empty path, got %q", f.Path)
		}
	}
	if !hasFinding(report.Warnings, "adr_legacy_format") {
		t.Errorf("missing adr_legacy_format warning: %#v", report.Warnings)
	}
}

func TestStatusAndDoctorReportLegacyADRsWithoutFailing(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-legacy-decision.md", "# ADR 001: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nBody.\n")

	var statusOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &statusOut}
	if err := a.status(); err != nil {
		t.Errorf("status should not fail for legacy ADR warning: %v", err)
	}
	assertContainsAll(t, statusOut.String(),
		`"ok": true`,
		`"code": "adr_legacy_format"`,
		`convert it to MADR front matter manually`,
	)

	var doctorOut strings.Builder
	a2 := app{opts: options{root: root, json: true}, out: &doctorOut}
	if err := a2.doctor(); err != nil {
		t.Errorf("doctor should not fail for legacy ADR warning: %v", err)
	}
	assertContainsAll(t, doctorOut.String(),
		`"ok": true`,
		`"code": "adr_legacy_format"`,
	)
}

func TestStatusReportsADRErrors(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-invalid-status.md", "---\nstatus: doing\ndate: 2026-06-01\n---\n# Invalid Status\n\nBody.\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	assertContainsAll(t, out.String(),
		`"ok": false`,
		`"code": "adr_invalid_status"`,
		`unsupported ADR status \"doing\"`,
	)
}
