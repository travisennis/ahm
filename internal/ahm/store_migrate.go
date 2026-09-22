package ahm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// `ahm store migrate --to home|project` moves this project's task records
// between the two layouts the home store introduces. It is the only command
// that moves records, and the only one that deletes them on purpose.
//
// The direction comes from --to alone and is never inferred from on-disk state,
// so a half-finished move can always be finished by repeating the command that
// started it. The move never renames: the store and the project may be on
// different volumes, so every record is read, written atomically into the
// destination through writeOwned, and removed from the source only afterwards.
// A crash or an interrupt therefore leaves a record on both sides, and
// re-running the command completes the move.
//
// Every read the move depends on happens before the first record moves, and the
// committed configuration is written last, after the records and the source's
// derived files are gone: it is the move's one commit point. Until it lands the
// configuration still names the source layout, so a repeated command takes the
// same path again and every other command still serializes on the lock this run
// holds. `ahm` never stages, commits, or moves HEAD: the project-side deletions
// and additions are left in the working tree for the user to review and commit.

// storeMigrateReport is the structured result of `ahm store migrate`.
type storeMigrateReport struct {
	To     string `json:"to"`
	DryRun bool   `json:"dry_run,omitempty"`

	// Moved is every record the run moves, with the source path it vacated and
	// the destination path it now fills.
	Moved []storeMoveReport `json:"moved"`
	// Written and Removed are the other files the run changes: the committed
	// configuration, the managed .gitignore of each layout, the generated
	// indexes of the destination, and the generated task indexes the source
	// vacated.
	Written []string `json:"written"`
	Removed []string `json:"removed"`
	// Deletions and Additions are the project paths the user must commit. A
	// move into the store leaves deletions, a move back into the project leaves
	// additions, and the other list stays empty.
	Deletions []string `json:"deletions,omitempty"`
	Additions []string `json:"additions,omitempty"`
}

// storeMoveReport is one moved record.
type storeMoveReport struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// empty reports whether the run had no work at all, which is what a repeated
// migration reports.
func (r storeMigrateReport) empty() bool {
	return len(r.Moved) == 0 && len(r.Written) == 0 && len(r.Removed) == 0
}

// RenderText prints the run as the actions it took, or, in dry-run mode, as the
// actions it would take. The commit lists are always printed, because they are
// the review step the command leaves to the user.
func (r storeMigrateReport) RenderText(w io.Writer) error {
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format+"\n", args...)
		return err
	}
	if r.empty() {
		return write("no work: task records already live in %s", locationLabel(r.To))
	}
	prefix := ""
	if r.DryRun {
		prefix = "would "
	}
	if len(r.Moved) == 0 {
		if err := write("%smove no records", prefix); err != nil {
			return err
		}
	} else {
		if err := write("%smove %d record(s) to %s", prefix, len(r.Moved), locationLabel(r.To)); err != nil {
			return err
		}
	}
	for _, move := range r.Moved {
		if err := write("  %s -> %s", move.From, move.To); err != nil {
			return err
		}
	}
	for _, path := range r.Written {
		if err := write("%swrite %s", prefix, path); err != nil {
			return err
		}
	}
	for _, path := range r.Removed {
		if err := write("%sremove %s", prefix, path); err != nil {
			return err
		}
	}
	for _, commit := range []struct {
		label string
		paths []string
	}{
		{"deletions", r.Deletions},
		{"additions", r.Additions},
	} {
		if len(commit.paths) == 0 {
			continue
		}
		header := commit.label + " to commit:"
		if r.DryRun {
			header = commit.label + " to commit after the move:"
		}
		if err := write("%s", header); err != nil {
			return err
		}
		for _, path := range commit.paths {
			if err := write("  %s", path); err != nil {
				return err
			}
		}
	}
	return nil
}

// locationLabel names a storage location for user-facing output.
func locationLabel(location string) string {
	if taskLocation(location) == locationHome {
		return "the home store"
	}
	return "the project"
}

