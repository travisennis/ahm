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
//
// roles carries the resolved agents and models for each phase.
// executable and args are already built for the implementation agent.
// reviewExecutable is empty when the review agent is the same as the
// implementation agent (no separate PATH lookup needed).
func (a *app) taskWorkWithSession(roles taskWorkRoles, executable string, args []string, reviewExecutable string, review bool, commit bool, task Task, timeout time.Duration) error {
	var stdoutBuf bytes.Buffer
	// Write captured output to both the user's terminal and the buffer.
	out := io.MultiWriter(a.out, &stdoutBuf)
	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, args, a.in, out, a.err); err != nil {
		return err
	}
	sessionID, parseErr := roles.implAgent.parseSessionID(stdoutBuf.Bytes())
	if parseErr != nil {
		if commit {
			return fmt.Errorf("cannot run commit handoff: could not capture session ID from %s output: %w", roles.implAgent.name, parseErr)
		}
		a.addWarning("could not capture session ID from %s output: %v", roles.implAgent.name, parseErr)
		return nil
	}
	if sessionID == "" {
		if commit {
			return fmt.Errorf("cannot run commit handoff: no session ID returned by %s", roles.implAgent.name)
		}
		a.addWarning("no session ID returned by %s", roles.implAgent.name)
		return nil
	}

	fmt.Fprintf(a.err, "%s session started: %s\n", roles.implAgent.name, truncatedID(sessionID, 8))

	if review {
		// The implementation agent may have updated acceptance notes, ExecPlan
		// metadata, or task status. Re-read after the external process finishes so
		// the reviewer sees the review-ready state rather than the intake snapshot.
		a.invalidateTasks()
		reviewTask, err := a.resolveTask(task.ID)
		if err != nil {
			return fmt.Errorf("cannot load task %s for review: %w", task.ID, err)
		}
		revExec := reviewExecutable
		if revExec == "" {
			revExec = executable
		}

		// Run independent review and capture feedback.
		feedback, err := a.runReview(roles.reviewAgent, revExec, sessionID, timeout, roles.reviewModel, buildTaskWorkReviewPrompt(reviewTask))
		if err != nil {
			return err
		}

		// Finalization: resume the implementation session to address feedback
		// (when present) and finalize the task.
		finalizationPrompt := buildFinalizationPrompt(task.ID, feedback)
		finalizeArgs := roles.implAgent.resumeArgs(sessionID, finalizationPrompt)
		if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, executable, finalizeArgs, a.in, a.out, a.err); err != nil {
			return fmt.Errorf("finalization failed: %w", err)
		}

		// Reload task and validate it was properly completed.
		a.invalidateTasks()
		finalizedTask, err := a.resolveTask(task.ID)
		if err != nil {
			return fmt.Errorf("cannot load task %s after finalization: %w", task.ID, err)
		}
		if finalizedTask.Status != "Completed" {
			return fmt.Errorf("task %s was not marked completed after finalization; status is %s", task.ID, finalizedTask.Status)
		}
		findings := parseAcceptanceNotes([]byte(finalizedTask.Body))
		for _, finding := range findings {
			a.addWarning("%s", finding.message(task.ID))
		}
		if len(findings) > 0 {
			meta, metaErr := readMetadata(a.opts.root)
			if metaErr == nil && meta.StrictAcceptance {
				return fmt.Errorf("task %s finalization failed: acceptance notes are incomplete under strict acceptance policy", task.ID)
			}
		}
	}
	if commit {
		return a.runCommit(roles.implAgent, executable, sessionID, task.ID, timeout)
	}
	return nil
}

const taskWorkReviewPromptMarker = "Review the current uncommitted changes."

