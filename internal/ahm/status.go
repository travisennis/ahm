package ahm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/travisennis/ahm/internal/templates"
)

func (a *app) status() error {
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
	// Add concise stale/unsynced records note when in ref mode
	a.emitRecordsStaleStatus(meta, metaErr)
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
	// Add concise stale/unsynced records note when in ref mode
	a.emitRecordsStaleStatus(meta, metaErr)
	if err := a.emit(report); err != nil {
		return err
	}
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

// emitRecordsStaleStatus adds a concise stale/unsynced records warning when
// workflow records are stored in ref mode and are out of sync. It never
// performs network operations — it is a purely local check.
func (a *app) emitRecordsStaleStatus(meta metadata, metaErr error) {
	if metaErr != nil {
		return
	}
	cfg := meta.recordsStorage()
	if cfg.Mode != recordStoreModeRef {
		return
	}
	ref := cfg.Ref
	if ref == "" {
		ref = defaultRecordsRef
	}
	ctx := context.Background()
	if _, err := resolveGitRef(ctx, a.opts.root, ref); err != nil {
		if errors.Is(err, errGitRefMissing) {
			a.addWarning("records: ref %s is missing; run 'ahm prime' or 'ahm records pull' to restore", ref)
		}
		return
	}
	working, err := recordsWorkingStatus(ctx, a.opts.root, ref)
	if err != nil {
		return
	}
	if !working.Clean {
		n := len(working.Added) + len(working.Modified) + len(working.Deleted)
		a.addWarning("records: %d unsnapshotted change(s); run 'ahm prime' or 'ahm records push'", n)
	}
	if meta.RecordsLastSync == "" {
		a.addWarning("records: never synced to remote; run 'ahm prime' to push")
	}
}
