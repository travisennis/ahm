package ahm

import (
	"path/filepath"
	"strings"
)

// toolRecordsDirName is the tool-owned directory that holds ahm workflow
// state: committed records, configuration, the managed .gitignore, generated
// indexes, and the workflow lock.
const toolRecordsDirName = ".ahm"

// taskLocation is where a project keeps its task records. It is the value of
// the tasks_location key in .ahm/config.json, and a missing key means
// locationProject so that existing repositories are unchanged.
type taskLocation string

const (
	locationProject taskLocation = "project"
	locationHome    taskLocation = "home"
)

// workflowPaths resolves every ahm-managed path for one repository root. A
// repository has two roots: a project root that owns the committed
// configuration and ADRs, and a records root that owns task records and their
// generated indexes. In project mode the records root is inside the project
// root; in home mode it is inside the user-level store, and store carries the
// resolved store location.
type workflowPaths struct {
	projectRoot string
	recordsRoot string
	store       storePaths // zero value when the records live in the project
	mode        taskLocation
}

// workflowPathsFor resolves the project-mode layout for one repository root.
// Project mode is the layout every existing repository uses, so this is what
// tests and callers that predate the home store resolve.
func workflowPathsFor(root string) workflowPaths {
	return workflowPaths{
		projectRoot: root,
		recordsRoot: filepath.Join(root, toolRecordsDirName, storeRecordsDirName),
		mode:        locationProject,
	}
}

// workflowPathsForStore resolves the store-mode layout for one repository root:
// the committed configuration and the ADRs stay in the project, and the records
// and their indexes live in the store's directory for the project. The records
// root comes from the resolved store, so the store's records directory has one
// definition.
func workflowPathsForStore(root string, store storePaths) workflowPaths {
	return workflowPaths{
		projectRoot: root,
		recordsRoot: store.recordsDir(),
		store:       store,
		mode:        locationHome,
	}
}

// resolveWorkflowPaths resolves the layout of one repository root from its
// committed configuration. The store is resolved only when the configuration
// names home, so a project-mode repository never reads Git and never fails
// because a store is unavailable.
func resolveWorkflowPaths(root string, meta metadata, configExists bool) (workflowPaths, error) {
	if resolveTaskLocation(meta, configExists) == locationProject {
		return workflowPathsFor(root), nil
	}
	store, err := resolveStore(root)
	if err != nil {
		return workflowPaths{}, err
	}
	return workflowPathsForStore(root, store), nil
}

// configPath is the committed .ahm/config.json, which always lives in the
// project.
func (p workflowPaths) configPath() string {
	return filepath.Join(p.projectRoot, filepath.FromSlash(configMetadataRelPath))
}

// adrDir is the committed docs/adr directory, which always lives in the
// project.
func (p workflowPaths) adrDir() string {
	return filepath.Join(p.projectRoot, "docs", "adr")
}

// adrIndexPath is the committed generated ADR index, which lives beside the
// ADRs in the project.
func (p workflowPaths) adrIndexPath() string {
	return filepath.Join(p.adrDir(), "index.md")
}

// inStore reports whether the records live in the user-level store rather than
// in the project. Every accessor that differs between the two layouts asks this
// one question, so the mode and the resolved store can never disagree about
// where the records are: a mode of home with no resolved store falls back to
// the project layout instead of producing a half-resolved path.
func (p workflowPaths) inStore() bool {
	return p.mode == locationHome && p.store.ProjectDir != ""
}

// recordsRel is the records root spelled relative to the directory that
// contains it: .ahm/tasks in project mode, tasks in the store.
func (p workflowPaths) recordsRel() string {
	if p.inStore() {
		return storeRecordsDirName
	}
	return toolRecordsDirName + "/" + storeRecordsDirName
}

// tasksBucketDir returns the directory holding one task bucket. The empty
// bucket is the records root itself.
func (p workflowPaths) tasksBucketDir(bucket string) string {
	return filepath.Join(p.recordsRoot, bucket)
}

// taskFile returns the record path of one task.
func (p workflowPaths) taskFile(bucket string, id string) string {
	return filepath.Join(p.tasksBucketDir(bucket), id+".md")
}

// ownedRoots are the directories a workflow write may land under: the project
// root for configuration, ADRs, and their index, and — when the records live
// in the store — the store's directory for this project. Every workflow write
// is scoped by writeOwned to one of them.
func (p workflowPaths) ownedRoots() []string {
	roots := []string{p.projectRoot}
	if p.inStore() {
		roots = append(roots, p.store.ProjectDir)
	}
	return roots
}

// stateRoots are the ahm-owned state directories that hold workflow files, and
// therefore the directories cleanupStaleTemps scans for orphaned temp files.
// They are the tool directory inside the project root and the store's
// directory for this project; a whole-root scan would reap unrelated .tmp
// files the user owns.
func (p workflowPaths) stateRoots() []string {
	roots := []string{filepath.Join(p.projectRoot, toolRecordsDirName)}
	if p.inStore() {
		roots = append(roots, p.store.ProjectDir)
	}
	return roots
}

