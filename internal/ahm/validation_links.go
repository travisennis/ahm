package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

var markdownLinkSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// inlineCodeSpanPattern matches inline code span content delimited by single
// backticks. Quoted example links written inside backticks (for example a
// span containing a markdown link) are text, not navigation, so the link
// extractor strips them before matching link syntax. Fenced code blocks are
// handled separately by the fence-tracking logic above this call.
var inlineCodeSpanPattern = regexp.MustCompile("`[^`]*`")

// stripInlineCodeSpans removes inline code span content from a line so the
// link extractor does not treat quoted example links inside backticks as
// navigation. Backticks are removed in pairs; an unmatched trailing backtick
// is left untouched.
func stripInlineCodeSpans(line string) string {
	return inlineCodeSpanPattern.ReplaceAllString(line, "")
}

// walkMarkdownLinks calls visit for each raw Markdown link target outside
// fenced and inline code, with the target's one-based source line number.
func walkMarkdownLinks(data []byte, visit func(lineNo int, target string)) {
	inFence := false
	for lineNo, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(stripInlineCodeSpans(line), -1) {
			visit(lineNo+1, match[1])
		}
	}
}

func validateMarkdownLinks(root string, paths workflowPaths, report *validationReport) {
	if _, err := report.readMetadata(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			report.addError("metadata_corrupt", configMetadataRelPath, fmt.Sprintf("workflow metadata is corrupt: %v", err))
		}
		return
	}
	index := newLinkReferenceIndex(root, paths, report)
	for _, path := range workflowMarkdownFilesForPaths(root, paths) {
		validateMarkdownFileLinks(paths, index, path, report)
	}
}

func workflowMarkdownFilesForPaths(root string, resolved workflowPaths) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}
	sourceDirs := []string{
		resolved.tasksBucketDir("active"),
		resolved.tasksBucketDir("completed"),
		resolved.tasksBucketDir("cancelled"),
	}
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "index.md" || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			add(filepath.Join(dir, entry.Name()))
		}
	}
	if adrPaths, err := adrFilePaths(root); err == nil {
		for _, path := range adrPaths {
			add(path)
		}
	}
	indexPaths := []string{
		filepath.Join(resolved.tasksBucketDir(""), "index.md"),
		filepath.Join(resolved.tasksBucketDir("active"), "index.md"),
		filepath.Join(resolved.tasksBucketDir("completed"), "index.md"),
		filepath.Join(resolved.tasksBucketDir("cancelled"), "index.md"),
		resolved.adrIndexPath(),
	}
	for _, path := range indexPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			add(path)
		}
	}
	sort.Strings(paths)
	return paths
}

func validateMarkdownFileLinks(paths workflowPaths, index *linkReferenceIndex, path string, report *validationReport) {
	data, err := report.cache.readFile(path)
	if err != nil {
		report.addWarning("markdown_link_check_failed", paths.displayPath(path), err.Error())
		return
	}
	walkMarkdownLinks(data, func(lineNo int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" {
			return
		}
		where := fmt.Sprintf("%s:%d", paths.displayPath(path), lineNo)
		if isAhmLinkTarget(target) {
			index.resolveAhmTarget(target, where, report)
			return
		}
		if shouldSkipMarkdownLink(target) {
			return
		}
		err := markdownLinkTargetError(paths, path, target)
		switch {
		case err == nil:
			return
		case !errors.Is(err, os.ErrNotExist):
			report.addWarning("markdown_link_check_failed", where, err.Error())
			return
		}
		report.addWarning("markdown_link_missing", where, fmt.Sprintf("relative Markdown link target does not exist: %s", target))
	})
}

// markdownLinkTargetError resolves one relative Markdown link from a record.
// The target is resolved against the record's own directory first and, only
// when that target does not exist, against the record's logical in-project
// directory, so a record that moved into the store keeps every link that was
// written under the committed-in-project model working (ADR 023). It returns
// nil when the target exists and the first resolution's error otherwise.
func markdownLinkTargetError(paths workflowPaths, path string, target string) error {
	err := statMarkdownLinkTarget(filepath.Dir(path), target)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return err
	}
	logical, ok := paths.inProjectRecordPath(path)
	if !ok {
		return err
	}
	if logicalErr := statMarkdownLinkTarget(filepath.Dir(logical), target); logicalErr == nil {
		return nil
	}
	return err
}

// statMarkdownLinkTarget stats a link target resolved from one directory.
func statMarkdownLinkTarget(dir string, target string) error {
	_, err := os.Stat(filepath.Clean(filepath.Join(dir, filepath.FromSlash(target))))
	return err
}

func normalizeMarkdownLinkTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "<") {
		if end := strings.Index(target, ">"); end >= 0 {
			target = target[1:end]
		}
	} else if fields := strings.Fields(target); len(fields) > 0 {
		target = fields[0]
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func shouldSkipMarkdownLink(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") || filepath.IsAbs(target) {
		return true
	}
	if strings.HasPrefix(target, "mailto:") {
		return true
	}
	return markdownLinkSchemePattern.MatchString(target)
}

