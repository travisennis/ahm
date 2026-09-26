package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// finalV1Release is the last release that reads the legacy .agents/ahm.json
// layout and can migrate a repository off it. Root detection refuses such a
// repository instead of treating it as unmanaged, so a legacy tree is never
// half-adopted by an init that would leave its records behind.
//
// v1.0.0 is not that release: it predates `ahm records migrate`, so it cannot
// perform the move. v1.1.0 is the tag on the last master commit that still
// carries both the legacy reader and the migration.
const finalV1Release = "v1.1.0"

// legacyLayoutError reports a repository that still holds the retired
// .agents/ahm.json workflow layout.
type legacyLayoutError struct {
	root string
}

func (e legacyLayoutError) Error() string {
	return fmt.Sprintf(
		"legacy ahm workflow layout %s: this version reads only %s; upgrade the repository with the final v1 release (ahm %s) before using this version",
		filepath.Join(e.root, filepath.FromSlash(legacyMetadataRelPath)),
		configMetadataRelPath,
		finalV1Release,
	)
}

func (a *app) detectRoot() error {
	if a.opts.root == "" {
		root, err := detectManagedRoot()
		if err != nil {
			return err
		}
		a.opts.root = root
	} else if err := rejectLegacyLayout(a.opts.root); err != nil {
		return err
	}
	// Resolve the records layout with the root, so a command that needs the
	// store fails before it touches anything.
	_, err := a.resolveWorkflowPaths()
	return err
}

// detectRootOrCWD is the lenient detection used by init: an unmanaged
// directory is initialized in place. A legacy layout is an error rather than a
// fallback, because initializing in place would leave its records behind.
func (a *app) detectRootOrCWD() error {
	if a.opts.root == "" {
		root, err := detectManagedRoot()
		if err != nil {
			var legacy legacyLayoutError
			if errors.As(err, &legacy) {
				return err
			}
			root, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		a.opts.root = root
	} else if err := rejectLegacyLayout(a.opts.root); err != nil {
		return err
	}
	_, err := a.resolveWorkflowPaths()
	return err
}

func detectManagedRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if err := rejectLegacyLayout(dir); err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if stat, err := os.Stat(filepath.Join(dir, ".ahm", "config.json")); err == nil && !stat.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a managed repository (no .git or .ahm/config.json found); use --root to specify a directory or run 'ahm init' to create a workflow")
		}
		dir = parent
	}
}

// rejectLegacyLayout fails when root holds the retired `.agents/ahm.json`
// metadata file and not the current `.ahm/config.json`.
//
// `.ahm/config.json` wins when both are present: the v1 records migration wrote
// it after moving the records and removed the legacy file last, so a repository
// that holds both has finished its move and only has stale metadata left. Every
// root-resolution entry point calls this, including the explicit `--root` path,
// so no command reads or writes a legacy tree as if it were managed.
func rejectLegacyLayout(root string) error {
	if stat, err := os.Stat(filepath.Join(root, filepath.FromSlash(configMetadataRelPath))); err == nil && !stat.IsDir() {
		return nil
	}
	stat, err := os.Stat(filepath.Join(root, filepath.FromSlash(legacyMetadataRelPath)))
	if err == nil && !stat.IsDir() {
		return legacyLayoutError{root: root}
	}
	return nil
}
