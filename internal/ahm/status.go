package ahm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

func (a *app) status() error {
	a.warnProjectDocsScopeDeprecation()
	validation, tasks := validateWorkflowScoped(a.opts.root, a.opts.check)
	meta, metaErr := readMetadata(a.opts.root)
	var installedVersion any
	if metaErr == nil {
		installedVersion = meta.Version
	}
	status := map[string]any{
		"root":              a.opts.root,
		"template_version":  templates.Version,
		"installed":         metaErr == nil,
		"installed_version": installedVersion,
		"tasks":             taskCounts(tasks),
		"validation":        validation,
	}
	if err := a.emit(status); err != nil {
		return err
	}
	a.emitWarnings()
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}

func (a *app) doctor() error {
	a.warnProjectDocsScopeDeprecation()
	_, gitErr := exec.LookPath("git")
	meta, metaErr := readMetadata(a.opts.root)
	validation, _ := validateWorkflowScoped(a.opts.root, a.opts.check)
	addOnboardDoctorFinding(a.opts.root, &validation)
	var installedVersion any
	if metaErr == nil {
		installedVersion = meta.Version
	}
	report := map[string]any{
		"root":               a.opts.root,
		"git_available":      gitErr == nil,
		"workflow_installed": metaErr == nil,
		"installed_version":  installedVersion,
		"template_version":   templates.Version,
		"validation":         validation,
	}
	if err := a.emit(report); err != nil {
		return err
	}
	a.emitWarnings()
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}

func addOnboardDoctorFinding(root string, report *validationReport) {
	path := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(path) // #nosec G304 -- fixed project-root guidance path.
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addWarning("agents_read_failed", "AGENTS.md", "cannot inspect AGENTS.md for the `ahm prime` bootstrap reference")
		}
		return
	}
	if !strings.Contains(string(data), "ahm prime") {
		report.addInfo("agents_prime_missing", "AGENTS.md", "AGENTS.md does not reference `ahm prime`; run `ahm onboard` for the current bootstrap snippet")
	}
}

// warnProjectDocsScopeDeprecation emits a deprecation warning when
// --check project-docs is used on status or doctor.
func (a *app) warnProjectDocsScopeDeprecation() {
	if containsScope(a.opts.check, CheckScopeProjectDocs) {
		a.addWarning("--check project-docs is deprecated; use `ahm docs check`")
	}
}

// docsCheck runs the expanded project-docs validation scope and emits the
// validation report. Exit 0 when clean or warnings-only; exit 1 on errors.
// --strict promotes warnings to errors.
func (a *app) docsCheck() error {
	validation, _ := validateWorkflowScoped(a.opts.root, []string{CheckScopeProjectDocs})

	// Under --strict, warnings become errors for exit-code purposes.
	if a.opts.strict {
		validation.Errors = append(validation.Errors, validation.Warnings...)
		validation.Warnings = nil
		validation.OK = len(validation.Errors) == 0
	}

	if err := a.emit(validation); err != nil {
		return err
	}
	a.emitWarnings()

	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}
