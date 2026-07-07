package ahm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type recordFile struct {
	RelPath string
	AbsPath string
}

type recordsSnapshot struct {
	Commit string
	Tree   string
	Files  []recordFile
}

type recordsRefComparison string

const (
	recordsRefEqual        recordsRefComparison = "equal"
	recordsRefLeftMissing  recordsRefComparison = "left_missing"
	recordsRefRightMissing recordsRefComparison = "right_missing"
	recordsRefAhead        recordsRefComparison = "ahead"
	recordsRefBehind       recordsRefComparison = "behind"
	recordsRefDiverged     recordsRefComparison = "diverged"
)

var recordSourceDirs = []string{
	".ahm/.tasks/active",
	".ahm/.tasks/completed",
	".ahm/.tasks/cancelled",
	".ahm/.research/inbox",
	".ahm/.research/investigations",
	".ahm/.research/sources",
	".ahm/.research/topics",
	".ahm/.research/archived",
	".ahm/exec-plans/active",
	".ahm/exec-plans/completed",
}

func collectRecordFiles(root string) ([]recordFile, error) {
	var files []recordFile
	for _, dir := range recordSourceDirs {
		absDir := filepath.Join(root, filepath.FromSlash(dir))
		err := filepath.WalkDir(absDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Base(path) == "index.md" || filepath.Ext(path) != ".md" {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("record file symlinks are not supported: %s", relPath(root, path))
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !isRecordRelPath(rel) {
				return fmt.Errorf("record file escaped .ahm record roots: %s", rel)
			}
			files = append(files, recordFile{RelPath: rel, AbsPath: path})
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("collect records from %s: %w", dir, err)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})
	return files, nil
}

func snapshotRecordsRef(ctx context.Context, root string, cfg recordsStorageConfig, message string) (recordsSnapshot, error) {
	if cfg.Ref == "" {
		cfg.Ref = defaultRecordsRef
	}
	if err := validateRecordsRef(ctx, root, cfg.Ref); err != nil {
		return recordsSnapshot{}, err
	}
	files, err := collectRecordFiles(root)
	if err != nil {
		return recordsSnapshot{}, err
	}
	tree, err := buildRecordsTree(ctx, root, files)
	if err != nil {
		return recordsSnapshot{}, err
	}
	if strings.TrimSpace(message) == "" {
		message = "Snapshot ahm workflow records"
	}
	parent, parentErr := resolveGitRef(ctx, root, cfg.Ref)
	if parentErr != nil && !errors.Is(parentErr, errGitRefMissing) {
		return recordsSnapshot{}, parentErr
	}
	// Reuse the existing snapshot commit when the records tree is unchanged so
	// routine post-mutation snapshots stay idempotent.
	if parent != "" {
		parentTree, treeErr := runGit(ctx, root, []string{"--verify", parent + "^{tree}"}, nil, nil, "rev-parse")
		if treeErr == nil && strings.TrimSpace(parentTree) == tree {
			return recordsSnapshot{Commit: parent, Tree: tree, Files: files}, nil
		}
	}
	var args []string
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, tree)
	commit, err := runGit(ctx, root, args, strings.NewReader(message+"\n"), gitIdentityEnv(time.Now().UTC()), "commit-tree")
	if err != nil {
		return recordsSnapshot{}, err
	}
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return recordsSnapshot{}, fmt.Errorf("git commit-tree returned an empty commit")
	}
	updateArgs := []string{cfg.Ref, commit}
	if parent != "" {
		updateArgs = append(updateArgs, parent)
	}
	if _, err := runGit(ctx, root, updateArgs, nil, nil, "update-ref"); err != nil {
		return recordsSnapshot{}, err
	}
	return recordsSnapshot{Commit: commit, Tree: tree, Files: files}, nil
}

