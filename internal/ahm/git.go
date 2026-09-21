package ahm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var gitRepositoryEnvironment = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_COMMON_DIR",
}

func cleanGitEnvironment() []string {
	env := os.Environ()
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if isGitRepositoryEnvironment(name) {
			continue
		}
		clean = append(clean, entry)
	}
	return clean
}

func isGitRepositoryEnvironment(name string) bool {
	for _, blocked := range gitRepositoryEnvironment {
		if strings.EqualFold(name, blocked) {
			return true
		}
	}
	return false
}

// readGitRemote returns the remote URL that identifies the project and reports
// whether one applies: the origin remote when the repository names one, and the
// only remote when there is exactly one. Several remotes without an origin are
// no candidate at all, and a candidate that names no URL leaves identity to the
// path rule rather than guessing.
//
// A project root without its own .git is not a Git repository for identity
// purposes, so a managed root nested inside another repository never inherits
// the outer repository's remote.
//
// A root that holds .git but that Git cannot read — git is missing, or the
// repository is broken or unreadable — is an error rather than a fallback,
// because silently deriving a path key would send the project to a different
// store directory and split its records.
func readGitRemote(root string) (string, bool, error) {
	if !hasGitMetadata(root) {
		return "", false, nil
	}
	names, err := gitRemoteNames(root)
	if err != nil {
		return "", false, fmt.Errorf("reading Git remotes in %s: %w", root, err)
	}
	name := remoteCandidate(names)
	if name == "" {
		return "", false, nil
	}
	remote, ok, err := gitRemoteURL(root, name)
	if err != nil {
		return "", false, fmt.Errorf("reading Git remote %s in %s: %w", name, root, err)
	}
	if !ok {
		return "", false, nil
	}
	return remote, true, nil
}

// remoteCandidate returns the remote that speaks for the project: origin when
// the repository names one, and the only remote otherwise. It returns "" when
// no single remote applies, which sends identity to the path rule.
func remoteCandidate(names []string) string {
	for _, name := range names {
		if name == "origin" {
			return "origin"
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	return ""
}

// hasGitMetadata reports whether root holds Git metadata of its own: a .git
// directory, or the .git file a linked worktree or submodule checkout uses.
func hasGitMetadata(root string) bool {
	_, err := os.Lstat(filepath.Join(root, ".git"))
	return err == nil
}

// gitRemoteURL reads one remote's URL. A remote that names no URL reports
// ok == false; a failed Git process is an error, because the caller asked for a
// remote Git itself listed. The name is passed after -- so a hand-edited remote
// name cannot be read as a Git option.
func gitRemoteURL(root string, name string) (string, bool, error) {
	out, err := runGit(root, "remote", "get-url", "--", name)
	if err != nil {
		return "", false, err
	}
	remote := strings.TrimSpace(out)
	if remote == "" {
		return "", false, nil
	}
	return remote, true, nil
}

// gitRemoteNames lists the repository's remotes. A failure is returned to the
// caller, which must not treat an unreadable repository as one without
// remotes.
func gitRemoteNames(root string) ([]string, error) {
	out, err := runGit(root, "remote")
	if err != nil {
		return nil, err
	}
	var names []string
	for line := range strings.Lines(out) {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// runGit runs one read-only Git command scoped to root with the inherited Git
// repository-location variables removed. Every ahm-owned Git subprocess goes
// through this helper, per ADR 018, and the error it returns carries Git's own
// diagnosis so a caller can report why a repository could not be read.
func runGit(root string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is not available: %w", err)
	}
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 // read-only git command scoped to the detected repository root
	cmd.Env = cleanGitEnvironment()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			// Git's diagnosis can span lines; collapse it so an error stays one
			// line of output.
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.Join(strings.Fields(message), " "))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}
