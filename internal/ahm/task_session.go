package ahm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
)

// taskWorkWithSession runs a session-capable agent, captures its stdout,
// parses the session ID from the response, and tees the full output to the
// user's terminal. The session ID is kept in memory for the current
// orchestration run and is available for later review, completion, and commit
// handoff steps.
func (a *app) taskWorkWithSession(agent taskWorkAgent, executable string, args []string, review bool, complete bool, commit bool, taskID string) error {
	var stdoutBuf bytes.Buffer
	// Write captured output to both the user's terminal and the buffer.
	out := io.MultiWriter(a.out, &stdoutBuf)
	if err := taskWorkRunCommand(context.Background(), a.opts.root, executable, args, a.in, out, a.err); err != nil {
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

	if review && agent.supportsReview {
		if err := a.runReview(agent, executable, sessionID); err != nil {
			return err
		}
	}

	if complete {
		if err := a.runCompletion(agent, executable, sessionID, taskID); err != nil {
			return err
		}
	}
	if commit {
		return a.runCommit(agent, executable, sessionID, taskID)
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
func (a *app) runReview(agent taskWorkAgent, executable, sessionID string) error {
	fmt.Fprintln(a.err, "--- Running review ---")

	reviewArgs := agent.reviewArgs(taskWorkReviewPrompt)

	var reviewBuf bytes.Buffer
	reviewOut := io.MultiWriter(a.out, &reviewBuf)

	if err := taskWorkRunCommand(context.Background(), a.opts.root, executable, reviewArgs, nil, reviewOut, a.err); err != nil {
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
	return taskWorkRunCommand(context.Background(), a.opts.root, executable, resumeArgs, a.in, a.out, a.err)
}

// runCompletion resumes the agent session with a completion handoff prompt,
// asking the delegated agent to fill acceptance notes, run verification, and
// mark the task completed through ahm.
func (a *app) runCompletion(agent taskWorkAgent, executable, sessionID, taskID string) error {
	fmt.Fprintln(a.err, "--- Running completion handoff ---")
	prompt := a.buildTaskWorkCompletionPrompt(taskID)
	resumeArgs := agent.resumeArgs(sessionID, prompt)
	if err := taskWorkRunCommand(context.Background(), a.opts.root, executable, resumeArgs, a.in, a.out, a.err); err != nil {
		return fmt.Errorf("completion handoff failed: %w", err)
	}
	return nil
}

// buildTaskWorkCompletionPrompt returns the prompt used to ask the delegated
// agent to complete a task. The agent is expected to fill acceptance notes,
// run verification, and run "ahm task complete <id>" when satisfied.
func (a *app) buildTaskWorkCompletionPrompt(taskID string) string {
	return fmt.Sprintf(`Complete task %s.

Fill the task Acceptance Notes, run the required verification (such as "just ci"), and mark the task completed with ahm when acceptance is satisfied:

  ahm task complete %s

If verification fails, address the findings and retry. Do not commit or push unless the user explicitly asked for that.`, taskID, taskID)
}

// runCommit resumes the agent session with a commit handoff prompt. The
// delegated agent owns the actual git operation; ahm only sends the prompt.
func (a *app) runCommit(agent taskWorkAgent, executable, sessionID, taskID string) error {
	fmt.Fprintln(a.err, "--- Running commit handoff ---")
	prompt := a.buildTaskWorkCommitPrompt(taskID)
	resumeArgs := agent.resumeArgs(sessionID, prompt)
	if err := taskWorkRunCommand(context.Background(), a.opts.root, executable, resumeArgs, a.in, a.out, a.err); err != nil {
		return fmt.Errorf("commit handoff failed: %w", err)
	}
	return nil
}

// buildTaskWorkCommitPrompt returns the prompt used to ask the delegated agent
// to commit completed task work. Commit message policy remains project-owned.
func (a *app) buildTaskWorkCommitPrompt(taskID string) string {
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
