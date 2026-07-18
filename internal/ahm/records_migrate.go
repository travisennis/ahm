package ahm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	recordsGitignoreRelPath = ".ahm/.gitignore"

	migrateActionCreate    = "create"
	migrateActionUpdate    = "update"
	migrateActionRemove    = "remove"
	migrateActionUnchanged = "unchanged"
	migrateActionAbsent    = "absent"
)

// legacyRecordMigrationRoots are the ahm-managed record trees that migration
// moves from the project-owned .agents/ namespace into tool-owned .ahm/.
// Project-owned .agents/ content outside these roots (prompt.md, skills,
// AGENTS.md guidance) is never touched.
var legacyRecordMigrationRoots = []string{
	".agents/.tasks",
	".agents/.research",
	".agents/exec-plans",
}

// recordsGitignoreEntries keep generated workflow indexes and machine-local
// state out of branch history while source records and .ahm/config.json stay
// committed.
var recordsGitignoreEntries = []string{
	"index.md",
	".lock/",
	"*.tmp",
}

const recordsGitignoreHeader = "# Managed by ahm. Generated workflow indexes and machine-local state stay local-only;\n# source records and config.json remain committed.\n"

type recordsMigrateMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type recordsMigrateReport struct {
	Action       string               `json:"action"`
	DryRun       bool                 `json:"dry_run,omitempty"`
	Moves        []recordsMigrateMove `json:"moves,omitempty"`
	Gitignore    string               `json:"gitignore"`
	Config       string               `json:"config"`
	LegacyConfig string               `json:"legacy_config"`
	GitCleanup   string               `json:"git_cleanup,omitempty"`
	Message      string               `json:"message"`
}

type recordsMigratePlan struct {
	meta         metadata
	moves        []recordsMigrateMove
	gitignore    string
	gitignoreAdd []string
	config       string
	legacyConfig string
	tracked      []string
}

func (p recordsMigratePlan) complete() bool {
	return len(p.moves) == 0 &&
		p.gitignore == migrateActionUnchanged &&
		p.config == migrateActionUnchanged &&
		p.legacyConfig == migrateActionAbsent
}

func (a *app) recordsMigrate() error {
	ctx := context.Background()
	if a.opts.dryRun {
		plan, err := a.buildRecordsMigratePlan(ctx)
		if err != nil {
			return err
		}
		return a.emit(newRecordsMigrateReport(plan, true))
	}
	release, err := acquireWorkflowRecordMigrationLocksForPaths(a.opts.root, a.workflowPaths())
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			fmt.Fprintln(a.err, err)
		}
	}()
	// Plan under the lock so a concurrent record mutation cannot slip
	// between planning and execution.
	plan, err := a.buildRecordsMigratePlan(ctx)
	if err != nil {
		return err
	}
	report := newRecordsMigrateReport(plan, false)
	if plan.complete() {
		return a.emit(report)
	}
	if err := a.executeRecordsMigratePlan(plan); err != nil {
		return err
	}
	a.invalidateWorkflowPaths()
	return a.emit(report)
}

func newRecordsMigrateReport(plan recordsMigratePlan, dryRun bool) recordsMigrateReport {
	return recordsMigrateReport{
		Action:       "migrate",
		DryRun:       dryRun,
		Moves:        plan.moves,
		Gitignore:    plan.gitignore,
		Config:       plan.config,
		LegacyConfig: plan.legacyConfig,
		GitCleanup:   legacyRecordsGitCleanupCommand(plan.tracked),
		Message:      recordsMigrateMessage(plan, dryRun),
	}
}

