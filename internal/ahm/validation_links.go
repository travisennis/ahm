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
	for _, path := range workflowMarkdownFilesForPaths(root, paths) {
		validateMarkdownFileLinks(paths, path, report)
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

func validateMarkdownFileLinks(paths workflowPaths, path string, report *validationReport) {
	data, err := report.cache.readFile(path)
	if err != nil {
		report.addWarning("markdown_link_check_failed", paths.displayPath(path), err.Error())
		return
	}
	walkMarkdownLinks(data, func(lineNo int, rawTarget string) {
		target := normalizeMarkdownLinkTarget(rawTarget)
		if target == "" || shouldSkipMarkdownLink(target) {
			return
		}
		err := markdownLinkTargetError(paths, path, target)
		switch {
		case err == nil:
			return
		case !errors.Is(err, os.ErrNotExist):
			report.addWarning("markdown_link_check_failed", fmt.Sprintf("%s:%d", paths.displayPath(path), lineNo), err.Error())
			return
		}
		report.addWarning("markdown_link_missing", fmt.Sprintf("%s:%d", paths.displayPath(path), lineNo), fmt.Sprintf("relative Markdown link target does not exist: %s", target))
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
