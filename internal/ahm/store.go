package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// storeHomeEnvVar overrides the user-level store root. It must name an
// absolute path so that every command resolves one store wherever it runs.
const storeHomeEnvVar = "AHM_HOME"

// storeFormatVersion is the store layout version ahm writes and the highest it
// reads. A store written by a newer ahm is refused rather than half-read.
const storeFormatVersion = 1

const (
	storeProjectsDirName  = "projects"
	storeRegistryFileName = "registry.json"
	storeStateFileName    = "project.json"
	// storeRecordsDirName mirrors the in-project tasks directory, so record
	// and generated-index paths keep their relative shape wherever the records
	// live.
	storeRecordsDirName = "tasks"
)

// storePaths is the resolved user-level store location of one project: the
// store root, the project's derived key and key kind, and the directory that
// holds the project's records.
type storePaths struct {
	Root       string
	Key        string
	Kind       string
	ProjectDir string

	// rawRemote is the remote the repository named even when it could not key
	// the project, and resolvedPath is the symlink-resolved project root.
	// resolveStore records both as observations; rawRemote is empty when no
	// remote applies.
	rawRemote    string
	resolvedPath string
}

func (s storePaths) statePath() string  { return filepath.Join(s.ProjectDir, storeStateFileName) }
func (s storePaths) recordsDir() string { return filepath.Join(s.ProjectDir, storeRecordsDirName) }
func (s storePaths) dirName() string    { return filepath.Base(s.ProjectDir) }

// storeRoot returns the user-level store root: the AHM_HOME value when it is
// set, and ~/.ahm otherwise. A relative AHM_HOME is a usage error, because the
// store must resolve the same way from every working directory, and a root that
// exists but is not a directory is refused instead of failing later on a
// confusing path.
func storeRoot() (string, error) {
	root := ""
	if home := strings.TrimSpace(os.Getenv(storeHomeEnvVar)); home != "" {
		if !filepath.IsAbs(home) {
			return "", usageError(fmt.Sprintf("%s must be an absolute path, got %q", storeHomeEnvVar, home))
		}
		root = filepath.Clean(home)
	} else {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving the home store root: %w", err)
		}
		root = filepath.Join(userHome, toolRecordsDirName)
	}
	if stat, err := os.Stat(root); err == nil && !stat.IsDir() {
		return "", fmt.Errorf("store root %s is not a directory", root)
	}
	return root, nil
}

// resolveStore derives the project's identity and resolves its store location.
// The registry wins over a recomputed directory name, so the mapping from a
// key to its directory survives a change to the naming rule.
//
// It reads Git through the isolated environment — at most two read-only
// calls — and writes nothing.
func resolveStore(projectRoot string) (storePaths, error) {
	root, err := storeRoot()
	if err != nil {
		return storePaths{}, err
	}
	key, kind, rawRemote, err := projectKeyFor(projectRoot)
	if err != nil {
		return storePaths{}, err
	}
	resolvedPath, err := resolveProjectPath(projectRoot)
	if err != nil {
		return storePaths{}, err
	}
	reg, err := loadRegistry(root)
	if err != nil {
		return storePaths{}, err
	}
	dir := storeDirName(key)
	if entry, ok := reg.Projects[key]; ok && entry.Dir != "" {
		if !validStoreDirName(entry.Dir) {
			return storePaths{}, fmt.Errorf("store registry entry for key %s names an unusable directory %q", key, entry.Dir)
		}
		dir = entry.Dir
	}
	return storePaths{
		Root:         root,
		Key:          key,
		Kind:         kind,
		ProjectDir:   filepath.Join(root, storeProjectsDirName, dir),
		rawRemote:    rawRemote,
		resolvedPath: resolvedPath,
	}, nil
}

