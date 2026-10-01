---
status: accepted
date: 2026-10-01
---
# Import task batches with prevalidation and rollback

## Context and Problem Statement

Task 281 needs to create a backlog from offline JSON while preserving provenance
and resolving dependencies and parents that occur later in the document.
Repeated task creation cannot validate a whole batch or recover a failed batch.

## Decision Drivers

- Preserve the existing task allocator, record format, and store identity.
- Report all record refusals before any write and acquire the record lock once.
- Keep dry runs read-only, including lock and counter state.

## Considered Options

- Repeated task create calls, which leave partial imports.
- A new database or durable transaction journal, which adds a storage format.
- Prevalidate the entire batch and restore written files on an ordinary failure.

## Decision Outcome

Use `task import --from-file` with a JSON array of task objects. Optional `ref`
keys identify records inside the file; `@ref` references denote batch records,
while numeric IDs denote existing records. Allocate top-level IDs in document
order, then children under resolved top-level parents, so forward parents work.
Dependencies resolve after every ID is allocated. Import title, body, status,
priority, effort, labels, created, parent, depends_on, and external_ref; generate
id, updated, path, and bucket. Unknown and duplicate fields are refused. Duplicate field names are compared
case-insensitively, matching Go's JSON decoder, and checked before building a
map so earlier values cannot be hidden. The owner confirmed duplicate-field
rejection on 2026-10-01 after review exposed this ambiguity. Comments and
cancellation provenance are carried in the Markdown body.

Malformed JSON or an invalid document shape exits 2. Semantic record refusals
exit 1 with all errors and the complete allocation plan. Relative Markdown links
remain the importer's responsibility and are checked by status's link scope.
A valid batch is written under one record lock and regenerates indexes once.
The importer snapshots affected index files and store counter state and restores
records, indexes, and counter on a reported write failure. Counter restoration
holds the store-state lock throughout the transaction to avoid rolling back
another command's observation. Registry state is never changed.

### Consequences

- Good: validation refusal writes nothing, and ordinary write failures roll back.
- Good: existing record formats and store resolution remain compatible.
- Bad: restoration can itself fail; report that error and recovery paths.
- Bad: process termination or power loss can interrupt the batch; individual
  files remain atomic, but there is no crash-atomic transaction. Inspect records
  and run index before retrying; retrying a successful import creates new tasks.

## More Information

Task 281 records the owner's accepted JSON, timestamp, forward-reference,
all-or-nothing validation, and external-reference decisions. ADR 001 defines
individual atomic writes; ADR 024 defines the store-state lock.
