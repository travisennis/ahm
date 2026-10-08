package ahm

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStatusReportsMarkdownLinksInWorkflowFiles(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	linkPath := writeLinkCarrierTask(t, root, "001", "[missing](missing.md)\n\n```md\n[ignored](also-missing.md)\n```\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": "`+relPath(root, linkPath)+`:12"`,
		`relative Markdown link target does not exist: missing.md`,
	)
	assertNotContains(t, got, "also-missing.md")
}

// writeLinkCarrierTask writes a valid task whose body carries Markdown so
// link-scope tests exercise a surviving record family. The body starts on
// line 12 of the rendered file.
func writeLinkCarrierTask(t *testing.T, root string, id string, body string) string {
	t.Helper()
	path := workflowPathsFor(root).taskFile("active", id)
	writeFile(t, path, "---\n"+
		"id: "+id+"\n"+
		"title: Links\n"+
		"status: Pending\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"depends_on: -\n"+
		"---\n"+
		"# Links\n\n"+
		body)
	return path
}

func TestWalkMarkdownLinks(t *testing.T) {
	data := []byte("[first](one.md)\n" +
		"`[inline](ignored-inline.md)` [second](two.md)\n" +
		"```md\n[fenced](ignored-backtick.md)\n```\n" +
		"~~~md\n[fenced](ignored-tilde.md)\n~~~\n" +
		"![image](image.png)\n")

	type link struct {
		lineNo int
		target string
	}
	var got []link
	walkMarkdownLinks(data, func(lineNo int, target string) {
		got = append(got, link{lineNo: lineNo, target: target})
	})

	want := []link{
		{lineNo: 1, target: "one.md"},
		{lineNo: 2, target: "two.md"},
		{lineNo: 9, target: "image.png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("walkMarkdownLinks() = %#v, want %#v", got, want)
	}
}

func TestStatusReportsMarkdownLinksInWorkflowFilesWithCodeSpans(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	// Quoted example links inside inline code spans and fenced code blocks must
	// not be treated as navigation, but a real broken link on the same line
	// (outside any backticks) must still be reported.
	writeLinkCarrierTask(t, root, "001",
		"Span: `[ADRs](adr/index.md)` and span2: `[broken](also-missing.md)`.\n\n"+
			"```md\n[fenced](fenced-missing.md)\n```\n\n"+
			"[real](real-missing.md)\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`relative Markdown link target does not exist: real-missing.md`,
	)
	assertNotContains(t, got, "adr/index.md")
	assertNotContains(t, got, "also-missing.md")
	assertNotContains(t, got, "fenced-missing.md")
}

func TestValidateManagedRecordLinksByFamily(t *testing.T) {
	families := []struct {
		name       string
		dir        func(string, workflowPaths) string
		sourceName string
		targetName string
	}{
		{
			name: "tasks",
			dir: func(root string, paths workflowPaths) string {
				return filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "active")
			},
			sourceName: "001.md",
			targetName: "002.md",
		},
		{
			name: "adrs",
			dir: func(root string, _ workflowPaths) string {
				return filepath.Join(root, "docs", "adr")
			},
			sourceName: "001-links.md",
			targetName: "002-target.md",
		},
	}

	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			root := t.TempDir()
			setupAhmRepo(t, root)
			paths := workflowPathsFor(root)
			dir := family.dir(root, paths)
			writeFile(t, filepath.Join(dir, family.targetName), "# Target\n")
			writeFile(t, filepath.Join(dir, family.sourceName),
				"# Links\n\n[valid]("+family.targetName+")\n[missing](missing.md)\n")

			report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
			var findings []validationFinding
			for _, finding := range report.Warnings {
				if finding.Code == "markdown_link_missing" {
					findings = append(findings, finding)
				}
			}
			if len(findings) != 1 {
				t.Fatalf("markdown_link_missing findings = %#v, want one", findings)
			}
			wantPath := relPath(root, filepath.Join(dir, family.sourceName)) + ":4"
			if findings[0].Path != wantPath {
				t.Errorf("finding path = %q, want %q", findings[0].Path, wantPath)
			}
			if strings.Contains(findings[0].Message, family.targetName) {
				t.Errorf("valid link target was reported missing: %#v", findings[0])
			}
		})
	}
}

func TestValidateManagedRecordLinksIncludesGeneratedIndexes(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	indexes := []string{
		filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "index.md"),
		filepath.Join(root, "docs", "adr", "index.md"),
	}
	for _, path := range indexes {
		writeFile(t, path, "# Index\n\n[missing](missing.md)\n")
	}

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	got := map[string]bool{}
	for _, finding := range report.Warnings {
		if finding.Code == "markdown_link_missing" {
			got[strings.TrimSuffix(finding.Path, ":3")] = true
		}
	}
	for _, path := range indexes {
		rel := relPath(root, path)
		if !got[rel] {
			t.Errorf("missing generated-index link finding for %s: %#v", rel, report.Warnings)
		}
	}
	if len(got) != len(indexes) {
		t.Errorf("generated-index finding paths = %#v, want exactly %d", got, len(indexes))
	}
}

func TestValidateManagedRecordLinksExcludesProjectOwnedMarkdown(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	taskDir := filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "active")
	writeFile(t, filepath.Join(taskDir, "001.md"), "# Managed\n\n[missing](managed-missing.md)\n")

	for _, path := range []string{
		"README.md",
		"AGENTS.md",
		"CLAUDE.md",
		"ARCHITECTURE.md",
		"docs/guide.md",
		".agents/NOTES.md",
		".agents/skills/example/SKILL.md",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)),
			"# Project owned\n\n[missing]("+strings.ReplaceAll(path, "/", "-")+"-missing.md)\n")
	}
	for _, path := range []string{
		filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "README.md"),
		filepath.Join(root, "docs", "adr", "README.md"),
	} {
		writeFile(t, path, "# Preserved scaffold\n\n[missing](scaffold-missing.md)\n")
	}
	for _, path := range []string{
		filepath.Join(paths.tasksBucketDir("active"), "project-notes", "guide.md"),
	} {
		writeFile(t, path, "# Nested project notes\n\n[missing](nested-missing.md)\n")
	}

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	var findings []validationFinding
	for _, finding := range report.Warnings {
		if finding.Code == "markdown_link_missing" {
			findings = append(findings, finding)
		}
	}
	if len(findings) != 1 {
		t.Fatalf("markdown_link_missing findings = %#v, want only the managed task finding", findings)
	}
	if !strings.Contains(findings[0].Message, "managed-missing.md") {
		t.Errorf("unexpected managed link finding: %#v", findings[0])
	}
}

// TestValidateAhmReferences covers the ahm: reference scheme of ADR 027: each
// kind resolves by identity or by project-relative path, a reference that
// names nothing keeps markdown_link_missing, a malformed reference is
// markdown_link_invalid, and relative links keep their own resolution.
func TestValidateAhmReferences(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	writeTaskFile(t, paths.taskFile("active", "001"), "001", "Active", "Pending", "depends_on: -\n")
	writeTaskFile(t, paths.taskFile("completed", "002"), "002", "Completed", "Completed", "depends_on: -\n")
	// Two children whose only short form is ambiguous: "4" prefixes both.
	writeTaskFile(t, paths.taskFile("active", "004a"), "004a", "Child A", "Pending", "depends_on: -\n")
	writeTaskFile(t, paths.taskFile("active", "004b"), "004b", "Child B", "Pending", "depends_on: -\n")
	writeADRFile(t, root, "001-example.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# Example\n\nBody.\n")
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")

	cases := []struct {
		target string
		code   string // empty when the reference must resolve
	}{
		{"ahm:task/001", ""},
		{"ahm:task/1", ""},
		{"ahm:task/002", ""},
		{"ahm:task/999", "markdown_link_missing"},
		{"ahm:task/4", "markdown_link_invalid"},
		{"AHM:TASK/001", ""},
		{"<ahm:task/001>", ""},
		{"ahm:adr/001", ""},
		{"ahm:adr/1", ""},
		{"ahm:adr/001-example", ""},
		{"ahm:adr/999", "markdown_link_missing"},
		{"ahm:adr/001-wrong-slug", "markdown_link_missing"},
		{"ahm:doc/docs/guide.md", ""},
		{"ahm:doc/docs/guide.md#anchor", ""},
		{"ahm:doc/docs/missing.md", "markdown_link_missing"},
		{"ahm:doc/../outside.md", "markdown_link_invalid"},
		{"ahm:doc/", "markdown_link_invalid"},
		{"ahm:task", "markdown_link_invalid"},
		{"ahm:widget/1", "markdown_link_invalid"},
		{"relative-missing.md", "markdown_link_missing"},
	}
	var body strings.Builder
	for _, c := range cases {
		fmt.Fprintf(&body, "[%s](%s)\n", c.target, c.target)
	}
	carrier := writeLinkCarrierTask(t, root, "003", body.String())

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	byLocation := map[string][]string{}
	for _, finding := range report.Warnings {
		byLocation[finding.Path] = append(byLocation[finding.Path], finding.Code)
	}
	carrierRel := relPath(root, carrier)
	for i, c := range cases {
		location := fmt.Sprintf("%s:%d", carrierRel, 12+i)
		got := byLocation[location]
		if c.code == "" {
			if len(got) != 0 {
				t.Errorf("%s (%s) = %#v, want no finding", c.target, location, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != c.code {
			t.Errorf("%s (%s) codes = %#v, want [%s]", c.target, location, got, c.code)
		}
	}
}

// TestValidateAhmReferencesSurviveBucketMoves pins the ADR 027 acceptance
// criterion that an ahm: reference keeps resolving after its target moves
// between lifecycle buckets: the reference names the record, not its path.
func TestValidateAhmReferencesSurviveBucketMoves(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	writeTaskFile(t, paths.taskFile("active", "002"), "002", "Moves", "Pending", "depends_on: -\n")
	writeLinkCarrierTask(t, root, "001", "[moves](ahm:task/002)\n")

	findings := func() []validationFinding {
		t.Helper()
		report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
		return report.Warnings
	}
	if got := findings(); len(got) != 0 {
		t.Fatalf("findings before the move = %#v, want none", got)
	}

	// Move the target between buckets by hand: the same bytes under the
	// completed bucket.
	data, err := os.ReadFile(paths.taskFile("active", "002"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.taskFile("active", "002")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, paths.taskFile("completed", "002"), string(data))

	if got := findings(); len(got) != 0 {
		t.Fatalf("findings after the move = %#v, want none", got)
	}
}

// TestValidateAhmReferencesDoNotWidenTheScanScope pins that the ahm: scheme
// does not extend link validation to project-owned Markdown: a broken
// reference there is not reported, exactly as a broken relative link is not.
func TestValidateAhmReferencesDoNotWidenTheScanScope(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	writeFile(t, filepath.Join(root, "README.md"), "# Readme\n\n[missing](ahm:task/999)\n")
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n\n[missing](ahm:adr/999)\n")

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	for _, finding := range report.Warnings {
		if finding.Code == "markdown_link_missing" || finding.Code == "markdown_link_invalid" {
			t.Errorf("project-owned Markdown was link-checked: %#v", finding)
		}
	}
}

func TestSplitAhmLinkTarget(t *testing.T) {
	cases := []struct {
		target string
		kind   string
		value  string
		ok     bool
	}{
		{"ahm:task/258", "task", "258", true},
		{"ahm:adr/001-example", "adr", "001-example", true},
		{"ahm:doc/docs/guide.md", "doc", "docs/guide.md", true},
		{"AHM:Task/263a", "task", "263a", true},
		{"ahm:task", "", "", false},
		{"ahmfoo.md", "", "", false},
		{"docs/adr/027-example.md", "", "", false},
	}
	for _, c := range cases {
		kind, value, ok := splitAhmLinkTarget(c.target)
		if kind != c.kind || value != c.value || ok != c.ok {
			t.Errorf("splitAhmLinkTarget(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.target, kind, value, ok, c.kind, c.value, c.ok)
		}
	}
	for _, target := range []string{"ahm:task/258", "AHM:doc/x.md", "ahm:"} {
		if !isAhmLinkTarget(target) {
			t.Errorf("isAhmLinkTarget(%q) = false, want true", target)
		}
	}
	for _, target := range []string{"ahmfoo.md", "docs/ahm:task.md", ""} {
		if isAhmLinkTarget(target) {
			t.Errorf("isAhmLinkTarget(%q) = true, want false", target)
		}
	}
}

// TestValidateLinksSkipsRetiredRecordFamilies pins acceptance criterion one of
// task 264b: nothing in the retired research and ExecPlan trees is read any
// more, so a broken link inside one of them is never reported and deleting the
// directory changes no ahm behavior.
func TestValidateLinksSkipsRetiredRecordFamilies(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	for _, file := range []string{
		".ahm/research/inbox/note.md",
		".ahm/research/topics/note.md",
		".ahm/research/index.md",
		".ahm/exec-plans/active/note.md",
		".ahm/exec-plans/active/index.md",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(file)),
			"# Retired record\n\n[missing](missing.md)\n")
	}

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	for _, finding := range report.Warnings {
		if finding.Code == "markdown_link_missing" {
			t.Errorf("retired record family was link-checked: %#v", finding)
		}
	}
}

func TestStatusAndDoctorManagedLinksScopesAndOutputModes(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeADRFile(t, root, "001-links.md",
		"---\nstatus: accepted\ndate: 2026-07-27\n---\n"+
			"# Links\n\n[missing](missing.md)\n")
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "status default text", args: []string{"status"}},
		{name: "doctor default text", args: []string{"doctor"}},
		{name: "status links JSON", args: []string{"--json", "status", "--check", "links"}},
		{name: "doctor links plain", args: []string{"--plain", "doctor", "--check", "links"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--root", root}, tt.args...)
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}
			assertContainsAll(t, stdout,
				"markdown_link_missing",
				"docs/adr/001-links.md:7",
				"relative Markdown link target does not exist: missing.md",
			)
		})
	}
}
