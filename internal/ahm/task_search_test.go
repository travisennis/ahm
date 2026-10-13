package ahm

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTaskSearchMatchesBody(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, ".ahm", "tasks", "active")
	// 001 matches only in the body, 002 matches in both, 003 matches neither,
	// and 004 is a body-only match that outranks 002 on priority.
	writeTaskFileWithBody(t, filepath.Join(dir, "001.md"), "001", "Add timeout handling", "Pending", "P2", "type:feature, area:cli", "The worker can hijack a slot and stall.")
	writeTaskFileWithBody(t, filepath.Join(dir, "002.md"), "002", "Document hijack behavior", "Open", "P2", "type:docs, area:docs", "Explains how a hijack is recorded.")
	writeTaskFileWithBody(t, filepath.Join(dir, "003.md"), "003", "Refresh index", "Pending", "P2", "type:task, area:cli", "Nothing relevant here.")
	writeTaskFileWithBody(t, filepath.Join(dir, "004.md"), "004", "Backfill worker", "Pending", "P0", "type:task, area:cli", "Can also hijack under load.")

	t.Run("returns a body-only hit", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling", "002 [Open] P2 S Document hijack behavior", "004 [Pending] P0 S Backfill worker")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("body match is case-insensitive", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("HIJACK", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending]", "002 [Open]")
	})

	t.Run("lists title matches before body-only matches", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		title := strings.Index(got, "002 [Open]")
		if title < 0 {
			t.Fatalf("expected the title match, got %q", got)
		}
		// Grouping outranks priority: the P2 title match precedes the P0
		// body-only match.
		for _, body := range []string{"001 [Pending]", "004 [Pending]"} {
			at := strings.Index(got, body)
			if at < 0 {
				t.Fatalf("expected %s in output: %q", body, got)
			}
			if title > at {
				t.Errorf("title match should precede body-only match %s: %q", body, got)
			}
		}
	})

	t.Run("a task matching in both fields appears once", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		if n := strings.Count(out.String(), "002 [Open]"); n != 1 {
			t.Errorf("expected 002 once, got %d:\n%s", n, out.String())
		}
	})

	t.Run("body hit composes with status filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", []string{"Open"}, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Open] P2 S Document hijack behavior")
		assertNotContains(t, got, "001 [Pending]", "004 [Pending]")
	})

	t.Run("body hit composes with label filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, []string{"area:docs"}, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Open] P2 S Document hijack behavior")
		assertNotContains(t, got, "001 [Pending]", "004 [Pending]")
	})

	t.Run("json output carries the matched body", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root, json: true}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		// 001 matches only through its body, and its title has no "hijack",
		// so this text can only appear if body search fed the JSON output.
		assertContainsAll(t, out.String(), "\"body\":", "The worker can hijack a slot and stall.")
	})

	t.Run("a miss matches no task", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("zebra", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		if strings.TrimSpace(out.String()) != "No tasks found." {
			t.Errorf("unexpected output: %q", out.String())
		}
	})
}

// searchGrammarFixtures writes records whose titles and bodies distinguish AND
// term matching from the old adjacent-phrase match. 001 matches both terms in
// its body, 005 matches one term in each field, 006 matches both terms in its
// title, 002 and 004 match only one term, and 003 matches neither.
func searchGrammarFixtures(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ".ahm", "tasks", "active")
	writeTaskFileWithBody(t, filepath.Join(dir, "001.md"), "001", "Add timeout handling", "Pending", "P2", "type:feature, area:cli", "The worker can hijack a slot and stall.")
	writeTaskFileWithBody(t, filepath.Join(dir, "002.md"), "002", "Document hijack behavior", "Open", "P2", "type:docs, area:docs", "Explains how a hijack is recorded.")
	writeTaskFileWithBody(t, filepath.Join(dir, "003.md"), "003", "Refresh index", "Pending", "P2", "type:task, area:cli", "Nothing relevant here.")
	writeTaskFileWithBody(t, filepath.Join(dir, "004.md"), "004", "Backfill worker", "Pending", "P0", "type:task, area:cli", "Can also hijack under load.")
	writeTaskFileWithBody(t, filepath.Join(dir, "005.md"), "005", "Stall playbook", "Pending", "P2", "type:task, area:cli", "Hijack metrics live here.")
	writeTaskFileWithBody(t, filepath.Join(dir, "006.md"), "006", "Hijack stall review", "Pending", "P2", "type:task, area:cli", "No extra context.")
}

