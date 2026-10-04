package ahm

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// The store keeps a durable high-water mark for every task ID it allocates, in
// its per-project state file: `next_id` for the top-level numbers and a
// `child_suffix_marks` entry for each parent's child letters.
//
// In project mode the records are committed files, and Git history proves that
// a deleted ID was used, so the records are the only source. A store has no
// history: a record deleted by hand would return its number or letter to the
// pool, and every record that already references it — a note, another task, an
// ADR — would silently come to mean a different task. The marks are therefore
// the store's durable record of what it has spent, persisted beside the records
// in the store's project state file.

// storeStatePath is the state file that holds the store's task ID marks. It
// reports false when the records live in the project, where no state file is
// needed.
func (p workflowPaths) storeStatePath() (string, bool) {
	if !p.inStore() {
		return "", false
	}
	return p.store.statePath(), true
}

// readTaskIDCounter returns the next top-level task ID the store will allocate,
// or 0 when the records live in the project or no counter is recorded yet.
func readTaskIDCounter(paths workflowPaths) (int, error) {
	if _, ok := paths.storeStatePath(); !ok {
		return 0, nil
	}
	state, err := readProjectState(paths.store)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return state.NextID, nil
}

// readChildSuffixMark returns the highest child letter the store has recorded
// under the parent, or "" when the records live in the project or no mark is
// recorded yet. A recorded value that is not a single lowercase letter is
// refused rather than skipped: the mark is the store's only evidence that a
// deleted child's letter was spent, so a value ahm cannot read is an error, not
// a silent reissue.
func readChildSuffixMark(paths workflowPaths, parentID string) (string, error) {
	if _, ok := paths.storeStatePath(); !ok {
		return "", nil
	}
	state, err := readProjectState(paths.store)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	mark := state.ChildSuffixMarks[parentID]
	if mark != "" && !validChildSuffix(mark) {
		return "", fmt.Errorf("corrupt store state %s: child suffix mark for parent %s is %q", paths.store.statePath(), parentID, mark)
	}
	return mark, nil
}

// writeTaskIDCounter raises the persisted counter to next and writes nothing
// when the file already holds that ID or a higher one, because the counter
// never decreases: an observation taken from a store that predates it must not
// roll one back. A next below 1 is ignored, because `next_id` is omitted when it
// is zero and a zero write would drop the field. The read-modify-write holds the
// store-state lock, so it cannot interleave with a `store path` observation of
// the same file. The write is contained by writeOwned like every other workflow
// write, because the state file sits in the store's project directory, which
// is an owned root when the records live there.
func writeTaskIDCounter(paths workflowPaths, next int) error {
	if _, ok := paths.storeStatePath(); !ok || next < 1 {
		return nil
	}
	return withStoreStateLock(paths.store, func() error {
		return mutateProjectStateLocked(paths, func(state *projectState) {
			state.NextID = higherTaskIDCounter(next, state.NextID)
		})
	})
}

// writeChildSuffixMark raises the persisted child suffix mark for the parent to
// suffix and writes nothing when the file already holds that letter or a higher
// one, because a mark never decreases: the letter of a deleted child must not
// return to the pool. The read-modify-write holds the store-state lock, so it
// cannot interleave with another project state writer.
func writeChildSuffixMark(paths workflowPaths, parentID string, suffix string) error {
	if _, ok := paths.storeStatePath(); !ok {
		return nil
	}
	if !validChildSuffix(suffix) {
		return fmt.Errorf("invalid child task suffix %q", suffix)
	}
	return withStoreStateLock(paths.store, func() error {
		return mutateProjectStateLocked(paths, func(state *projectState) {
			raiseChildSuffixMark(state, parentID, suffix)
		})
	})
}

// mutateProjectStateLocked reads the store's state file, applies mutate, and
// writes the result back unless the bytes are unchanged. The caller holds the
// store-state lock. Every field the file holds is a high-water mark, so mutate
// may only raise values; writeProjectState merges the higher of a carried and a
// persisted value as defense in depth for a writer that does not take the lock,
// such as a pre-mark ahm binary.
func mutateProjectStateLocked(paths workflowPaths, mutate func(*projectState)) error {
	path, ok := paths.storeStatePath()
	if !ok {
		return nil
	}
	state, err := readProjectState(paths.store)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	before, err := marshalStoreJSON(state)
	if err != nil {
		return err
	}
	state.Version = storeFormatVersion
	mutate(&state)
	after, err := marshalStoreJSON(state)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}
	storeStateWriteHook(path)
	return writeOwned(paths, path, after)
}

