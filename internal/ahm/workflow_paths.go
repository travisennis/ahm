package ahm

import "path/filepath"

const (
	legacyRecordsDirName = ".agents"
	toolRecordsDirName   = ".ahm"
)

// workflowPaths resolves where ahm-managed workflow records live for a
// repository. Legacy committed-record repositories keep records under
// project-owned .agents/; repositories that opted into the ADR 013 storage
// migration keep records under tool-owned .ahm/.
type workflowPaths struct {
	root       string
	recordsDir string // legacyRecordsDirName or toolRecordsDirName
}

// workflowPathsFor derives the record paths for root from workflow metadata.
// Missing or unreadable metadata preserves legacy committed-record paths.
// The records directory is determined by which config file anchors the
// repository: .ahm/config.json selects the migrated layout (.ahm/ paths);
// .agents/ahm.json (or absent metadata) selects the legacy layout (.agents/ paths).
func workflowPathsFor(root string) workflowPaths {
	recordsDir := legacyRecordsDirName
	if _, source, err := readMetadataWithSource(root); err == nil && source == configMetadataRelPath {
		recordsDir = toolRecordsDirName
	}
	return workflowPaths{root: root, recordsDir: recordsDir}
}

func (p workflowPaths) tasksRel() string {
	if p.recordsDir == toolRecordsDirName {
		return p.recordsDir + "/tasks"
	}
	return p.recordsDir + "/.tasks"
}

func (p workflowPaths) tasksBucketDir(bucket string) string {
	if p.recordsDir == toolRecordsDirName {
		return filepath.Join(p.root, p.recordsDir, "tasks", bucket)
	}
	return filepath.Join(p.root, p.recordsDir, ".tasks", bucket)
}

func (p workflowPaths) taskFile(bucket string, id string) string {
	return filepath.Join(p.tasksBucketDir(bucket), id+".md")
}

func (p workflowPaths) researchRel() string {
	if p.recordsDir == toolRecordsDirName {
		return p.recordsDir + "/research"
	}
	return p.recordsDir + "/.research"
}

func (p workflowPaths) execPlansRel(bucket string) string {
	if bucket == "" {
		return p.recordsDir + "/exec-plans"
	}
	return p.recordsDir + "/exec-plans/" + bucket
}

func (p workflowPaths) execPlansDir(bucket string) string {
	return filepath.Join(p.root, filepath.FromSlash(p.execPlansRel(bucket)))
}
