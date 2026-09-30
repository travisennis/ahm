package ahm

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The CLI reference pages carry machine-readable inventories: fenced blocks
// whose info string is "text ahm-inventory <name>", one entry per line.
// TestCLIDocumentationParity compares each block with the Cobra tree it
// describes, so a command, alias, or global flag registered in code without a
// documentation entry fails the check, and a documented entry that no longer
// resolves fails it too. The blocks are the contract; the prose around them
// stays human-maintained, and `ahm <command> --help` remains the authority for
// per-command flag detail.
var inventoryFence = regexp.MustCompile("^```text ahm-inventory ([a-z0-9-]+)$")

const (
	globalContractPage = "global-contract.md"
	commandsPage       = "commands.md"
	taskCommandsPage   = "task-commands.md"
)

type docInventory struct {
	page       string
	block      string
	registered []string
}

// TestCLIDocumentationParity keeps the documented command surface and the
// executable one in step. Contributors change CLI wiring in
// internal/ahm/cli.go, task_commands.go, task_deps.go, adr_commands.go, and
// store.go, and update the matching inventory block in
// docs/references/cli/.
func TestCLIDocumentationParity(t *testing.T) {
	root := (&app{}).command()
	// Cobra materializes its built-in help flag, version flag, help command,
	// and completion command while executing, not while wiring. The
	// documented surface includes them, so the check materializes them too.
	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	task := mustChildCommand(t, root, "task")
	dep := mustChildCommand(t, task, "dep")

	inventories := []docInventory{
		{globalContractPage, "global-flags", globalFlagNames(root)},
		{commandsPage, "commands", subcommandNames(root)},
		{commandsPage, "adr-subcommands", subcommandNames(mustChildCommand(t, root, "adr"))},
		{commandsPage, "store-subcommands", subcommandNames(mustChildCommand(t, root, "store"))},
		{commandsPage, "command-aliases", aliasEntries(root)},
		{taskCommandsPage, "task-subcommands", subcommandNames(task)},
		{taskCommandsPage, "dep-subcommands", subcommandNames(dep)},
	}

	dir := cliDocsDir(t)
	blocks := readInventories(t, dir)
	for _, inventory := range inventories {
		t.Run(inventory.page+"/"+inventory.block, func(t *testing.T) {
			documented, ok := blocks[inventory.page][inventory.block]
			if !ok {
				t.Fatalf("docs/references/cli/%s has no ```text ahm-inventory %s block", inventory.page, inventory.block)
			}
			if drift := inventoryDrift(documented, inventory.registered); len(drift) > 0 {
				t.Errorf("docs/references/cli/%s block %q does not match the Cobra tree:\n  %s\nUpdate the block in the same change that touches the command wiring, then run `just cli-parity`.",
					inventory.page, inventory.block, strings.Join(drift, "\n  "))
			}
		})
	}

	for page, defined := range blocks {
		for name := range defined {
			if !slices.ContainsFunc(inventories, func(inventory docInventory) bool {
				return inventory.page == page && inventory.block == name
			}) {
				t.Errorf("docs/references/cli/%s defines inventory block %q, which no check compares against the Cobra tree", page, name)
			}
		}
	}
}

// TestInventoryDriftReportsBothDirections pins the two failures the parity
// check exists to catch: a documented entry that the CLI does not register,
// and a registered command or flag the documentation never mentions.
func TestInventoryDriftReportsBothDirections(t *testing.T) {
	drift := inventoryDrift([]string{"--json", "--quiet"}, []string{"--json", "--plain"})
	for _, want := range []string{"documented but not registered: --quiet", "registered but not documented: --plain"} {
		if !slices.Contains(drift, want) {
			t.Errorf("inventoryDrift = %v, want it to contain %q", drift, want)
		}
	}

	if got := inventoryDrift([]string{"--json"}, []string{"--json"}); len(got) > 0 {
		t.Errorf("inventoryDrift on a matching pair = %v, want no drift", got)
	}

	if got := inventoryDrift([]string{"--json", "--json"}, []string{"--json"}); len(got) != 1 || !strings.Contains(got[0], "listed twice: --json") {
		t.Errorf("inventoryDrift on a duplicated entry = %v, want one duplicate report", got)
	}
}

// inventoryDrift reports every way a documented inventory and the registered
// surface disagree, so one run names all of them.
func inventoryDrift(documented []string, registered []string) []string {
	documentedCounts := make(map[string]int, len(documented))
	for _, name := range documented {
		documentedCounts[name]++
	}

	var drift []string
	remaining := make(map[string]bool, len(documentedCounts))
	for _, name := range slices.Sorted(maps.Keys(documentedCounts)) {
		remaining[name] = true
		if documentedCounts[name] > 1 {
			drift = append(drift, "listed twice: "+name)
		}
	}

	for _, name := range registered {
		if !remaining[name] {
			drift = append(drift, "registered but not documented: "+name)
		}
		delete(remaining, name)
	}
	for _, name := range slices.Sorted(maps.Keys(remaining)) {
		drift = append(drift, "documented but not registered: "+name)
	}
	return drift
}

