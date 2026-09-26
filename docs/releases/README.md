# Release Notes

One file per tagged version, named for its tag: `v2.0.0.md` is the release
notes for `v2.0.0`. The release workflow publishes the file whose name matches
the tag as that release's GitHub Release body.

`CHANGELOG.md` cannot hold this prose. It is generated from Conventional
Commits by `git-cliff`, and `just prepare-release` overwrites the whole file,
so anything hand-written there is lost on the next release.

## What belongs here

Write a notes file for a release that removes or renames a command, a
configuration key, or a record family; changes a file format or a config
schema; or changes where records are stored. Those are the changes a reader
cannot reconstruct from a commit subject, and they are the ones that break an
existing repository.

Routine releases do not need a file. The release workflow falls back to the
changelog GoReleaser generates from the commit log.

## How to write one

- Lead with whether the release is breaking, and say so in the first line.
- Name every removed command, configuration key, and record family, and say
  what replaces it or that nothing does.
- Give the upgrade steps as commands a reader can paste, starting with the one
  step every upgrade needs.
- Say what is not changed, so a reader can rule the release out quickly.
- Do not delete an older file when a new release lands. Each file is the record
  of one published release.

See [`docs/release.md`](../release.md) for the release process that publishes
these files.
