package ahm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	legacyMetadataRelPath = ".agents/ahm.json"
	configMetadataRelPath = ".ahm/config.json"

	recordsGitignoreRelPath = ".ahm/.gitignore"

	// gitignoreFileName is the managed .gitignore's name in each layout: the
	// project's .ahm/.gitignore, and the store project directory's .gitignore.
	gitignoreFileName = ".gitignore"
)

// recordsGitignoreEntries keep generated workflow indexes and machine-local
// state out of branch history while source records and .ahm/config.json stay
// committed. Only the task indexes are generated under .ahm/; the ADR index is
// committed documentation, and the retired research and ExecPlan trees are
// ordinary project files whose content is no longer ignored.
var recordsGitignoreEntries = []string{
	"tasks/index.md",
	"tasks/*/index.md",
	".lock/",
	gitignoreTempPattern,
}

// gitignoreTempPattern is the temp-file pattern every managed .gitignore carries.
// An atomic write creates its temp file beside its target, so the states ahm
// writes into — .ahm/ in project mode, the store's project directory in home
// mode — always need it, even once the records themselves have moved away.
const gitignoreTempPattern = "*.tmp"

// storeGitignoreEntries are the entries the store's directory for a project
// ignores. The records live in the store rather than in a repository, so the
// list covers the same generated indexes, lock directory, and temp files, plus
// the project state file, which holds store-local state and changes with almost
// every task mutation.
var storeGitignoreEntries = []string{
	storeStateFileName,
	"tasks/index.md",
	"tasks/*/index.md",
	".lock/",
	gitignoreTempPattern,
}

const recordsGitignoreHeader = "# Managed by ahm. Generated workflow indexes and machine-local state stay local-only;\n# source records and config.json remain committed.\n"

// homeRecordsGitignoreHeader heads the committed .ahm/.gitignore once the
// records live in the store: the task records are not committed any more, and
// the only ahm write left in .ahm/ is the atomic rewrite of config.json.
const homeRecordsGitignoreHeader = "# Managed by ahm. Machine-local state stays local-only; config.json remains\n# committed, and the task records live in the user-level store.\n"

// storeRecordsGitignoreHeader heads the managed .gitignore of the store's
// project directory, where the records are machine-local and are never part of
// a repository's history.
const storeRecordsGitignoreHeader = "# Managed by ahm. Generated task indexes and machine-local state stay local-only;\n# these task records live in the user-level store, outside a project's history.\n"

// recordsGitignoreContent is the complete .ahm/.gitignore that ahm owns.
// workflowPaths.workflowGitignoreContent returns the same content for the
// in-project layout and the store's header for the home layout.
func recordsGitignoreContent() []byte {
	return []byte(recordsGitignoreHeader + strings.Join(recordsGitignoreEntries, "\n") + "\n")
}

// metadata is the committed .ahm/config.json payload. Version preserves the
// obsolete template-version field in existing metadata. Fresh metadata omits
// it, and ahm no longer reads or updates it.
type metadata struct {
	Version          string                     `json:"version,omitempty"`
	StrictAcceptance bool                       `json:"strict_acceptance"`
	TasksLocation    string                     `json:"tasks_location,omitempty"`
	Files            map[string]string          `json:"files"`
	Extra            map[string]json.RawMessage `json:"-"`
}

func (m *metadata) UnmarshalJSON(data []byte) error {
	type metadataAlias metadata
	var alias metadataAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{
		"version",
		"strict_acceptance",
		"tasks_location",
		// Consume obsolete ahm-owned keys without preserving them in Extra so
		// the next metadata write removes them.
		"default_work_agent",
		"taskWork",
		"projectDocs",
		"research",
		"files",
	} {
		delete(raw, key)
	}
	*m = metadata(alias)
	if len(raw) > 0 {
		m.Extra = raw
	}
	return nil
}