// readInventories returns the inventory blocks of every Markdown page in the
// CLI reference directory, keyed by page and then by block name. Reading all of
// them lets the check report an inventory block on a page no comparison
// consumes, instead of leaving it unchecked.
func readInventories(t *testing.T, dir string) map[string]map[string][]string {
	t.Helper()
	pages, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("list the CLI reference pages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatalf("no Markdown pages in %s", dir)
	}
	blocks := make(map[string]map[string][]string, len(pages))
	for _, path := range pages {
		page := filepath.Base(path)
		blocks[page] = readInventory(t, dir, page)
	}
	return blocks
}

// readInventory returns the inventory blocks of one Markdown page, keyed by
// block name. A block is a fenced code block whose info string is
// "text ahm-inventory <name>"; blank lines are ignored and every other line is
// one entry.
func readInventory(t *testing.T, dir string, page string) map[string][]string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, page))
	if err != nil {
		t.Fatalf("read %s: %v", page, err)
	}

	inventories := map[string][]string{}
	seen := map[string]bool{}
	name := ""
	for i, line := range strings.Split(string(content), "\n") {
		switch {
		case inventoryFence.MatchString(line):
			if name != "" {
				t.Fatalf("docs/references/cli/%s:%d opens a nested block, or the %q block above it is missing its closing fence", page, i+1, name)
			}
			name = strings.TrimPrefix(line, "```text ahm-inventory ")
			if seen[name] {
				t.Fatalf("docs/references/cli/%s:%d repeats the %q block", page, i+1, name)
			}
			seen[name] = true
			inventories[name] = nil
		case name != "" && strings.TrimSpace(line) == "```":
			name = ""
		case name != "" && strings.TrimSpace(line) != "":
			inventories[name] = append(inventories[name], strings.TrimSpace(line))
		}
	}
	if name != "" {
		t.Fatalf("docs/references/cli/%s leaves the %q block unclosed", page, name)
	}
	return inventories
}

// cliDocsDir returns the CLI reference directory in this checkout, so the check
// reads the documentation the way a reader does rather than a copy.
func cliDocsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "docs", "references", "cli")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory; the CLI documentation check needs the repository checkout")
		}
		dir = parent
	}
}

// subcommandNames returns the sorted names of a command's visible direct
// children. Cobra's own commands (`help`, `completion`) are included: they are
// reachable and appear in generated help, so they are part of the documented
// surface. A hidden command is not, so it is not inventoried.
func subcommandNames(cmd *cobra.Command) []string {
	var names []string
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		names = append(names, child.Name())
	}
	slices.Sort(names)
	return names
}

// globalFlagNames returns the sorted long names of every flag that can precede
// a command: the root's persistent flags plus the help flag and, because a
// version is set, the version flag Cobra adds to the root. The built-in names
// come from a bare command rather than a hardcoded list, so the documented set
// follows the library. A flag that applies to one command alone is not global
// and is not documented here.
func globalFlagNames(root *cobra.Command) []string {
	names := flagNames(root.PersistentFlags())
	names = append(names, cobraBuiltinFlagNames(root)...)
	slices.Sort(names)
	return slices.Compact(names)
}

// cobraBuiltinFlagNames returns the long names of the flags Cobra adds to a
// root command of its own accord.
func cobraBuiltinFlagNames(root *cobra.Command) []string {
	bare := &cobra.Command{Use: root.Use, Version: root.Version}
	bare.InitDefaultHelpFlag()
	bare.InitDefaultVersionFlag()
	return flagNames(bare.Flags())
}

// flagNames returns the sorted long names of the flags in a set.
func flagNames(flags *pflag.FlagSet) []string {
	var names []string
	flags.VisitAll(func(flag *pflag.Flag) {
		names = append(names, "--"+flag.Name)
	})
	slices.Sort(names)
	return names
}

// aliasEntries returns every visible command alias as
// "<alias path> = <canonical path>", with paths relative to `ahm`, so the
// documented alias block states which command each alias reaches rather than
// only that it exists. Aliases are one inventory for the whole tree, so the
// block lives on the commands page rather than on a per-family page.
func aliasEntries(root *cobra.Command) []string {
	var entries []string
	var walk func(prefix string, parent *cobra.Command)
	walk = func(prefix string, parent *cobra.Command) {
		for _, child := range parent.Commands() {
			if child.Hidden {
				continue
			}
			path := strings.TrimSpace(prefix + " " + child.Name())
			for _, alias := range child.Aliases {
				entries = append(entries, strings.TrimSpace(prefix+" "+alias)+" = "+path)
			}
			walk(path, child)
		}
	}
	walk("", root)
	slices.Sort(entries)
	return entries
}

func commandPath(cmd *cobra.Command) string {
	if cmd.Parent() == nil {
		return cmd.Name()
	}
	return commandPath(cmd.Parent()) + " " + cmd.Name()
}

func mustChildCommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	t.Fatalf("command tree has no %q under %q", name, commandPath(parent))
	return nil
}