func materializeRecordsRef(ctx context.Context, root string, ref string) ([]string, error) {
	if ref == "" {
		ref = defaultRecordsRef
	}
	if err := validateRecordsRef(ctx, root, ref); err != nil {
		return nil, err
	}
	out, err := runGit(ctx, root, []string{"-r", "-z", ref}, nil, nil, "ls-tree")
	if err != nil {
		return nil, err
	}
	refFiles := map[string]bool{}
	var written []string
	for _, entry := range bytes.Split([]byte(out), []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		meta, relBytes, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("parse git ls-tree entry %q", string(entry))
		}
		fields := strings.Fields(string(meta))
		if len(fields) != 3 {
			return nil, fmt.Errorf("parse git ls-tree metadata %q", string(meta))
		}
		if fields[1] != "blob" {
			continue
		}
		rel := filepath.ToSlash(string(relBytes))
		if !isRecordRelPath(rel) {
			continue
		}
		refFiles[rel] = true
		data, err := runGitBytes(ctx, root, []string{"blob", fields[2]}, nil, nil, "cat-file")
		if err != nil {
			return nil, err
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := writeFileAtomic(target, data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	localFiles, err := collectRecordFiles(root)
	if err != nil {
		return nil, err
	}
	for _, file := range localFiles {
		if refFiles[file.RelPath] {
			continue
		}
		if err := os.Remove(file.AbsPath); err != nil {
			return nil, err
		}
		written = append(written, file.RelPath)
	}
	sort.Strings(written)
	return written, nil
}

func fetchRecordsRef(ctx context.Context, root string, cfg recordsStorageConfig) (string, error) {
	if cfg.Remote == "" {
		cfg.Remote = defaultRecordsRemote
	}
	if cfg.Ref == "" {
		cfg.Ref = defaultRecordsRef
	}
	if err := validateRecordsRef(ctx, root, cfg.Ref); err != nil {
		return "", err
	}
	trackingRef, err := recordsRemoteTrackingRef(ctx, root, cfg)
	if err != nil {
		return "", err
	}
	_, err = runGit(ctx, root, []string{cfg.Remote, "+" + cfg.Ref + ":" + trackingRef}, nil, nil, "fetch")
	if err != nil {
		return "", err
	}
	return trackingRef, nil
}

func pushRecordsRef(ctx context.Context, root string, cfg recordsStorageConfig) error {
	if cfg.Remote == "" {
		cfg.Remote = defaultRecordsRemote
	}
	if cfg.Ref == "" {
		cfg.Ref = defaultRecordsRef
	}
	if err := validateRecordsRef(ctx, root, cfg.Ref); err != nil {
		return err
	}
	_, err := runGit(ctx, root, []string{cfg.Remote, cfg.Ref + ":" + cfg.Ref}, nil, nil, "push")
	return err
}

func compareRecordsRefs(ctx context.Context, root string, leftRef string, rightRef string) (recordsRefComparison, error) {
	if err := validateRecordsRef(ctx, root, leftRef); err != nil {
		return "", err
	}
	if err := validateRecordsRef(ctx, root, rightRef); err != nil {
		return "", err
	}
	left, leftErr := resolveGitRef(ctx, root, leftRef)
	right, rightErr := resolveGitRef(ctx, root, rightRef)
	leftMissing := errors.Is(leftErr, errGitRefMissing)
	rightMissing := errors.Is(rightErr, errGitRefMissing)
	if leftErr != nil && !leftMissing {
		return "", leftErr
	}
	if rightErr != nil && !rightMissing {
		return "", rightErr
	}
	switch {
	case leftMissing && rightMissing:
		return recordsRefEqual, nil
	case leftMissing:
		return recordsRefLeftMissing, nil
	case rightMissing:
		return recordsRefRightMissing, nil
	case left == right:
		return recordsRefEqual, nil
	}
	leftAncestor, err := isGitAncestor(ctx, root, left, right)
	if err != nil {
		return "", err
	}
	rightAncestor, err := isGitAncestor(ctx, root, right, left)
	if err != nil {
		return "", err
	}
	switch {
	case rightAncestor:
		return recordsRefAhead, nil
	case leftAncestor:
		return recordsRefBehind, nil
	default:
		return recordsRefDiverged, nil
	}
}

func recordsRemoteTrackingRef(ctx context.Context, root string, cfg recordsStorageConfig) (string, error) {
	if cfg.Remote == "" {
		cfg.Remote = defaultRecordsRemote
	}
	if cfg.Ref == "" {
		cfg.Ref = defaultRecordsRef
	}
	suffix := strings.TrimPrefix(cfg.Ref, "refs/ahm/")
	trackingRef := "refs/ahm/remotes/" + cfg.Remote + "/" + suffix
	if err := validateRecordsRef(ctx, root, trackingRef); err != nil {
		return "", err
	}
	return trackingRef, nil
}

type recordsTreeNode struct {
	Blobs map[string]string
	Dirs  map[string]*recordsTreeNode
}

func buildRecordsTree(ctx context.Context, root string, files []recordFile) (string, error) {
	tree := &recordsTreeNode{Blobs: map[string]string{}, Dirs: map[string]*recordsTreeNode{}}
	for _, file := range files {
		data, err := os.ReadFile(file.AbsPath) // #nosec G304 // record files are selected from fixed .ahm roots.
		if err != nil {
			return "", err
		}
		oid, err := runGit(ctx, root, []string{"-w", "--stdin"}, bytes.NewReader(data), nil, "hash-object")
		if err != nil {
			return "", err
		}
		insertRecordBlob(tree, strings.Split(file.RelPath, "/"), strings.TrimSpace(oid))
	}
	return writeGitTree(ctx, root, tree)
}

func insertRecordBlob(root *recordsTreeNode, parts []string, oid string) {
	node := root
	for _, part := range parts[:len(parts)-1] {
		if node.Dirs[part] == nil {
			node.Dirs[part] = &recordsTreeNode{Blobs: map[string]string{}, Dirs: map[string]*recordsTreeNode{}}
		}
		node = node.Dirs[part]
	}
	node.Blobs[parts[len(parts)-1]] = oid
}

func writeGitTree(ctx context.Context, root string, node *recordsTreeNode) (string, error) {
	var input strings.Builder
	for _, name := range sortedRecordTreeKeys(node.Dirs) {
		oid, err := writeGitTree(ctx, root, node.Dirs[name])
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&input, "040000 tree %s\t%s\n", oid, name)
	}
	for _, name := range sortedRecordTreeKeys(node.Blobs) {
		fmt.Fprintf(&input, "100644 blob %s\t%s\n", node.Blobs[name], name)
	}
	oid, err := runGit(ctx, root, nil, strings.NewReader(input.String()), nil, "mktree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(oid), nil
}

func sortedRecordTreeKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isRecordRelPath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean != path || strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	if filepath.Base(clean) == "index.md" || filepath.Ext(clean) != ".md" {
		return false
	}
	for _, dir := range recordSourceDirs {
		if strings.HasPrefix(clean, dir+"/") {
			return true
		}
	}
	return false
}

var errGitRefMissing = errors.New("git ref missing")

func resolveGitRef(ctx context.Context, root string, ref string) (string, error) {
	out, err := runGit(ctx, root, []string{"--verify", ref + "^{commit}"}, nil, nil, "rev-parse")
	if err != nil {
		if isGitExitError(err) {
			return "", errGitRefMissing
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func isGitAncestor(ctx context.Context, root string, ancestor string, descendant string) (bool, error) {
	_, err := runGit(ctx, root, []string{"--is-ancestor", ancestor, descendant}, nil, nil, "merge-base")
	if err == nil {
		return true, nil
	}
	if isGitExitError(err) {
		return false, nil
	}
	return false, err
}

func validateRecordsRef(ctx context.Context, root string, ref string) error {
	if !strings.HasPrefix(ref, "refs/ahm/") {
		return fmt.Errorf("records ref %q must be under refs/ahm/", ref)
	}
	_, err := runGit(ctx, root, []string{ref}, nil, nil, "check-ref-format")
	return err
}

func runGit(ctx context.Context, root string, args []string, stdin io.Reader, env []string, subcommand string) (string, error) {
	out, err := runGitBytes(ctx, root, args, stdin, env, subcommand)
	return string(out), err
}

func runGitBytes(ctx context.Context, root string, args []string, stdin io.Reader, env []string, subcommand string) ([]byte, error) {
	gitArgs := append([]string{"-C", root, subcommand}, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...) // #nosec G204 // git subcommands and args are constructed by internal records helpers.
	cmd.Stdin = stdin
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %w: %s", subcommand, err, msg)
	}
	return out, nil
}

func gitIdentityEnv(now time.Time) []string {
	date := now.Format(time.RFC3339)
	return []string{
		"GIT_AUTHOR_NAME=ahm",
		"GIT_AUTHOR_EMAIL=ahm@localhost",
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=ahm",
		"GIT_COMMITTER_EMAIL=ahm@localhost",
		"GIT_COMMITTER_DATE=" + date,
	}
}

func isGitExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