func (m metadata) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	if m.Version != "" {
		if err := writeJSONField(&buf, &first, "version", m.Version); err != nil {
			return nil, err
		}
	}
	if err := writeJSONField(&buf, &first, "strict_acceptance", m.StrictAcceptance); err != nil {
		return nil, err
	}
	if m.TasksLocation != "" {
		if err := writeJSONField(&buf, &first, "tasks_location", m.TasksLocation); err != nil {
			return nil, err
		}
	}
	if err := writeJSONField(&buf, &first, "files", m.Files); err != nil {
		return nil, err
	}
	for _, key := range sortedMetadataKeys(m.Extra) {
		if err := writeRawJSONField(&buf, &first, key, m.Extra[key]); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalMetadata renders metadata as the exact bytes ahm writes to
// .ahm/config.json.
func marshalMetadata(meta metadata) ([]byte, error) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func sortedMetadataKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeJSONField(buf *bytes.Buffer, first *bool, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeRawJSONField(buf, first, key, data)
}

func writeRawJSONField(buf *bytes.Buffer, first *bool, key string, value json.RawMessage) error {
	keyData, err := json.Marshal(key)
	if err != nil {
		return err
	}
	if !*first {
		buf.WriteByte(',')
	}
	*first = false
	buf.Write(keyData)
	buf.WriteByte(':')
	buf.Write(value)
	return nil
}

// retiredManagedFiles were owned by older ahm versions and are now entirely
// project-owned or no longer generated: procedure skills, record scaffold
// READMEs, and the generated indexes of retired record families. ahm never
// inspects, reports, overwrites, or removes them; install only discards any
// stale ownership hash so the metadata stops claiming them.
var retiredManagedFiles = []string{
	".agents/skills/preflight/SKILL.md",
	".agents/skills/grooming-backlog/SKILL.md",
	".agents/skills/finding-improvements/SKILL.md",
	".ahm/tasks/README.md",
	".ahm/research/README.md",
	"docs/adr/README.md",
	".agents/.research/index.md",
	".agents/exec-plans/active/index.md",
	".agents/exec-plans/completed/index.md",
	".ahm/.research/index.md",
	".ahm/research/index.md",
	".ahm/exec-plans/active/index.md",
	".ahm/exec-plans/completed/index.md",
}

func relinquishMetadataOwnership(meta *metadata, targets []string) {
	for _, target := range targets {
		delete(meta.Files, target)
	}
}

// resolveTaskLocation maps a repository's committed configuration to its
// storage mode. A configuration that names home uses the store, and a missing
// key, every unrecognized value, and every configuration written before the
// store existed keep the records in the project. A repository with no
// configuration at all is a new project, and a new project keeps its records in
// the store.
func resolveTaskLocation(meta metadata, configExists bool) taskLocation {
	if !configExists {
		return locationHome
	}
	if taskLocation(meta.TasksLocation) == locationHome {
		return locationHome
	}
	return locationProject
}

// reconcileMetadata drops ahm-owned metadata that this version no longer
// maintains and keeps everything else, including unknown fields.
func reconcileMetadata(meta *metadata) {
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	relinquishMetadataOwnership(meta, retiredManagedFiles)
	// Generated indexes are derived data, so they never carry an ownership
	// hash.
	relinquishMetadataOwnership(meta, generatedIndexTargets())
}

// install creates ahm-owned workflow state when it is absent and reconciles it
// when it is present. It writes a file only when the bytes on disk differ from
// the bytes ahm owns, so an up-to-date repository is left completely
// untouched. Project-owned files, including AGENTS.md, are never created,
// replaced, or removed. Root resolution refuses a legacy layout before this
// runs, so install never writes into a retired tree.
func (a *app) install() error {
	defer a.emitWarnings()
	root := a.opts.root

	meta, err := readMetadata(root)
	configExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("corrupt workflow metadata %s: %v", configMetadataRelPath, err)
	}
	reconcileMetadata(&meta)
	if !configExists {
		// A repository with no configuration is a new project, and a new project
		// keeps its records in the store. Writing the key here, before the layout
		// is resolved below, is what makes this run lay the store down and every
		// later run read the mode back. An existing configuration is preserved
		// untouched, so a repository that predates the store stays in the
		// project.
		meta.TasksLocation = string(locationHome)
	}

	// Install resolves the layout from the configuration it owns rather than
	// from the cached resolution taken before this command ran, so the mode this
	// run writes is the mode this run lays down directories for.
	paths, err := resolveWorkflowPaths(root, meta, configExists)
	if err != nil {
		return err
	}
	a.useWorkflowPaths(paths)
	a.warnAboutStrandedProjectRecords(paths)

	result := map[string][]string{"created": {}, "updated": {}, "directories": {}}
	created, err := a.ensureWorkflowDirs()
	if err != nil {
		return err
	}
	result["directories"] = created

	for _, gitignore := range paths.managedGitignores() {
		if err := a.reconcileFile(gitignore.path, paths.displayPath(gitignore.path), gitignore.content, result); err != nil {
			return err
		}
	}
	config, err := marshalMetadata(meta)
	if err != nil {
		return err
	}
	if err := a.reconcileFile(paths.configPath(), configMetadataRelPath, config, result); err != nil {
		return err
	}
	if err := a.reconcileIndexes(result); err != nil {
		return err
	}
	if err := a.initializeTaskIDCounter(paths); err != nil {
		return err
	}
	if err := a.recordStoreInstall(paths); err != nil {
		return err
	}
	return a.emit(result)
}

