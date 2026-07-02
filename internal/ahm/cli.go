package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/travisennis/ahm/internal/version"
)

type options struct {
	root   string
	json   bool
	plain  bool
	text   bool
	dryRun bool
	force  bool
	check  []string
}

type app struct {
	opts       options
	out        io.Writer
	err        io.Writer
	in         io.Reader
	tasksCache []Task   // cached result of collectTasks, nil when stale
	warnings   []string // non-fatal errors accumulated during a command
}

func (a *app) addWarning(format string, args ...any) {
	a.warnings = append(a.warnings, fmt.Sprintf(format, args...))
}

func (a *app) emitWarnings() {
	if a.err == nil || len(a.warnings) == 0 {
		return
	}
	// Dedupe exact duplicates within one batch so the same message added
	// by nested call sites prints only once.
	seen := make(map[string]bool, len(a.warnings))
	for _, w := range a.warnings {
		if !seen[w] {
			fmt.Fprintln(a.err, "warning:", w)
			seen[w] = true
		}
	}
	a.warnings = nil
}

// getTasks returns the cached task list or reads it from disk.
// The cache is invalidated after any write that modifies task files.
func (a *app) getTasks() ([]Task, error) {
	if a.tasksCache != nil {
		return a.tasksCache, nil
	}
	tasks, err := collectTasks(a.opts.root)
	if err == nil {
		a.tasksCache = tasks
	}
	return tasks, err
}

// invalidateTasks clears the cached task list so the next call to
// getTasks re-reads from disk.
func (a *app) invalidateTasks() {
	a.tasksCache = nil
}

// Main runs the CLI and returns a process exit code.
func Main(argv []string, stdout io.Writer, stderr io.Writer) int {
	a := app{out: stdout, err: stderr, in: os.Stdin}
	if err := a.run(argv); err != nil {
		var usage usageError
		if errors.As(err, &usage) {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if errors.Is(err, errValidationFailed) {
			return 1
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

var errValidationFailed = errors.New("workflow has validation errors")

type usageError string

func (e usageError) Error() string {
	return string(e)
}

// noArgs is like cobra.NoArgs but wraps the error as a usageError so that
// Main can distinguish usage errors from runtime errors by type assertion.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError(fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()))
	}
	return nil
}

func (a *app) run(argv []string) error {
	cmd := a.command()
	cmd.SetArgs(argv)
	return cmd.Execute()
}

func (a *app) command() *cobra.Command {
	root := &cobra.Command{
		Use:   "ahm",
		Short: "Manage repo-local .agents workflows",
		Long: `Manage repo-local .agents workflow files for tasks, research notes,
ADRs, generated indexes, and coding-agent delegation.

When run with no command, ahm runs 'status', which exits with code 1
when validation errors are found. For a briefing that always succeeds,
run 'ahm context'.

Examples:
  ahm
  ahm context
  ahm status
  ahm --json doctor`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Binary,
		Args:          noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.status()
		},
	}
	root.SetOut(a.out)
	root.SetErr(a.err)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(err.Error())
	})
	root.PersistentFlags().StringVar(&a.opts.root, "root", "", "Target repository root")
	root.PersistentFlags().BoolVar(&a.opts.json, "json", false, "Print JSON")
	root.PersistentFlags().BoolVar(&a.opts.plain, "plain", false, "Print stable plain output")
	root.PersistentFlags().BoolVar(&a.opts.text, "text", false, "Print human-friendly text (default)")
	root.PersistentFlags().BoolVar(&a.opts.dryRun, "dry-run", false, "Preview supported writes")
	root.PersistentFlags().BoolVar(&a.opts.force, "force", false, "Force supported overwrites")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Long: `Print the ahm binary version (release tag injected at build time).

Examples:
  ahm version`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(a.out, version.Binary)
			return nil
		},
	})
	root.AddCommand(a.lenientCommand("init", "Install workflow files", `Install the managed .agents workflow into the target repository root.

Examples:
  ahm init
  ahm --dry-run init
  ahm --force init`, func() error {
		return a.install(false)
	}))
	root.AddCommand(a.lenientCommand("upgrade", "Update managed workflow files", `Update managed workflow files to the embedded template version.

Examples:
  ahm upgrade
  ahm --force upgrade
  ahm --dry-run upgrade`, func() error {
		return a.install(true)
	}))
	root.AddCommand(a.contextCommand())
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show workflow health",
		Long: `Show workflow health with validation findings.

Examples:
  ahm status
  ahm --check workflow status
  ahm --check links --json status
  ahm --check project-docs status`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			if err := a.validateCheckScopes(); err != nil {
				return err
			}
			return a.status()
		},
	}
	statusCmd.Flags().StringSliceVar(&a.opts.check, "check", nil, "Validation scope (comma-separated or repeatable): workflow, links, project-docs")
	root.AddCommand(statusCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show environment checks",
		Long: `Show environment and workflow checks.

Examples:
  ahm doctor
  ahm --check workflow doctor`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			if err := a.validateCheckScopes(); err != nil {
				return err
			}
			return a.doctor()
		},
	}
	doctorCmd.Flags().StringSliceVar(&a.opts.check, "check", nil, "Validation scope (comma-separated or repeatable): workflow, links, project-docs")
	root.AddCommand(doctorCmd)
	root.AddCommand(a.simpleCommand("index", "Regenerate task indexes and clean up orphaned temp files", `Regenerate generated task, research, ExecPlan, and ADR indexes.

Examples:
  ahm index
  ahm --dry-run index`, func() error {
		if err := cleanupStaleTemps(a.opts.root); err != nil {
			// Best-effort cleanup of crash leftovers; surface partial failures
			// (e.g. permission denied) without aborting index regeneration.
			a.addWarning("%v", err)
		}
		return a.writeIndexes()
	}))
	root.AddCommand(a.agentsCommand())
	root.AddCommand(a.adrCommand())
	root.AddCommand(a.taskCommand())
	return root
}

