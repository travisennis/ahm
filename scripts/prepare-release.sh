#!/usr/bin/env bash
set -euo pipefail

usage() {
	echo "usage: scripts/prepare-release.sh [vX.Y.Z]" >&2
	exit 2
}

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
current_branch="$(git branch --show-current)"
if [[ -z "$current_branch" ]]; then
	echo "prepare-release: cannot determine current branch" >&2
	exit 1
fi

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
  git tag -a $version -m "$version"
  git push origin $current_branch
  git push origin $version

EOF