func (a *app) buildRecordsMigratePlan(ctx context.Context) (recordsMigratePlan, error) {
	root := a.opts.root
	if out, err := runGit(ctx, root, []string{"--is-inside-work-tree"}, "rev-parse"); err != nil || strings.TrimSpace(out) != "true" {
		return recordsMigratePlan{}, fmt.Errorf("records migration requires a git work tree at %s", root)
	}
	meta, source, err := readMetadataWithSource(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return recordsMigratePlan{}, fmt.Errorf("workflow metadata %s not found; run 'ahm init' before migrating records", metadataErrorPath(err))
		}
		return recordsMigratePlan{}, fmt.Errorf("%s: %w", metadataCorruptMessage(err), err)
	}
	relinquishedSkills := relinquishProjectOwnedProcedureSkills(&meta)
	relinquishedScaffolds := relinquishPreservedScaffoldFiles(&meta)
	plan := recordsMigratePlan{meta: meta}
	plan.moves, err = collectRecordsMigrateMoves(root)
	if err != nil {
		return recordsMigratePlan{}, err
	}
	// Determine config action: create if metadata is still in legacy
	// location, update if already in .ahm/config.json (to strip any
	// stale ref-back fields), or unchanged if no moves are needed and
	// config is already in the right place.
	switch {
	case source != configMetadataRelPath:
		plan.config = migrateActionCreate
	case len(plan.moves) > 0 || relinquishedSkills || relinquishedScaffolds:
		plan.config = migrateActionUpdate
	default:
		plan.config = migrateActionUnchanged
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(legacyMetadataRelPath))); err == nil {
		plan.legacyConfig = migrateActionRemove
	} else if errors.Is(err, os.ErrNotExist) {
		plan.legacyConfig = migrateActionAbsent
	} else {
		return recordsMigratePlan{}, err
	}
	plan.gitignore, plan.gitignoreAdd, err = planRecordsGitignore(root)
	if err != nil {
		return recordsMigratePlan{}, err
	}
	plan.tracked, err = trackedLegacyRecordPaths(ctx, root)
	if err != nil {
		return recordsMigratePlan{}, err
	}
	return plan, nil
}

func (a *app) executeRecordsMigratePlan(plan recordsMigratePlan) error {
	root := a.opts.root
	for _, move := range plan.moves {
		if err := moveRecordFile(root, move); err != nil {
			return err
		}
	}
	if err := removeEmptyLegacyRecordDirs(root); err != nil {
		return err
	}
	if plan.gitignore != migrateActionUnchanged {
		if err := writeRecordsGitignore(root, plan.gitignoreAdd); err != nil {
			return err
		}
	}
	if plan.config != migrateActionUnchanged {
		if err := writeConfigMetadata(root, plan.meta); err != nil {
			return err
		}
	}
	if plan.legacyConfig == migrateActionRemove {
		legacy := filepath.Join(root, filepath.FromSlash(legacyMetadataRelPath))
		if err := os.Remove(legacy); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func collectRecordsMigrateMoves(root string) ([]recordsMigrateMove, error) {
	var moves []recordsMigrateMove
	for _, dir := range legacyRecordMigrationRoots {
		absDir := filepath.Join(root, filepath.FromSlash(dir))
		err := filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("record file symlinks are not supported: %s", relPath(root, path))
			}
			move := recordsMigrateMove{From: relPath(root, path)}
			rel := strings.TrimPrefix(move.From, ".agents/")
			// Strip the leading dot from .tasks/, .research/ to use non-dot names under .ahm/.
			move.To = ".ahm/" + strings.TrimPrefix(rel, ".")
			if err := checkRecordsMigrateTarget(root, move); err != nil {
				return err
			}
			moves = append(moves, move)
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("plan records migration from %s: %w", dir, err)
		}
	}
	// Migrate legacy dot-prefixed subdirectories under .ahm/ (task 165) to
	// non-dot names. This handles repositories that migrated before the
	// non-dot-record-directory convention was introduced.
	oldAHMRecordDirs := []string{
		".ahm/.tasks",
		".ahm/.research",
	}
	for _, dir := range oldAHMRecordDirs {
		absDir := filepath.Join(root, filepath.FromSlash(dir))
		err := filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("record file symlinks are not supported: %s", relPath(root, path))
			}
			move := recordsMigrateMove{From: relPath(root, path)}
			// Strip the extra dot: .ahm/.tasks/... -> .ahm/tasks/...
			move.To = ".ahm/" + strings.TrimPrefix(move.From, ".ahm/.")
			if err := checkRecordsMigrateTarget(root, move); err != nil {
				return err
			}
			moves = append(moves, move)
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("plan records migration from %s: %w", dir, err)
		}
	}
	sort.Slice(moves, func(i, j int) bool {
		return moves[i].From < moves[j].From
	})
	return moves, nil
}

