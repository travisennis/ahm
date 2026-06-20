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
	"syscall"

	"github.com/travisennis/ahm/internal/templates"
)

type metadata struct {
	Version          string            `json:"version"`
	StrictAcceptance bool              `json:"strict_acceptance"`
	DefaultWorkAgent string            `json:"default_work_agent,omitempty"`
	Files            map[string]string `json:"files"`
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
		Target: "docs/adr/README.md",
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
		return fmt.Errorf("corrupt workflow metadata .agents/ahm.json: %w", err)
	}
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	if !a.opts.dryRun {
		for _, target := range generatedIndexTargets() {
			delete(meta.Files, target)
		}
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
		var content []byte
		if item.Target == "AGENTS.md" {
			content = []byte(templates.RenderAgentsMarkdown())
		} else {
			var err error
			content, err = fs.ReadFile(templates.FS, item.Source)
			if err != nil {
				return err
			}
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
	dirs, err := a.ensureWorkflowDirs()
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		result["directories"] = dirs
	}
	result["metadata"] = []string{".agents/ahm.json"}
	indexes, err := a.indexWriteTargets()
	if err != nil {
		return err
	}
	result["indexes"] = indexes
	if !a.opts.dryRun {
		meta.Version = templates.Version
		if err := writeMetadata(root, meta); err != nil {
			return err
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

func (a *app) ensureWorkflowDirs() ([]string, error) {
	dirs := []string{
		".agents/.tasks/active",
		".agents/.tasks/completed",
		".agents/.tasks/cancelled",
		".agents/.research/inbox",
		".agents/.research/investigations",
		".agents/.research/sources",
		".agents/.research/topics",
		".agents/.research/archived",
		".agents/exec-plans/active",
		".agents/exec-plans/completed",
		".agents/skills/preflight",
		".agents/skills/grooming-backlog",
		".agents/skills/finding-improvements",
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
	var meta metadata
	data, err := os.ReadFile(filepath.Join(root, ".agents", "ahm.json")) // #nosec G304 // path constructed from project root, not user input
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}

func writeMetadata(root string, meta metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(root, ".agents", "ahm.json"), append(data, '\n'), 0o644)
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