func TestTaskSearchANDTerms(t *testing.T) {
	root := projectRoot(t)
	searchGrammarFixtures(t, root)

	t.Run("every term must match", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack stall", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending]", "005 [Pending]", "006 [Pending]")
		assertNotContains(t, got, "002 [Open]", "003 [Pending]", "004 [Pending]")
	})

	t.Run("terms match anywhere, not only adjacent", func(t *testing.T) {
		for _, query := range []string{"hijack slot", "slot hijack"} {
			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			if err := a.taskSearch(query, nil, nil, nil, nil, "", false); err != nil {
				t.Error(err)
			}
			got := out.String()
			assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling")
			assertNotContains(t, got, "005 [Pending]", "006 [Pending]")
		}
	})

	t.Run("multi-term match is case-insensitive", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("HIJACK STALL", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending]", "005 [Pending]", "006 [Pending]")
	})

	t.Run("terms match inside words", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("ijack tall", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending]", "005 [Pending]", "006 [Pending]")
	})

	t.Run("title matches precede body matches", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack stall", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		// 006 is a title match; 001 and 005 match without every term in the
		// title, so they follow in priority-then-ID order even though 004
		// (excluded here) is P0.
		assertOrderedStrings(t, out.String(), "006 [Pending]", "001 [Pending]", "005 [Pending]")
	})
}

func TestTaskSearchQuotedPhrase(t *testing.T) {
	root := projectRoot(t)
	searchGrammarFixtures(t, root)

	t.Run("quoted span matches an adjacent phrase", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch(`"hijack a slot"`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling")
		assertNotContains(t, got, "005 [Pending]", "006 [Pending]")
	})

	t.Run("phrase order is literal", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch(`"hijack a slot"`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending] P2 S Add timeout handling")

		out.Reset()
		if err := a.taskSearch(`"slot a hijack"`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		if strings.TrimSpace(out.String()) != "No tasks found." {
			t.Errorf("reversed phrase should not match: %q", out.String())
		}
	})

	t.Run("phrase matches in the title", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch(`"hijack stall"`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "006 [Pending] P2 S Hijack stall review")
		assertNotContains(t, got, "001 [Pending]", "005 [Pending]")
	})

	t.Run("phrase is case-insensitive", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch(`"HIJACK A SLOT"`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		assertContainsAll(t, out.String(), "001 [Pending] P2 S Add timeout handling")
	})

	t.Run("phrase composes with other terms", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch(`"hijack a slot" stall`, nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending]")
		assertNotContains(t, got, "005 [Pending]", "006 [Pending]")
	})
}

func TestTaskSearchQueryErrors(t *testing.T) {
	root := projectRoot(t)
	searchGrammarFixtures(t, root)

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "unterminated quote", query: `"foo`, want: "unterminated double quote"},
		{name: "quote glued to leading text", query: `foo"bar`, want: "must delimit a whole term"},
		{name: "quote glued to trailing text", query: `"foo"bar`, want: "must delimit a whole term"},
		{name: "quote glued to a term end", query: `foo"`, want: "must delimit a whole term"},
		{name: "empty quoted span", query: `""`, want: "empty quoted term"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			err := a.taskSearch(tc.query, nil, nil, nil, nil, "", false)
			if err == nil {
				t.Fatalf("expected a usage error for %q", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v does not name %q", err, tc.want)
			}
		})
	}

	t.Run("well-formed queries parse", func(t *testing.T) {
		cases := []struct {
			query string
			want  []string
		}{
			{query: `"hijack a slot" stall`, want: []string{"hijack a slot", "stall"}},
			{query: `foo "bar baz"`, want: []string{"foo", "bar baz"}},
			{query: `"Two Words"`, want: []string{"two words"}},
		}
		for _, tc := range cases {
			got, err := parseTaskSearchQuery(tc.query)
			if err != nil {
				t.Errorf("query %q: %v", tc.query, err)
				continue
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("query %q: terms = %q, want %q", tc.query, got, tc.want)
			}
		}
	})
}