// warnAboutStrandedProjectRecords warns when the layout install is about to
// write keeps the records in the store while task records are still in the
// project: the mode becomes permanent in this run, and no command reads those
// records afterwards. It reports the finding that status, doctor, and prime
// report, at the command that decides the mode. A dry run warns too, because
// the preview is where a person sees what the run would leave behind.
func (a *app) warnAboutStrandedProjectRecords(paths workflowPaths) {
	if !paths.inStore() {
		return
	}
	var report validationReport
	validateRecordsNotInProject(paths, &report)
	for _, finding := range report.Errors {
		a.addWarning("%s", finding.Message)
	}
}

// recordStoreInstall records the project in the store when install lays the
// store down: the registry entry, which holds the observations a later adoption
// or migration command reads, and the state file beside the records. A fresh
// home project is the default path, so install is where its identity first
// becomes an observation. It writes nothing in project mode, nothing on a dry
// run, and nothing when the store already holds those bytes.
func (a *app) recordStoreInstall(paths workflowPaths) error {
	if a.opts.dryRun || !paths.inStore() {
		return nil
	}
	return recordStoreProject(paths.store)
}

// initializeTaskIDCounter records the store's task ID counter from the records
// present, and does nothing when the records live in the project, where Git
// history already proves which numbers were spent. Recording the high-water
// mark at install time is what makes a later deletion of the newest record safe
// - including one deleted before the next create, which the allocation scan
// would no longer see. A partial task set is enough: the counter only ever
// moves up, and any record that did not parse is reported by the index
// reconciliation above.
func (a *app) initializeTaskIDCounter(paths workflowPaths) error {
	if a.opts.dryRun {
		return nil
	}
	if _, ok := paths.taskIDCounterPath(); !ok {
		// Project mode has no counter file, and needs none: Git history proves
		// which numbers were spent. Nothing is read, either.
		return nil
	}
	tasks, err := a.getTasks()
	if err != nil && tasks == nil {
		return err
	}
	return writeTaskIDCounter(paths, highestTaskNumber(tasks, paths)+1)
}

// reconcileIndexes reports the generated indexes that are missing or stale and
// regenerates them on a real run. Post-mutation validation runs as part of the
// regeneration, so init surfaces record findings like every other
// index-writing command.
func (a *app) reconcileIndexes(result map[string][]string) error {
	writes, err := a.indexWrites()
	if err != nil && writes == nil {
		return err
	}
	stale := make([]string, 0, len(writes))
	for _, path := range sortedKeys(writes) {
		if isStaleIndex(nil, path, writes[path]) {
			stale = append(stale, a.workflowPaths().displayPath(path))
		}
	}
	result["indexes"] = stale
	if a.opts.dryRun {
		return nil
	}
	return a.writeIndexes()
}

