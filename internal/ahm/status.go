package ahm

import (
	"os/exec"

	"github.com/travisennis/ahm/internal/version"
)

func (a *app) status() error {
	paths := a.workflowPaths()
	validation, tasks := a.validateWorkflow(a.opts.check)
	// A records-only selection has no project metadata to read; selecting the
	// project from the registry is itself the evidence it is installed.
	installed := true
	if !paths.isRecordsOnly() {
		_, metaErr := a.readMetadataFor(a.opts.root)
		installed = metaErr == nil
	}
	var installedVersion any
	if installed {
		installedVersion = version.Binary
	}
	status := map[string]any{
		"root":              a.opts.root,
		"installed":         installed,
		"installed_version": installedVersion,
		"tasks":             taskCounts(tasks),
		"validation":        validation,
	}
	// The store is reported only for an installed project whose records live
	// there: a repository with no configuration is not installed yet, and its
	// store directories appear with the first command that writes into them.
	if installed {
		if storeStatus, ok := paths.recordsStatus(); ok {
			status["store"] = storeStatus
		}
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
	paths := a.workflowPaths()
	_, gitErr := exec.LookPath("git")
	installed := true
	if !paths.isRecordsOnly() {
		_, metaErr := a.readMetadataFor(a.opts.root)
		installed = metaErr == nil
	}
	validation, _ := a.validateWorkflow(a.opts.check)
	var installedVersion any
	if installed {
		installedVersion = version.Binary
	}
	report := map[string]any{
		"root":               a.opts.root,
		"git_available":      gitErr == nil,
		"workflow_installed": installed,
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
