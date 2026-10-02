package ahm

import (
	"os/exec"

	"github.com/travisennis/ahm/internal/version"
)

func (a *app) status() error {
	paths := a.workflowPaths()
	validation, tasks := a.validateWorkflow(a.opts.check)
	// A records-only selection has no project metadata to read; selecting the
	// project from the registry is itself the evidence it is installed. The
	// effective strict_acceptance stays unknown there, because the committed
	// configuration it lives in is deliberately not read. The records mode is
	// the layout this command resolved, so it is known whenever the project is
	// installed.
	installed := true
	var strictAcceptance any
	var recordsMode any
	if !paths.isRecordsOnly() {
		meta, metaErr := a.readMetadataFor(a.opts.root)
		installed = metaErr == nil
		if installed {
			strictAcceptance = meta.StrictAcceptance
		}
	}
	if installed {
		recordsMode = string(paths.mode)
	}
	var installedVersion any
	if installed {
		installedVersion = version.Binary
	}
	status := map[string]any{
		"root":              a.opts.root,
		"installed":         installed,
		"installed_version": installedVersion,
		"records_mode":      recordsMode,
		"strict_acceptance": strictAcceptance,
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
	// See status: strict_acceptance is unknown when the committed
	// configuration was not read, while the resolved records mode is known
	// whenever the project is installed.
	installed := true
	var strictAcceptance any
	var recordsMode any
	if !paths.isRecordsOnly() {
		meta, metaErr := a.readMetadataFor(a.opts.root)
		installed = metaErr == nil
		if installed {
			strictAcceptance = meta.StrictAcceptance
		}
	}
	if installed {
		recordsMode = string(paths.mode)
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
		"records_mode":       recordsMode,
		"strict_acceptance":  strictAcceptance,
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
