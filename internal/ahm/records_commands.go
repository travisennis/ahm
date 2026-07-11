package ahm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type recordsStatusReport struct {
	Mode            string                     `json:"mode"`
	Ref             string                     `json:"ref"`
	Remote          string                     `json:"remote"`
	RemoteURL       string                     `json:"remote_url,omitempty"`
	RemoteSupported bool                       `json:"remote_supported"`
	LocalCommit     string                     `json:"local_commit,omitempty"`
	RemoteCommit    string                     `json:"remote_commit,omitempty"`
	TrackingRef     string                     `json:"tracking_ref,omitempty"`
	Relation        string                     `json:"relation"`
	Working         recordsWorkingStatusReport `json:"working"`
	Error           string                     `json:"error,omitempty"`
}

type recordsWorkingStatusReport struct {
	Clean    bool     `json:"clean"`
	Added    []string `json:"added,omitempty"`
	Modified []string `json:"modified,omitempty"`
	Deleted  []string `json:"deleted,omitempty"`
}

type recordsOperationReport struct {
	Action       string   `json:"action"`
	DryRun       bool     `json:"dry_run,omitempty"`
	Ref          string   `json:"ref"`
	Remote       string   `json:"remote,omitempty"`
	LocalCommit  string   `json:"local_commit,omitempty"`
	RemoteCommit string   `json:"remote_commit,omitempty"`
	TrackingRef  string   `json:"tracking_ref,omitempty"`
	Files        []string `json:"files,omitempty"`
	Message      string   `json:"message"`
}

type recordsDoctorReport struct {
	OK     bool              `json:"ok"`
	Checks map[string]string `json:"checks"`
}

func (a *app) recordsCommand() *cobra.Command {
	records := &cobra.Command{
		Use:   "records",
		Short: "Sync ref-backed workflow records",
		Long: `Sync ref-backed ahm workflow records through refs/ahm/records.

Examples:
  ahm records migrate
  ahm records status
  ahm records pull
  ahm records push
  ahm records sync
  ahm records doctor`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath()))
			}
			return usageError("records requires a subcommand\n  ahm records status")
		},
	}
	records.AddCommand(a.simpleCommand("migrate", "Opt into ref-backed record storage", `Migrate ahm-managed workflow state from .agents/ into tool-owned .ahm/.

Migration moves task, research, and ExecPlan files (including generated
indexes) to the same relative paths under .ahm/, installs internal
.ahm/.gitignore entries, writes committed .ahm/config.json metadata with
store_mode "ref", removes legacy .agents/ahm.json, and seeds the local
refs/ahm/records ref. It prints the git rm -r --cached command needed to
untrack legacy record paths instead of running it, and it never touches
project-owned .agents/ content such as .agents/prompt.md.

The command is safe to re-run: an interrupted migration resumes, and a fully
migrated repository reports its remaining git-index cleanup, if any.

Examples:
  ahm --dry-run records migrate
  ahm records migrate`, func() error {
		return a.recordsMigrate()
	}))
	records.AddCommand(a.simpleCommand("status", "Show records sync state", `Show local and remote ref-backed workflow record state.

Examples:
  ahm records status
  ahm --json records status`, func() error {
		return a.recordsStatus()
	}))
	records.AddCommand(a.simpleCommand("pull", "Pull remote records", `Fetch refs/ahm/records and materialize records into .ahm.

Examples:
  ahm records pull
  ahm --dry-run records pull`, func() error {
		return a.recordsPull()
	}))
	records.AddCommand(a.simpleCommand("push", "Push local records", `Snapshot .ahm records and push refs/ahm/records.

Examples:
  ahm records push
  ahm --dry-run records push`, func() error {
		return a.recordsPush()
	}))
	records.AddCommand(a.simpleCommand("sync", "Synchronize records", `Synchronize .ahm records with refs/ahm/records.

Examples:
  ahm records sync
  ahm --dry-run records sync`, func() error {
		return a.recordsSync()
	}))
	records.AddCommand(a.simpleCommand("doctor", "Diagnose records sync setup", `Diagnose ref-backed records configuration, remote support, and ref accessibility.

Examples:
  ahm records doctor
  ahm --json records doctor`, func() error {
		return a.recordsDoctor()
	}))
	return records
}

