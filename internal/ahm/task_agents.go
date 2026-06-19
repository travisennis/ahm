package ahm

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const codexBypassApprovalsAndSandboxFlag = "--dangerously-bypass-approvals-and-sandbox"

type taskWorkAgent struct {
	name                string
	executable          string
	args                func(string) []string
	supportsSessions    bool
	resumeArgs          func(string, string) []string // sessionID, prompt
	parseSessionID      func([]byte) (string, error)
	supportsReview      bool
	reviewArgs          func(string) []string // prompt
	parseReviewFeedback func([]byte) (string, error)
}

func (a *app) selectTaskWorkAgent(flagValue string) (taskWorkAgent, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		meta, err := readMetadata(a.opts.root)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// No metadata yet, no default agent configured.
		case err != nil:
			a.addWarning("corrupt workflow metadata .agents/ahm.json, using default agent")
		default:
			value = meta.DefaultWorkAgent
		}
	}
	if value == "" {
		value = "cake"
	}
	return parseTaskWorkAgent(value)
}

func parseTaskWorkAgent(value string) (taskWorkAgent, error) {
	switch enumKey(value) {
	case "cake":
		return taskWorkAgent{
			name:       "cake",
			executable: "cake",
			args: func(prompt string) []string {
				return []string{"--output-format", "stream-json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       cakeResumeArgs,
			parseSessionID:   parseCakeSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"--no-session", "--skills", "preflight", "--output-format", "stream-json", prompt}
			},
			parseReviewFeedback: parseCakeReviewFeedback,
		}, nil
	case "codex":
		return taskWorkAgent{
			name:       "codex",
			executable: "codex",
			args: func(prompt string) []string {
				return []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       codexResumeArgs,
			parseSessionID:   parseCodexSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json", prompt}
			},
			parseReviewFeedback: parseCodexReviewFeedback,
		}, nil
	case "cursor", "cursoragent":
		return taskWorkAgent{
			name:       "cursor",
			executable: "cursor-agent",
			args: func(prompt string) []string {
				return []string{"-p", "--output-format", "stream-json", "--trust", prompt}
			},
			supportsSessions: true,
			resumeArgs:       cursorResumeArgs,
			parseSessionID:   parseCursorSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"-p", "--output-format", "stream-json", "--mode", "ask", "--trust", prompt}
			},
			parseReviewFeedback: parseCursorReviewFeedback,
		}, nil
	case "claude", "claudecode":
		return taskWorkAgent{
			name:       "claude",
			executable: "claude",
			args: func(prompt string) []string {
				return []string{"-p", "--verbose", "--output-format", "stream-json", prompt}
			},
			supportsSessions: true,
			resumeArgs:       claudeResumeArgs,
			parseSessionID:   parseClaudeSessionID,
			supportsReview:   true,
			reviewArgs: func(prompt string) []string {
				return []string{"-p", "--verbose", "--output-format", "stream-json", prompt}
			},
			parseReviewFeedback: parseClaudeReviewFeedback,
		}, nil
	default:
		return taskWorkAgent{}, usageError(fmt.Sprintf("unsupported task work agent %q (supported: cake, claude, codex, cursor)", value))
	}
}
