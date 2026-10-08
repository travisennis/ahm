package ahm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// `ahm store list` reports what the store holds: every registered project with
// its identity, store directory, recorded paths, and record count. The registry
// only ever grows, and nothing prunes a stale path or a dead entry, so a listing
// is the only way to see that a mapping has gone stale or that a store directory
// is empty or gone; `store unregister` (task 276) is the remedy it points at.
//
// It is deliberately the opposite of `store path`, which observes and records
// the current project on every run. `store list` is read-only: it reads the
// registry and each project's record buckets, needs no project root, and writes
// nothing, so it works from any directory and is safe to run where there is no
// checkout at all.

// storeListPath is one recorded project path and whether it still resolves on
// disk. A path that no longer exists is the stale mapping the listing exists to
// surface.
type storeListPath struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// storeListEntry is one registry entry: the project's identity and store
// directory, the observations the registry holds for it, and the record count
// the listing computed from its buckets.
type storeListEntry struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Dir is the store directory that holds the project's records.
	Dir string `json:"dir"`
	// DirExists reports whether that directory is present on disk, so an entry
	// whose store directory is gone is visible rather than implied.
	DirExists bool            `json:"dir_exists"`
	Remotes   []string        `json:"remotes,omitempty"`
	Paths     []storeListPath `json:"paths"`
	Created   string          `json:"created,omitempty"`
	// MigratedFrom names the layout the records came from, when a store
	// migration recorded one.
	MigratedFrom string `json:"migrated_from,omitempty"`
	// Records is the number of task records in the project's store directory.
	Records int `json:"records"`
}

// storeListReport is the structured output of `ahm store list`: the store root
// the listing read and one entry per registered project, ordered by key.
type storeListReport struct {
	Root    string           `json:"root"`
	Entries []storeListEntry `json:"entries"`
}

// RenderText prints the store root, the project count, and one block per
// project. Paths under the user's home abbreviate to ~, and every path that no
// longer exists is marked, so a stale mapping is visible without decoding a
// path. Record counts come from the listing's bucket scan.
func (r storeListReport) RenderText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "root:     %s\n", abbreviateHome(r.Root)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "projects: %d\n", len(r.Entries)); err != nil {
		return err
	}
	for _, entry := range r.Entries {
		if err := entry.renderText(w); err != nil {
			return err
		}
	}
	return nil
}

func (e storeListEntry) renderText(w io.Writer) error {
	field := func(label string, value string) error {
		_, err := fmt.Fprintf(w, "  %-14s %s\n", label+":", value)
		return err
	}
	kind := e.Kind
	if _, err := fmt.Fprintf(w, "\n%s (%s)\n", e.Key, kind); err != nil {
		return err
	}
	dir := abbreviateHome(e.Dir)
	if !e.DirExists {
		dir += " (missing)"
	}
	if err := field("dir", dir); err != nil {
		return err
	}
	if e.Created != "" {
		if err := field("created", e.Created); err != nil {
			return err
		}
	}
	if e.MigratedFrom != "" {
		if err := field("migrated_from", e.MigratedFrom); err != nil {
			return err
		}
	}
	if err := field("records", fmt.Sprintf("%d", e.Records)); err != nil {
		return err
	}
	for _, remote := range e.Remotes {
		if err := field("remote", remote); err != nil {
			return err
		}
	}
	if len(e.Paths) == 0 {
		return field("paths", "none")
	}
	if _, err := fmt.Fprintln(w, "  paths:"); err != nil {
		return err
	}
	for _, path := range e.Paths {
		state := "missing"
		if path.Exists {
			state = "exists"
		}
		if _, err := fmt.Fprintf(w, "    %s (%s)\n", abbreviateHome(path.Path), state); err != nil {
			return err
		}
	}
	return nil
}

