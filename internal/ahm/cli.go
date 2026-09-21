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
	tasksCache []Task      // cached result of collectTasks, nil when stale
	store      *storePaths // resolved home store location, nil until first use
	warnings   []string    // non-fatal errors accumulated during a command
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
	tasks, err := collectTasksForPaths(a.opts.root, a.workflowPaths())
	if err == nil {
		a.tasksCache = tasks
	}
	return tasks, err
}

func (a *app) workflowPaths() workflowPaths {
	return workflowPathsFor(a.opts.root)
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
		Short: "Manage repo-local task and ADR records",
		Long: `Manage repo-local task and ADR records under .ahm/ for tasks and
docs/adr/ for ADRs, with generated indexes for both.

When run with no command, ahm runs 'status', which exits with code 1
when validation errors are found. For a session briefing with live backlog
state, run 'ahm prime'.

Examples:
  ahm
  ahm prime
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
	root.AddCommand(a.lenientCommand("init", "Create or reconcile ahm-owned workflow state", `Create ahm-owned workflow state when it is absent and reconcile it when
it is present.

The command creates the .ahm/ record directories, .ahm/config.json, the
managed .ahm/.gitignore, and the generated indexes when they are missing,
and rewrites them when they differ from what ahm owns. Obsolete ahm-owned
configuration keys are dropped and unknown metadata is preserved. An
up-to-date repository is left untouched. Project-owned files, including
AGENTS.md, are never created, replaced, or removed.

Examples:
  ahm init
  ahm --dry-run init
  ahm --force init`, func() error {
		return a.install()
	}))
	primeCmd := &cobra.Command{
		Use:   "prime",
		Short: "Session briefing with live backlog state",
		Long: `Print a session briefing with repository state and the task backlog.

The briefing reports:
- Dirty-worktree warning when the working tree is not clean.
- Repository root, workflow version, and validation status.
- In-progress and ready task lists (ready capped at 5).
- Blocked and open task counts.

It regenerates the generated indexes first and prescribes nothing: no command
to run and no workflow step.

Supports --json, --plain, and --text output.

Examples:
  ahm prime
  ahm --json prime
  ahm --plain prime`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.prime()
		},
	}
	root.AddCommand(primeCmd)
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show workflow health",
		Long: `Show workflow health with validation findings.

Examples:
  ahm status
  ahm --check workflow status
  ahm --check links --json status`,
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
	statusCmd.Flags().StringSliceVar(&a.opts.check, "check", nil, "Validation scope (comma-separated or repeatable): workflow, links")
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
	doctorCmd.Flags().StringSliceVar(&a.opts.check, "check", nil, "Validation scope (comma-separated or repeatable): workflow, links")
	root.AddCommand(doctorCmd)

	root.AddCommand(a.simpleCommand("index", "Regenerate task and ADR indexes and clean up stale temp files", `Regenerate the generated task and ADR indexes from source records.

Also removes stale .tmp files older than five minutes anywhere under .ahm/,
including leftovers from an interrupted write. A cleanup failure warns instead
of failing the command, and --dry-run removes nothing.

Examples:
  ahm index
  ahm --dry-run index`, func() error {
		if !a.opts.dryRun {
			if err := cleanupStaleTemps(a.opts.root); err != nil {
				// Best-effort cleanup of crash leftovers; surface partial failures
				// (e.g. permission denied) without aborting index regeneration.
				a.addWarning("%v", err)
			}
		}
		return a.writeIndexes()
	}))
	root.AddCommand(a.adrCommand())
	root.AddCommand(a.taskCommand())
	root.AddCommand(a.storeCommand())
	return root
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
