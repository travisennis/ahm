package ahm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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
