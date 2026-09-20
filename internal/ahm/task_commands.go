package ahm

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) taskCommand() *cobra.Command {
	task := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		Long: `Manage tasks.

Examples:
  ahm task list
  ahm task create "My task" --priority P1
  ahm task show 001`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("task requires a subcommand\n  ahm task <subcommand>")
		},
	}

	createArgs := taskCreateArgs{
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:unknown",
		status:   "Open",
	}
	create := &cobra.Command{
		Use:   "create <title> [flags]",
		Short: "Create a task",
		Long: `Create a new task and regenerate indexes.

Examples:
  ahm task create "Add release workflow"
  ahm task create "Fix bug" --priority P1 --effort S --labels "type:bug,area:cli"
  ahm task create "Complex work" --priority P2 --effort M --body-file body.md
  ahm task create "Follow-up" --depends-on 001,002`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("task create requires a title\n  ahm task create <title>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			createArgs.title = strings.Join(args, " ")
			return a.taskCreateParsed(createArgs)
		},
	}
	create.Flags().StringVarP(&createArgs.priority, "priority", "p", createArgs.priority, "Set task priority")
	create.Flags().StringVar(&createArgs.effort, "effort", createArgs.effort, "Set task effort")
	create.Flags().StringVar(&createArgs.labels, "labels", createArgs.labels, "Set task labels")
	create.Flags().StringVar(&createArgs.status, "status", createArgs.status, "Set initial task status")
	create.Flags().StringVarP(&createArgs.description, "description", "d", "", "Set task summary text")
	create.Flags().StringVar(&createArgs.bodyFile, "body-file", "", "Full Markdown body from a file (or - for stdin); ahm handles ID, front matter, and indexes")
	create.Flags().StringVar(&createArgs.parent, "parent", "", "Parent task ID for subtask creation; allocates a suffixed child ID like 137a, 137b")
	create.Flags().StringVar(&createArgs.dependsOn, "depends-on", "", "Comma-separated task IDs this task depends on")
	task.AddCommand(create)

	task.AddCommand(a.taskListCommand("list", []string{"ls"}, "List tasks", "all", `List parsed tasks, optionally filtered by status, labels, priority, or effort.

Examples:
  ahm task list
  ahm task list --status pending
  ahm task list --status pending,completed
  ahm task list --label type:feature --label area:cli
  ahm task list --priority P0
  ahm task list --priority P0,P1 --effort S,M
  ahm task list --sort updated --reverse`))
	task.AddCommand(a.taskListCommand("ready", nil, "List ready tasks", "ready", `List pending tasks whose dependencies are all completed, plus tracking tasks (with at least one child) whose child tasks are all completed or cancelled and whose own dependencies are satisfied.

Examples:
  ahm task ready
  ahm task ready --label area:cli
  ahm task ready --sort effort
  ahm --json task ready`))
	task.AddCommand(a.taskListCommand("blocked", nil, "List blocked tasks", "blocked", `List blocked tasks.

Examples:
  ahm task blocked
  ahm task blocked --label risk:external-service
  ahm task blocked --sort status --reverse`))
	task.AddCommand(&cobra.Command{
		Use:   "labels",
		Short: "List task labels",
		Long: `List labels used by parsed task files with counts.

Examples:
  ahm task labels
  ahm --json task labels`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskLabels()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "next",
		Short: "Show the next ready task",
		Long: `Show the next ready task by priority and ID.

Examples:
  ahm task next
  ahm --json task next`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskNext()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Normalize legacy task front matter",
		Long: `Normalize legacy task front matter to the current schema.

Examples:
  ahm --dry-run task migrate
  ahm task migrate
  ahm --json task migrate
  ahm --json task migrate --dry-run`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskMigrate()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "show <id> [<id>...]",
		Short: "Show one or more tasks",
		Long: `Show one or more tasks by ID.

With a single ID, prints the raw task file. With multiple IDs, prints each
file separated by ---. With --json, emits a single object for one task or an
array for multiple tasks.

Examples:
  ahm task show 001
  ahm task show 001 002 003
  ahm --json task show 001
  ahm --json task show 001 002`,
		Args: minimumArgs(1, "task show requires at least one id\n  ahm task show <id> [<id>...]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskShow(args)
		},
	})

	var searchStatuses []string
	var searchLabels []string
	search := &cobra.Command{
		Use:   "search <query>",
		Short: "Search tasks by title",
		Long: `Search tasks by case-insensitive substring match on the title.

Output matches task list: ID [Status] Priority Effort Title. Supports the
--status and --label filters to scope results.

Examples:
  ahm task search timeout
  ahm task search "release workflow"
  ahm task search timeout --status Open
  ahm --json task search cli --label area:cli`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("task search requires a query\n  ahm task search <query>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskSearch(strings.Join(args, " "), searchStatuses, searchLabels)
		},
	}
	search.Flags().StringSliceVar(&searchStatuses, "status", nil, "Filter tasks by status; valid: Open, Pending, In Progress, Blocked, Tracking, Completed, Cancelled (comma-separated or repeatable)")
	search.Flags().StringSliceVar(&searchLabels, "label", nil, "Filter tasks by label; all labels must match (comma-separated or repeatable)")
	task.AddCommand(search)

	for _, spec := range []struct {
		use        string
		aliases    []string
		short      string
		long       string
		status     string
		withReason bool
	}{
		{use: "accept <id>", short: "Accept a task into the ready queue", long: `Accept an Open task into the ready backlog as Pending.

Examples:
  ahm task accept 001
  ahm --dry-run task accept 001`, status: "Pending"},
		{use: "start <id>", short: "Mark a task in progress", long: `Mark a task as In Progress.

Examples:
  ahm task start 001
  ahm --dry-run task start 001`, status: "In Progress"},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", long: `Mark a task as Completed and regenerate indexes.

Examples:
  ahm task complete 001
  ahm task close 001
  ahm --dry-run task complete 001`, status: "Completed"},
		{use: "cancel <id>", short: "Mark a task cancelled", long: `Mark a task as Cancelled with a required reason.

Examples:
  ahm task cancel 001 --reason "Superseded by 002"
  ahm --dry-run task cancel 001 --reason "Duplicate"`, status: "Cancelled", withReason: true},
		{use: "reopen <id>", short: "Reopen a task", long: `Reopen a completed or cancelled task back to Pending.

Examples:
  ahm task reopen 001
  ahm --dry-run task reopen 001`, status: "Pending"},
	} {
		status := spec.status
		reason := ""
		cmd := &cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Long:    spec.long,
			Args:    exactArgs(1, "task status command requires an id\n  ahm task accept|start|complete|cancel|reopen <id>"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.taskStatusWithArgs(taskStatusArgs{
					ids:    args,
					status: status,
					reason: reason,
				})
			},
		}
		if spec.withReason {
			cmd.Flags().StringVar(&reason, "reason", "", "Reason for cancelling the task")
		}
		task.AddCommand(cmd)
	}

	task.AddCommand(a.taskCommentCommand())
	task.AddCommand(a.taskDepCommand())
	return task
}

func (a *app) taskListCommand(use string, aliases []string, short string, mode string, long string) *cobra.Command {
	var statuses []string
	var labels []string
	var priorities []string
	var efforts []string
	var sortField string
	var reverse bool
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Long:    long,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskListSorted(mode, statuses, labels, priorities, efforts, sortField, reverse)
		},
	}
	if mode == "all" {
		cmd.Flags().StringSliceVar(&statuses, "status", nil, "Filter tasks by status; valid: Open, Pending, In Progress, Blocked, Tracking, Completed, Cancelled (comma-separated or repeatable)")
		cmd.Flags().StringSliceVar(&priorities, "priority", nil, "Filter tasks by priority; valid: P0, P1, P2, P3, P4 (comma-separated or repeatable)")
		cmd.Flags().StringSliceVar(&efforts, "effort", nil, "Filter tasks by effort; valid: XS, S, M, L, XL (comma-separated or repeatable)")
	}
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Filter tasks by label; all labels must match (comma-separated or repeatable)")
	cmd.Flags().StringVar(&sortField, "sort", "", "Sort by priority, id, created, updated, effort, status, or title (default priority)")
	cmd.Flags().BoolVar(&reverse, "reverse", false, "Reverse the selected sort order, including task ID tie-breakers")
	return cmd
}