// storeListCommand is the `store list` subcommand.
func (a *app) storeListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the projects the store holds",
		Long: `List every project the store holds: each registry entry's key, kind, store
directory, recorded paths, creation time, and migration source, with the record
count in its store directory.

The command is read-only. Unlike 'store path', which records the current project
in the registry, 'store list' reports what is already registered, so it writes
nothing, creates no state, and needs no project: it works from any directory.
A recorded path that no longer exists on disk, and an entry whose store
directory is gone, are marked missing, because a stale mapping is the failure
this command exists to surface.

The store root is ~/.ahm, or AHM_HOME when it names an absolute path; the
listing names the root it read. Record counts are read per entry on every
invocation.

This command reports the whole store from any directory, so the project-scoped
--root and --project flags do not apply to it; use 'store path' to inspect one
project.

Supports --json, --plain, and --text output.

Examples:
  ahm store list
  ahm --json store list`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.storeList()
		},
	}
}

// storeList resolves the store root, reads its registry, and reports every
// entry. It reads no checkout and takes no lock, so it never registers the
// current directory and never writes. The command reports the whole store from
// any directory, so a --project selector (which names one project's records) and
// a --root (which selects a checkout) have no meaning here and are usage errors
// rather than silently ignored.
func (a *app) storeList() error {
	if a.opts.project != "" {
		return usageError("--project selects one project's records; 'store list' reports every project in the store")
	}
	if a.opts.root != "" {
		return usageError("--root selects a checkout; 'store list' reads only the store, so use 'store path' to inspect a project")
	}
	report, err := buildStoreListReport()
	if err != nil {
		return err
	}
	return a.emit(report)
}

// buildStoreListReport reads the store registry and counts each project's
// records. The registry is read once; each project's buckets are read on demand.
func buildStoreListReport() (storeListReport, error) {
	root, err := storeRoot()
	if err != nil {
		return storeListReport{}, err
	}
	reg, err := loadRegistry(root)
	if err != nil {
		return storeListReport{}, err
	}
	report := storeListReport{Root: root, Entries: []storeListEntry{}}
	for _, key := range registryKeys(reg) {
		entry, err := storeListEntryFor(root, key, reg.Projects[key])
		if err != nil {
			return storeListReport{}, err
		}
		report.Entries = append(report.Entries, entry)
	}
	return report, nil
}

// storeListEntryFor builds one listing entry. The registry is the authority for
// the key-to-directory mapping, so an entry's recorded directory is honored and
// validated the way resolveStore validates it; an entry with no directory falls
// back to the name the key derives.
func storeListEntryFor(root string, key string, entry projectEntry) (storeListEntry, error) {
	item := storeListEntry{
		Key:          key,
		Kind:         entry.Kind,
		Created:      entry.Created,
		MigratedFrom: entry.MigratedFrom,
		Remotes:      redactRemotes(entry.Remotes),
		Paths:        []storeListPath{},
	}
	for _, path := range entry.Paths {
		item.Paths = append(item.Paths, storeListPath{Path: path, Exists: pathExists(path)})
	}
	dir := entry.Dir
	if dir == "" {
		dir = storeDirName(key)
	} else if !validStoreDirName(dir) {
		return storeListEntry{}, fmt.Errorf("store registry entry for key %s names an unusable directory %q", key, dir)
	}
	item.Dir = filepath.Join(root, storeProjectsDirName, dir)
	if info, err := os.Stat(item.Dir); err == nil && info.IsDir() {
		item.DirExists = true
	}
	count, err := storeProjectRecordCount(item.Dir)
	if err != nil {
		return storeListEntry{}, err
	}
	item.Records = count
	return item, nil
}

// storeProjectRecordCount counts the task records in one store project
// directory. A missing directory counts zero, which is how an empty or orphaned
// entry becomes visible.
func storeProjectRecordCount(projectDir string) (int, error) {
	files, err := taskFilePathsFor(workflowPathsForSelection("", storePaths{ProjectDir: projectDir}))
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

// redactRemotes returns the remote spellings a listing may print: userinfo is
// stripped the way `store path` strips it before persisting, so re-redacting a
// registry entry that ahm wrote is a no-op and one a user hand-edited cannot
// leak a credential through the listing either.
func redactRemotes(remotes []string) []string {
	var redacted []string
	for _, remote := range remotes {
		if safe := redactRemote(remote); safe != "" {
			redacted = append(redacted, safe)
		}
	}
	return redacted
}

// pathExists reports whether path names an existing file or directory.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
