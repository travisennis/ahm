package ahm

import (
	"path/filepath"
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

// displayPath renders a path for user-facing output. A store path is shown as
// store: followed by its path inside the store, and a project path is shown
// relative to the project root.
func (p workflowPaths) displayPath(path string) string {
	if p.inStore() && pathWithin(p.store.ProjectDir, path) {
		if rel, err := filepath.Rel(p.store.ProjectDir, path); err == nil {
			return "store:" + filepath.ToSlash(rel)
		}
	}
	return relPath(p.projectRoot, path)
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