// checkRecordsMigrateTarget allows resuming an interrupted migration: a target
// that already holds identical content is fine, but differing content needs a
// human decision instead of a silent overwrite.
func checkRecordsMigrateTarget(root string, move recordsMigrateMove) error {
	target := filepath.Join(root, filepath.FromSlash(move.To))
	existing, err := os.ReadFile(target) // #nosec G304 // migration targets stay under the fixed .ahm record roots.
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(move.From))) // #nosec G304 // migration sources stay under the fixed .agents record roots.
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, source) {
		return fmt.Errorf("migration target %s already exists with different content than %s; resolve the conflict before migrating", move.To, move.From)
	}
	return nil
}

func moveRecordFile(root string, move recordsMigrateMove) error {
	source := filepath.Join(root, filepath.FromSlash(move.From))
	if err := checkRecordsMigrateTarget(root, move); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(move.To))
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		data, err := os.ReadFile(source) // #nosec G304 // migration sources stay under the fixed .agents record roots.
		if err != nil {
			return err
		}
		if err := writeFileAtomic(target, data, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return os.Remove(source)
}

func removeEmptyLegacyRecordDirs(root string) error {
	// Clean up emptied legacy .agents/ record dirs.
	for _, dir := range legacyRecordMigrationRoots {
		absDir := filepath.Join(root, filepath.FromSlash(dir))
		var dirs []string
		err := filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				dirs = append(dirs, path)
			}
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		// Children sort after their parent prefix, so reverse order removes
		// the deepest directories first.
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		for _, path := range dirs {
			err := os.Remove(path)
			if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
				return err
			}
		}
	}
	// Clean up emptied old dot-prefixed .ahm/ record dirs (task 165).
	oldAHMDirs := []string{
		".ahm/.tasks",
		".ahm/.research",
	}
	for _, dir := range oldAHMDirs {
		absDir := filepath.Join(root, filepath.FromSlash(dir))
		var dirs []string
		err := filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				dirs = append(dirs, path)
			}
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		for _, path := range dirs {
			err := os.Remove(path)
			if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
				return err
			}
		}
	}
	return nil
}

