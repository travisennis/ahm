package ahm

// metadataCache memoizes committed .ahm/config.json reads for the span of one
// command: one Stat/ReadFile/Unmarshal per root per run, and one observed
// configuration for every validator that reads it.
//
// It is the configuration counterpart to recordCache's file reuse, but with a
// longer life: a command's whole flow reads the same configuration, whereas the
// record reuse is an explicit handoff between two steps. Backing the app makes
// the reuse automatic and the observed configuration stable, so validation
// findings all derive from one configuration even if the file changes mid-run.
//
// The cache is keyed by project root. Today a command resolves exactly one
// metadata root — a --project selection is records-only and reads no committed
// configuration — but keying by root keeps the cache honest if a future command
// reads more than one.
//
// A nil *metadataCache reads through to disk on every call, so callers with
// nothing to hand off (a standalone validator, or an app that never resolved a
// layout) still see current on-disk state.
//
// The cache is not safe for concurrent use; it is confined to one command's
// sequential flow.
type metadataCache struct {
	entries map[string]cachedMetadata
}

type cachedMetadata struct {
	meta metadata
	err  error
}

func newMetadataCache() *metadataCache {
	return &metadataCache{}
}

// read returns the configuration for root, reading it from disk on the first
// call and serving later calls from memory. A read failure is memoized too, so
// a missing or corrupt configuration is read once per command, exactly as
// recordCache memoizes a failed file read. The returned value is a copy, so a
// caller that mutates it — install reconciles the metadata it reads before
// writing it back — cannot disturb what a later caller sees.
func (c *metadataCache) read(root string) (metadata, error) {
	if c == nil {
		return readMetadata(root)
	}
	if entry, ok := c.entries[root]; ok {
		return entry.meta.clone(), entry.err
	}
	meta, err := readMetadata(root)
	if c.entries == nil {
		c.entries = map[string]cachedMetadata{}
	}
	c.entries[root] = cachedMetadata{meta: meta, err: err}
	return meta.clone(), err
}

// invalidate drops root's entry so the next read observes bytes the command
// just wrote. Every write of .ahm/config.json made while the cache is live must
// report through it, or a later read in the same command sees the pre-write
// configuration.
func (c *metadataCache) invalidate(root string) {
	if c == nil {
		return
	}
	delete(c.entries, root)
}
