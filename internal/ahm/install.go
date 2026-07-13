package ahm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	{
		Target:    ".agents/skills/preflight/SKILL.md",
		EmptyDirs: []string{".agents/skills/preflight", ".agents/skills"},
	},
	{
		Target:    ".agents/skills/grooming-backlog/SKILL.md",
		EmptyDirs: []string{".agents/skills/grooming-backlog", ".agents/skills"},
	},
	{
		Target:    ".agents/skills/finding-improvements/SKILL.md",
		EmptyDirs: []string{".agents/skills/finding-improvements", ".agents/skills"},
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
		recordsDir = workflowPathsFor(root).recordsDir
	}

	result := map[string][]string{
		"created":   {},
		"updated":   {},
		"removed":   {},
		"skipped":   {},
		"conflicts": {},
	}
	if err := a.removeObsoleteManagedFiles(upgrade, &meta, result); err != nil {
		return err
	}
	for _, item := range templates.Files() {
		content, err := fs.ReadFile(templates.FS, item.Source)
		if err != nil {
			return err
		}
		content, err = renderWorkflowTemplateFor(root, item.Source, content, recordsDir)
		if err != nil {
			return err
		}
		target := filepath.Join(root, item.Target)
		hash := hashBytes(content)
		existing, readErr := readWorkflowFile(target)
		switch {
		case errors.Is(readErr, os.ErrNotExist):
			result["created"] = append(result["created"], item.Target)
			if !a.opts.dryRun {
				if err := writeFileAtomic(target, content, 0o644); err != nil {
					return err
				}
			}
			if !a.opts.dryRun {
				if item.CreateOnly {
					delete(meta.Files, item.Target)
				} else {
					meta.Files[item.Target] = hash
				}
			}
		case readErr != nil:
			return readErr
		case item.CreateOnly:
			result["skipped"] = append(result["skipped"], item.Target)
			if !a.opts.dryRun {
				delete(meta.Files, item.Target)
			}
		case !a.opts.force && meta.Files[item.Target] == "":
			// File exists but is not tracked in metadata. Auto-adopt when
			// content matches the template; otherwise report as a conflict.
			if hashBytes(existing) == hash {
				result["adopted"] = append(result["adopted"], item.Target)
				if !a.opts.dryRun {
					meta.Files[item.Target] = hash
				}
			} else {
				result["conflicts"] = append(result["conflicts"], item.Target)
			}
		case !upgrade && !a.opts.force:
			result["skipped"] = append(result["skipped"], item.Target)
		case a.opts.force || meta.Files[item.Target] == hashBytes(existing):
			if string(existing) != string(content) {
				result["updated"] = append(result["updated"], item.Target)
				if !a.opts.dryRun {
					if err := writeFileAtomic(target, content, 0o644); err != nil {
						return err
					}
				}
			} else {
				result["skipped"] = append(result["skipped"], item.Target)
			}
			if !a.opts.dryRun {
				meta.Files[item.Target] = hash
			}
		default:
			result["conflicts"] = append(result["conflicts"], item.Target)
		}
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
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath))) // #nosec G304 // path constructed from project root, not user input
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