// reconcileFile writes content to an ahm-owned file unless it already holds
// those exact bytes, so a second run of a reconciling command writes nothing. It
// records label as created or updated when a write is needed, or would be needed
// in dry-run mode.
func (a *app) reconcileFile(path string, label string, content []byte, result map[string][]string) error {
	existing, err := os.ReadFile(path) // #nosec G304 // path built from a workflow accessor, not user input
	switch {
	case errors.Is(err, os.ErrNotExist):
		result["created"] = append(result["created"], label)
	case err != nil:
		return err
	case bytes.Equal(existing, content):
		return nil
	default:
		result["updated"] = append(result["updated"], label)
	}
	if a.opts.dryRun {
		return nil
	}
	return writeOwned(a.workflowPaths(), path, content)
}

// ensureWorkflowGitignore creates each managed .gitignore for the resolved
// records location when it is missing and leaves an existing file untouched.
// prime calls it to prepare the worktree; ahm init reconciles the content
// instead.
func (a *app) ensureWorkflowGitignore() error {
	if a.opts.dryRun {
		return nil
	}
	paths := a.workflowPaths()
	for _, gitignore := range paths.managedGitignores() {
		if _, err := os.Stat(gitignore.path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeOwned(paths, gitignore.path, gitignore.content); err != nil {
			return err
		}
	}
	return nil
}

// ensureWorkflowDirs creates the record directories ahm owns and returns the
// ones that were missing, which in dry-run mode are the ones a real run would
// create.
func (a *app) ensureWorkflowDirs() ([]string, error) {
	paths := a.workflowPaths()
	// Each directory carries the label the report uses, so the reported path
	// follows the records into the store when they live there.
	dirs := []struct{ path, label string }{
		{paths.tasksBucketDir("active"), paths.displayPath(paths.tasksBucketDir("active"))},
		{paths.tasksBucketDir("completed"), paths.displayPath(paths.tasksBucketDir("completed"))},
		{paths.tasksBucketDir("cancelled"), paths.displayPath(paths.tasksBucketDir("cancelled"))},
		{paths.adrDir(), "docs/adr"},
	}
	created := []string{}
	for _, dir := range dirs {
		stat, err := os.Stat(dir.path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			created = append(created, dir.label)
		case err != nil:
			return nil, err
		case !stat.IsDir():
			return nil, fmt.Errorf("%s exists and is not a directory", dir.path)
		default:
			continue
		}
		if a.opts.dryRun {
			continue
		}
		if err := os.MkdirAll(dir.path, 0o755); err != nil { // #nosec G301 // 0755 is the standard directory permission for workflow directories
			return nil, err
		}
	}
	return created, nil
}

// readMetadata reads the committed .ahm/config.json. A missing file is
// reported as os.ErrNotExist so callers can distinguish an uninstalled
// workflow from an unreadable one.
func readMetadata(root string) (metadata, error) {
	var meta metadata
	path := workflowPathsFor(root).configPath()
	data, err := os.ReadFile(path) // #nosec G304,G703 // path constructed from project root, not user input
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

// readWorkflowFileHook supports instrumented tests that count filesystem reads
// of managed workflow files.
var readWorkflowFileHook = func(string) {}

// readWorkflowFile reads a file and normalizes CRLF (\r\n) line endings to
// LF (\n) so that downstream parsing functions do not need to handle both.
func readWorkflowFile(path string) ([]byte, error) {
	readWorkflowFileHook(path)
	data, err := os.ReadFile(path) // #nosec G304 // path is under project root, read from managed workflow files
	if err != nil {
		return nil, err
	}
	// Strip UTF-8 BOM if present.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return data, nil
}
