package ahm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/travisennis/ahm/internal/version"
)

func (a *app) status() error {
	validation, tasks := a.validateWorkflow(a.opts.check)
	_, metaErr := readMetadata(a.opts.root)
	var installedVersion any
	if metaErr == nil {
		installedVersion = version.Binary
	}
	status := map[string]any{
		"root":              a.opts.root,
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
	_, gitErr := exec.LookPath("git")
	_, metaErr := readMetadata(a.opts.root)
	validation, _ := a.validateWorkflow(a.opts.check)
	addOnboardDoctorFinding(a.opts.root, &validation)
	var installedVersion any
	if metaErr == nil {
		installedVersion = version.Binary
	}
	report := map[string]any{
		"root":               a.opts.root,
		"git_available":      gitErr == nil,
		"workflow_installed": metaErr == nil,
		"installed_version":  installedVersion,
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