// storeMigrateCommand is the `store migrate` subcommand.
func (a *app) storeMigrateCommand() *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "migrate --to home|project",
		Short: "Move task records between the project and the home store",
		Long: `Move this project's task records between the project and the user-level store.

--to is required and names the destination; the direction is never inferred
from the current configuration, so repeating the command after an interrupt
finishes the move that was started.

Each record is read, written atomically into the destination, and then removed
from the source, never renamed across filesystems. Indexes are regenerated on
the destination side, the ADR index stays under the project root, the committed
tasks_location key and the managed .gitignore are rewritten for the new mode,
and the store's task ID counter is initialized from the records present.

The command refuses to move records out of the project when they have
uncommitted changes, because Git history is then the only record of what they
were; --force moves them anyway. It also refuses a store directory that is
registered to another project key, and a destination that already holds the
arriving record's identity — the same name with different bytes, or the same ID
in another bucket; --force moves the record anyway, overwriting the first and
leaving the second beside it. ahm never stages or commits: the deletions or
additions the move leaves in the project are printed for the user to review and
commit.

Supports --dry-run, --json, --plain, and --text output.

Examples:
  ahm --dry-run store migrate --to home
  ahm store migrate --to home
  ahm store migrate --to project`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.detectRoot(); err != nil {
				return err
			}
			location, err := parseTaskLocation(to)
			if err != nil {
				return err
			}
			return a.storeMigrate(location, a.opts.force)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "Destination for task records: home or project")
	return cmd
}

// parseTaskLocation validates the --to flag. The direction is required instead
// of inferred, so a migration is always the move the user asked for.
func parseTaskLocation(value string) (taskLocation, error) {
	switch taskLocation(value) {
	case locationHome:
		return locationHome, nil
	case locationProject:
		return locationProject, nil
	case "":
		return "", usageError("store migrate requires --to\n  ahm store migrate --to home|project")
	default:
		return "", usageError(fmt.Sprintf("unknown --to value %q (valid: home, project)", value))
	}
}

// storeMigrate moves this project's task records to the requested layout. The
// whole move holds the record-mutation lock of the layout the configuration
// names, which is the lock every other command serializes on; a dry run takes
// no lock and writes nothing.
func (a *app) storeMigrate(to taskLocation, force bool) error {
	from, destination, err := a.migrationLayouts(to)
	if err != nil {
		return err
	}
	if err := checkMigrationDestination(destination); err != nil {
		return err
	}
	return a.withWorkflowRecordLock(!a.opts.dryRun, func() error {
		return a.migrateTaskRecords(from, destination, force)
	})
}

// migrationLayouts resolves the source and destination layouts of a migration.
// The source is always the other layout, whatever the configuration currently
// says, so re-running the command continues a move instead of inverting it: an
// interrupted move leaves records on both sides, and the destination layout
// already holds the ones that arrived.
func (a *app) migrationLayouts(to taskLocation) (workflowPaths, workflowPaths, error) {
	store, err := resolveStore(a.opts.root)
	if err != nil {
		return workflowPaths{}, workflowPaths{}, err
	}
	project := workflowPathsFor(a.opts.root)
	home := workflowPathsForStore(a.opts.root, store)
	if to == locationHome {
		return project, home, nil
	}
	return home, project, nil
}

// checkMigrationDestination refuses a destination whose store directory is
// registered to a different project key. The registry is the authority for the
// mapping from a key to its directory, so it is where a collision between two
// projects' records is visible; moving into such a directory would mix two
// backlogs in one store location.
func checkMigrationDestination(to workflowPaths) error {
	if !to.inStore() {
		return nil
	}
	reg, err := loadRegistry(to.store.Root)
	if err != nil {
		return err
	}
	var claimedBy []string
	for key, entry := range reg.Projects {
		if key != to.store.Key && entry.Dir == to.store.dirName() {
			claimedBy = append(claimedBy, key)
		}
	}
	if len(claimedBy) == 0 {
		return nil
	}
	sort.Strings(claimedBy)
	return fmt.Errorf(
		"refusing to move task records for %s into %s: that store directory is registered to %s, not %s",
		to.projectRoot, to.store.ProjectDir, strings.Join(claimedBy, ", "), to.store.Key,
	)
}