func TestTaskSearchCLIArguments(t *testing.T) {
	root := projectRoot(t)
	searchGrammarFixtures(t, root)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "search", "hijack", "stall")
	if code != 0 {
		t.Fatalf("multi-arg search exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 [Pending]", "005 [Pending]", "006 [Pending]")

	_, stderr, code = runCLI(t, "--root", root, "task", "search", `foo"bar`)
	if code != 2 {
		t.Errorf("glued-quote exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "must delimit a whole term") {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "search", `"foo`)
	if code != 2 {
		t.Errorf("unterminated-quote exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "unterminated double quote") {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "search", `""`)
	if code != 2 {
		t.Errorf("empty-span exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "empty quoted term") {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "search", "   ")
	if code != 2 {
		t.Errorf("whitespace-only exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "task search requires a query") {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	stdout, helpErr, helpCode := runCLI(t, "task", "search", "--help")
	if helpCode != 0 {
		t.Fatalf("task search --help exit code = %d, stderr = %s", helpCode, helpErr)
	}
	assertContainsAll(t, stdout, "every term must appear", "double-quoted span", "usage error", "--priority", "--effort", "--sort", "--reverse")
}

// searchFilterFixtures writes records that differ in matched field, priority,
// effort, and label so the search filters and sorting can be told apart. For
// the query "hijack": 001 is a body-only P2/S match, 002 a P2/S title match,
// 003 a P0/XS title match, and 004 matches only "timeout".
func searchFilterFixtures(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ".ahm", "tasks", "active")
	writeTaskFileWithBody(t, filepath.Join(dir, "001.md"), "001", "Add timeout handling", "Pending", "P2", "type:feature, area:cli", "The worker can hijack a slot and stall.")
	writeTaskFileWithBody(t, filepath.Join(dir, "002.md"), "002", "Document hijack behavior", "Open", "P2", "type:docs, area:docs", "Explains how a hijack is recorded.")
	writeTaskFileWithPriority(t, filepath.Join(dir, "003.md"), "003", "Hijack backfill worker", "Pending", "P0", "effort: XS\nlabels: type:task, area:cli\n")
	writeTaskFileWithPriority(t, filepath.Join(dir, "004.md"), "004", "Timeout defaults review", "Open", "P1", "effort: M\nlabels: type:task, area:docs\n")
}

func TestTaskSearchPriorityEffortFilters(t *testing.T) {
	root := projectRoot(t)
	searchFilterFixtures(t, root)

	t.Run("priority filter keeps only the named priorities", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, []string{"P0"}, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "003 [Pending] P0 XS Hijack backfill worker")
		assertNotContains(t, got, "001 [Pending]", "002 [Open]")
	})

	t.Run("priority values are normalized", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, []string{"p0"}, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "003 [Pending] P0 XS Hijack backfill worker")
		assertNotContains(t, got, "001 [Pending]", "002 [Open]")
	})

	t.Run("priority composes with status and keeps the grouping", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", []string{"Pending"}, nil, []string{"P0", "P2"}, nil, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertNotContains(t, got, "002 [Open]")
		assertOrderedStrings(t, got, "003 [Pending]", "001 [Pending]")
	})

	t.Run("invalid priority is a usage error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("hijack", nil, nil, []string{"P5"}, nil, "", false)
		if err == nil {
			t.Fatal("expected a usage error for P5")
		}
		if !strings.Contains(err.Error(), "unsupported task priority") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("effort filter keeps only the named efforts", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, []string{"XS"}, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "003 [Pending] P0 XS Hijack backfill worker")
		assertNotContains(t, got, "001 [Pending]", "002 [Open]")
	})

	t.Run("effort values are normalized and compose", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("timeout", nil, nil, nil, []string{"s", "M"}, "", false); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling", "004 [Open] P1 M Timeout defaults review")
		assertNotContains(t, got, "002 [Open]", "003 [Pending]")
	})

	t.Run("invalid effort is a usage error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("hijack", nil, nil, nil, []string{"XXL"}, "", false)
		if err == nil {
			t.Fatal("expected a usage error for XXL")
		}
		if !strings.Contains(err.Error(), "unsupported task effort") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("filters compose with sort in one query", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("hijack", []string{"Pending"}, []string{"area:cli"}, []string{"P0", "P2"}, []string{"S", "XS"}, "id", false)
		if err != nil {
			t.Error(err)
		}
		got := out.String()
		assertNotContains(t, got, "002 [Open]", "004 [Open]")
		assertOrderedStrings(t, got, "001 [Pending] P2 S Add timeout handling", "003 [Pending] P0 XS Hijack backfill worker")
	})
}

func TestTaskSearchSortAndReverse(t *testing.T) {
	root := projectRoot(t)
	searchFilterFixtures(t, root)

	t.Run("default output keeps the title/body grouping", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "", false); err != nil {
			t.Error(err)
		}
		// 003 and 002 are title matches in priority order; the body-only
		// 001 follows them even though its ID sorts first.
		assertOrderedStrings(t, out.String(), "003 [Pending]", "002 [Open]", "001 [Pending]")
	})

	t.Run("sort replaces the grouping", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "id", false); err != nil {
			t.Error(err)
		}
		// 001 is a body-only match and precedes the 002 title match under
		// --sort id, which the default grouping would not allow.
		got := out.String()
		assertOrderedStrings(t, got, "001 [Pending]", "002 [Open]", "003 [Pending]")
		assertNotContains(t, got, "004 [Open]")
	})

	t.Run("sort by priority orders the whole result set", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "priority", false); err != nil {
			t.Error(err)
		}
		assertOrderedStrings(t, out.String(), "003 [Pending]", "001 [Pending]", "002 [Open]")
	})

	t.Run("reverse reverses the sort order", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "priority", true); err != nil {
			t.Error(err)
		}
		assertOrderedStrings(t, out.String(), "002 [Open]", "001 [Pending]", "003 [Pending]")
	})

	t.Run("reverse reverses the id sort", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("hijack", nil, nil, nil, nil, "id", true); err != nil {
			t.Error(err)
		}
		assertOrderedStrings(t, out.String(), "003 [Pending]", "002 [Open]", "001 [Pending]")
	})

	t.Run("sort field values match task list", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("hijack", nil, nil, nil, nil, "bogus", false)
		if err == nil {
			t.Fatal("expected a usage error for an unsupported sort field")
		}
		if !strings.Contains(err.Error(), "unsupported task sort field") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("reverse without sort is a usage error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("hijack", nil, nil, nil, nil, "", true)
		if err == nil {
			t.Fatal("expected a usage error for --reverse without --sort")
		}
		if !strings.Contains(err.Error(), "--reverse requires --sort") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestTaskSearchCLIFiltersAndSort(t *testing.T) {
	root := projectRoot(t)
	searchFilterFixtures(t, root)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "search", "hijack", "--priority", "P0", "--sort", "id")
	if code != 0 {
		t.Fatalf("search filters exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "003 [Pending] P0 XS Hijack backfill worker")
	assertNotContains(t, stdout, "001 [Pending]", "002 [Open]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "search", "hijack", "--effort", "XS")
	if code != 0 {
		t.Fatalf("search effort exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "003 [Pending] P0 XS Hijack backfill worker")
	assertNotContains(t, stdout, "001 [Pending]", "002 [Open]")

	_, stderr, code = runCLI(t, "--root", root, "task", "search", "hijack", "--reverse")
	if code != 2 {
		t.Errorf("reverse-without-sort exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "--reverse requires --sort") {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "search", "hijack", "--sort", "bogus")
	if code != 2 {
		t.Errorf("invalid-sort exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "unsupported task sort field") {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}