func (a *app) recordsStatus() error {
	report, err := a.buildRecordsStatus(context.Background(), true)
	if err != nil {
		return err
	}
	return a.emit(report)
}

func (a *app) recordsDoctor() error {
	ctx := context.Background()
	report := recordsDoctorReport{OK: true, Checks: map[string]string{}}
	meta, err := readMetadata(a.opts.root)
	if err != nil {
		report.OK = false
		report.Checks["metadata"] = metadataErrorPath(err) + ": " + err.Error()
		return a.emit(report)
	}
	cfg := meta.recordsStorage()
	report.Checks["mode"] = string(cfg.Mode)

	// First run the migration diagnostic, which works for both legacy and
	// committed .ahm layouts. Ref-mode diagnostics follow for repos that
	// still have the unreleased ref layout (will be removed in 172f).
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

	if cfg.Mode == recordStoreModeRef {
		if err := validateRecordsRef(ctx, a.opts.root, cfg.Ref); err != nil {
			report.OK = false
			report.Checks["ref"] = err.Error()
		} else {
			report.Checks["ref"] = cfg.Ref
		}
		remoteOK := false
		remoteURL, err := recordsRemoteURL(ctx, a.opts.root, cfg.Remote)
		switch {
		case err != nil:
			report.OK = false
			report.Checks["remote"] = classifyRecordsRemoteError(cfg, err)
		case !isSupportedRecordsRemoteURL(remoteURL):
			report.OK = false
			report.Checks["remote"] = unsupportedRecordsRemoteMessage(cfg.Remote, remoteURL)
		default:
			remoteOK = true
			report.Checks["remote"] = cfg.Remote + " " + remoteURL
		}
		if _, err := resolveGitRef(ctx, a.opts.root, cfg.Ref); err != nil {
			if errors.Is(err, errGitRefMissing) {
				report.Checks["local_ref"] = "missing"
			} else {
				report.OK = false
				report.Checks["local_ref"] = err.Error()
			}
		} else {
			report.Checks["local_ref"] = "present"
		}
		if !remoteOK {
			report.Checks["remote_ref"] = "skipped"
		} else if _, err := lsRemoteRecordsRef(ctx, a.opts.root, cfg); err != nil {
			if errors.Is(err, errGitRefMissing) {
				report.Checks["remote_ref"] = "missing"
			} else {
				report.OK = false
				report.Checks["remote_ref"] = classifyRecordsRemoteError(cfg, err)
			}
		} else {
			report.Checks["remote_ref"] = "present"
		}
	} else {
		// Non-ref mode: report storage state.
		if migrationOK {
			report.Checks["storage"] = "committed .ahm records"
		} else {
			report.Checks["storage"] = "legacy .agents records; run 'ahm records migrate' to migrate"
		}
	}
	return a.emit(report)
}

