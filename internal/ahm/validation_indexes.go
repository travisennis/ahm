package ahm

import (
	"errors"
	"fmt"
	"os"
)

func validateGeneratedIndexes(root string, paths workflowPaths, tasks []Task, report *validationReport) {
	if !validateGeneratedIndexMetadata(paths, report) {
		return
	}
	writes, err := indexWritesForPaths(root, tasks, paths, report.cache)
	if err != nil {
		if writes == nil {
			report.addWarning("generated_index_check_failed", "", err.Error())
			return
		}
		// validateADRs reports each malformed ADR. Keep checking the indexes
		// rendered from the readable records instead of duplicating that finding.
	}
	validateGeneratedIndexWrites(paths, writes, report)
}

// validateGeneratedIndexMetadata reports whether generated indexes should be
// checked: only when the workflow metadata that owns them is present. A
// records-only layout has no project metadata to read, and its task indexes live
// with the records, so they are always checked.
func validateGeneratedIndexMetadata(paths workflowPaths, report *validationReport) bool {
	if paths.isRecordsOnly() {
		return true
	}
	if _, err := report.readMetadata(paths.projectRoot); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", configMetadataRelPath, fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
		return false
	}
	return true
}

func validateGeneratedIndexWrites(paths workflowPaths, writes map[string]string, report *validationReport) {
	for _, path := range sortedKeys(writes) {
		data, err := report.cache.readFile(path)
		if errors.Is(err, os.ErrNotExist) {
			report.addError("generated_index_missing", paths.displayPath(path), "generated index is missing; run ahm index")
			continue
		}
		if err != nil {
			report.addError("generated_index_unreadable", paths.displayPath(path), err.Error())
			continue
		}
		if string(data) != writes[path] {
			report.addWarning("generated_index_stale", paths.displayPath(path), "generated index is stale; run ahm index")
		}
	}
}