// migrateTaskRecords performs one migration. Every write it makes is idempotent
// and is skipped in dry-run mode, so a repeated command reports no work instead
// of rewriting files.
//
// Order matters here. Everything the move owes after it has moved the records
// is read first, while the records are still in their source layout, so a
// precondition that cannot be met fails before the first byte moves. The
// records move next. The derived files that described them are removed after
// that, because the committed .gitignore write is what stops ignoring them. The
// committed tasks_location key is written last: it is the move's single commit
// point, so until it lands the configuration still names the source layout, a
// re-run of the same command takes this path again, and the lock this run holds
// is still the lock every other command takes.
func (a *app) migrateTaskRecords(from workflowPaths, to workflowPaths, force bool) error {
	defer a.emitWarnings()

	files, err := taskFilePathsFor(from)
	if err != nil {
		return err
	}
	report := storeMigrateReport{To: string(to.mode), DryRun: a.opts.dryRun}

	// The configuration already names the destination and the source holds no
	// records: the move itself has nothing to do. A run that stopped before it
	// cleaned up its source may still have left the source's derived files
	// behind, so they are cleaned up here rather than skipped: nothing else
	// removes them once the committed .gitignore stopped ignoring them.
	if len(files) == 0 && a.workflowPaths().inStore() == to.inStore() {
		return a.finishSourceCleanup(from, &report)
	}

	// Every read the move depends on happens before the records move.
	plan, err := a.planMigration(from, to)
	if err != nil {
		return err
	}

	// Records moving out of the project are about to be deleted, and Git
	// history is the only record of their uncommitted content.
	if to.inStore() && len(files) > 0 && !force {
		if err := refuseUncommittedRecords(a.opts.root, from); err != nil {
			return err
		}
	}
	// A destination that already holds different bytes for a record is not the
	// resume case, and it must not be overwritten in silence.
	if !force {
		if err := refuseRecordConflicts(from, to, files); err != nil {
			return err
		}
	}

	sources := make([]string, 0, len(files))
	for _, file := range files {
		sources = append(sources, file.Path)
	}
	if !a.opts.dryRun {
		moved, moveErr := moveTaskRecords(from, to)
		if moveErr != nil {
			return moveErr
		}
		sources = moved
	}
	for _, src := range sources {
		dst, pathErr := migratedRecordPath(from, to, src)
		if pathErr != nil {
			return pathErr
		}
		report.Moved = append(report.Moved, storeMoveReport{From: from.displayPath(src), To: to.displayPath(dst)})
		if to.inStore() {
			report.Deletions = append(report.Deletions, from.displayPath(src))
		} else {
			report.Additions = append(report.Additions, to.displayPath(dst))
		}
	}

	// The records live in the destination from here on, so the destination is
	// the layout every following write and read resolves through.
	a.useWorkflowPaths(to)
	a.invalidateTasks()

	// The store's state file and registry are store-local bookkeeping, and a
	// preview creates no store at all. The task ID counter is initialized from
	// the records that arrive, so neither path can hand out an ID already in use.
	if !a.opts.dryRun {
		if err := a.initializeTaskIDCounter(to); err != nil {
			return err
		}
		if plan.record {
			if err := recordStoreMigration(plan.store, from.mode); err != nil {
				return err
			}
		}
	}
	// The records are gone from the source, so the derived files that described
	// them go too, along with the record directories they emptied.
	if err := a.removeSourceIndexes(from, &report); err != nil {
		return err
	}
	if !a.opts.dryRun {
		pruneEmptyRecordsDirs(from)
	}
	if err := a.regenerateMigratedIndexes(to, &report); err != nil {
		return err
	}
	// The managed .gitignore of the destination is written before the committed
	// configuration, and only after the source's generated indexes are gone,
	// because it is the write that stops ignoring them.
	if err := a.writeMigratedGitignores(to, &report); err != nil {
		return err
	}
	// The committed configuration is the move's commit point, so it is written
	// last of all.
	if storeMigrateCommitHook != nil {
		if err := storeMigrateCommitHook(); err != nil {
			return err
		}
	}
	if err := a.writeMigratedConfig(to, plan.config, &report); err != nil {
		return err
	}
	return a.emit(report)
}