func (a *app) recordsPull() error {
	ctx := context.Background()
	meta, cfg, err := a.requireRefRecordsConfig()
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		return a.emit(recordsOperationReport{
			Action:  "pull",
			DryRun:  true,
			Ref:     cfg.Ref,
			Remote:  cfg.Remote,
			Message: "would fetch remote records, update the local records ref, and materialize .ahm records",
		})
	}
	if err := requireSupportedRecordsRemote(ctx, a.opts.root, cfg); err != nil {
		return err
	}
	remoteCommit, err := lsRemoteRecordsRef(ctx, a.opts.root, cfg)
	if err != nil {
		if errors.Is(err, errGitRefMissing) {
			return fmt.Errorf("remote records ref %s is missing on %s; run 'ahm records push' from a repository with local records first", cfg.Ref, cfg.Remote)
		}
		return fmt.Errorf("pull records: %s", classifyRecordsRemoteError(cfg, err))
	}
	working, err := recordsWorkingStatus(ctx, a.opts.root, cfg.Ref)
	if err != nil {
		return err
	}
	if !working.Clean {
		return fmt.Errorf("local .ahm records have unsnapshotted changes; run 'ahm records push' or 'ahm records sync' before pulling")
	}
	trackingRef, err := fetchRecordsRef(ctx, a.opts.root, cfg)
	if err != nil {
		return fmt.Errorf("pull records: %s", classifyRecordsRemoteError(cfg, err))
	}
	if err := updateRecordsRef(ctx, a.opts.root, cfg.Ref, remoteCommit); err != nil {
		return err
	}
	written, err := materializeRecordsRef(ctx, a.opts.root, cfg.Ref)
	if err != nil {
		return err
	}
	if err := a.markRecordsSynced(meta); err != nil {
		return err
	}
	return a.emit(recordsOperationReport{
		Action:       "pull",
		Ref:          cfg.Ref,
		Remote:       cfg.Remote,
		LocalCommit:  remoteCommit,
		RemoteCommit: remoteCommit,
		TrackingRef:  trackingRef,
		Files:        written,
		Message:      "pulled remote records",
	})
}

func (a *app) recordsPush() error {
	ctx := context.Background()
	meta, cfg, err := a.requireRefRecordsConfig()
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		return a.emit(recordsOperationReport{
			Action:  "push",
			DryRun:  true,
			Ref:     cfg.Ref,
			Remote:  cfg.Remote,
			Message: "would snapshot .ahm records and push the records ref",
		})
	}
	if err := requireSupportedRecordsRemote(ctx, a.opts.root, cfg); err != nil {
		return err
	}
	snapshot, err := snapshotRecordsRef(ctx, a.opts.root, cfg, "Snapshot ahm workflow records")
	if err != nil {
		return err
	}
	if err := pushRecordsRef(ctx, a.opts.root, cfg); err != nil {
		return fmt.Errorf("push records: %s", classifyRecordsPushError(cfg, err))
	}
	if err := a.markRecordsSynced(meta); err != nil {
		return err
	}
	return a.emit(recordsOperationReport{
		Action:      "push",
		Ref:         cfg.Ref,
		Remote:      cfg.Remote,
		LocalCommit: snapshot.Commit,
		Files:       recordFileRelPaths(snapshot.Files),
		Message:     "pushed local records",
	})
}

