package ahm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// The two kinds of project key. A key comes from the canonical remote URL when
// the repository names one candidate remote, and from the symlink-resolved
// project path otherwise.
const (
	storeKindRemote = "remote"
	storeKindPath   = "path"
)

// defaultRemotePorts maps a URL scheme to the port that is dropped from a
// canonical key, so that the same repository addressed with and without its
// default port derives one key. A scheme that is absent here keeps whatever
// port it names.
var defaultRemotePorts = map[string]string{
	"http":  "80",
	"https": "443",
	"ssh":   "22",
	"git":   "9418",
}

// storeDirSlugMax bounds the human-readable part of a store directory name.
const storeDirSlugMax = 24

// canonicalRemoteKey returns the store key for a Git remote: the lowercased
// host/owner/repo form with scheme, userinfo, default port, trailing .git, and
// trailing slash removed.
//
// It reports ok == false for a remote that names no host — a file:// URL or a
// local path — so the caller falls back to the path rule. Credentials in a URL
// never reach the key, because userinfo is dropped before the key is formed.
func canonicalRemoteKey(rawRemote string) (key string, ok bool) {
	remote := strings.ToLower(strings.TrimSpace(rawRemote))
	if remote == "" {
		return "", false
	}
	if !strings.Contains(remote, "://") {
		host, path, ok := splitScpLikeRemote(remote)
		if !ok {
			return "", false
		}
		return canonicalRemoteParts(host, "", path)
	}
	parsed, err := url.Parse(remote)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "file" {
		return "", false
	}
	port := parsed.Port()
	if port == defaultRemotePorts[scheme] {
		port = ""
	}
	return canonicalRemoteParts(parsed.Hostname(), port, parsed.Path)
}

// splitScpLikeRemote parses the scp-like remote syntax [user@]host:path. It
// reports ok == false for the local spellings, which name no host: an absolute
// path, a relative path, a Windows drive path, and any spelling without a
// colon.
func splitScpLikeRemote(remote string) (host string, path string, ok bool) {
	if strings.HasPrefix(remote, "/") || strings.HasPrefix(remote, ".") || strings.Contains(remote, `\`) {
		return "", "", false
	}
	hostPart, path, found := strings.Cut(remote, ":")
	if !found {
		return "", "", false
	}
	host = hostPart
	if idx := strings.LastIndex(hostPart, "@"); idx >= 0 {
		host = hostPart[idx+1:]
	}
	// A single character before the colon is a Windows drive letter, and a
	// slash means the path was absolute all along; neither names a host.
	if len(host) < 2 || strings.Contains(host, "/") {
		return "", "", false
	}
	return host, path, true
}

// canonicalRemoteParts assembles host, port, and repository path into a key.
// It reports ok == false when either the host or the path is missing, because
// such a remote does not identify one repository.
func canonicalRemoteParts(host string, port string, path string) (string, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.ToLower(strings.TrimSpace(path))
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	if host == "" || path == "" {
		return "", false
	}
	if port != "" {
		return host + ":" + port + "/" + path, true
	}
	return host + "/" + path, true
}

// canonicalPathKey returns the store key for a project that has no usable
// remote: the SHA-256 of the symlink-resolved absolute project root.
func canonicalPathKey(projectRoot string) (key string, err error) {
	resolved, err := resolveProjectPath(projectRoot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:]), nil
}

// resolveProjectPath returns the symlink-resolved absolute form of a project
// root, so a path that reaches one project through a symlink, a platform alias
// such as /var on macOS, or a redundant segment derives one identity. Casing is
// not normalized: the spelling the operating system reports is the spelling
// that is hashed.
func resolveProjectPath(projectRoot string) (string, error) {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolving project root %s: %w", projectRoot, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving project root %s: %w", abs, err)
	}
	return filepath.Clean(resolved), nil
}

// projectKeyFor derives the identity of one project. rawRemote is the remote
// the repository named, even when that remote could not key the project, so the
// store can record it as an observation; it is empty when no remote applies.
func projectKeyFor(projectRoot string) (key string, kind string, rawRemote string, err error) {
	remote, hasRemote, err := readGitRemote(projectRoot)
	if err != nil {
		return "", "", "", err
	}
	if hasRemote {
		if remoteKey, ok := canonicalRemoteKey(remote); ok {
			return remoteKey, storeKindRemote, remote, nil
		}
	}
	pathKey, err := canonicalPathKey(projectRoot)
	if err != nil {
		return "", "", "", err
	}
	return pathKey, storeKindPath, remote, nil
}

// storeDirName returns the store directory name for a project key:
// <slug>-<hash8>. The slug is the key's last segment, so a remote key names
// its repository, and the hash is the first eight hex characters of the key's
// SHA-256, which keeps two projects that share a slug apart.
func storeDirName(key string) string {
	return slugForKey(key) + "-" + shortKeyHash(key)
}

// slugForKey returns the human-readable part of a store directory name. A key
// with no repository segment is a path digest, which has no name to slug.
func slugForKey(key string) string {
	idx := strings.LastIndex(key, "/")
	if idx < 0 {
		return "project"
	}
	slug := sanitizeSlug(key[idx+1:])
	if slug == "" {
		return "project"
	}
	return slug
}

// sanitizeSlug reduces a repository name to a lowercase alphanumeric slug.
func sanitizeSlug(name string) string {
	name = strings.TrimSuffix(strings.ToLower(name), ".git")
	var built strings.Builder
	built.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			built.WriteRune(r)
		default:
			built.WriteByte('-')
		}
	}
	slug := built.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if len(slug) > storeDirSlugMax {
		slug = strings.Trim(slug[:storeDirSlugMax], "-")
	}
	return slug
}

// shortKeyHash is the eight-character digest that separates store directories
// whose slugs collide.
func shortKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:8]
}

// redactRemote returns a remote spelling that is safe to persist in the store
// registry: userinfo, which may carry a credential, is removed from URL-form
// remotes. Other spellings are returned unchanged, because scp-like syntax has
// no credential field.
func redactRemote(rawRemote string) string {
	remote := strings.TrimSpace(rawRemote)
	schemeEnd := strings.Index(remote, "://")
	if schemeEnd < 0 {
		return remote
	}
	schemeEnd += len("://")
	rest := remote[schemeEnd:]
	if at := strings.Index(rest, "@"); at >= 0 {
		// Strip userinfo only inside the authority, so a path that holds an @
		// survives intact.
		if slash := strings.Index(rest, "/"); slash < 0 || at < slash {
			rest = rest[at+1:]
		}
	}
	return remote[:schemeEnd] + rest
}