// higherTaskIDCounter returns the higher of two counter observations that meet
// in the store state file. The store-state lock is what keeps a stale
// observation out of the write between cooperating writers — `task create`
// raises the counter under the record lock, and `store path` records its own
// observation of the same file — so the merge is defense in depth: it holds the
// line for a writer that does not take the lock, such as a pre-lock ahm binary.
// The counter exists to remember the numbers a store has spent, and no writer
// may move it down.
func higherTaskIDCounter(carried int, persisted int) int {
	return max(carried, persisted)
}

// higherChildSuffix returns the higher of two child suffix observations,
// treating an empty or invalid value as no observation. Child suffixes are
// single lowercase letters, so lexical order is allocation order.
func higherChildSuffix(carried string, persisted string) string {
	if !validChildSuffix(carried) {
		carried = ""
	}
	if !validChildSuffix(persisted) {
		persisted = ""
	}
	return max(carried, persisted)
}

// higherChildSuffixMarks merges two observations of the store's per-parent
// child suffix marks, keeping the higher letter for every parent. It is the
// child-mark counterpart of higherTaskIDCounter, with the same defense-in-depth
// role: a stale observation may not return a spent letter to the pool.
func higherChildSuffixMarks(carried map[string]string, persisted map[string]string) map[string]string {
	if len(persisted) == 0 {
		return carried
	}
	merged := make(map[string]string, len(carried)+len(persisted))
	for parent, suffix := range carried {
		merged[parent] = suffix
	}
	for parent, suffix := range persisted {
		merged[parent] = higherChildSuffix(merged[parent], suffix)
	}
	return merged
}

// validChildSuffix reports whether suffix is a single lowercase letter, the
// only shape a child task ID suffix can have.
func validChildSuffix(suffix string) bool {
	return len(suffix) == 1 && suffix[0] >= 'a' && suffix[0] <= 'z'
}

// childSuffixAfter returns the letter after suffix in allocation order, "a"
// when suffix is empty, and "" when no letter is left. suffix must be empty or
// a single lowercase letter.
func childSuffixAfter(suffix string) string {
	if suffix == "" {
		return "a"
	}
	if suffix == "z" {
		return ""
	}
	return string(suffix[0] + 1)
}

// raiseChildSuffixMark raises state's mark for one parent to suffix, creating
// the map on first use. The mark only ever moves up.
func raiseChildSuffixMark(state *projectState, parentID string, suffix string) {
	if state.ChildSuffixMarks == nil {
		state.ChildSuffixMarks = map[string]string{}
	}
	state.ChildSuffixMarks[parentID] = higherChildSuffix(state.ChildSuffixMarks[parentID], suffix)
}

// persistTaskIDAllocation raises the store's high-water mark past the ID a
// command just allocated, so the next allocation cannot reuse it. A top-level
// ID advances the `next_id` counter; a child ID advances its parent's suffix
// mark, because a child letter says nothing about the numbers the store has
// spent, and the parent record that justifies the child's number is present by
// construction.
func persistTaskIDAllocation(paths workflowPaths, id string) error {
	number, suffix, ok := splitTaskID(id)
	if !ok {
		return nil
	}
	if suffix == "" {
		return writeTaskIDCounter(paths, number+1)
	}
	return writeChildSuffixMark(paths, fmt.Sprintf("%03d", number), suffix)
}

// highestTaskNumber returns the highest top-level number present among the
// parsed records and the record directory entries. The directory scan covers a
// record that failed to parse, so a malformed file still reserves its number
// instead of letting the next create collide with it.
func highestTaskNumber(tasks []Task, paths workflowPaths) int {
	highest := 0
	forEachRecordID(tasks, paths, func(id string) {
		if number, suffix, ok := splitTaskID(id); ok && suffix == "" && number > highest {
			highest = number
		}
	})
	return highest
}

// childSuffixMarksFromRecords returns the highest child letter present under
// each parent, keyed by the parent's canonical ID, from the parsed records and
// the record directory entries. Install and migration record these marks so a
// child deleted before the next allocation is still spent.
func childSuffixMarksFromRecords(tasks []Task, paths workflowPaths) map[string]string {
	marks := map[string]string{}
	forEachRecordID(tasks, paths, func(id string) {
		number, suffix, ok := splitTaskID(id)
		if !ok || !validChildSuffix(suffix) {
			return
		}
		parent := fmt.Sprintf("%03d", number)
		marks[parent] = higherChildSuffix(marks[parent], suffix)
	})
	return marks
}

// forEachRecordID calls consider for every task ID present: the parsed tasks
// and the record directory entries across all three buckets, so a file that
// failed to parse still reserves its ID.
func forEachRecordID(tasks []Task, paths workflowPaths, consider func(id string)) {
	for _, task := range tasks {
		consider(task.ID)
	}
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		entries, err := os.ReadDir(paths.tasksBucketDir(bucket))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "index.md" {
				continue
			}
			consider(strings.TrimSuffix(entry.Name(), ".md"))
		}
	}
}
