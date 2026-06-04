package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/travisennis/ahm/internal/templates"
)

type options struct {
	root   string
	json   bool
	plain  bool
	text   bool
	dryRun bool
	force  bool
}

type app struct {
	opts       options
	out        io.Writer
	err        io.Writer
	in         io.Reader
	tasksCache []Task // cached result of collectTasks, nil when stale
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
		Use:           "ahm",
		Short:         "Manage repo-local .agents workflows",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       templates.Version,
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
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(a.out, templates.Version)
			return nil
		},
	})
	root.AddCommand(a.lenientCommand("init", "Install workflow files", func() error {
		return a.install(false)
	}))
	root.AddCommand(a.lenientCommand("upgrade", "Update managed workflow files", func() error {
		return a.install(true)
	}))
	root.AddCommand(a.simpleCommand("status", "Show workflow health", func() error {
		return a.status()
	}))
	root.AddCommand(a.simpleCommand("doctor", "Show environment checks", func() error {
		return a.doctor()
	}))
	root.AddCommand(a.simpleCommand("index", "Regenerate task indexes", func() error {
		return a.writeIndexes()
	}))
	root.AddCommand(a.agentsCommand())
	root.AddCommand(a.taskCommand())
	return root
}

func (a *app) agentsCommand() *cobra.Command {
	var showAll bool
	agents := &cobra.Command{
		Use:   "agents",
		Short: "Show AGENTS.md guidance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("agents requires a subcommand")
		},
	}
	suggestions := &cobra.Command{
		Use:   "suggestions",
		Short: "Print suggested AGENTS.md additions",
		Args:  noArgs,
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

func (a *app) simpleCommand(use string, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return run()
		},
	}
}

func (a *app) lenientCommand(use string, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRootOrCWD(); err != nil {
				return err
			}
			return run()
		},
	}
}
