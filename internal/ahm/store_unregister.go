package ahm

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// `ahm store unregister` removes a project from the store registry, or one
// recorded path from its entry. The registry maps a project key to its store
// directory and only ever grows: `store path` appends the project's path on
// every run and nothing prunes a stale path or a dead entry, so recovery is
// otherwise hand-editing a machine-level registry.json that Git cannot restore.
//
// Removing a path and removing a whole entry are different operations — the
// first trims one observation, the second drops the project's mapping — so they
// are separate modes (`--path`, and the default) rather than one flag that
// conflates them. The registry is derived data, so neither mode loses a record:
// a later `store path` in the same project re-registers it, and the command
// never deletes a store directory or a task or ADR record.

// storeUnregisterReport is the structured result of `ahm store unregister`. It
// names the registry entry the run touched and the recorded paths it removed,
// so the change is auditable. It carries no remote spelling and no credential:
// unregistering edits the registry mapping only.
type storeUnregisterReport struct {
	Key  string `json:"key"`
	Kind string `json:"kind,omitempty"`
	// EntryRemoved reports whether the whole registry entry was removed. When
	// false, the run removed one recorded path and the entry remains.
	EntryRemoved bool `json:"entry_removed"`
	// PathsRemoved lists the recorded paths the run removed. Removing the whole
	// entry lists every path the entry held.
	PathsRemoved []string `json:"paths_removed"`
	// RemainingPaths lists the recorded paths the entry still holds after a
	// single-path removal; it is empty when the entry itself was removed.
	RemainingPaths []string `json:"remaining_paths,omitempty"`
	DryRun         bool     `json:"dry_run,omitempty"`
}

// RenderText prints the removal as the action taken, or, in dry-run mode, the
// action it would take. Paths under the user's home abbreviate to ~, matching
// `store path`; the key and the remaining paths name what did and did not
// change.
func (r storeUnregisterReport) RenderText(w io.Writer) error {
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format+"\n", args...)
		return err
	}
	prefix := ""
	if r.DryRun {
		prefix = "would "
	}
	kind := ""
	if r.Kind != "" {
		kind = " (kind " + r.Kind + ")"
	}

	if r.EntryRemoved {
		if err := write("%sunregister %s%s", prefix, r.Key, kind); err != nil {
			return err
		}
		if len(r.PathsRemoved) == 0 {
			return write("  entry recorded no paths")
		}
		for _, path := range r.PathsRemoved {
			if err := write("  remove path %s", abbreviateHome(path)); err != nil {
				return err
			}
		}
		return nil
	}

	if len(r.PathsRemoved) == 1 {
		if err := write("%sunregister path %s from %s%s", prefix, abbreviateHome(r.PathsRemoved[0]), r.Key, kind); err != nil {
			return err
		}
	}
	if len(r.RemainingPaths) == 0 {
		return write("  entry still records no paths")
	}
	if err := write("  entry still records:"); err != nil {
		return err
	}
	for _, path := range r.RemainingPaths {
		if err := write("    %s", abbreviateHome(path)); err != nil {
			return err
		}
	}
	return nil
}

// storeUnregisterCommand is the `store unregister` subcommand.
func (a *app) storeUnregisterCommand() *cobra.Command {
	var removePath string
	cmd := &cobra.Command{
		Use:   "unregister",
		Short: "Remove this project's registry entry, or one recorded path",
		Long: `Remove this project's entry from the store registry, or one recorded path
from its entry.

The registry maps a project key to its store directory. It only ever grows:
'store path' appends the project's path on every run and nothing prunes a stale
path or a dead entry. This command removes that mapping, so a stale entry or a
path that no longer exists can be cleared without hand-editing registry.json.

By default it removes the current project's whole entry. With --path it removes
one recorded path from the entry and leaves the entry and its other paths alone.
The two are different operations, so they are different modes: removing a path
never drops the project's mapping, and removing the entry never needs the path.

The registry is derived data, so removing an entry loses no records: a later
'store path' in the same project re-registers it. This command never deletes a
task or ADR record, and it never deletes a store directory; a store directory
left behind is a separate cleanup concern.

Unregistering a project that is not registered, or a path the entry does not
record, is an error, not a silent no-op.

With the global --project <selector>, the command unregisters the selected
project instead of this checkout's, resolving it from the registry alone, so a
stale entry whose checkout is gone can still be removed; --root must not be set.

Supports --dry-run, --json, --plain, and --text output.

Examples:
  ahm store unregister
  ahm store unregister --path /tmp/ahm-root-probe
  ahm --project ahm store unregister`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.storeUnregister(removePath)
		},
	}
	cmd.Flags().StringVar(&removePath, "path", "", "Remove only this recorded path from the entry, keeping the entry")
	return cmd
}