// migrationPlan is what a move resolves before it touches the source records:
// the committed configuration it will write, and the store whose state and
// registry it will record. Resolving both up front is what keeps a precondition
// failure from stranding records in the destination with no report and no way
// back, because every read the move depends on then happens while the records
// are still in their source layout.
type migrationPlan struct {
	config []byte
	store  storePaths
	// record says whether recording the move in the store is meaningful: a move
	// out of a store that does not exist holds nothing to record, because every
	// record is already in the project, and creating the store to say so would
	// give a project that keeps its records in the project a store for nothing.
	record bool
}

// planMigration reads and validates everything the move owes after it has moved
// the records: the committed configuration, and — on a real run — the store
// state and registry it writes. A dry run writes neither, so it reads neither,
// which keeps its effect on the machine to reading the records it previews.
func (a *app) planMigration(from workflowPaths, to workflowPaths) (migrationPlan, error) {
	meta, err := readMetadata(a.opts.root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return migrationPlan{}, fmt.Errorf("reading workflow metadata %s: %w", configMetadataRelPath, err)
	}
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	meta.TasksLocation = string(to.mode)
	config, err := marshalMetadata(meta)
	if err != nil {
		return migrationPlan{}, err
	}
	store := to.store
	if !to.inStore() {
		store = from.store
	}
	plan := migrationPlan{config: config, store: store, record: to.inStore()}
	if a.opts.dryRun {
		return plan, nil
	}
	_, stateErr := readProjectState(store)
	if stateErr != nil && !errors.Is(stateErr, fs.ErrNotExist) {
		return migrationPlan{}, stateErr
	}
	plan.record = plan.record || stateErr == nil
	if _, err := loadRegistry(store.Root); err != nil {
		return migrationPlan{}, err
	}
	return plan, nil
}

// finishSourceCleanup removes the derived files and directories a run that
// stopped partway may have left in the source layout, and emits report. It is
// the whole job of a run whose records already live in the destination: the
// move itself has nothing to move, but nothing else removes the source's
// generated indexes, and the run that left them behind reports no work on every
// re-run.
func (a *app) finishSourceCleanup(from workflowPaths, report *storeMigrateReport) error {
	if err := a.removeSourceIndexes(from, report); err != nil {
		return err
	}
	if !a.opts.dryRun {
		pruneEmptyRecordsDirs(from)
	}
	return a.emit(*report)
}

// refuseRecordConflicts fails a move whose destination already holds a record
// that is not the record arriving: the same name with different bytes, or the
// same ID in another bucket. A record an interrupted move already delivered is
// byte-identical and lands in the same bucket, so neither case is a resume. The
// store copy has no history to recover from, the project copy may be the only
// committed one, and two records with one ID is a state no command can resolve,
// so both paths are named and --force is the documented override.
func refuseRecordConflicts(from workflowPaths, to workflowPaths, files []taskFileInfo) error {
	for _, file := range files {
		dst, err := migratedRecordPath(from, to, file.Path)
		if err != nil {
			return err
		}
		source, err := os.ReadFile(file.Path) // #nosec G304 // record path produced by the task scan under the resolved records root
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		existing, err := os.ReadFile(dst) // #nosec G304 // destination built from the resolved records root
		switch {
		case err == nil && !bytes.Equal(existing, source):
			return fmt.Errorf(
				"refusing to move %s: %s already holds a different record; pass --force to overwrite it",
				from.displayPath(file.Path), to.displayPath(dst),
			)
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return err
		}
		// The record's own destination does not conflict, but a second record
		// wearing its ID in another bucket would survive the move beside it.
		id := strings.TrimSuffix(filepath.Base(file.Path), ".md")
		for _, bucket := range taskBuckets {
			if bucket == file.Bucket {
				continue
			}
			other := to.taskFile(bucket, id)
			switch _, statErr := os.Stat(other); {
			case statErr == nil:
				return fmt.Errorf(
					"refusing to move %s: %s already holds a record with ID %s; pass --force to move it anyway",
					from.displayPath(file.Path), to.displayPath(other), id,
				)
			case !errors.Is(statErr, fs.ErrNotExist):
				return statErr
			}
		}
	}
	return nil
}

