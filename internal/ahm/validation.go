package ahm

// CheckScope values for validation scopes.
const (
	CheckScopeWorkflow = "workflow"
	CheckScopeLinks    = "links"
)

// validCheckScopes returns the list of recognised check-scope values.
func validCheckScopes() []string {
	return []string{CheckScopeWorkflow, CheckScopeLinks}
}

func (a *app) validateWorkflow(scopes []string) (validationReport, []Task) {
	return validateWorkflowScopedForPathsWithCache(a.opts.root, scopes, a.workflowPaths(), newRecordCache(), a.ensureMetadataCache())
}

func validateWorkflowScopedForPaths(root string, scopes []string, paths workflowPaths) (validationReport, []Task) {
	return validateWorkflowScopedForPathsWithCache(root, scopes, paths, newRecordCache(), nil)
}

// validateWorkflowScopedForPathsWithCache is the disk-reading validation path.
// The cache only dedupes reads within this one run — status and doctor pass a
// fresh cache, so they still see the current on-disk state and still report
// out-of-band edits and stale indexes. meta behaves the same way for the
// committed configuration: nil reads it fresh on every call, while a shared
// cache pins every validator to one observed configuration.
func validateWorkflowScopedForPathsWithCache(root string, scopes []string, paths workflowPaths, cache *recordCache, meta *metadataCache) (validationReport, []Task) {
	report := newValidationReport(cache, meta)

	all := len(scopes) == 0
	want := func(s string) bool { return all || containsScope(scopes, s) }

	var tasks []Task
	if want(CheckScopeWorkflow) {
		tasks = validateManagedFiles(root, paths, &report)
		validateTaskDependencies(paths, tasks, &report)
		validateBlockedDepsComplete(paths, tasks, &report)
		validateBlockedReason(paths, tasks, &report)
		validateTrackingChildrenComplete(paths, tasks, &report)
		validateTaskBuckets(paths, tasks, &report)
		// A records-only layout has no checkout, so ADR and generated-index checks
		// that read the project are skipped; the task records and their indexes are
		// still validated.
		if !paths.isRecordsOnly() {
			validateADRs(root, &report)
		}
		validateGeneratedIndexes(root, paths, tasks, &report)
	}
	if want(CheckScopeLinks) && !paths.isRecordsOnly() {
		validateMarkdownLinks(root, paths, &report)
	}
	report.OK = len(report.Errors) == 0
	return report, tasks
}

// validateWorkflowStateForPaths validates a complete, already-parsed task set
// and already-rendered generated indexes. Mutation paths use it only when the
// task parse had no errors; standalone status and doctor keep the independent
// disk-reading path above.
//
// cache carries the records the caller's index generation already read, so this
// pass reuses them instead of re-reading. meta carries the command's shared
// configuration cache, so this pass observes the same .ahm/config.json as the
// steps around it. Pass nil for either to read everything fresh.
func validateWorkflowStateForPaths(root string, paths workflowPaths, tasks []Task, writes map[string]string, cache *recordCache, meta *metadataCache) validationReport {
	report := newValidationReport(cache, meta)
	if !paths.isRecordsOnly() {
		_ = validateMetadata(root, &report)
	}
	validateTaskDuplicateIDs(paths, tasks, &report)
	for _, task := range tasks {
		validateTaskFrontMatterMeta(task.meta, paths.displayPath(task.Path), &report)
		validateTaskAcceptance(paths, task, &report)
	}
	validateTaskDependencies(paths, tasks, &report)
	validateBlockedDepsComplete(paths, tasks, &report)
	validateBlockedReason(paths, tasks, &report)
	validateTrackingChildrenComplete(paths, tasks, &report)
	validateTaskBuckets(paths, tasks, &report)
	if !paths.isRecordsOnly() {
		validateADRs(root, &report)
	}
	if validateGeneratedIndexMetadata(paths, &report) {
		validateGeneratedIndexWrites(paths, writes, &report)
	}
	report.OK = len(report.Errors) == 0
	return report
}

func containsScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// emitPostMutationFindings runs workflow-scope validation after a successful
// non-dry-run mutation and emits findings as warnings. It uses only the
// workflow scope so that markdown-link false positives do not drown out core
// workflow drift. The validation runs on every writeIndexes call, which
// covers task create, lifecycle commands, dep updates, comments, ADR lifecycle
// commands, and explicit ahm index.
//
// cache carries the records the index generation for this mutation already
// read, including the generated indexes it just wrote, so this pass reads none
// of them a second time.
func (a *app) emitPostMutationFindings(tasks []Task, writes map[string]string, reuseState bool, cache *recordCache) {
	if a.opts.dryRun {
		return
	}
	var report validationReport
	if reuseState {
		report = validateWorkflowStateForPaths(a.opts.root, a.workflowPaths(), tasks, writes, cache, a.ensureMetadataCache())
	} else {
		report, _ = validateWorkflowScopedForPathsWithCache(a.opts.root, []string{CheckScopeWorkflow}, a.workflowPaths(), cache, a.ensureMetadataCache())
	}
	for _, finding := range report.Errors {
		a.addWarning("%s", finding.Message)
	}
	for _, finding := range report.Warnings {
		a.addWarning("%s", finding.Message)
	}
}