// validStoreDirName reports whether dir is a single path segment, so a
// hand-edited registry cannot point a project at a directory outside the
// store.
func validStoreDirName(dir string) bool {
	if dir == "" || dir == "." || dir == ".." {
		return false
	}
	if strings.ContainsAny(dir, `/\`) {
		return false
	}
	return filepath.Base(dir) == dir
}

// storePathsFor resolves the store location of the command's project root at
// most once per command. A repository that already resolved its records layout
// has the store in hand, so the store is read once per command even when the
// command reports both.
func (a *app) storePathsFor() (storePaths, error) {
	if a.store != nil {
		return *a.store, nil
	}
	if paths, err := a.resolveWorkflowPaths(); err == nil && paths.inStore() {
		a.store = &paths.store
		return paths.store, nil
	}
	paths, err := resolveStore(a.opts.root)
	if err != nil {
		return storePaths{}, err
	}
	a.store = &paths
	return paths, nil
}

// projectEntry is one project's registry record: its derived identity, the
// directory that holds it, and the observations that make a later adoption of
// a changed key or path possible at all.
type projectEntry struct {
	Key          string   `json:"key"`
	Kind         string   `json:"kind"`
	Dir          string   `json:"dir"`
	Remotes      []string `json:"remotes,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	Created      string   `json:"created"`
	MigratedFrom string   `json:"migrated_from,omitempty"`
}

// registry is <store>/registry.json: the authority for the mapping from a
// project key to its store directory. It is derived data, and it is never the
// authority for record contents.
type registry struct {
	Version  int                     `json:"version"`
	Projects map[string]projectEntry `json:"projects"`
}

// loadRegistry reads the store registry at root. A missing registry is an
// empty one, because the mapping can always be recomputed from the keys.
func loadRegistry(root string) (registry, error) {
	path := filepath.Join(root, storeRegistryFileName)
	data, err := os.ReadFile(path) // #nosec G304 // path is under the resolved store root
	if errors.Is(err, fs.ErrNotExist) {
		return registry{Version: storeFormatVersion, Projects: map[string]projectEntry{}}, nil
	}
	if err != nil {
		return registry{}, fmt.Errorf("reading store registry %s: %w", path, err)
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return registry{}, fmt.Errorf("corrupt store registry %s: %w", path, err)
	}
	if err := checkStoreVersion(path, reg.Version); err != nil {
		return registry{}, err
	}
	if reg.Projects == nil {
		reg.Projects = map[string]projectEntry{}
	}
	return reg, nil
}

// projectState is one project's store state file, <ProjectDir>/project.json:
// the layout version ahm writes, and the persisted state that must survive
// record deletion.
type projectState struct {
	Version int `json:"version"`
}

// readProjectState reads a project's store state file. A missing file reports
// fs.ErrNotExist so the caller can tell "not created yet" from "unreadable".
func readProjectState(s storePaths) (projectState, error) {
	path := s.statePath()
	data, err := os.ReadFile(path) // #nosec G304 // path is under the resolved store root
	if err != nil {
		return projectState{}, err
	}
	var state projectState
	if err := json.Unmarshal(data, &state); err != nil {
		return projectState{}, fmt.Errorf("corrupt store state %s: %w", path, err)
	}
	if err := checkStoreVersion(path, state.Version); err != nil {
		return projectState{}, err
	}
	return state, nil
}

// checkStoreVersion refuses a store file written by a newer ahm instead of
// reading it with rules that may no longer hold.
func checkStoreVersion(path string, version int) error {
	if version > storeFormatVersion {
		return fmt.Errorf("%s is store format version %d and this ahm reads version %d", path, version, storeFormatVersion)
	}
	return nil
}

// recordStoreProject records the project in the store: its registry entry, the
// key kind, the directory, the redacted remote spellings, and the absolute
// paths seen, plus its state file. Credentials never reach the registry, and
// each file is written only when the bytes on disk change, so a repeated
// command leaves the store untouched.
func recordStoreProject(s storePaths) error {
	state, err := readProjectState(s)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	state.Version = storeFormatVersion
	if err := writeProjectState(s, state); err != nil {
		return err
	}

	reg, err := loadRegistry(s.Root)
	if err != nil {
		return err
	}
	entry := reg.Projects[s.Key]
	if entry.Key == "" {
		entry.Key = s.Key
		entry.Kind = s.Kind
		entry.Dir = s.dirName()
		entry.Created = time.Now().Format(time.RFC3339)
	}
	if entry.Kind == "" {
		entry.Kind = s.Kind
	}
	if entry.Dir == "" {
		entry.Dir = s.dirName()
	}
	entry.Remotes = appendUniqueString(entry.Remotes, redactRemote(s.rawRemote))
	entry.Paths = appendUniqueString(entry.Paths, s.resolvedPath)
	reg.Projects[s.Key] = entry
	return writeRegistry(s.Root, reg)
}

// writeProjectState writes a project's state file unless it already holds
// those exact bytes.
func writeProjectState(s storePaths, state projectState) error {
	data, err := marshalStoreJSON(state)
	if err != nil {
		return err
	}
	return writeStoreFile(s.statePath(), data)
}

// writeRegistry writes the store registry unless it already holds those exact
// bytes.
func writeRegistry(root string, reg registry) error {
	reg.Version = storeFormatVersion
	data, err := marshalStoreJSON(reg)
	if err != nil {
		return err
	}
	return writeStoreFile(filepath.Join(root, storeRegistryFileName), data)
}

// marshalStoreJSON renders store state as the exact bytes ahm writes.
func marshalStoreJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writeStoreFile writes store state atomically unless the file already holds
// those exact bytes, so repeated commands write nothing.
func writeStoreFile(path string, data []byte) error {
	existing, err := os.ReadFile(path) // #nosec G304 // path is under the resolved store root
	switch {
	case err == nil && bytes.Equal(existing, data):
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	return writeFileAtomic(path, data, 0o644)
}

// appendUniqueString appends value when it is non-empty and not present.
func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// storePathReport is the structured output of `ahm store path`.
type storePathReport struct {
	Root    string `json:"root"`
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Records string `json:"records"`
}

// RenderText prints one aligned field per line. The paths are store locations,
// so a path under the user's home directory is abbreviated and text output
// does not embed the machine's absolute home path.
func (r storePathReport) RenderText(w io.Writer) error {
	rows := [][2]string{
		{"root:", abbreviateHome(r.Root)},
		{"key:", r.Key},
		{"records:", abbreviateHome(r.Records)},
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row[0]))
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-*s %s\n", width, row[0], row[1]); err != nil {
			return err
		}
	}
	return nil
}