// migratedRecordPath is where a source record lands in the destination layout.
// Both layouts keep the same shape under their records root, so the bucket and
// the file name carry over unchanged.
func migratedRecordPath(from workflowPaths, to workflowPaths, src string) (string, error) {
	rel, err := filepath.Rel(from.recordsRoot, src)
	if err != nil {
		return "", fmt.Errorf("resolving the destination of %s: %w", src, err)
	}
	bucket := filepath.Dir(rel)
	if bucket == "." {
		bucket = ""
	}
	return to.taskFile(bucket, strings.TrimSuffix(filepath.Base(rel), ".md")), nil
}

// storeMigrateRecordHook, when non-nil, runs after each record has been copied
// into the destination and removed from the source. It lets tests interrupt a
// move at a deterministic point and verify that re-running it finishes the job.
var storeMigrateRecordHook func(moved int) error

// storeMigrateCommitHook, when non-nil, runs once after every write the move
// owes except the committed configuration, and before that last write. It lets
// tests interrupt a move at its commit point and verify that the configuration
// it has not written yet is what makes a re-run finish the move.
var storeMigrateCommitHook func() error

// moveTaskRecords moves every task record from one layout's records root into
// the other's and returns the source paths it vacated.
//
// Each record is read, written atomically into the destination, and only then
// removed from the source, so an interrupted move can always be finished by
// re-running it: a record that already arrived keeps its destination bytes and
// only its source is removed. Nothing is renamed, because the store and the
// project may be on different volumes. A source record that vanishes mid-move
// is skipped rather than reported, because another process removing it means
// the move it belongs to already happened.
//
// The caller holds the record-mutation lock for the whole move.
func moveTaskRecords(from workflowPaths, to workflowPaths) (moved []string, err error) {
	files, err := taskFilePathsFor(from)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		dst, err := migratedRecordPath(from, to, file.Path)
		if err != nil {
			return moved, err
		}
		if err := copyRecord(to, dst, file.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return moved, err
		}
		if err := os.Remove(file.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return moved, err
		}
		moved = append(moved, file.Path)
		if storeMigrateRecordHook != nil {
			if err := storeMigrateRecordHook(len(moved)); err != nil {
				return moved, err
			}
		}
	}
	return moved, nil
}

// copyRecord writes the bytes of one record into the destination through
// writeOwned, leaving the source in place. An identical destination file is left
// untouched, so the record an interrupted move already copied keeps its bytes
// and its modification time. The caller has already refused a destination record
// with different bytes unless --force was given, so the write that survives that
// check is either the resume case or the override.
func copyRecord(to workflowPaths, dst string, src string) error {
	data, err := os.ReadFile(src) // #nosec G304 // record path produced by the task scan under the resolved records root
	if err != nil {
		return err
	}
	switch existing, readErr := os.ReadFile(dst); { // #nosec G304 // destination built from the resolved records root
	case readErr == nil && bytes.Equal(existing, data):
		return nil
	case readErr != nil && !errors.Is(readErr, fs.ErrNotExist):
		return readErr
	}
	return writeOwned(to, dst, data)
}