func (a *app) recordsSync() error {
	ctx := context.Background()
	meta, cfg, err := a.requireRefRecordsConfig()
	if err != nil {
		return err
	}
	if a.opts.dryRun {
		return a.emit(recordsOperationReport{
			Action:  "sync",
			DryRun:  true,
			Ref:     cfg.Ref,
			Remote:  cfg.Remote,
			Message: "would compare local and remote records, then pull or push as needed",
		})
	}
	if err := requireSupportedRecordsRemote(ctx, a.opts.root, cfg); err != nil {
		return err
	}
	remoteCommit, remoteErr := lsRemoteRecordsRef(ctx, a.opts.root, cfg)
	if remoteErr != nil && !errors.Is(remoteErr, errGitRefMissing) {
		return fmt.Errorf("sync records: %s", classifyRecordsRemoteError(cfg, remoteErr))
	}
	localCommit, localErr := resolveGitRef(ctx, a.opts.root, cfg.Ref)
	if localErr != nil && !errors.Is(localErr, errGitRefMissing) {
		return localErr
	}
	working, err := recordsWorkingStatus(ctx, a.opts.root, cfg.Ref)
	if err != nil {
		return err
	}
	if errors.Is(remoteErr, errGitRefMissing) {
		snapshot, err := snapshotRecordsRef(ctx, a.opts.root, cfg, "Snapshot ahm workflow records")
		if err != nil {
			return err
		}
		if err := pushRecordsRef(ctx, a.opts.root, cfg); err != nil {
			return fmt.Errorf("sync records: %s", classifyRecordsPushError(cfg, err))
		}
		if err := a.markRecordsSynced(meta); err != nil {
			return err
		}
		return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: snapshot.Commit, Files: recordFileRelPaths(snapshot.Files), Message: "remote records ref was missing; pushed local records"})
	}
	if errors.Is(localErr, errGitRefMissing) {
		if !working.Clean {
			return fmt.Errorf("local records ref is missing but .ahm records exist; run 'ahm records push' to publish them or remove them before pulling")
		}
		trackingRef, err := fetchRecordsRef(ctx, a.opts.root, cfg)
		if err != nil {
			return fmt.Errorf("sync records: %s", classifyRecordsRemoteError(cfg, err))
		}
		if err := updateRecordsRef(ctx, a.opts.root, cfg.Ref, remoteCommit); err != nil {
			return err
		}
		written, err := materializeRecordsRef(ctx, a.opts.root, cfg.Ref)
		if err != nil {
			return err
		}
		if err := a.markRecordsSynced(meta); err != nil {
			return err
		}
		return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: remoteCommit, RemoteCommit: remoteCommit, TrackingRef: trackingRef, Files: written, Message: "local records ref was missing; pulled remote records"})
	}
	trackingRef, err := fetchRecordsRef(ctx, a.opts.root, cfg)
	if err != nil {
		return fmt.Errorf("sync records: %s", classifyRecordsRemoteError(cfg, err))
	}
	cmp, err := compareRecordsRefs(ctx, a.opts.root, cfg.Ref, trackingRef)
	if err != nil {
		return err
	}
	switch cmp {
	case recordsRefEqual:
		if !working.Clean {
			snapshot, err := snapshotRecordsRef(ctx, a.opts.root, cfg, "Snapshot ahm workflow records")
			if err != nil {
				return err
			}
			if err := pushRecordsRef(ctx, a.opts.root, cfg); err != nil {
				return fmt.Errorf("sync records: %s", classifyRecordsPushError(cfg, err))
			}
			if err := a.markRecordsSynced(meta); err != nil {
				return err
			}
			return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: snapshot.Commit, RemoteCommit: snapshot.Commit, TrackingRef: trackingRef, Files: recordFileRelPaths(snapshot.Files), Message: "snapshotted and pushed local record changes"})
		}
		if err := a.markRecordsSynced(meta); err != nil {
			return err
		}
		return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: localCommit, RemoteCommit: remoteCommit, TrackingRef: trackingRef, Message: "records already synchronized"})
	case recordsRefAhead:
		if !working.Clean {
			snapshot, err := snapshotRecordsRef(ctx, a.opts.root, cfg, "Snapshot ahm workflow records")
			if err != nil {
				return err
			}
			localCommit = snapshot.Commit
		}
		if err := pushRecordsRef(ctx, a.opts.root, cfg); err != nil {
			return fmt.Errorf("sync records: %s", classifyRecordsPushError(cfg, err))
		}
		if err := a.markRecordsSynced(meta); err != nil {
			return err
		}
		return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: localCommit, RemoteCommit: localCommit, TrackingRef: trackingRef, Message: "pushed local records"})
	case recordsRefBehind:
		if !working.Clean {
			return fmt.Errorf("remote records are ahead but local .ahm records have unsnapshotted changes; resolve local changes before syncing")
		}
		if err := updateRecordsRef(ctx, a.opts.root, cfg.Ref, remoteCommit); err != nil {
			return err
		}
		written, err := materializeRecordsRef(ctx, a.opts.root, cfg.Ref)
		if err != nil {
			return err
		}
		if err := a.markRecordsSynced(meta); err != nil {
			return err
		}
		return a.emit(recordsOperationReport{Action: "sync", Ref: cfg.Ref, Remote: cfg.Remote, LocalCommit: remoteCommit, RemoteCommit: remoteCommit, TrackingRef: trackingRef, Files: written, Message: "pulled remote records"})
	default:
		return fmt.Errorf("local and remote records have diverged; inspect with 'ahm records status' and resolve before syncing")
	}
}

