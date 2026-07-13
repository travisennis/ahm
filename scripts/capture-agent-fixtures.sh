#!/bin/bash
# capture-agent-fixtures.sh — Refresh the golden agent transcripts under
# internal/ahm/testdata/agents/ by running the real agent CLIs.
#
# Usage: just capture-agent-fixtures
#
# Each capture makes real LLM calls and costs money. Run it manually after an
# agent CLI upgrade or a suspected output-schema change; never run it in CI.
# Agents that are not on PATH are skipped. Prompts are deliberately trivial
# and token limits are kept low where the CLI supports them.
#
# Captures run inside a throwaway git repository so transcripts never leak
# repository content; the scratch path, $HOME, and the local username are
# scrubbed afterwards.
set -euo pipefail

command -v jq >/dev/null 2>&1 || { echo "Error: jq is not installed or not on PATH" >&2; exit 1; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
fixtures_dir="$repo_root/internal/ahm/testdata/agents"
mkdir -p "$fixtures_dir"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
git -C "$scratch" init -q
# Install the ahm workflow so the scratch repo matches a real `ahm task work`
# environment and carries the normal repository guidance.
(cd "$repo_root" && go run ./cmd/ahm --root "$scratch" init >/dev/null)
# Seed an uncommitted change for the review capture to inspect.
printf 'In order to demonstrate, this file utilizes very unique words.\n' >"$scratch/notes.txt"

work_prompt="Reply with the single word: ok. Do not use any tools or read any files."
resume_prompt="Reply with the single word: done. Do not use any tools or read any files."
# Must match the stable marker at the start of the prompt runReview sends; the
# capture exercises the external output contract, while focused Go tests cover
# the full task-specific prompt. TestCaptureScriptUsesReviewPrompt guards drift.
review_prompt="Review the current uncommitted changes."

scrub() {
  # /private covers macOS, where mktemp paths resolve through /private/var.
  # The generic /var/folders and /tmp rules catch other machine-local temp
  # paths an agent may stumble into while exploring.
  sed -E \
    -e "s|/private$scratch|/scratch|g" \
    -e "s|$scratch|/scratch|g" \
    -e "s|$HOME|/home/user|g" \
    -e "s|$(id -un)|user|g" \
    -e "s|(/private)?/var/folders/[A-Za-z0-9/_.+-]*|/scratch-other|g" \
    -e "s|/tmp/[A-Za-z0-9/_.+-]+|/scratch-other|g" "$1" >"$1.tmp"
  mv "$1.tmp" "$1"
}

# capture <name> <version> <command...> runs the command in the scratch repo,
# writes the scrubbed transcript to <name>.jsonl, and records provenance in
# <name>.meta from the same argv so the sidecar cannot drift from the run.
capture() {
  local name="$1" version="$2"
  shift 2
  echo "── Capturing $name transcript ──" >&2
  (cd "$scratch" && "$@" </dev/null) >"$fixtures_dir/$name.jsonl"
  scrub "$fixtures_dir/$name.jsonl"
  printf 'agent: %s\ncaptured: %s\ncommand: %s\n' "$version" "$(date +%Y-%m-%d)" "$*" \
    >"$fixtures_dir/$name.meta"
}

if command -v cake >/dev/null 2>&1; then
  cake_version="$(cake --version)"
  capture cake-work "$cake_version" \
    cake --max-tokens 512 --output-format stream-json "$work_prompt"
  capture cake-review "$cake_version" \
    cake --max-tokens 512 --output-format stream-json "$review_prompt"
else
  echo "── cake not on PATH, skipping cake captures ──" >&2
fi

if command -v codex >/dev/null 2>&1; then
  codex_version="$(codex --version)"
  capture codex-exec "$codex_version" \
    codex exec -c model_reasoning_effort=low --dangerously-bypass-approvals-and-sandbox --json "$work_prompt"

  capture codex-review "$codex_version" \
    codex exec -c model_reasoning_effort=low --dangerously-bypass-approvals-and-sandbox --json "$review_prompt"

  thread_id="$(jq -r 'select(.type == "thread.started") | .thread_id' "$fixtures_dir/codex-exec.jsonl" | head -n 1)"
  if [ -z "$thread_id" ] || [ "$thread_id" = "null" ]; then
    echo "Error: no thread.started thread_id in codex exec transcript; cannot capture resume" >&2
    exit 1
  fi

  capture codex-resume "$codex_version" \
    codex exec resume -c model_reasoning_effort=low --dangerously-bypass-approvals-and-sandbox --json "$thread_id" "$resume_prompt"
else
  echo "── codex not on PATH, skipping codex captures ──" >&2
fi

echo "── Captures written to $fixtures_dir ──" >&2