// ahmLinkScheme prefixes the resolvable reference form ADR 027 defines:
// ahm:<kind>/<target>. A reference names a workflow record by identity
// (ahm:task/<id>, ahm:adr/<ref>) or a project file by its repository-relative
// path (ahm:doc/<path>), so it keeps resolving after the target moves between
// lifecycle buckets or storage layouts. Relative paths keep their own
// resolution and are supported alongside the scheme.
const ahmLinkScheme = "ahm:"

// errAhmReferenceInvalid marks a malformed ahm: reference, as opposed to one
// that follows the scheme but names nothing, so validation can report the two
// as different findings.
var errAhmReferenceInvalid = errors.New("invalid ahm reference")

// isAhmLinkTarget reports whether a normalized link target uses the ahm:
// reference scheme. The scheme is matched case-insensitively, as URL schemes
// are; the canonical spelling is lower case.
func isAhmLinkTarget(target string) bool {
	return len(target) >= len(ahmLinkScheme) && strings.EqualFold(target[:len(ahmLinkScheme)], ahmLinkScheme)
}

// splitAhmLinkTarget parses an ahm: reference into its kind and value. It
// returns ok=false when the target omits the kind/value separator.
func splitAhmLinkTarget(target string) (kind string, value string, ok bool) {
	if !isAhmLinkTarget(target) {
		return "", "", false
	}
	kind, value, found := strings.Cut(target[len(ahmLinkScheme):], "/")
	if !found {
		return "", "", false
	}
	return strings.ToLower(kind), value, true
}

// linkReferenceIndex resolves ahm: references. Task and ADR records are
// collected once per validation run through the report's caches, so resolving
// a reference reuses the reads the rest of validation already made.
type linkReferenceIndex struct {
	root  string
	tasks []Task
	adrs  []ADR
}

// newLinkReferenceIndex collects the records an ahm: reference can name. A
// record that cannot be read or parsed is left out rather than failing the
// link check; the record's own validator reports the parse problem.
func newLinkReferenceIndex(root string, paths workflowPaths, report *validationReport) *linkReferenceIndex {
	index := &linkReferenceIndex{root: root}
	if files, err := taskFilePathsFor(paths); err == nil {
		for _, file := range files {
			data, err := report.cache.readFile(file.Path)
			if err != nil {
				continue
			}
			task, err := parseTaskFromData(data, file.Path, file.Bucket)
			if err != nil {
				continue
			}
			index.tasks = append(index.tasks, task)
		}
	}
	adrs, _ := report.cache.adrList(root)
	index.adrs = adrs
	return index
}

// resolveAhmTarget resolves one ahm: reference and reports a finding when it
// is malformed or names nothing. Task and ADR references resolve through the
// same resolvers the CLI uses, so a reference is exactly as resolvable as the
// command that would open it.
func (index *linkReferenceIndex) resolveAhmTarget(target string, where string, report *validationReport) {
	kind, value, ok := splitAhmLinkTarget(target)
	if !ok || value == "" {
		report.addWarning("markdown_link_invalid", where, "ahm reference must name a kind and a target: "+target)
		return
	}
	switch kind {
	case "task":
		// The task resolver reports "not found" or "ambiguous", so a target
		// that names several records is reported as invalid rather than missing.
		if _, err := resolveTaskFromTasks(value, index.tasks); err != nil {
			if isTaskNotFoundError(err) {
				report.addWarning("markdown_link_missing", where, "ahm reference target does not exist: "+target)
				return
			}
			report.addWarning("markdown_link_invalid", where, "ahm reference is ambiguous: "+target)
		}
	case "adr":
		if _, err := resolveADR(value, index.adrs); err != nil {
			report.addWarning("markdown_link_missing", where, "ahm reference target does not exist: "+target)
		}
	case "doc":
		switch err := index.docTargetError(value); {
		case err == nil:
		case errors.Is(err, errAhmReferenceInvalid):
			report.addWarning("markdown_link_invalid", where, "ahm reference is malformed: "+target)
		case errors.Is(err, os.ErrNotExist):
			report.addWarning("markdown_link_missing", where, "ahm reference target does not exist: "+target)
		default:
			// A stat failure that is not "missing" is a check failure, exactly
			// as the relative-link path reports it.
			report.addWarning("markdown_link_check_failed", where, err.Error())
		}
	default:
		report.addWarning("markdown_link_invalid", where, "ahm reference has an unknown kind: "+target)
	}
}

// docTargetError resolves an ahm:doc/ value against the project root. An
// absolute value, or one that escapes the root, is malformed rather than
// missing.
func (index *linkReferenceIndex) docTargetError(value string) error {
	if strings.HasPrefix(value, "/") || filepath.IsAbs(filepath.FromSlash(value)) {
		return errAhmReferenceInvalid
	}
	target := filepath.Join(index.root, filepath.FromSlash(value))
	if !pathWithin(index.root, target) {
		return errAhmReferenceInvalid
	}
	_, err := os.Stat(filepath.Clean(target))
	return err
}
