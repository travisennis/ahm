package ahm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type recordFile struct {
	RelPath string
	AbsPath string
}

var recordSourceDirs = []string{
	".ahm/tasks/active",
	".ahm/tasks/completed",
	".ahm/tasks/cancelled",
	".ahm/research/inbox",
	".ahm/research/investigations",
	".ahm/research/sources",
	".ahm/research/topics",
	".ahm/research/archived",
	".ahm/exec-plans/active",
	".ahm/exec-plans/completed",
	// Legacy dot-prefixed paths under .ahm/ (task 165). Repositories that
	// migrated before the non-dot-record-directory convention was introduced
	// may still have records at the old paths; scanning both during the
	// transition window prevents data loss until migration is re-run.
	".ahm/.tasks/active",
	".ahm/.tasks/completed",
	".ahm/.tasks/cancelled",
	".ahm/.research/inbox",
	".ahm/.research/investigations",
	".ahm/.research/sources",
	".ahm/.research/topics",
	".ahm/.research/archived",
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

func runGit(ctx context.Context, root string, args []string, subcommand string) (string, error) {
	out, err := runGitBytes(ctx, root, args, subcommand)
	return string(out), err
}

func runGitBytes(ctx context.Context, root string, args []string, subcommand string) ([]byte, error) {
	gitArgs := append([]string{"-C", root, subcommand}, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...) // #nosec G204 // git subcommands and args are constructed by internal records helpers.
	cmd.Stdin = nil
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
