package ahm

// recordCache memoizes managed-record reads for the span of a single command
// so a later step can reuse what an earlier step already read from disk.
//
// The reuse is an explicit handoff, never a process-lifetime cache: index
// generation creates a cache and hands it to post-mutation validation, and
// standalone validation (status, doctor, check) creates a fresh one per run.
// Every command therefore still reads each record from disk at least once and
// still detects out-of-band edits and stale indexes.
//
// A nil *recordCache reads through to disk on every call, so callers with
// nothing to hand off pass nil.
//
// The cache is not safe for concurrent use; it is confined to one command's
// sequential generate-then-validate flow.
type recordCache struct {
	files    map[string]cachedFile
	sections map[string]map[string]execPlanSection

	// adrs memoizes the whole ADR collection rather than individual files,
	// so a second collectADRs call costs neither reads nor parses. adrsRoot
	// guards the entry against a cache reused across roots.
	adrsRoot   string
	adrsLoaded bool
	adrs       []ADR
	adrsErr    error
}

type cachedFile struct {
	data []byte
	err  error
}

func newRecordCache() *recordCache {
	return &recordCache{}
}

// readFile returns the contents of a managed workflow file, reading it from
// disk on the first call and serving later calls from memory. Read failures
// are memoized too, so a missing file is stat-ed once per command.
func (c *recordCache) readFile(path string) ([]byte, error) {
	if c == nil {
		return readWorkflowFile(path)
	}
	if entry, ok := c.files[path]; ok {
		return entry.data, entry.err
	}
	data, err := readWorkflowFile(path)
	if c.files == nil {
		c.files = map[string]cachedFile{}
	}
	c.files[path] = cachedFile{data: data, err: err}
	return data, err
}

// put records content the command just wrote to path, so a later read in the
// same command observes the new bytes instead of a stale memoized copy. Every
// write performed while a cache is live must be reported through put.
func (c *recordCache) put(path string, data []byte) {
	if c == nil {
		return
	}
	if c.files == nil {
		c.files = map[string]cachedFile{}
	}
	c.files[path] = cachedFile{data: data}
	delete(c.sections, path)
}

// execPlanSections returns the parsed sections of the ExecPlan at path, parsing
// each plan at most once per command.
func (c *recordCache) execPlanSections(path string) (map[string]execPlanSection, error) {
	if c == nil {
		return parseExecPlanSections(path)
	}
	if sections, ok := c.sections[path]; ok {
		return sections, nil
	}
	data, err := c.readFile(path)
	if err != nil {
		return nil, err
	}
	sections := parseExecPlanSectionsFromData(data)
	if c.sections == nil {
		c.sections = map[string]map[string]execPlanSection{}
	}
	c.sections[path] = sections
	return sections, nil
}

// adrList returns the ADRs under root, collecting them at most once per
// command. The returned slice is shared with later callers and must not be
// mutated.
func (c *recordCache) adrList(root string) ([]ADR, error) {
	if c == nil {
		return collectADRs(root)
	}
	if c.adrsLoaded && c.adrsRoot == root {
		return c.adrs, c.adrsErr
	}
	adrs, err := collectADRs(root)
	c.adrsRoot = root
	c.adrsLoaded = true
	c.adrs = adrs
	c.adrsErr = err
	return adrs, err
}
