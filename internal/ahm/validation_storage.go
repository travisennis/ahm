package ahm

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func validateMetadata(root string, report *validationReport) error {
	_, metaErr := report.readMetadata(root)
	if metaErr != nil {
		if errors.Is(metaErr, os.ErrNotExist) {
			report.addError("metadata_missing", configMetadataRelPath, "workflow metadata is missing")
		} else {
			report.addError("metadata_corrupt", configMetadataRelPath, fmt.Sprintf("workflow metadata is corrupt: %v", metaErr))
		}
	}
	return metaErr
}

func validateManagedFiles(root string, paths workflowPaths, report *validationReport) []Task {
	// A records-only layout has no checkout to read metadata or records-in-project
	// drift from, so it checks only the store directory that holds the records.
	if paths.isRecordsOnly() {
		validateStoreReadable(paths, report)
	} else {
		metaErr := validateMetadata(root, report)
		validateRecordLocation(paths, metaErr, report)
	}
	tasks := validateTaskFiles(paths, report)
	validateTaskDuplicateIDs(paths, tasks, report)
	validateTaskDuplicateTitles(paths, tasks, report)
	return tasks
}

// validateRecordLocation reports drift between the configured storage location
// and where the records actually are. It only reads: it never moves a record.
func validateRecordLocation(paths workflowPaths, metaErr error, report *validationReport) {
	if !paths.inStore() {
		return
	}
	// Records left in the project are drift whether or not the configuration is
	// present: the resolved mode is home, so no command reads them there.
	validateRecordsNotInProject(paths, report)
	// The store's directories appear with the first command that writes into
	// them, so a repository whose configuration is missing has not been installed
	// yet and cannot be missing them; `metadata_missing` already describes it.
	if !errors.Is(metaErr, os.ErrNotExist) {
		validateStoreReadable(paths, report)
	}
}

// validateRecordsNotInProject reports task records that are still in the
// project while the mode is home. No command reads them there, so the mode and
// the records disagree and the finding is an error rather than a warning. The
// generated task indexes inside the project are ignored: they are derived,
// ignored by Git, and regenerated in the store.
func validateRecordsNotInProject(paths workflowPaths, report *validationReport) {
	project := workflowPathsFor(paths.projectRoot)
	records := 0
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		dir := project.tasksBucketDir(bucket)
		// Stat before the read so a bucket path that exists but is not a
		// directory is reported the same way on every platform. os.ReadDir
		// cannot carry that distinction on Windows: syscall.ENOTDIR is
		// ERROR_PATH_NOT_FOUND there, and Errno.Is accepts it as
		// os.ErrNotExist, so a regular file at the bucket's path would read as
		// absent and be skipped. adrFilePaths and taskFilePathsFor carry the
		// same guard for the same reason.
		info, err := os.Stat(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			report.addError("task_records_in_project", project.displayPath(dir), fmt.Sprintf("could not read %s while tasks_location is home: %v", project.displayPath(dir), err))
			continue
		}
		if !info.IsDir() {
			report.addError("task_records_in_project", project.displayPath(dir), fmt.Sprintf("%s is not a directory while tasks_location is home", project.displayPath(dir)))
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			report.addError("task_records_in_project", project.displayPath(dir), fmt.Sprintf("could not read %s while tasks_location is home: %v", project.displayPath(dir), err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && entry.Name() != "index.md" && strings.HasSuffix(entry.Name(), ".md") {
				records++
			}
		}
	}
	if records == 0 {
		return
	}
	message := fmt.Sprintf("%d task records remain in the project while tasks_location is home", records)
	if records == 1 {
		message = "a task record remains in the project while tasks_location is home"
	}
	report.addError("task_records_in_project", project.displayPath(project.tasksBucketDir("")), message)
}

// validateStoreReadable reports a home-mode project whose store records
// directory is missing or unreadable. Every command then sees an empty
// backlog, so the finding is an error rather than a silently empty task list.
func validateStoreReadable(paths workflowPaths, report *validationReport) {
	_, err := os.ReadDir(paths.recordsRoot)
	if err == nil {
		return
	}
	message := "the store's task records directory is missing"
	if !errors.Is(err, os.ErrNotExist) {
		message = fmt.Sprintf("the store's task records directory could not be read: %v", err)
	}
	report.addError("store_dir_unreadable", paths.displayPath(paths.recordsRoot), message)
}