func (a *app) buildRecordsStatus(ctx context.Context, checkRemote bool) (recordsStatusReport, error) {
	meta, err := readMetadata(a.opts.root)
	if err != nil {
		return recordsStatusReport{}, fmt.Errorf("workflow metadata %s: %w", metadataErrorPath(err), err)
	}
	cfg := meta.recordsStorage()
	report := recordsStatusReport{
		Mode:     string(cfg.Mode),
		Ref:      cfg.Ref,
		Remote:   cfg.Remote,
		Relation: "unknown",
	}
	trackingRef, trackingErr := recordsRemoteTrackingRef(ctx, a.opts.root, cfg)
	if trackingErr == nil {
		report.TrackingRef = trackingRef
	}
	localCommit, localErr := resolveGitRef(ctx, a.opts.root, cfg.Ref)
	if localErr == nil {
		report.LocalCommit = localCommit
	} else if !errors.Is(localErr, errGitRefMissing) {
		report.Error = localErr.Error()
	}
	working, err := recordsWorkingStatus(ctx, a.opts.root, cfg.Ref)
	if err != nil {
		return recordsStatusReport{}, err
	}
	report.Working = working
	remoteURL, err := recordsRemoteURL(ctx, a.opts.root, cfg.Remote)
	if err != nil {
		report.Error = classifyRecordsRemoteError(cfg, err)
	} else {
		report.RemoteURL = remoteURL
		report.RemoteSupported = isSupportedRecordsRemoteURL(remoteURL)
		if !report.RemoteSupported {
			report.Error = unsupportedRecordsRemoteMessage(cfg.Remote, remoteURL)
		}
	}
	switch {
	case checkRemote && report.RemoteSupported:
		remoteCommit, err := lsRemoteRecordsRef(ctx, a.opts.root, cfg)
		if err != nil {
			if errors.Is(err, errGitRefMissing) {
				report.Relation = "local_only"
			} else {
				report.Error = classifyRecordsRemoteError(cfg, err)
			}
		} else {
			report.RemoteCommit = remoteCommit
			report.Relation = recordsRefRelation(ctx, a.opts.root, cfg.Ref, localCommit, localErr, remoteCommit, trackingRef)
		}
	case localErr == nil:
		report.Relation = "local_only"
	default:
		report.Relation = "missing"
	}
	return report, nil
}

func recordsRefRelation(ctx context.Context, root string, localRef string, localCommit string, localErr error, remoteCommit string, trackingRef string) string {
	if errors.Is(localErr, errGitRefMissing) && remoteCommit == "" {
		return "missing"
	}
	if errors.Is(localErr, errGitRefMissing) {
		return "remote_only"
	}
	if remoteCommit == "" {
		return "local_only"
	}
	if localCommit == remoteCommit {
		return "equal"
	}
	if trackingRef != "" {
		trackingCommit, err := resolveGitRef(ctx, root, trackingRef)
		if err == nil && trackingCommit == remoteCommit {
			cmp, err := compareRecordsRefs(ctx, root, localRef, trackingRef)
			if err == nil {
				return string(cmp)
			}
		}
	}
	return "different"
}

func (a *app) requireRefRecordsConfig() (metadata, recordsStorageConfig, error) {
	meta, err := readMetadata(a.opts.root)
	if err != nil {
		return metadata{}, recordsStorageConfig{}, fmt.Errorf("workflow metadata %s: %w", metadataErrorPath(err), err)
	}
	cfg := meta.recordsStorage()
	if cfg.Mode != recordStoreModeRef {
		return metadata{}, recordsStorageConfig{}, fmt.Errorf("records storage mode is %q; run the opt-in records migration before using pull, push, or sync", cfg.Mode)
	}
	return meta, cfg, nil
}