func (a *app) contextCommand() *cobra.Command {
	validScopes := map[string]bool{
		"":         true,
		"task":     true,
		"adr":      true,
		"research": true,
		"plan":     true,
		"docs":     true,
	}
	return &cobra.Command{
		Use:   "context [task|adr|research|plan|docs]",
		Short: "Repository briefing or managed-work reference",
		Long: `Print a repository briefing or managed-work reference.

With no scope, prints a live repository briefing with repository state,
validation status, git branch, task counts, and useful follow-up commands.

Scopes:
  task     Task workflow reference for creating, choosing, and working on tasks
  adr      ADR workflow reference for numbering, naming, and template rules
  research Research workflow reference for organizing and using research notes
  plan     ExecPlan workflow reference for planning larger tasks
  docs     Documentation workflow reference for auditing and updating docs

Examples:
  ahm context
  ahm context task
  ahm --json context adr`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError(fmt.Sprintf("unknown command %q for %q", args[1], cmd.CommandPath()))
			}
			if len(args) == 1 && !validScopes[args[0]] {
				return usageError(fmt.Sprintf("unknown context scope %q (valid: task, adr, research, plan, docs)\n  ahm context <scope>", args[0]))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			scope := ""
			if len(args) == 1 {
				scope = args[0]
			}
			return a.context(scope)
		},
	}
}

func (a *app) agentsCommand() *cobra.Command {
	var showAll bool
	agents := &cobra.Command{
		Use:   "agents",
		Short: "Show AGENTS.md guidance",
		Long: `Show AGENTS.md guidance and suggestions.

Examples:
  ahm agents suggestions
  ahm agents suggestions --all
  ahm --json agents suggestions`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("agents requires a subcommand\n  ahm agents suggestions")
		},
	}
	suggestions := &cobra.Command{
		Use:   "suggestions",
		Short: "Print suggested AGENTS.md additions",
		Long: `Print advisory AGENTS.md integration instructions.

The suggestions help adapt an existing AGENTS.md to use ahm's managed-work
intake and ownership boundaries. This command never writes AGENTS.md.

Examples:
  ahm agents suggestions
  ahm agents suggestions --all
  ahm --json agents suggestions`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRootOrCWD(); err != nil {
				return err
			}
			return a.agentsSuggestions(showAll)
		},
	}
	suggestions.Flags().BoolVar(&showAll, "all", false, "Print all suggestions, including ones that appear present")
	agents.AddCommand(suggestions)
	return agents
}

func (a *app) simpleCommand(use string, short string, long string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return run()
		},
	}
}

func (a *app) lenientCommand(use string, short string, long string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRootOrCWD(); err != nil {
				return err
			}
			return run()
		},
	}
}

func (a *app) validateCheckScopes() error {
	for _, s := range a.opts.check {
		if !containsScope(validCheckScopes(), s) {
			return usageError(fmt.Sprintf("unknown check scope %q (valid: %s)", s, strings.Join(validCheckScopes(), ", ")))
		}
	}
	return nil
}