// abbreviateHome shortens a path under the user's home directory to a leading
// ~. Paths elsewhere are returned unchanged.
func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}

func (a *app) storeCommand() *cobra.Command {
	store := &cobra.Command{
		Use:   "store",
		Short: "Inspect the user-level home store",
		Long: `Inspect where this project's records live in the user-level store.

The store is ~/.ahm, or AHM_HOME when it names an absolute path. Records live
per project under a key derived from the project's identity. A repository keeps
its records in the project until it opts in, so this group inspects and reports
the store location without moving anything.

Examples:
  ahm store path`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("store requires a subcommand\n  ahm store <subcommand>")
		},
	}
	store.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the store root, project key, and records directory",
		Long: `Print where this project's records live in the store: the store root, the
project key, and the records directory.

The key is derived per command. With a Git remote it is the canonical origin
URL — host/owner/repo, lowercased, without scheme, credentials, default port,
or .git. A repository whose only remote is not origin uses that remote; several
remotes without an origin fall back to the path rule rather than guessing. With
no remote, or a file:// or local-path remote, the key is the SHA-256 of the
symlink-resolved project root. A root that holds .git but that Git cannot read
fails instead of falling back, so identity never changes silently.

The command records the project in the store registry and its state file, and
writes nothing else.

Supports --json, --plain, and --text output.

Examples:
  ahm store path
  ahm --json store path`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.storePath()
		},
	})
	return store
}

// storePath resolves and reports the project's store location. It records the
// observation in the store unless --dry-run is given, so a preview writes
// nothing.
func (a *app) storePath() error {
	paths, err := a.storePathsFor()
	if err != nil {
		return err
	}
	if !a.opts.dryRun {
		if err := recordStoreProject(paths); err != nil {
			return err
		}
	}
	return a.emit(storePathReport{
		Root:    paths.Root,
		Key:     paths.Key,
		Kind:    paths.Kind,
		Records: paths.recordsDir(),
	})
}