// refuseUncommittedRecords fails a move out of the project when Git reports
// changes to the records, because the move deletes them and only committed
// content is recoverable afterwards. The affected paths are named, and --force
// is the documented override.
func refuseUncommittedRecords(root string, from workflowPaths) error {
	paths, err := uncommittedRecordPaths(root, from)
	if err != nil {
		return fmt.Errorf("checking %s for uncommitted changes: %w", from.displayPath(from.recordsRoot), err)
	}
	if len(paths) == 0 {
		return nil
	}
	var message strings.Builder
	message.WriteString("refusing to move task records: " + from.displayPath(from.recordsRoot) + " has uncommitted changes\n")
	for _, path := range paths {
		message.WriteString("  " + path + "\n")
	}
	message.WriteString("commit or discard them first, or pass --force to move them anyway")
	return errors.New(message.String())
}

// uncommittedRecordPaths returns the project-relative paths under the records
// root that hold record content only the working tree has: modified, staged, or
// untracked task records. Deletions are not reported, because the deleted
// content is either still in Git's history or already in the destination, and a
// half-finished move leaves exactly such a deletion behind. Generated indexes
// and any other file the records tree holds are not reported either: they are
// derived or foreign, the move neither deletes nor protects them, and naming
// them would ask the user to commit output ahm regenerates. A root without Git
// metadata of its own has no history to protect and no Git to ask, so it reports
// none.
func uncommittedRecordPaths(root string, paths workflowPaths) ([]string, error) {
	if !hasGitMetadata(root) {
		return nil, nil
	}
	out, err := runGit(root, "status", "--porcelain", "--untracked-files=all", "--", paths.recordsRel())
	if err != nil {
		return nil, err
	}
	return recordPathsOnly(paths, parsePorcelainPaths(out)), nil
}

// recordPathsOnly keeps the porcelain entries that name task record content: a
// .md file directly inside one of the layout's record buckets, which is exactly
// the set the task scan reads. A generated index is derived and regenerated on
// the destination side, and the move removes the source's indexes on purpose;
// anything else the records tree holds is foreign. None of them is record
// content the move can lose, so naming one would ask the user to deal with a
// file the move either regenerates or never touches.
func recordPathsOnly(paths workflowPaths, entries []string) []string {
	prefix := filepath.ToSlash(paths.recordsRel()) + "/"
	records := make([]string, 0, len(entries))
	for _, entry := range entries {
		rest, ok := strings.CutPrefix(filepath.ToSlash(entry), prefix)
		if !ok {
			continue
		}
		segments := strings.Split(rest, "/")
		if len(segments) != 2 || !slices.Contains(taskBuckets, segments[0]) {
			continue
		}
		if name := segments[1]; !strings.HasSuffix(name, ".md") || name == "index.md" {
			continue
		}
		records = append(records, entry)
	}
	return records
}

// parsePorcelainPaths extracts one path per `git status --porcelain` entry. The
// status codes are two characters and a space; a rename or copy entry carries
// "old -> new" and is reported by its new path; Git C-quotes a path that needs
// escaping, and the quoting is undone here so findings carry plain paths. An
// entry that only records a deletion is skipped: the deleted content is either
// still in Git's history or already in the destination, so it is not content the
// move can lose — and a half-finished move leaves exactly such a deletion
// behind.
func parsePorcelainPaths(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		status, entry := line[:2], line[3:]
		if strings.ContainsRune(status, 'D') {
			continue
		}
		entry = strings.TrimSpace(entry)
		if idx := strings.LastIndex(entry, " -> "); idx >= 0 {
			entry = strings.TrimSpace(entry[idx+4:])
		}
		if unquoted, err := strconv.Unquote(entry); err == nil {
			entry = unquoted
		}
		if entry != "" {
			paths = append(paths, entry)
		}
	}
	return paths
}

// writeMigratedConfig writes the committed configuration with tasks_location
// naming the destination, from the bytes the move resolved before it moved a
// record. The key is written explicitly in both directions: a repository that
// moves back to the project must keep the project layout even once a later
// release defaults a configuration without the key to the store.
func (a *app) writeMigratedConfig(to workflowPaths, config []byte, report *storeMigrateReport) error {
	return a.writeMigratedFile(to, to.configPath(), config, report)
}

