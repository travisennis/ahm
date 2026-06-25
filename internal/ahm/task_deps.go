package ahm

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *app) taskDepCommand() *cobra.Command {
	dep := &cobra.Command{
		Use:   "dep",
		Short: "Manage task dependencies",
		Long: `Manage task dependency relationships.

Examples:
  ahm task dep add 002 001
  ahm task dep remove 002 001
  ahm task dep tree 002
  ahm task dep cycles`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("task dep requires a subcommand")
		},
	}
	dep.AddCommand(a.taskDepUpdateCommand("add", nil, "Add a task dependency", true, `Add a dependency to a task.

Examples:
  ahm task dep add 002 001
  ahm --dry-run task dep add 002 001`))
	dep.AddCommand(a.taskDepUpdateCommand("remove", []string{"rm"}, "Remove a task dependency", false, `Remove a dependency from a task.

Examples:
  ahm task dep remove 002 001
  ahm task dep rm 002 001
  ahm --dry-run task dep remove 002 001`))
	dep.AddCommand(&cobra.Command{
		Use:   "tree <id>",
		Short: "Print a task dependency tree",
		Long: `Print a dependency tree rooted at a task.

Examples:
  ahm task dep tree 002`,
		Args: exactArgs(1, "task dep tree requires an id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepTree(args)
		},
	})
	dep.AddCommand(&cobra.Command{
		Use:   "cycles",
		Short: "Print dependency cycles",
		Long: `Print active dependency cycles.

Examples:
  ahm task dep cycles`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepCycles()
		},
	})
	return dep
}

func (a *app) taskDepUpdateCommand(use string, aliases []string, short string, add bool, long string) *cobra.Command {
	return &cobra.Command{
		Use:     use + " <id> <dependency-id>",
		Aliases: aliases,
		Short:   short,
		Long:    long,
		Args:    exactArgs(2, "task dep add/remove requires task id and dependency id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			return a.taskDepUpdate(args, add)
		},
	}
}

func exactArgs(count int, message string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return usageError(message)
		}
		return nil
	}
}
func (a *app) taskDepUpdate(argv []string, add bool) error {
	task, err := a.resolveTask(argv[0])
	if err != nil {
		return err
	}
	dep, err := a.resolveTask(argv[1])
	if err != nil {
		return err
	}

	if add {
		if task.ID == dep.ID {
			return fmt.Errorf("task %s cannot depend on itself", task.ID)
		}
		if dep.Status == "Cancelled" {
			return fmt.Errorf("task %s cannot depend on cancelled task %s", task.ID, dep.ID)
		}
	}

	set := map[string]bool{}
	for _, item := range task.DependsOn {
		set[item] = true
	}

	// Skip write if the dependency set is unchanged.
	if add {
		if set[dep.ID] {
			fmt.Fprintf(a.out, "%s already depends on %s\n", task.ID, dep.ID)
			return nil
		}
		set[dep.ID] = true
	} else {
		if !set[dep.ID] {
			fmt.Fprintf(a.out, "%s does not depend on %s\n", task.ID, dep.ID)
			return nil
		}
		delete(set, dep.ID)
	}

	keys := make([]string, 0, len(set))
	for item := range set {
		keys = append(keys, item)
	}
	sort.Slice(keys, func(i, j int) bool {
		return taskLess(keys[i], keys[j])
	})

	// For add operations, check that the new dependency set does not introduce a cycle.
	if add {
		tasks, err := a.getTasks()
		if err != nil {
			// If we can't read all tasks, we can't check for cycles — reject to be safe.
			return fmt.Errorf("cannot verify dependency cycle safety: %w", err)
		}
		byID := map[string]Task{}
		for _, t := range tasks {
			byID[t.ID] = t
		}
		modified := task
		modified.DependsOn = keys
		byID[modified.ID] = modified

		modifiedTasks := make([]Task, 0, len(byID))
		for _, t := range byID {
			modifiedTasks = append(modifiedTasks, t)
		}
		cycles := taskDependencyCycles(modifiedTasks)
		if len(cycles) > 0 {
			return fmt.Errorf("adding dependency %s to %s would create a cycle: %s", dep.ID, task.ID, strings.Join(cycles[0], " -> "))
		}
	}

	task.DependsOn = keys

	task.Updated = time.Now().Format(time.RFC3339)
	if a.opts.dryRun {
		return a.emit(map[string]any{"task": task.ID, "depends_on": task.DependsOn})
	}
	if err := writeFileAtomic(task.Path, []byte(renderTask(task)), 0o644); err != nil {
		return err
	}
	if err := a.writeIndexes(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "%s depends_on: %s\n", task.ID, formatList(task.DependsOn))
	return nil
}

func (a *app) taskDepTree(argv []string) error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	root, err := resolveTaskFromTasks(argv[0], tasks)
	if err != nil {
		return err
	}
	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	var walk func(id string, prefix string, seen map[string]bool)
	walk = func(id string, prefix string, seen map[string]bool) {
		task, ok := byID[id]
		if !ok {
			fmt.Fprintf(a.out, "%s%s [missing]\n", prefix, id)
			return
		}
		if seen[id] {
			fmt.Fprintf(a.out, "%s  cycle to %s\n", prefix, id)
			return
		}
		fmt.Fprintf(a.out, "%s%s [%s] %s\n", prefix, task.ID, task.Status, task.Title)
		seen[id] = true
		for _, dep := range task.DependsOn {
			walk(dep, prefix+"  ", seen)
		}
		delete(seen, id)
	}
	walk(root.ID, "", map[string]bool{})
	return nil
}

func (a *app) taskDepCycles() error {
	defer a.emitWarnings()
	tasks, err := a.getTasks()
	if err != nil {
		a.addWarning("some task files could not be parsed and were skipped")
	}
	cycles := taskDependencyCycles(tasks)
	if len(cycles) == 0 {
		fmt.Fprintln(a.out, "No dependency cycles found")
		return nil
	}
	for _, cycle := range cycles {
		fmt.Fprintln(a.out, strings.Join(cycle, " -> "))
	}
	return nil
}
