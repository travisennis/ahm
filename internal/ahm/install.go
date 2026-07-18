package ahm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/travisennis/ahm/internal/templates"
)

type taskWorkRoleConfig struct {
	Agent string `json:"agent,omitempty"`
	Model string `json:"model,omitempty"`
}

type taskWorkConfig struct {
	PromptFile     string              `json:"promptFile,omitempty"`
	Implementation *taskWorkRoleConfig `json:"implementation,omitempty"`
	Review         *taskWorkRoleConfig `json:"review,omitempty"`
}

const (
	legacyMetadataRelPath = ".agents/ahm.json"
	configMetadataRelPath = ".ahm/config.json"
)

// projectDocsConfig holds optional configuration for ahm docs check.
// All fields are optional; with zero configuration the static checks run
// with defaults.
type projectDocsConfig struct {
	EntryPointBudget int      `json:"entryPointBudget,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	DocMap           []struct {
		Paths []string `json:"paths"`
		Docs  []string `json:"docs"`
	} `json:"docMap,omitempty"`
}

// defaultEntryPointBudget is the line-count budget for root AGENTS.md when
// projectDocs.entryPointBudget is unset or zero.
const defaultEntryPointBudget = 150

type metadata struct {
	Version          string                     `json:"version"`
	StrictAcceptance bool                       `json:"strict_acceptance"`
	DefaultWorkAgent string                     `json:"default_work_agent,omitempty"`
	TaskWork         *taskWorkConfig            `json:"taskWork,omitempty"`
	ProjectDocs      *projectDocsConfig         `json:"projectDocs,omitempty"`
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
		"default_work_agent",
		"taskWork",
		"projectDocs",
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
	if err := writeJSONField(&buf, &first, "version", m.Version); err != nil {
		return nil, err
	}
	if err := writeJSONField(&buf, &first, "strict_acceptance", m.StrictAcceptance); err != nil {
		return nil, err
	}
	if m.DefaultWorkAgent != "" {
		if err := writeJSONField(&buf, &first, "default_work_agent", m.DefaultWorkAgent); err != nil {
			return nil, err
		}
	}
	if m.TaskWork != nil {
		if err := writeJSONField(&buf, &first, "taskWork", m.TaskWork); err != nil {
			return nil, err
		}
	}
	if m.ProjectDocs != nil {
		if err := writeJSONField(&buf, &first, "projectDocs", m.ProjectDocs); err != nil {
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

func sortedMetadataKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type metadataReadError struct {
	RelPath string
	Err     error
}

func (e metadataReadError) Error() string {
	return e.Err.Error()
}

func (e metadataReadError) Unwrap() error {
	return e.Err
}

type obsoleteManagedFile struct {
	Target    string
	EmptyDirs []string
}

// projectOwnedProcedureSkills were installed by older ahm versions, but now
// belong entirely to the project. Commands may discard stale ownership hashes
// for these paths, but must never inspect, report, overwrite, or remove them.
var projectOwnedProcedureSkills = []string{
	".agents/skills/preflight/SKILL.md",
	".agents/skills/grooming-backlog/SKILL.md",
	".agents/skills/finding-improvements/SKILL.md",
}

// preservedScaffoldFiles are no longer created or managed. Existing consumer
// copies are preserved while any stale metadata ownership is relinquished.
var preservedScaffoldFiles = []string{
	".ahm/tasks/README.md",
	".ahm/research/README.md",
	"docs/adr/README.md",
}

func relinquishMetadataOwnership(meta *metadata, targets []string) bool {
	changed := false
	for _, target := range targets {
		if _, ok := meta.Files[target]; ok {
			delete(meta.Files, target)
			changed = true
		}
	}
	return changed
}

func relinquishProjectOwnedProcedureSkills(meta *metadata) bool {
	return relinquishMetadataOwnership(meta, projectOwnedProcedureSkills)
}

func relinquishPreservedScaffoldFiles(meta *metadata) bool {
	return relinquishMetadataOwnership(meta, preservedScaffoldFiles)
}

var obsoleteManagedFiles = []obsoleteManagedFile{
	{
		Target: ".agents/TASKS.md",
	},
	{
		Target: ".agents/PLANS.md",
	},
	{
		Target: ".agents/RESEARCH.md",
	},
	{
		Target: ".agents/DOCS.md",
	},
	{
		Target: ".agents/.tasks/README.md",
	},
	{
		Target: ".agents/.research/README.md",
	},
	{
		Target:    ".agents/skills/deslop/SKILL.md",
		EmptyDirs: []string{".agents/skills/deslop"},
	},
}

func (a *app) install(upgrade bool) error {
	defer a.emitWarnings()
	root := a.opts.root
	meta, err := readMetadata(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("corrupt workflow metadata %s: %w", metadataErrorPath(err), err)
	}
	hasExistingMeta := err == nil
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	relinquishProjectOwnedProcedureSkills(&meta)
	relinquishPreservedScaffoldFiles(&meta)
	if !a.opts.dryRun {
		for _, target := range generatedIndexTargets() {
			delete(meta.Files, target)
		}
	}

	// Fresh init with no prior metadata creates the committed .ahm/ layout
	// directly. When existing metadata is present (either .agents/ahm.json or
	// .ahm/config.json), preserve the existing layout. Upgrade always preserves
	// the existing layout.
	recordsDir := toolRecordsDirName // default for fresh init
	if upgrade || hasExistingMeta {
		recordsDir = a.workflowPaths().recordsDir
	}

	result := map[string][]string{
		"adopted":   {},
		"created":   {},
		"updated":   {},
		"removed":   {},
		"skipped":   {},
		"conflicts": {},
	}
	if err := a.removeObsoleteManagedFiles(upgrade, &meta, result); err != nil {
		return err
	}
	dirs, err := a.ensureWorkflowDirs(recordsDir)
	if err != nil {
		return err
	}
	if err := a.ensureWorkflowGitignore(recordsDir); err != nil {
		return err
	}
	if a.opts.dryRun {
		result["directories"] = dirs
	}
	metaRelPath := metadataWriteRelPath(root)
	// Fresh init with no prior metadata writes .ahm/config.json.
	// When existing metadata is present, preserve the existing path.
	if !upgrade && !hasExistingMeta {
		metaRelPath = configMetadataRelPath
	}
	result["metadata"] = []string{metaRelPath}
	indexes, err := a.indexWriteTargetsFor(workflowPaths{root: root, recordsDir: recordsDir})
	if err != nil {
		return err
	}
	result["indexes"] = indexes
	if !a.opts.dryRun {
		meta.Version = templates.Version
		// Fresh init with no prior metadata writes .ahm/config.json.
		// When existing metadata is present, preserve the existing path.
		if upgrade || hasExistingMeta {
			if err := writeMetadata(root, meta); err != nil {
				return err
			}
		} else {
			if err := writeConfigMetadata(root, meta); err != nil {
				return err
			}
		}
		a.invalidateWorkflowPaths()
		if err := a.writeIndexes(); err != nil {
			return err
		}
	}
	return a.emit(result)
}

func (a *app) removeObsoleteManagedFiles(upgrade bool, meta *metadata, result map[string][]string) error {
	if !upgrade {
		return nil
	}
	for _, item := range obsoleteManagedFiles {
		target := filepath.Join(a.opts.root, item.Target)
		existing, err := readWorkflowFile(target)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if !a.opts.dryRun {
				delete(meta.Files, item.Target)
			}
			continue
		case err != nil:
			return err
		}

		if !a.opts.force && (meta.Files[item.Target] == "" || meta.Files[item.Target] != hashBytes(existing)) {
			result["conflicts"] = append(result["conflicts"], item.Target)
			continue
		}

		result["removed"] = append(result["removed"], item.Target)
		if a.opts.dryRun {
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		delete(meta.Files, item.Target)
		for _, dir := range item.EmptyDirs {
			err := os.Remove(filepath.Join(a.opts.root, dir))
			if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
				return err
			}
		}
	}
	return nil
}

// ensureWorkflowGitignore creates the managed .ahm/.gitignore with the
// standard ignore entries if it does not already exist. It is a no-op in
// legacy layout (where .agents/ tracks everything under normal Git) and in
// dry-run mode.
func (a *app) ensureWorkflowGitignore(recordsDir string) error {
	if recordsDir != toolRecordsDirName {
		return nil
	}
	if a.opts.dryRun {
		return nil
	}
	gitignorePath := filepath.Join(a.opts.root, filepath.FromSlash(recordsGitignoreRelPath))
	if _, err := os.Stat(gitignorePath); errors.Is(err, os.ErrNotExist) {
		content := recordsGitignoreHeader + strings.Join(recordsGitignoreEntries, "\n") + "\n"
		return writeFileAtomic(gitignorePath, []byte(content), 0o644)
	} else if err != nil {
		return err
	}
	return nil
}

func (a *app) ensureWorkflowDirs(recordsDir string) ([]string, error) {
	paths := workflowPaths{root: a.opts.root, recordsDir: recordsDir}
	dirs := []string{
		paths.tasksRel() + "/active",
		paths.tasksRel() + "/completed",
		paths.tasksRel() + "/cancelled",
		paths.researchRel() + "/inbox",
		paths.researchRel() + "/investigations",
		paths.researchRel() + "/sources",
		paths.researchRel() + "/topics",
		paths.researchRel() + "/archived",
		paths.execPlansRel("active"),
		paths.execPlansRel("completed"),
		"docs/adr",
	}
	var created []string
	for _, dir := range dirs {
		path := filepath.Join(a.opts.root, dir)
		if a.opts.dryRun {
			stat, err := os.Stat(path)
			switch {
			case errors.Is(err, os.ErrNotExist):
				created = append(created, dir)
			case err != nil:
				return nil, err
			case !stat.IsDir():
				return nil, fmt.Errorf("%s exists and is not a directory", path)
			}
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil { // #nosec G301 // 0755 is the standard directory permission for workflow directories
			return nil, err
		}
	}
	return created, nil
}

func readMetadata(root string) (metadata, error) {
	meta, _, err := readMetadataWithSource(root)
	return meta, err
}

func readMetadataWithSource(root string) (metadata, string, error) {
	var meta metadata
	relPath := metadataReadRelPath(root)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath))) // #nosec G304,G703 // path constructed from project root, not user input
	if err != nil {
		return meta, relPath, metadataReadError{RelPath: relPath, Err: err}
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, relPath, metadataReadError{RelPath: relPath, Err: err}
	}
	return meta, relPath, nil
}

func writeMetadata(root string, meta metadata) error {
	return writeMetadataRel(root, metadataWriteRelPath(root), meta)
}

func writeConfigMetadata(root string, meta metadata) error {
	return writeMetadataRel(root, configMetadataRelPath, meta)
}

func writeMetadataRel(root string, relPath string, meta metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, filepath.FromSlash(relPath)), append(data, '\n'), 0o644)
}

func metadataReadRelPath(root string) string {
	if stat, err := os.Stat(filepath.Join(root, ".ahm", "config.json")); err == nil && !stat.IsDir() { // #nosec G703 // path constructed from project root, not user input
		return configMetadataRelPath
	}
	return legacyMetadataRelPath
}

func metadataWriteRelPath(root string) string {
	if stat, err := os.Stat(filepath.Join(root, ".ahm", "config.json")); err == nil && !stat.IsDir() { // #nosec G703 // path constructed from project root, not user input
		return configMetadataRelPath
	}
	return legacyMetadataRelPath
}

func metadataErrorPath(err error) string {
	var metaErr metadataReadError
	if errors.As(err, &metaErr) && metaErr.RelPath != "" {
		return metaErr.RelPath
	}
	return legacyMetadataRelPath
}

func metadataCorruptMessage(err error) string {
	return fmt.Sprintf("corrupt workflow metadata %s", metadataErrorPath(err))
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// readWorkflowFile reads a file and normalizes CRLF (\r\n) line endings to
// LF (\n) so that downstream parsing functions do not need to handle both.
func readWorkflowFile(path string) ([]byte, error) {
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