// storeUnregister removes the current project's registry entry, or one recorded
// path from it. A real run applies the removal under the store-state lock the
// other store writers hold; --dry-run reports the same removal, takes no lock,
// and writes nothing. Neither mode touches a record or a store directory.
func (a *app) storeUnregister(removePath string) error {
	paths, err := a.storePathsFor()
	if err != nil {
		return err
	}
	if paths.Root == "" {
		return fmt.Errorf("unregistering a store project needs a resolved store location")
	}
	if a.opts.dryRun {
		reg, err := loadRegistry(paths.Root)
		if err != nil {
			return err
		}
		_, report, err := planStoreUnregister(reg, paths.Key, removePath)
		if err != nil {
			return err
		}
		report.DryRun = true
		return a.emit(report)
	}
	report, err := unregisterStoreProject(paths, removePath)
	if err != nil {
		return err
	}
	return a.emit(report)
}

// unregisterStoreProject applies a removal to the store registry under the
// store-state lock, so two concurrent store commands cannot drop each other's
// change. It writes the registry and nothing else.
func unregisterStoreProject(s storePaths, removePath string) (storeUnregisterReport, error) {
	if s.Root == "" {
		return storeUnregisterReport{}, fmt.Errorf("unregistering a store project needs a resolved store location")
	}
	var report storeUnregisterReport
	err := withStoreStateLock(s, func() error {
		reg, err := loadRegistry(s.Root)
		if err != nil {
			return err
		}
		updated, planned, err := planStoreUnregister(reg, s.Key, removePath)
		if err != nil {
			return err
		}
		report = planned
		return writeRegistry(s.Root, updated)
	})
	if err != nil {
		return storeUnregisterReport{}, err
	}
	return report, nil
}

// planStoreUnregister validates a removal against the registry and returns the
// registry the run would write and the report it would print. It reads no disk
// and writes nothing, so a --dry-run and a real run plan the same removal, and
// a caller that finds no entry or no such path returns an error rather than a
// silent success. removePath empty selects whole-entry removal.
func planStoreUnregister(reg registry, key string, removePath string) (registry, storeUnregisterReport, error) {
	entry, ok := reg.Projects[key]
	if !ok {
		return reg, storeUnregisterReport{}, fmt.Errorf("project %s is not registered in the store; nothing to unregister", key)
	}
	report := storeUnregisterReport{Key: key, Kind: entry.Kind}

	if removePath == "" {
		report.EntryRemoved = true
		report.PathsRemoved = append([]string(nil), entry.Paths...)
		delete(reg.Projects, key)
		return reg, report, nil
	}

	index := recordedPathIndex(entry.Paths, removePath)
	if index < 0 {
		return reg, storeUnregisterReport{}, fmt.Errorf("path %s is not recorded for project %s; recorded paths: %s", removePath, key, recordedPathsList(entry.Paths))
	}
	report.PathsRemoved = []string{entry.Paths[index]}
	entry.Paths = append(entry.Paths[:index:index], entry.Paths[index+1:]...)
	report.RemainingPaths = append([]string(nil), entry.Paths...)
	reg.Projects[key] = entry
	return reg, report, nil
}

// recordedPathIndex returns the position of target in paths, comparing cleaned
// spellings so a trailing separator or a redundant "." does not hide a recorded
// path. It returns -1 when no recorded path matches.
func recordedPathIndex(paths []string, target string) int {
	want := filepath.Clean(target)
	for i, path := range paths {
		if filepath.Clean(path) == want {
			return i
		}
	}
	return -1
}

// recordedPathsList renders an entry's recorded paths for an error message.
func recordedPathsList(paths []string) string {
	if len(paths) == 0 {
		return "none"
	}
	return strings.Join(paths, ", ")
}
