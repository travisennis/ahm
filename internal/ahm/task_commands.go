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
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("task requires a subcommand")
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
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageError("task create requires a title")
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
	task.AddCommand(create)

	task.AddCommand(a.taskListCommand("list", []string{"ls"}, "List tasks", "all"))
	task.AddCommand(a.taskListCommand("ready", nil, "List ready tasks", "ready"))
	task.AddCommand(a.taskListCommand("blocked", nil, "List blocked tasks", "blocked"))
	task.AddCommand(&cobra.Command{
		Use:   "labels",
		Short: "List task labels",
		Args:  noArgs,
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
		Args:  noArgs,
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
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskMigrate()
		},
	})
	task.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a task",
		Args:  exactArgs(1, "task show requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskShow(args)
		},
	})

	workArgs := taskWorkArgs{}
	work := &cobra.Command{
		Use:   "work <id>",
		Short: "Hand a task to a coding-agent CLI",
		Args:  exactArgs(1, "task work requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			workArgs.id = args[0]
			return a.taskWork(workArgs)
		},
	}
	work.Flags().StringVar(&workArgs.agent, "agent", "", "Agent to run: cake, claude, codex, or cursor")
	work.Flags().BoolVar(&workArgs.review, "review", false, "Run review orchestration after work session")
	work.Flags().BoolVar(&workArgs.complete, "complete", false, "Run completion handoff after work session")
	work.Flags().BoolVar(&workArgs.commit, "commit", false, "Run commit handoff after work session")
	task.AddCommand(work)

	for _, spec := range []struct {
		use        string
		aliases    []string
		short      string
		status     string
		withReason bool
	}{
		{use: "accept <id>", short: "Accept a task into the ready queue", status: "Pending"},
		{use: "start <id>", short: "Mark a task in progress", status: "In Progress"},
		{use: "complete <id>", aliases: []string{"close"}, short: "Mark a task completed", status: "Completed"},
		{use: "cancel <id>", short: "Mark a task cancelled", status: "Cancelled", withReason: true},
		{use: "reopen <id>", short: "Reopen a task", status: "Pending"},
	} {
		status := spec.status
		reason := ""
		cmd := &cobra.Command{
			Use:     spec.use,
			Aliases: spec.aliases,
			Short:   spec.short,
			Args:    exactArgs(1, "task status command requires an id"),
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

	task.AddCommand(a.taskDepCommand())
	return task
}

func (a *app) taskListCommand(use string, aliases []string, short string, mode string) *cobra.Command {
	var statuses []string
	var labels []string
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   short,
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskList(mode, statuses, labels)
		},
	}
	if mode == "all" {
		cmd.Flags().StringSliceVar(&statuses, "status", nil, "Filter tasks by status; valid: Open, Pending, In Progress, Blocked, Tracking, Completed, Cancelled (comma-separated or repeatable)")
	}
	cmd.Flags().StringSliceVar(&labels, "label", nil, "Filter tasks by label; all labels must match (comma-separated or repeatable)")
	return cmd
}
