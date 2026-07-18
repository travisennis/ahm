package ahm

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type recordsDoctorReport struct {
	OK     bool              `json:"ok"`
	Checks map[string]string `json:"checks"`
}

func (a *app) recordsCommand() *cobra.Command {
	records := &cobra.Command{
		Use:   "records",
		Short: "Manage workflow records and migration",
		Long: `Manage ahm workflow records and migration state.

Examples:
  ahm records migrate
  ahm records doctor`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("records requires a subcommand\n  ahm records migrate")
		},
	}
	records.AddCommand(a.simpleCommand("migrate", "Opt into committed .ahm records", `Migrate ahm-managed workflow state from .agents/ into tool-owned .ahm/.

Migration moves task, research, and ExecPlan files (including generated
indexes) to the same relative paths under .ahm/, installs internal
.ahm/.gitignore entries, writes committed .ahm/config.json, removes legacy
.agents/ahm.json, and prints the git rm -r --cached command needed to
untrack legacy record paths instead of running it. It never touches
project-owned .agents/ content such as .agents/prompt.md.

The command is safe to re-run: an interrupted migration resumes, and a fully
migrated repository reports its remaining git-index cleanup, if any.

Examples:
  ahm --dry-run records migrate
  ahm records migrate`, func() error {
		return a.recordsMigrate()
	}))
	records.AddCommand(a.simpleCommand("doctor", "Diagnose records migration state", `Diagnose records migration state.

Checks for leftover legacy record files or config under .agents/, legacy
dot-prefixed record paths under .ahm/, and legacy record paths still tracked
in the project git index. Reports migration status or pointers to
ahm records migrate.

Examples:
  ahm records doctor
  ahm --json records doctor`, func() error {
		return a.recordsDoctor()
	}))
	return records
}

func (a *app) recordsDoctor() error {
	ctx := context.Background()
	report := recordsDoctorReport{OK: true, Checks: map[string]string{}}
	_, err := readMetadata(a.opts.root)
	if err != nil {
		report.OK = false
		report.Checks["metadata"] = metadataErrorPath(err) + ": " + err.Error()
		return a.emit(report)
	}
	migration, migrationOK, err := recordsMigrationDiagnostic(ctx, a.opts.root)
	if err != nil {
		report.OK = false
		report.Checks["migration"] = err.Error()
	} else {
		if !migrationOK {
			report.OK = false
		}
		report.Checks["migration"] = migration
	}
	return a.emit(report)
}

func (r recordsDoctorReport) RenderText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "ok: %v\n", r.OK); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "checks:"); err != nil {
		return err
	}
	for _, key := range sortedStringKeys(r.Checks) {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", key, r.Checks[key]); err != nil {
			return err
		}
	}
	return nil
}
