package ahm

import (
	"path/filepath"
	"strings"
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

// pathWithin reports whether path is root itself or lies below it. Both are
// cleaned first, and the comparison is made on the relative path so that a
// sibling whose name merely shares a prefix (root "a/b" and path "a/bc") is
// not mistaken for a child. A relative path is never within an absolute root.
func pathWithin(root string, path string) bool {
	if root == "" || path == "" {
		return false
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanRoot {
		return true
	}
	if filepath.IsAbs(cleanRoot) != filepath.IsAbs(cleanPath) {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
