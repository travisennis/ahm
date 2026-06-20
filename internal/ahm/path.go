package ahm

import (
	"path/filepath"
)

// relPath converts an absolute path to a slash-separated relative path
// from root. If root is empty or path is not absolute, it returns the
// path as-is with slashes.
func relPath(root string, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