func planRecordsGitignore(root string) (string, []string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(recordsGitignoreRelPath))) // #nosec G304 // path constructed from project root, not user input
	if errors.Is(err, os.ErrNotExist) {
		return migrateActionCreate, recordsGitignoreEntries, nil
	}
	if err != nil {
		return "", nil, err
	}
	present := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		present[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, entry := range recordsGitignoreEntries {
		if !present[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return migrateActionUnchanged, nil, nil
	}
	return migrateActionUpdate, missing, nil
}

func writeRecordsGitignore(root string, missing []string) error {
	path := filepath.Join(root, filepath.FromSlash(recordsGitignoreRelPath))
	existing, err := os.ReadFile(path) // #nosec G304 // path constructed from project root, not user input
	if errors.Is(err, os.ErrNotExist) {
		content := recordsGitignoreHeader + strings.Join(recordsGitignoreEntries, "\n") + "\n"
		return writeFileAtomic(path, []byte(content), 0o644)
	}
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	for _, entry := range missing {
		buf.WriteString(entry)
		buf.WriteByte('\n')
	}
	return writeFileAtomic(path, buf.Bytes(), 0o644)
}

func trackedLegacyRecordPaths(ctx context.Context, root string) ([]string, error) {
	candidates := append([]string{}, legacyRecordMigrationRoots...)
	candidates = append(candidates, legacyMetadataRelPath)
	var tracked []string
	for _, candidate := range candidates {
		out, err := runGit(ctx, root, []string{"-z", "--", candidate}, "ls-files")
		if err != nil {
			return nil, err
		}
		if strings.Trim(out, "\x00") != "" {
			tracked = append(tracked, candidate)
		}
	}
	return tracked, nil
}

func legacyRecordsGitCleanupCommand(tracked []string) string {
	if len(tracked) == 0 {
		return ""
	}
	return "git add .ahm/ && git rm -r --cached " + strings.Join(tracked, " ")
}

func recordsMigrateMessage(plan recordsMigratePlan, dryRun bool) string {
	switch {
	case dryRun:
		return "dry run: previewed records migration; no files, metadata, or gitignore were changed"
	case plan.complete() && len(plan.tracked) > 0:
		return "records storage is already migrated; run the printed git_cleanup command to finish Git tracking"
	case plan.complete():
		return "records storage is already migrated"
	case len(plan.tracked) > 0:
		return "migrated workflow records to .ahm/; review with git status, then run the printed git_cleanup command and commit"
	default:
		return "migrated workflow records to .ahm/; review with git status, then commit the new .ahm/ paths"
	}
}

// recordsMigrationDiagnostic reports incomplete or partially migrated state:
// leftover legacy record files, a leftover legacy config, or legacy record
// paths still tracked in the project git index.
func recordsMigrationDiagnostic(ctx context.Context, root string) (string, bool, error) {
	var leftovers []string
	for _, dir := range legacyRecordMigrationRoots {
		present, err := dirContainsFiles(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return "", false, err
		}
		if present {
			leftovers = append(leftovers, dir)
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(legacyMetadataRelPath))); err == nil {
		leftovers = append(leftovers, legacyMetadataRelPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if len(leftovers) > 0 {
		return "legacy record paths remain (" + strings.Join(leftovers, ", ") + "); run 'ahm records migrate'", false, nil
	}
	// Check for legacy dot-prefixed subdirectories under .ahm/ (task 165).
	oldAHMDirs := []string{
		".ahm/.tasks",
		".ahm/.research",
	}
	for _, dir := range oldAHMDirs {
		present, err := dirContainsFiles(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return "", false, err
		}
		if present {
			leftovers = append(leftovers, dir)
		}
	}
	if len(leftovers) > 0 {
		return "legacy dot-prefixed record paths under .ahm/ remain (" + strings.Join(leftovers, ", ") + "); run 'ahm records migrate'", false, nil
	}
	tracked, err := trackedLegacyRecordPaths(ctx, root)
	if err != nil {
		return "", false, err
	}
	if len(tracked) > 0 {
		return "project git index still tracks legacy record paths; run '" + legacyRecordsGitCleanupCommand(tracked) + "' and commit the result", false, nil
	}
	return "complete", true, nil
}

func dirContainsFiles(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		found = true
		return fs.SkipAll
	})
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return found, nil
}

func (r recordsMigrateReport) RenderText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "action: %s\n", r.Action); err != nil {
		return err
	}
	if r.DryRun {
		if _, err := fmt.Fprintln(w, "dry_run: true"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "moves: %d\n", len(r.Moves)); err != nil {
		return err
	}
	for _, move := range r.Moves {
		if _, err := fmt.Fprintf(w, "  %s -> %s\n", move.From, move.To); err != nil {
			return err
		}
	}
	lines := []string{
		"gitignore: " + r.Gitignore,
		"config: " + r.Config,
		"legacy_config: " + r.LegacyConfig,
	}
	lines = append(lines, "git_cleanup: "+emptyAsNone(r.GitCleanup), "message: "+r.Message)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func emptyAsNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
