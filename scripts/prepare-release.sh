#!/usr/bin/env bash
set -euo pipefail

usage() {
	echo "usage: scripts/prepare-release.sh [vX.Y.Z]" >&2
	exit 2
}

# Releases are cut from master only. Guard before the svu/git-cliff tool
# checks so a feature branch fails fast with this message. Master takes direct
# commits, so the changelog commit below lands on master like any other.
current_branch="$(git branch --show-current)"
if [[ -z "$current_branch" ]]; then
	echo "prepare-release: cannot determine current branch" >&2
	exit 1
fi
if [[ "$current_branch" != "master" ]]; then
	echo "prepare-release: releases are cut from master; current branch is $current_branch" >&2
	exit 1
fi

version="${1:-}"
if [[ -z "$version" ]]; then
	if ! command -v svu >/dev/null 2>&1; then
		echo "prepare-release: missing required command: svu" >&2
		exit 1
	fi
	version="$(svu next --tag.output tag.prefix)"
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
	usage
fi

if ! command -v git-cliff >/dev/null 2>&1; then
	echo "prepare-release: missing required command: git-cliff" >&2
	exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
	echo "prepare-release: worktree must be clean before preparing a release" >&2
	exit 1
fi

if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	echo "prepare-release: tag already exists: $version" >&2
	exit 1
fi

previous_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"

if [[ -n "$previous_tag" ]]; then
	git-cliff --tag "$version" --output CHANGELOG.md
else
	git-cliff --unreleased --tag "$version" --output CHANGELOG.md
fi

just release-check

cat <<EOF
Prepared $version.

Review CHANGELOG.md, then run:

  git add CHANGELOG.md
  git commit -m "chore(release): prepare $version"
  git push origin master
  gh run list --limit 3                 # confirm CI is green on that commit
  git tag -a $version -m "$version"
  git push origin $version

EOF
