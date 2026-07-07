package ahm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// taskWorkWithSession runs a session-capable agent, captures its stdout,
// parses the session ID from the response, and tees the full output to the
// user's terminal. The session ID is kept in memory for the current
// orchestration run and is available for later review and commit
// handoff steps.
func (a *app) taskWorkWithSession(agent taskWorkAgent, executable string, args []string, review bool, commit bool, taskID string, timeout time.Duration) error {
	var stdoutBuf bytes.Buffer
	// Write captured output to both the user's terminal and the buffer.
	out := io.MultiWriter(a.out, &stdoutBuf)
	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, args, a.in, out, a.err); err != nil {
		return err
	}
	sessionID, parseErr := agent.parseSessionID(stdoutBuf.Bytes())
	if parseErr != nil {
		if commit {
			return fmt.Errorf("cannot run commit handoff: could not capture session ID from %s output: %w", agent.name, parseErr)
		}
		a.addWarning("could not capture session ID from %s output: %v", agent.name, parseErr)
		return nil
	}
	if sessionID == "" {
		if commit {
			return fmt.Errorf("cannot run commit handoff: no session ID returned by %s", agent.name)
		}
		a.addWarning("no session ID returned by %s", agent.name)
		return nil
	}

	fmt.Fprintf(a.err, "%s session started: %s\n", agent.name, truncatedID(sessionID, 8))

	if review {
		if err := a.runReview(agent, executable, sessionID, timeout); err != nil {
			return err
		}
	}
	if commit {
		return a.runCommit(agent, executable, sessionID, taskID, timeout)
	}
	return nil
}

// taskWorkReviewPrompt is the prompt runReview sends to every review-capable
// agent. It asks the agent to run the repo-owned preflight review skill against
// current uncommitted changes. The fixture capture script
// (scripts/capture-agent-fixtures.sh) replays the same prompt so the committed
// review golden reflects a real review run;
// TestCaptureScriptUsesReviewPrompt keeps the two in sync.
const taskWorkReviewPrompt = "Run the preflight skill on the current uncommitted changes."

// runReview runs an independent review pass using the agent's review
// capability, then feeds actionable feedback back into the original work
// session. If the review produces no feedback, the feedback-resume step is
// skipped. If the review command itself fails, the error is surfaced.
func (a *app) runReview(agent taskWorkAgent, executable, sessionID string, timeout time.Duration) error {
	fmt.Fprintln(a.err, "--- Running review ---")

	reviewArgs := agent.reviewArgs(taskWorkReviewPrompt)

	var reviewBuf bytes.Buffer
	reviewOut := io.MultiWriter(a.out, &reviewBuf)

	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, reviewArgs, nil, reviewOut, a.err); err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	feedback, parseErr := agent.parseReviewFeedback(reviewBuf.Bytes())
	if parseErr != nil {
		a.addWarning("could not parse review feedback from %s output: %v", agent.name, parseErr)
		return nil
	}

	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		fmt.Fprintln(a.err, "No review feedback to address, skipping feedback step.")
		return nil
	}

	fmt.Fprintf(a.err, "Review produced feedback, applying to session %s...\n", truncatedID(sessionID, 8))
	resumePrompt := fmt.Sprintf("Please address the following review feedback:\n\n%s", feedback)
	resumeArgs := agent.resumeArgs(sessionID, resumePrompt)
	return taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, resumeArgs, a.in, a.out, a.err)
}

// runCommit resumes the agent session with a commit handoff prompt. The
// delegated agent owns the actual git operation; ahm only sends the prompt.
func (a *app) runCommit(agent taskWorkAgent, executable, sessionID, taskID string, timeout time.Duration) error {
	fmt.Fprintln(a.err, "--- Running commit handoff ---")
	prompt := a.buildTaskWorkCommitPrompt(taskID)
	resumeArgs := agent.resumeArgs(sessionID, prompt)
	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, resumeArgs, a.in, a.out, a.err); err != nil {
		return fmt.Errorf("commit handoff failed: %w", err)
	}
	return nil
}

// buildTaskWorkCommitPrompt returns the prompt used to ask the delegated agent
// to commit completed task work. Commit message policy remains project-owned.
// Legacy repositories commit task records with the source change; migrated
// repositories keep gitignored .ahm/ records out of project commits because
// ahm snapshots them to the records ref instead.
func (a *app) buildTaskWorkCommitPrompt(taskID string) string {
	if workflowPathsFor(a.opts.root).recordsDir == toolRecordsDirName {
		return fmt.Sprintf(`Commit the completed work for task %s.

Make sure the task is marked completed before committing. Task records are managed by ahm outside the project branch: commit only the project source changes, and do not add or force-add gitignored .ahm/ files.

Do not push or open a pull request.`, taskID)
	}
	return fmt.Sprintf(`Commit the completed work for task %s.

Make sure the task is marked completed before committing. Include both task files and project source files in a single commit.

Do not push or open a pull request.`, taskID)
}

func truncatedID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}
