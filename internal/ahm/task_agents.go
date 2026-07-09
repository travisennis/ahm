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
	args                func(string, string) []string // prompt, model
	resumeArgs          func(string, string) []string // sessionID, prompt
	parseSessionID      func([]byte) (string, error)
	reviewArgs          func(string, string) []string // prompt, model
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
			a.addWarning("%s, using default agent", metadataCorruptMessage(err))
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
			args: func(prompt, model string) []string {
				base := []string{"--output-format", "stream-json"}
				if model != "" {
					base = append(base, "--model", model)
				}
				return append(base, prompt)
			},
			resumeArgs:     cakeResumeArgs,
			parseSessionID: parseCakeSessionID,
			reviewArgs: func(prompt, model string) []string {
				base := []string{"--skills", "preflight", "--output-format", "stream-json"}
				if model != "" {
					base = append(base, "--model", model)
				}
				return append(base, prompt)
			},
			parseReviewFeedback: parseCakeReviewFeedback,
		}, nil
	case "codex":
		return taskWorkAgent{
			name:       "codex",
			executable: "codex",
			args: func(prompt, model string) []string {
				base := []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json"}
				if model != "" {
					base = append([]string{"exec", "--model", model, codexBypassApprovalsAndSandboxFlag, "--json"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			resumeArgs:     codexResumeArgs,
			parseSessionID: parseCodexSessionID,
			reviewArgs: func(prompt, model string) []string {
				base := []string{"exec", codexBypassApprovalsAndSandboxFlag, "--json"}
				if model != "" {
					base = append([]string{"exec", "--model", model, codexBypassApprovalsAndSandboxFlag, "--json"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			parseReviewFeedback: parseCodexReviewFeedback,
		}, nil
	case "cursor", "cursoragent":
		return taskWorkAgent{
			name:       "cursor",
			executable: "cursor-agent",
			args: func(prompt, model string) []string {
				base := []string{"-p", "--output-format", "stream-json", "--trust"}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--output-format", "stream-json", "--trust"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			resumeArgs:     cursorResumeArgs,
			parseSessionID: parseCursorSessionID,
			reviewArgs: func(prompt, model string) []string {
				base := []string{"-p", "--output-format", "stream-json", "--mode", "ask", "--trust"}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--output-format", "stream-json", "--mode", "ask", "--trust"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			parseReviewFeedback: parseCursorReviewFeedback,
		}, nil
	case "claude", "claudecode":
		return taskWorkAgent{
			name:       "claude",
			executable: "claude",
			args: func(prompt, model string) []string {
				base := []string{"-p", "--verbose", "--output-format", "stream-json"}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--verbose", "--output-format", "stream-json"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			resumeArgs:     claudeResumeArgs,
			parseSessionID: parseClaudeSessionID,
			reviewArgs: func(prompt, model string) []string {
				base := []string{"-p", "--verbose", "--output-format", "stream-json"}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--verbose", "--output-format", "stream-json"}, prompt)
					return base
				}
				return append(base, prompt)
			},
			parseReviewFeedback: parseClaudeReviewFeedback,
		}, nil
	default:
		return taskWorkAgent{}, usageError(fmt.Sprintf("unsupported task work agent %q (supported: cake, claude, codex, cursor)", value))
	}
}
