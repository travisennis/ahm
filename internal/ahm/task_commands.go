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
			createArgs.bodySet = cmd.Flags().Changed("body")
			createArgs.bodyFileSet = cmd.Flags().Changed("body-file")
			return a.taskCreateParsed(createArgs)
		},
	}
	create.Flags().StringVarP(&createArgs.priority, "priority", "p", createArgs.priority, "Set task priority")
	create.Flags().StringVar(&createArgs.effort, "effort", createArgs.effort, "Set task effort")
	create.Flags().StringVar(&createArgs.labels, "labels", createArgs.labels, "Set task labels")
	create.Flags().StringVar(&createArgs.status, "status", createArgs.status, "Set initial task status")
	create.Flags().StringVarP(&createArgs.description, "description", "d", "", "Set task summary text")
	create.Flags().StringVarP(&createArgs.body, "body", "b", "", "Full Markdown body text; exclusive with --body-file and --description")
	create.Flags().StringVarP(&createArgs.bodyFile, "body-file", "F", "", "Full Markdown body from a file (or - for stdin); ahm handles ID, front matter, and indexes")
	create.Flags().StringVar(&createArgs.externalRef, "external-ref", "", "External reference recorded in the task front matter")
	create.Flags().StringVar(&createArgs.parent, "parent", "", "Parent task ID for subtask creation; allocates a suffixed child ID like 137a, 137b")
	create.Flags().StringVar(&createArgs.dependsOn, "depends-on", "", "Comma-separated task IDs this task depends on")
	task.AddCommand(create)

	task.AddCommand(a.taskImportCommand())
	task.AddCommand(a.taskEditCommand())

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
		Short: "Search tasks by title and body",
		Long: `Search tasks by case-insensitive substring match on the title or body.

Title matches are listed before body-only matches. Line format matches task
list: ID [Status] Priority Effort Title. Supports the --status and --label
filters to scope results.

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
		verb       string
		withReason bool
		withBlock  bool
	}{
		{use: "accept <id>", short: "Accept a task into the ready queue", verb: "accept", long: `Accept an Open task into the ready backlog as Pending.

Accepting a task that is already Pending reports that it is already Pending;
any other status is a usage error.

Examples:
  ahm task accept 001
  ahm --dry-run task accept 001`},
		{use: "start <id>", short: "Mark a task in progress", verb: "start", long: `Mark a Pending task In Progress.

Starting a task that is already In Progress reports that it is already In
Progress; any other status is a usage error.

Examples:
  ahm task start 001
  ahm --dry-run task start 001`},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", verb: "complete", long: `Mark a task as Completed and regenerate indexes.

Applies to an Open, Pending, In Progress, Blocked, or Tracking task.
Completing an already Completed task reports that it is already Completed; a
Cancelled task is a usage error.

Examples:
  ahm task complete 001
  ahm task close 001
  ahm --dry-run task complete 001`},
		{use: "cancel <id>", short: "Mark a task cancelled", verb: "cancel", long: `Mark a task as Cancelled with a required reason.

Applies to an Open, Pending, In Progress, Blocked, or Tracking task. Cancelling
an already Cancelled task reports that it is already Cancelled; a Completed task
is a usage error. --reason is required whether or not the transition happens.

Examples:
  ahm task cancel 001 --reason "Superseded by 002"
  ahm --dry-run task cancel 001 --reason "Duplicate"`, withReason: true},
		{use: "reopen <id>", short: "Reopen a task", verb: "reopen", long: `Reopen a Completed, Cancelled, or Pending task to Open.

Reopening returns the task to the untriaged Open queue, so 'ahm task accept' is
required to queue it again. Reopening an Open task reports that it is already
Open; any other status is a usage error.

Examples:
  ahm task reopen 001
  ahm --dry-run task reopen 001`},
		{use: "block <id>", short: "Block a task with a reason", verb: "block", long: `Mark a task Blocked and record why, in the front-matter fields blocked_reason and blocked_ref.

--reason is required; --ref records an optional external reference such as an
issue URL. Applies to an Open, Pending, or Tracking task. Blocking an already
Blocked task reports that it is already Blocked; release it with 'ahm task
unblock' and block again to correct the reason. Any other status is a usage
error.

Examples:
  ahm task block 042 --reason "Waiting on the storage decision"
  ahm task block 042 --reason "Upstream bug" --ref https://github.com/owner/repo/issues/1
  ahm --dry-run task block 042 --reason "Waiting on ADR"`, withBlock: true},
		{use: "unblock <id>", short: "Release a blocked task", verb: "unblock", long: `Return a Blocked task to Pending and clear its recorded block reason.

The task must currently be Blocked; a Pending task reports that it is already
Pending, and any other status is a usage error. This is distinct from the
automatic unblock that happens when a task's dependencies complete.

Examples:
  ahm task unblock 042
  ahm --dry-run task unblock 042`},
	} {
		verb := spec.verb
		reason := ""
		blockReason := ""
		blockRef := ""
		cmd := &cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Long:    spec.long,
			Args:    exactArgs(1, "task status command requires an id\n  ahm task accept|start|complete|cancel|reopen|block|unblock <id>"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := a.detectRoot(); err != nil {
					return err
				}
				return a.taskStatusWithArgs(taskStatusArgs{
					id:          args[0],
					verb:        verb,
					reason:      reason,
					blockReason: blockReason,
					blockRef:    blockRef,
				})
			},
		}
		if spec.withReason {
			cmd.Flags().StringVar(&reason, "reason", "", "Reason for cancelling the task")
		}
		if spec.withBlock {
			cmd.Flags().StringVar(&blockReason, "reason", "", "Reason the task is blocked (required)")
			cmd.Flags().StringVar(&blockRef, "ref", "", "Optional external reference for the block")
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