func (a *app) markRecordsSynced(meta metadata) error {
	meta.RecordsLastSync = time.Now().UTC().Format(time.RFC3339)
	return writeMetadata(a.opts.root, meta)
}

func recordsWorkingStatus(ctx context.Context, root string, ref string) (recordsWorkingStatusReport, error) {
	local, err := collectRecordFiles(root)
	if err != nil {
		return recordsWorkingStatusReport{}, err
	}
	refHashes, err := recordsRefFileHashes(ctx, root, ref)
	if err != nil && !errors.Is(err, errGitRefMissing) {
		return recordsWorkingStatusReport{}, err
	}
	if errors.Is(err, errGitRefMissing) {
		refHashes = map[string]string{}
	}
	status := recordsWorkingStatusReport{}
	seen := map[string]bool{}
	for _, file := range local {
		seen[file.RelPath] = true
		hash, err := hashRecordFile(ctx, root, file.AbsPath)
		if err != nil {
			return recordsWorkingStatusReport{}, err
		}
		refHash, ok := refHashes[file.RelPath]
		switch {
		case !ok:
			status.Added = append(status.Added, file.RelPath)
		case refHash != hash:
			status.Modified = append(status.Modified, file.RelPath)
		}
	}
	for rel := range refHashes {
		if !seen[rel] {
			status.Deleted = append(status.Deleted, rel)
		}
	}
	sort.Strings(status.Added)
	sort.Strings(status.Modified)
	sort.Strings(status.Deleted)
	status.Clean = len(status.Added) == 0 && len(status.Modified) == 0 && len(status.Deleted) == 0
	return status, nil
}

func recordsRefFileHashes(ctx context.Context, root string, ref string) (map[string]string, error) {
	if ref == "" {
		ref = defaultRecordsRef
	}
	if err := validateRecordsRef(ctx, root, ref); err != nil {
		return nil, err
	}
	out, err := runGit(ctx, root, []string{"-r", "-z", ref}, nil, nil, "ls-tree")
	if err != nil {
		if isGitExitError(err) {
			return nil, errGitRefMissing
		}
		return nil, err
	}
	hashes := map[string]string{}
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		meta, rel, ok := strings.Cut(entry, "\t")
		if !ok || !isRecordRelPath(rel) {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) == 3 && fields[1] == "blob" {
			hashes[rel] = fields[2]
		}
	}
	return hashes, nil
}

func hashRecordFile(ctx context.Context, root string, path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 // record files are selected from fixed .ahm roots.
	if err != nil {
		return "", err
	}
	out, err := runGit(ctx, root, []string{"--stdin"}, bytes.NewReader(data), nil, "hash-object")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func recordsRemoteURL(ctx context.Context, root string, remote string) (string, error) {
	if remote == "" {
		remote = defaultRecordsRemote
	}
	out, err := runGit(ctx, root, []string{"get-url", remote}, nil, nil, "remote")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func requireSupportedRecordsRemote(ctx context.Context, root string, cfg recordsStorageConfig) error {
	remoteURL, err := recordsRemoteURL(ctx, root, cfg.Remote)
	if err != nil {
		return fmt.Errorf("%s", classifyRecordsRemoteError(cfg, err))
	}
	if !isSupportedRecordsRemoteURL(remoteURL) {
		return fmt.Errorf("%s", unsupportedRecordsRemoteMessage(cfg.Remote, remoteURL))
	}
	return nil
}

func isSupportedRecordsRemoteURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "git@github.com:") || strings.HasPrefix(raw, "ssh://git@github.com/") || strings.HasPrefix(raw, "https://github.com/") {
		return true
	}
	if strings.HasPrefix(raw, "file://") {
		return true
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return true
	}
	if u, err := url.Parse(raw); err == nil && u.Host == "github.com" {
		return true
	}
	return false
}

func unsupportedRecordsRemoteMessage(remote string, remoteURL string) string {
	return fmt.Sprintf("records remote %s uses unsupported URL %q; initial records sync supports GitHub remotes", remote, remoteURL)
}

