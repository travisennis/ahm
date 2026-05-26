#!/bin/bash
# task-workflow.sh — Work a ticket through cake with automated deslop review
#
# Usage: ./task-workflow.sh <task-id>
#
# Runs four sequential cake invocations:
#   1. Work the ticket (captured session)
#   2. Run the deslop skill in a fresh session to get a review
#   3. Resume the working session, feeding in deslop feedback
#   4. Commit the result
#
set -euo pipefail

command -v cake >/dev/null 2>&1 || { echo "Error: cake is not installed or not on PATH" >&2; exit 1; }
command -v jq   >/dev/null 2>&1 || { echo "Error: jq is not installed or not on PATH" >&2; exit 1; }

task_id="${1:?Usage: $0 <task-id>}"

# ────────────────────────────────────────────────────────────────────────────
# Inlined work-task.md prompt (same content as the repo's work-task.md,
# with the task ID substituted in)
# ────────────────────────────────────────────────────────────────────────────
work_prompt="Work on ticket ${task_id}.

Once you are done with the work, anticipate everything I will give you
feedback on and fix it."

# ────────────────────────────────────────────────────────────────────────────
# Step 1 — Work on the ticket
# ────────────────────────────────────────────────────────────────────────────
echo "── Step 1/4: Working on ticket ${task_id} ──" >&2

json1=$(cake --output-format json "$work_prompt" 2>/dev/null)

session_id=$(echo "$json1" | jq -r '.session_id')
if [ -z "$session_id" ] || [ "$session_id" = "null" ]; then
  echo "Error: failed to capture session ID from cake output" >&2
  exit 1
fi
echo "  Session: ${session_id}" >&2
echo "  Result:" >&2
echo "$json1" | jq -r '.result // "No response"'
echo "" >&2

# ────────────────────────────────────────────────────────────────────────────
# Step 2 — Run the deslop skill in a fresh session to get an independent
#          review of the current state of the repo.
# ────────────────────────────────────────────────────────────────────────────
echo "── Step 2/4: Running deslop review ──" >&2

deslop_json=$(cake --no-session --model glm5-1 --skills deslop --output-format json \
  "Run the deslop skill on the current changes.
Review all uncommitted modifications and report any issues that need
to be addressed before commit." 2>/dev/null)

deslop_feedback=$(echo "$deslop_json" | jq -r '.result // .error // "No feedback from deslop"')

# Show a summary of the feedback
echo "  Deslop feedback: ${#deslop_feedback} characters" >&2
echo "$deslop_feedback" | head -5
echo "  (...)" >&2
echo "" >&2

# ────────────────────────────────────────────────────────────────────────────
# Step 3 — Resume the original session and ask the agent to address each
#          issue the deslop review raised.
# ────────────────────────────────────────────────────────────────────────────
if [ -z "$deslop_feedback" ] || [ "$deslop_feedback" = "No feedback from deslop" ]; then
  echo "  No deslop feedback to address, skipping Step 3" >&2
else
  echo "── Step 3/4: Addressing deslop feedback in session ${session_id:0:8}… ──" >&2

  {
    printf '%s\n' "Please address the following deslop review feedback:"
    printf '%s\n' ""
    printf '%s\n' "$deslop_feedback"
  } | cake --resume "$session_id" --output-format json - 2>/dev/null | jq -r '.result // "No response"'

  echo "" >&2
fi

# ────────────────────────────────────────────────────────────────────────────
# Step 4 — Commit the completed work
# ────────────────────────────────────────────────────────────────────────────
echo "── Step 4/4: Committing ──" >&2
cake --resume "$session_id" "Now that all of the deslop feedback has been addressed, commit the completed work for ticket ${task_id}. Make sure the task is marked completed before committing. Include both task files and project source files in a single commit."
echo "" >&2

echo "── Done ──" >&2
