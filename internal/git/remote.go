package git

import (
	"net/url"
	"path/filepath"
	"strings"
)

// ParseRemote extracts a host and owner from common git remote URL formats.
// It returns empty strings for local paths or unrecognised URLs.
func ParseRemote(raw string) (host, owner string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	// SSH short form: git@github.com:owner/repo.git
	if before, after, ok := strings.Cut(raw, "@"); ok && strings.HasPrefix(before, "git") {
		if hostPath, _, ok := strings.Cut(after, ":"); ok {
			parts := strings.SplitN(after, ":", 3)
			if len(parts) == 2 {
				host = hostPath
				owner = ownerFromPath(parts[1])
				return host, owner
			}
		}
	}

	// Standard URL form.
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host, ownerFromPath(u.Path)
	}

	// SCP-like form without scheme: github.com:owner/repo.git
	if hostPath, path, ok := strings.Cut(raw, ":"); ok && !filepath.IsAbs(raw) {
		return hostPath, ownerFromPath(path)
	}

	return "", ""
}

func ownerFromPath(p string) string {
	p = strings.TrimSuffix(p, ".git")
	p = strings.Trim(p, "/")
	parts := strings.Split(p, "/")
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return ""
	default:
		return parts[len(parts)-2]
	}
}
