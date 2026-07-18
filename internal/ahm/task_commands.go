package ahm

import (
	"fmt"
	"strings"
	"time"

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
  ahm task show 001
  ahm task work 001 --agent codex`,
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
  ahm task create "Complex work" --priority P2 --effort M --body-file body.md`,
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
	task.AddCommand(a.taskListCommand("ready", nil, "List ready tasks", "ready", `List pending tasks whose dependencies are all completed.

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

	groomArgs := taskGroomArgs{}
	groom := &cobra.Command{
		Use:   "groom [<id>]",
		Short: "Delegate backlog grooming and apply task verdicts",
		Long: `Delegate backlog grooming to the configured coding agent.

With no id, all Open tasks are groomed and Blocked tasks are reviewed for
staleness. With an id, exactly that task is groomed. The agent returns a
schema-constrained verdict; ahm validates the complete result before applying
acceptance, comments, dependency corrections, or label normalization.

Examples:
  ahm task groom
  ahm task groom 157 --agent codex
  ahm --dry-run task groom`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError("task groom accepts at most one id\n  ahm task groom [<id>]")
			}
			if len(args) == 1 {
				groomArgs.id = args[0]
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			if groomArgs.timeout <= 0 {
				return usageError("task groom --timeout must be greater than 0 (e.g. 30m, 2h, 90s)\n  ahm task groom --timeout 2h")
			}
			return a.taskGroom(groomArgs)
		},
	}
	groom.Flags().StringVar(&groomArgs.agent, "agent", "", "Coding-agent CLI (cake, claude, codex, cursor)")
	groom.Flags().StringVar(&groomArgs.model, "model", "", "Model override for the delegated agent")
	groom.Flags().DurationVar(&groomArgs.timeout, "timeout", taskWorkDefaultTimeout, "Maximum delegation duration")
	task.AddCommand(groom)

	workArgs := taskWorkArgs{}
	work := &cobra.Command{
		Use:   "work <id>",
		Short: "Hand a task to a coding-agent CLI",
		Long: `Hand a task to a coding-agent CLI for implementation.

The implementation and review phases each resolve their agent and model from
the following precedence:
  1. --agent / --model CLI flags (apply to all phases)
  2. Role-specific config under "taskWork" in ahm config
  3. Legacy default_work_agent
  4. Built-in default: "cake"

When no review-specific agent is configured, review uses the same agent as
implementation. Feedback-resume and commit handoff always use the
implementation agent because they resume the implementation session.

Examples:
  ahm task work 001
  ahm task work 001 --agent codex
  ahm task work 001 --agent cursor --no-review
  ahm task work 001 --timeout 2h
  ahm task work 001 --model o4-mini
  ahm task work 001 --agent codex --model o3-mini
  ahm --dry-run task work 001 --agent cake`,
		Args: exactArgs(1, "task work requires an id\n  ahm task work <id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			workArgs.id = args[0]
			if workArgs.timeout <= 0 {
				return usageError("task work --timeout must be greater than 0 (e.g. 30m, 2h, 90s)\n  ahm task work <id> --timeout 2h")
			}
			return a.taskWork(workArgs)
		},
	}
	work.Flags().StringVar(&workArgs.agent, "agent", "", "Agent to run: cake, claude, codex, or cursor")
	work.Flags().StringVar(&workArgs.model, "model", "", "Model override for the selected agent")
	work.Flags().BoolVar(&workArgs.noReview, "no-review", false, "Skip review orchestration (review runs by default)")
	work.Flags().BoolVar(&workArgs.noCommit, "no-commit", false, "Skip commit handoff (commit runs by default)")
	work.Flags().BoolVar(&workArgs.noProjectPrompt, "no-project-prompt", false, "Skip project instructions file inclusion")
	work.Flags().DurationVar(&workArgs.timeout, "timeout", 30*time.Minute, "Maximum time for each phase before timeout (e.g. 2h, 45m); must be > 0")
	task.AddCommand(work)

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
