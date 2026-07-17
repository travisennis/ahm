package ahm

import (
	"errors"
	"fmt"
	"os"
)

// taskWorkRoles holds resolved agents and models for each task work phase.
type taskWorkRoles struct {
	implAgent   taskWorkAgent
	implModel   string
	reviewAgent taskWorkAgent
	reviewModel string
}

const (
	codexBypassApprovalsAndSandboxFlag = "--dangerously-bypass-approvals-and-sandbox"
	claudeSkipPermissionsFlag          = "--dangerously-skip-permissions"
)

type taskWorkAgent struct {
	name                string
	executable          string
	args                func(string, string) []string // prompt, model
	resumeArgs          func(string, string) []string // sessionID, prompt
	parseSessionID      func([]byte) (string, error)
	reviewArgs          func(string, string) []string // prompt, model
	parseReviewFeedback func([]byte) (string, error)
	blockedEnvVars      []string // environment variables stripped before spawning the agent
}

// envFilter returns a copy of env with this agent's blockedEnvVars removed.
// When there are no blocked variables it returns nil so callers inherit the
// parent environment unchanged.
func (a taskWorkAgent) envFilter(env []string) []string {
	if len(a.blockedEnvVars) == 0 {
		return nil
	}
	return filterBlockedEnv(env, a.blockedEnvVars)
}

// resolveTaskWorkRoles resolves agents and models for the implementation and
// review phases. Precedence:
//  1. flagAgent / flagModel (from --agent / --model CLI flags)
//  2. Role-specific config under taskWork (implementation / review)
//  3. Legacy default_work_agent
//  4. Built-in default "cake" for agent, "" (no override) for model
//
// Review falls back to the resolved implementation agent when no review-specific
// agent is configured and no --agent flag is provided. Feedback resume and
// commit handoff always use the implementation agent because they resume the
// implementation session.
func (a *app) resolveTaskWorkRoles(flagAgent, flagModel string) (taskWorkRoles, error) {
	meta, err := readMetadata(a.opts.root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.addWarning("%s, using default agent", metadataCorruptMessage(err))
	}

	// Resolve implementation agent.
	implName := flagAgent
	if implName == "" && meta.TaskWork != nil && meta.TaskWork.Implementation != nil && meta.TaskWork.Implementation.Agent != "" {
		implName = meta.TaskWork.Implementation.Agent
	}
	if implName == "" {
		implName = meta.DefaultWorkAgent
	}
	if implName == "" {
		implName = "cake"
	}

	implAgent, err := parseTaskWorkAgent(implName)
	if err != nil {
		return taskWorkRoles{}, err
	}

	// Resolve implementation model.
	implModel := flagModel
	if implModel == "" && meta.TaskWork != nil && meta.TaskWork.Implementation != nil {
		implModel = meta.TaskWork.Implementation.Model
	}

	// Resolve review agent: flag wins, then review role config, then fall back
	// to the resolved implementation agent (which already incorporates legacy
	// default_work_agent and built-in default).
	reviewName := flagAgent
	if reviewName == "" && meta.TaskWork != nil && meta.TaskWork.Review != nil && meta.TaskWork.Review.Agent != "" {
		reviewName = meta.TaskWork.Review.Agent
	}
	if reviewName == "" {
		reviewName = implName
	}

	reviewAgent, err := parseTaskWorkAgent(reviewName)
	if err != nil {
		return taskWorkRoles{}, err
	}

	// Resolve review model.
	reviewModel := flagModel
	if reviewModel == "" && meta.TaskWork != nil && meta.TaskWork.Review != nil {
		reviewModel = meta.TaskWork.Review.Model
	}

	return taskWorkRoles{
		implAgent:   implAgent,
		implModel:   implModel,
		reviewAgent: reviewAgent,
		reviewModel: reviewModel,
	}, nil
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
				base := []string{"--output-format", "stream-json"}
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
				base := []string{"-p", "--verbose", "--output-format", "stream-json", claudeSkipPermissionsFlag}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--verbose", "--output-format", "stream-json", claudeSkipPermissionsFlag}, prompt)
					return base
				}
				return append(base, prompt)
			},
			resumeArgs:     claudeResumeArgs,
			parseSessionID: parseClaudeSessionID,
			reviewArgs: func(prompt, model string) []string {
				base := []string{"-p", "--verbose", "--output-format", "stream-json", claudeSkipPermissionsFlag}
				if model != "" {
					base = append([]string{"-p", "--model", model, "--verbose", "--output-format", "stream-json", claudeSkipPermissionsFlag}, prompt)
					return base
				}
				return append(base, prompt)
			},
			parseReviewFeedback: parseClaudeReviewFeedback,
			blockedEnvVars:      []string{"ANTHROPIC_API_KEY"},
		}, nil
	default:
		return taskWorkAgent{}, usageError(fmt.Sprintf("unsupported task work agent %q (supported: cake, claude, codex, cursor)", value))
	}
}
