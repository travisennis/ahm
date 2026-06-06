package ahm

import (
	"os/exec"

	"github.com/travisennis/ahm/internal/templates"
)

func (a *app) status() error {
	validation, tasks := validateWorkflow(a.opts.root)
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
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}

func (a *app) doctor() error {
	_, gitErr := exec.LookPath("git")
	meta, metaErr := readMetadata(a.opts.root)
	validation, _ := validateWorkflow(a.opts.root)
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
	if len(validation.Errors) > 0 {
		return errValidationFailed
	}
	return nil
}