// displayPath renders a path for user-facing output: findings, error messages,
// and index listings. A store path is shown as store: followed by its path
// inside the store, and a project path is shown relative to the project root,
// which is what every one of those messages printed before the home store.
func (p workflowPaths) displayPath(path string) string {
	if p.inStore() && pathWithin(p.store.ProjectDir, path) {
		if rel, err := filepath.Rel(p.store.ProjectDir, path); err == nil {
			return "store:" + filepath.ToSlash(rel)
		}
	}
	return relPath(p.projectRoot, path)
}

// payloadPath renders a record path for a structured payload: the JSON path
// field of a task, and the dry-run create, move, and unblock previews. A store
// path is displayed the same way displayPath displays it, and a project path is
// returned unchanged, because those payloads carry the record's own path and
// ADR 023 keeps project-mode output byte-identical.
func (p workflowPaths) payloadPath(path string) string {
	if p.inStore() && pathWithin(p.store.ProjectDir, path) {
		return p.displayPath(path)
	}
	return path
}

// inProjectRecordPath maps a store record path to the in-project path the
// record would have if it still lived in the project. It reports false for a
// project record path. Markdown link validation uses it as the fallback base
// for a relative link written under the committed-in-project model, so that a
// record that moved into the store keeps resolving its links (ADR 023).
func (p workflowPaths) inProjectRecordPath(path string) (string, bool) {
	if !p.inStore() || !pathWithin(p.recordsRoot, path) {
		return "", false
	}
	rel, err := filepath.Rel(p.recordsRoot, path)
	if err != nil {
		return "", false
	}
	return filepath.Join(p.projectRoot, toolRecordsDirName, storeRecordsDirName, rel), true
}

// workflowGitignorePath is the managed .gitignore for ahm-owned workflow state:
// .ahm/.gitignore in project mode, and the store project directory's .gitignore
// in home mode, where the generated task indexes and the records lock live. A
// store-root write is deliberately not part of this: the store's project
// directory is the only owned root inside the store.
func (p workflowPaths) workflowGitignorePath() string {
	if p.inStore() {
		return filepath.Join(p.store.ProjectDir, gitignoreFileName)
	}
	return p.projectGitignorePath()
}

// managedGitignore is one .gitignore ahm owns for a resolved layout, with the
// content ahm writes into it.
type managedGitignore struct {
	path    string
	content []byte
}

// managedGitignores are the managed .gitignore files the resolved mode owns.
// Project mode owns one: the committed .ahm/.gitignore, which covers the
// generated task indexes, the records lock, and temp files. Home mode owns two:
// the committed .ahm/.gitignore, which keeps the atomic rewrite of config.json
// from leaving an untracked temp file behind, and the store's own .gitignore
// beside the records, where the indexes, the lock, and the store state live.
// Every command that reconciles or rewrites them — install, prime, and the
// migration — owns the same set.
func (p workflowPaths) managedGitignores() []managedGitignore {
	files := []managedGitignore{{p.projectGitignorePath(), p.projectGitignoreContent()}}
	if p.inStore() {
		files = append(files, managedGitignore{p.workflowGitignorePath(), p.workflowGitignoreContent()})
	}
	return files
}

// projectGitignorePath is the committed .ahm/.gitignore, which lives in the
// project in both layouts.
func (p workflowPaths) projectGitignorePath() string {
	return filepath.Join(p.projectRoot, filepath.FromSlash(recordsGitignoreRelPath))
}

// projectGitignoreContent is the content of the committed .ahm/.gitignore for
// the resolved mode. Project mode owns the full list of generated task indexes,
// the records lock, and temp files. Home mode owns only the temp-file pattern:
// the task indexes and the lock moved into the store with the records, and the
// one write ahm still makes in .ahm/ is the atomic rewrite of config.json,
// whose leftover temp file must stay out of Git.
func (p workflowPaths) projectGitignoreContent() []byte {
	if p.inStore() {
		return []byte(homeRecordsGitignoreHeader + gitignoreTempPattern + "\n")
	}
	return recordsGitignoreContent()
}

// workflowGitignoreContent is the complete .gitignore ahm owns for the resolved
// workflow state. Both layouts ignore the generated task indexes, the lock
// directory, and temp files; the store's project directory additionally ignores
// its state file.
func (p workflowPaths) workflowGitignoreContent() []byte {
	if p.inStore() {
		return []byte(storeRecordsGitignoreHeader + strings.Join(storeGitignoreEntries, "\n") + "\n")
	}
	return recordsGitignoreContent()
}

// recordsStatus reports where this project's records live: the store root, the
// project key and its kind, and the storage location. It reports false when the
// records live in the project, so project-mode output is byte-identical to the
// output before the home store existed.
func (p workflowPaths) recordsStatus() (map[string]string, bool) {
	if !p.inStore() {
		return nil, false
	}
	return map[string]string{
		"root":     p.store.Root,
		"key":      p.store.Key,
		"kind":     p.store.Kind,
		"location": string(p.mode),
	}, true
}

// lockDirName is the directory inside a workflow state root that holds the
// record-mutation lock.
const lockDirName = ".lock"

// lockDir is the directory that holds this project's record-mutation lock. It
// sits beside the records directory rather than inside it — .ahm/.lock in
// project mode, .lock in the store's project directory — so two clones that
// share one store serialize on one lock.
func (p workflowPaths) lockDir() string {
	return filepath.Join(filepath.Dir(p.recordsRoot), lockDirName)
}