// writeMigratedGitignores writes the managed .gitignore of both layouts for the
// destination mode: the committed .ahm/.gitignore, and — when the records move
// into the store — the store's own .gitignore beside them.
func (a *app) writeMigratedGitignores(to workflowPaths, report *storeMigrateReport) error {
	projectGitignore := to.projectGitignorePath()
	if err := a.writeMigratedFile(to, projectGitignore, to.projectGitignoreContent(), report); err != nil {
		return err
	}
	if !to.inStore() {
		return nil
	}
	storeGitignore := to.workflowGitignorePath()
	return a.writeMigratedFile(to, storeGitignore, to.workflowGitignoreContent(), report)
}

// removeSourceIndexes removes the generated task indexes the source layout
// owned. They are derived, the records they listed just left, and in home mode
// the committed .gitignore stops ignoring them, so leaving them behind would
// turn them into untracked files that describe an empty backlog.
func (a *app) removeSourceIndexes(from workflowPaths, report *storeMigrateReport) error {
	// The records root index plus one per bucket, the same set index
	// generation writes.
	for _, bucket := range []string{"", "active", "completed", "cancelled"} {
		path := filepath.Join(from.tasksBucketDir(bucket), "index.md")
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return err
		}
		report.Removed = append(report.Removed, from.displayPath(path))
		if a.opts.dryRun {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// regenerateMigratedIndexes regenerates the destination's generated indexes:
// its task indexes and the ADR index, which lives in the project in both
// layouts. Post-mutation findings are emitted so a record that fails to parse or
// a dependency that no longer resolves is reported by the move that exposed it.
func (a *app) regenerateMigratedIndexes(to workflowPaths, report *storeMigrateReport) error {
	tasks, taskErr := a.getTasks()
	if taskErr != nil {
		if tasks == nil {
			return taskErr
		}
		a.addWarning("some task files could not be parsed and were skipped: %s", taskErr)
	}
	cache := newRecordCache()
	writes, adrErr := indexWritesForPaths(a.opts.root, tasks, to, cache)
	if adrErr != nil {
		if writes == nil {
			return adrErr
		}
		a.addWarning("%s", adrErr)
	}
	for _, path := range sortedKeys(writes) {
		if !isStaleIndex(nil, path, writes[path]) {
			continue
		}
		if err := a.writeMigratedFile(to, path, []byte(writes[path]), report); err != nil {
			return err
		}
	}
	if a.opts.dryRun {
		return nil
	}
	// The flag reports the task scan's completeness, not the ADR list's: a
	// partial task set makes the reuse path validate the records it has instead
	// of re-reading the tree, exactly as the index-writing path does.
	if taskErr == nil {
		a.tasksCache = tasks
	}
	a.emitPostMutationFindings(tasks, writes, taskErr == nil, cache)
	return nil
}

// writeMigratedFile writes content to an ahm-owned path unless it already holds
// those exact bytes, so a repeated command writes nothing, and records the write
// in the report. It refuses a path outside the layout's owned roots, like every
// other workflow write.
func (a *app) writeMigratedFile(paths workflowPaths, path string, content []byte, report *storeMigrateReport) error {
	existing, err := os.ReadFile(path) // #nosec G304 // path built from a workflow accessor, not user input
	switch {
	case err == nil && bytes.Equal(existing, content):
		return nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	}
	report.Written = append(report.Written, paths.displayPath(path))
	if a.opts.dryRun {
		return nil
	}
	return writeOwned(paths, path, content)
}

// pruneEmptyRecordsDirs removes the source's now-empty records directories after
// a move, so a migrated project keeps no empty records tree and a migrated store
// keeps no empty records directory. Only empty directories are removed: anything
// left behind stays where it is and is reported by validation as drift.
func pruneEmptyRecordsDirs(from workflowPaths) {
	for _, bucket := range taskBuckets {
		_ = os.Remove(from.tasksBucketDir(bucket))
	}
	_ = os.Remove(from.recordsRoot)
}
