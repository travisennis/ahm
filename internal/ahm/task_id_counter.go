package ahm

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

// A top-level task ID is the higher of the store's persisted counter and one past
// the highest number the records show.
//
// In project mode the records are committed files, and Git history proves that
// a deleted ID was used, so the records are the only source. A store has no
// history: a record deleted by hand would return its number to the pool, and
// every record that already references it — a note, another task, an ADR —
// would silently come to mean a different task. The counter is therefore the
// store's durable high-water mark, persisted beside the records in the store's
// project state file.

// taskIDCounterPath is the state file that holds the store's task ID counter.
// It reports false when the records live in the project, where no counter is
// needed.
func (p workflowPaths) taskIDCounterPath() (string, bool) {
	if !p.inStore() {
		return "", false
	}
	return p.store.statePath(), true
}

// readTaskIDCounter returns the next top-level task ID the store will allocate,
// or 0 when the records live in the project or no counter is recorded yet.
func readTaskIDCounter(paths workflowPaths) (int, error) {
	if _, ok := paths.taskIDCounterPath(); !ok {
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

// writeTaskIDCounter raises the persisted counter to next and writes nothing
// when the file already holds that ID or a higher one, because the counter
// never decreases: an observation taken from a store that predates it must not
// roll one back. A next below 1 is ignored, because `next_id` is omitted when it
// is zero and a zero write would drop the field. The write is contained by
// writeOwned like every other workflow write, because the state file sits in the
// store's project directory, which is an owned root when the records live
// there.
func writeTaskIDCounter(paths workflowPaths, next int) error {
	path, ok := paths.taskIDCounterPath()
	if !ok || next < 1 {
		return nil
	}
	state, err := readProjectState(paths.store)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	next = higherTaskIDCounter(next, state.NextID)
	if next == state.NextID {
		// The file already holds this ID or a higher one, so there is nothing to
		// raise and nothing to write.
		return nil
	}
	state.Version = storeFormatVersion
	state.NextID = next
	data, err := marshalStoreJSON(state)
	if err != nil {
		return err
	}
	return writeOwned(paths, path, data)
}

// higherTaskIDCounter returns the higher of two counter observations that meet
// in the store state file. `ahm task create` writes the counter through
// writeOwned inside the record lock, while `store path` records its observation
// of the same file through writeFileAtomic with no lock, so both take the
// higher value: the counter exists to remember the numbers a store has spent,
// and no writer may move it down.
func higherTaskIDCounter(carried int, persisted int) int {
	return max(carried, persisted)
}

// persistTaskIDCounter raises the store's counter past the ID a command just
// allocated, so the next allocation cannot reuse it. A child ID carries its
// parent's number, and the parent record that justifies that number is present
// by construction, so only a top-level allocation advances the counter.
func persistTaskIDCounter(paths workflowPaths, id string) error {
	number, suffix, ok := splitTaskID(id)
	if !ok || suffix != "" {
		return nil
	}
	return writeTaskIDCounter(paths, number+1)
}

// highestTaskNumber returns the highest top-level number present among the
// parsed records and the record directory entries. The directory scan covers a
// record that failed to parse, so a malformed file still reserves its number
// instead of letting the next create collide with it.
func highestTaskNumber(tasks []Task, paths workflowPaths) int {
	highest := 0
	consider := func(id string) {
		if number, suffix, ok := splitTaskID(id); ok && suffix == "" && number > highest {
			highest = number
		}
	}
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
	return highest
}