const taskWorkReviewProcedure = `Determine change size with read-only git commands, including untracked files:

  git diff --stat
  git status --short

Scale the review to the actual change: XS/S gets one combined pass; M gets two
sequential passes (rules/documentation conformance, then correctness and source
of truth); L/XL gets three sequential passes (those two plus overengineering
and simplification). Read the repository guidance and only relevant task, plan,
architecture, and design context. Keep each pass focused.

Check repository rules and documentation coverage, then boundary validation,
canonical models and schemas, failure/resource handling, state transitions,
and available compiler/linter/test checks. For L/XL, separately remove
unnecessary abstraction or indirection. Synthesize findings, ignore speculative
or scope-widening advice, and apply every clearly worthwhile fix. The goal is
the smallest clear diff that fully satisfies the task. Run focused verification
after fixes and the repository's documented final verification when required.
Never commit or push during this review.`

func buildTaskWorkReviewPrompt(task Task) string {
	acceptance := taskAcceptanceSection(task.Body)
	if acceptance == "" {
		acceptance = "(No acceptance section found; flag this as a completion issue.)"
	}
	return fmt.Sprintf(`%s

Task: %s — %s

Acceptance notes:
%s

Review procedure:
%s

Managed-work completion checklist:
- Acceptance notes are checked off and match the implemented behavior.
- The task's exec_plan, when present, is updated or completed.
- User-visible or durable behavior has the required documentation updates.
- Generated indexes were not hand-edited; source records and ahm commands own them.
- Task lifecycle changes use ahm task commands.
`, taskWorkReviewPromptMarker, task.ID, task.Title, acceptance, taskWorkReviewProcedure)
}

func taskAcceptanceSection(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	start, level := acceptanceSectionStart(lines)
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if next := headingLevel(strings.TrimSpace(lines[i])); next > 0 && next <= level {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
}

// runReview runs an independent review pass using the reviewAgent's review
// capability and returns the parsed feedback string. It does not resume the
// implementation session; the caller (taskWorkWithSession) handles finalization
// via a separate resume with the feedback incorporated.
// If the review produces no feedback or feedback cannot be parsed, the returned
// string is empty.
// If the review command itself fails, the error is surfaced.
func (a *app) runReview(reviewAgent taskWorkAgent, reviewExecutable string, sessionID string, timeout time.Duration, reviewModel string, prompt string) (string, error) {
	fmt.Fprintln(a.err, "--- Running review ---")

	reviewArgs := reviewAgent.reviewArgs(prompt, reviewModel)

	var reviewBuf bytes.Buffer
	reviewOut := io.MultiWriter(a.out, &reviewBuf)

	if err := taskWorkRunCommand(withTaskWorkTimeout(context.Background(), timeout), a.opts.root, reviewExecutable, reviewArgs, nil, reviewOut, a.err); err != nil {
		return "", fmt.Errorf("review failed: %w", err)
	}

	feedback, parseErr := reviewAgent.parseReviewFeedback(reviewBuf.Bytes())
	if parseErr != nil {
		a.addWarning("could not parse review feedback from %s output: %v", reviewAgent.name, parseErr)
		return "", nil
	}

	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		fmt.Fprintln(a.err, "No review feedback to address, proceeding to finalization.")
		return "", nil
	}

	fmt.Fprintf(a.err, "Review produced feedback for session %s.\n", truncatedID(sessionID, 8))
	return feedback, nil
}

// buildFinalizationPrompt returns a prompt that resumes the implementation
// session to finalize the task after review. When feedback is non-empty, the
// prompt asks the agent to address the feedback first, then verify, update
// Acceptance Notes, and complete the task. When feedback is empty, the prompt
// asks the agent to verify and complete the task directly.
func buildFinalizationPrompt(taskID string, feedback string) string {
	if feedback == "" {
		return fmt.Sprintf(`Finalize task %s.

The review found no issues to address. Verify the implementation meets the Acceptance Notes, run any remaining verification, update the task Acceptance Notes to reflect the verified state, and mark the task complete with ahm. Do not commit or push.`, taskID)
	}
	return fmt.Sprintf(`Finalize task %s.

Address the following review feedback:

%s

After addressing the feedback, verify the implementation meets the Acceptance Notes, run any remaining verification, update the task Acceptance Notes to reflect the verified state, and mark the task complete with ahm. Do not commit or push.`, taskID, feedback)
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
// Both legacy (.agents/) and migrated (.ahm/) layouts keep source records
// committed, so the prompt is the same for both layouts.
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
