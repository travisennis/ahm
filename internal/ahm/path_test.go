package ahm

import (
	"testing"
)

func TestPathWithin(t *testing.T) {
	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{name: "root itself", root: "/a/b", path: "/a/b", want: true},
		{name: "direct child", root: "/a/b", path: "/a/b/c.md", want: true},
		{name: "deep child", root: "/a/b", path: "/a/b/c/d/e.md", want: true},
		{name: "sibling sharing a name prefix", root: "/a/b", path: "/a/bc/d.md", want: false},
		{name: "parent", root: "/a/b", path: "/a", want: false},
		{name: "escape through parent traversal", root: "/a/b", path: "/a/b/../../etc/passwd", want: false},
		{name: "relative path under an absolute root", root: "/a/b", path: "a/b/c.md", want: false},
		{name: "empty root", root: "", path: "/a/b", want: false},
		{name: "empty path", root: "/a/b", path: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathWithin(tt.root, tt.path); got != tt.want {
				t.Errorf("pathWithin(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
			}
		})
	}
}
