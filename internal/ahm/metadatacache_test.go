package ahm

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestMetadataCacheReadsEachRootOnce pins the reuse contract: a second read of
// the same root is served from memory, so the file can change (here it is
// removed outright) without a later reader observing it, and the cache is keyed
// by root rather than holding a single value.
func TestMetadataCacheReadsEachRootOnce(t *testing.T) {
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{StrictAcceptance: false, Files: map[string]string{"keep": "v"}})
	cache := newMetadataCache()

	first, err := cache.read(root)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}

	// Delete the file: a read that went back to disk would fail instead of
	// returning the configuration the first read cached.
	if err := os.Remove(workflowPathsFor(root).configPath()); err != nil {
		t.Fatal(err)
	}
	second, err := cache.read(root)
	if err != nil {
		t.Fatalf("cached read after removal: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached read = %+v, want %+v", second, first)
	}

	// A different root is a different key and still reads from disk.
	other := t.TempDir()
	if _, err := cache.read(other); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read of an unconfigured root = %v, want os.ErrNotExist", err)
	}
}

// TestMetadataCacheReturnsCopy proves a caller cannot mutate the cached value.
// The metadata holds two maps, so the copy must reach the map contents: a
// shallow return would let a writer such as install disturb what a later reader
// sees.
func TestMetadataCacheReturnsCopy(t *testing.T) {
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{
		StrictAcceptance: false,
		Files:            map[string]string{"a": "1"},
		Extra:            map[string]json.RawMessage{"x": json.RawMessage(`{"n":1}`)},
	})
	cache := newMetadataCache()

	m, err := cache.read(root)
	if err != nil {
		t.Fatal(err)
	}
	originalExtra := string(m.Extra["x"])

	// Mutate everything reachable through the returned value.
	m.Files["a"] = "mutated"
	m.Files["new"] = "n"
	m.TasksLocation = "home"
	m.StrictAcceptance = true
	m.Extra["x"][0] = 'X'
	m.Extra["y"] = json.RawMessage(`1`)

	again, err := cache.read(root)
	if err != nil {
		t.Fatal(err)
	}
	if again.Files["a"] != "1" || again.Files["new"] != "" {
		t.Errorf("Files = %#v, want the cached map unchanged", again.Files)
	}
	if again.TasksLocation != "" || again.StrictAcceptance {
		t.Errorf("scalars = %q/%v, want the cached values unchanged", again.TasksLocation, again.StrictAcceptance)
	}
	if string(again.Extra["x"]) != originalExtra {
		t.Errorf("Extra[x] = %s, want %s", again.Extra["x"], originalExtra)
	}
	if _, ok := again.Extra["y"]; ok {
		t.Errorf("Extra = %#v, want the cached map unchanged", again.Extra)
	}
}

// TestNilMetadataCacheReadsThrough keeps the nil cache honest: a caller with
// nothing to hand off must still see current on-disk state on every call.
func TestNilMetadataCacheReadsThrough(t *testing.T) {
	root := t.TempDir()
	var cache *metadataCache
	if _, err := cache.read(root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("nil cache read of unconfigured root = %v, want os.ErrNotExist", err)
	}
	writeMetadataFile(t, root, metadata{Files: map[string]string{}})
	if _, err := cache.read(root); err != nil {
		t.Errorf("nil cache read = %v, want nil", err)
	}
}

// TestValidatorsShareOneMetadataRead is acceptance criterion two: every
// validator in one run observes the same configuration even when the file is
// rewritten mid-run. The first validator reads through the run's shared cache;
// the file then changes out-of-band; the remaining validators read the same
// cache and therefore the same configuration. The uncached control shows the
// rewrite would otherwise surface as a finding.
func TestValidatorsShareOneMetadataRead(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	cache := newMetadataCache()
	report := newValidationReport(newRecordCache(), cache)

	if err := validateMetadata(root, &report); err != nil {
		t.Fatalf("validateMetadata: %v", err)
	}
	// Rewrite the configuration mid-run.
	writeFile(t, paths.configPath(), "{ not json")

	validateGeneratedIndexMetadata(paths, &report)
	validateMarkdownLinks(root, paths, &report)

	if hasFinding(report.Errors, "metadata_corrupt") {
		t.Fatalf("validators observed different metadata mid-run: %#v", report.Errors)
	}

	// Control: without the shared cache the rewrite is visible.
	uncached := newValidationReport(newRecordCache(), nil)
	if err := validateMetadata(root, &uncached); err == nil {
		t.Fatal("control expected a corrupt read, got nil")
	}
	if !hasFinding(uncached.Errors, "metadata_corrupt") {
		t.Fatalf("control expected metadata_corrupt, got %#v", uncached.Errors)
	}
}

// TestInstallInvalidatesMetadataCache is acceptance criterion three: a command
// that writes the configuration observes its own write. install primes the
// cache with the absent configuration, writes the new project's config, and a
// later read through the same app must see it rather than the memoized miss.
func TestInstallInvalidatesMetadataCache(t *testing.T) {
	useTemporaryStoreHome(t)
	root := t.TempDir()
	a := app{opts: options{root: root}, out: &strings.Builder{}, err: &strings.Builder{}}

	if _, err := a.readMetadataFor(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-install read = %v, want os.ErrNotExist", err)
	}
	if err := a.install(); err != nil {
		t.Fatal(err)
	}
	meta, err := a.readMetadataFor(root)
	if err != nil {
		t.Fatalf("post-install cached read: %v", err)
	}
	if meta.TasksLocation != string(locationHome) {
		t.Errorf("post-install TasksLocation = %q, want %q", meta.TasksLocation, locationHome)
	}
}