func lsRemoteRecordsRef(ctx context.Context, root string, cfg recordsStorageConfig) (string, error) {
	if cfg.Remote == "" {
		cfg.Remote = defaultRecordsRemote
	}
	if cfg.Ref == "" {
		cfg.Ref = defaultRecordsRef
	}
	out, err := runGit(ctx, root, []string{cfg.Remote, cfg.Ref}, nil, nil, "ls-remote")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == cfg.Ref {
			return fields[0], nil
		}
	}
	return "", errGitRefMissing
}

func updateRecordsRef(ctx context.Context, root string, ref string, commit string) error {
	if err := validateRecordsRef(ctx, root, ref); err != nil {
		return err
	}
	if strings.TrimSpace(commit) == "" {
		return fmt.Errorf("records commit is empty")
	}
	_, err := runGit(ctx, root, []string{ref, commit}, nil, nil, "update-ref")
	return err
}

func classifyRecordsRemoteError(cfg recordsStorageConfig, err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "no such remote") || strings.Contains(lower, "does not appear to be a git repository"):
		return fmt.Sprintf("records remote %s is not configured; set records_remote or add a GitHub remote", cfg.Remote)
	case strings.Contains(lower, "authentication failed") || strings.Contains(lower, "could not read username") || strings.Contains(lower, "permission denied"):
		return fmt.Sprintf("records remote %s authentication failed; check GitHub credentials and repository access", cfg.Remote)
	default:
		return msg
	}
}

func classifyRecordsPushError(cfg recordsStorageConfig, err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "non-fast-forward") || strings.Contains(lower, "fetch first") || strings.Contains(lower, "stale info") || strings.Contains(lower, "failed to push some refs") {
		return fmt.Sprintf("remote records ref %s on %s is not a fast-forward; run 'ahm records pull' or resolve divergence before pushing", cfg.Ref, cfg.Remote)
	}
	return classifyRecordsRemoteError(cfg, err)
}

func recordFileRelPaths(files []recordFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.RelPath)
	}
	sort.Strings(paths)
	return paths
}

func (r recordsStatusReport) RenderText(w io.Writer) error {
	lines := []string{
		"mode: " + r.Mode,
		"ref: " + r.Ref,
		"remote: " + r.Remote,
	}
	if r.RemoteURL != "" {
		lines = append(lines, "remote_url: "+r.RemoteURL)
	}
	lines = append(lines,
		fmt.Sprintf("remote_supported: %v", r.RemoteSupported),
		"local_commit: "+emptyAsMissing(r.LocalCommit),
		"remote_commit: "+emptyAsMissing(r.RemoteCommit),
		"relation: "+r.Relation,
		fmt.Sprintf("working_clean: %v", r.Working.Clean),
		fmt.Sprintf("working_added: %d", len(r.Working.Added)),
		fmt.Sprintf("working_modified: %d", len(r.Working.Modified)),
		fmt.Sprintf("working_deleted: %d", len(r.Working.Deleted)),
	)
	if r.Error != "" {
		lines = append(lines, "diagnostic: "+r.Error)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func (r recordsOperationReport) RenderText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "action: %s\n", r.Action); err != nil {
		return err
	}
	if r.DryRun {
		if _, err := fmt.Fprintln(w, "dry_run: true"); err != nil {
			return err
		}
	}
	for _, line := range []string{
		"ref: " + r.Ref,
		"remote: " + r.Remote,
		"local_commit: " + emptyAsMissing(r.LocalCommit),
		"remote_commit: " + emptyAsMissing(r.RemoteCommit),
		"message: " + r.Message,
	} {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if len(r.Files) > 0 {
		if _, err := fmt.Fprintln(w, "files:"); err != nil {
			return err
		}
		for _, file := range r.Files {
			if _, err := fmt.Fprintln(w, "  "+file); err != nil {
				return err
			}
		}
	}
	return nil
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

func emptyAsMissing(value string) string {
	if value == "" {
		return "missing"
	}
	return value
}
