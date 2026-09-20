package ahm

import "path/filepath"

// toolRecordsDirName is the tool-owned directory that holds ahm workflow
// state: committed records, configuration, the managed .gitignore, generated
// indexes, and the workflow lock.
const toolRecordsDirName = ".ahm"

// workflowPaths derives the ahm-managed record paths for one repository root.
// There is exactly one layout: records live under the tool-owned .ahm/
// directory. The legacy .agents/ layout is not readable by this version, and
// root detection rejects it before any path is derived.
type workflowPaths struct {
	root string
}

func workflowPathsFor(root string) workflowPaths {
	return workflowPaths{root: root}
}

func (p workflowPaths) tasksRel() string {
	return toolRecordsDirName + "/tasks"
}

func (p workflowPaths) tasksBucketDir(bucket string) string {
	return filepath.Join(p.root, toolRecordsDirName, "tasks", bucket)
}

func (p workflowPaths) taskFile(bucket string, id string) string {
	return filepath.Join(p.tasksBucketDir(bucket), id+".md")
}
