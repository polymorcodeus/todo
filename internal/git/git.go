// Package git shells out to the git CLI for repo metadata.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// RepoRoot resolves the root of the current git repo.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git repo root resolved to empty string")
	}
	return root, nil
}

// RemoteURL returns the URL of the named remote with any trailing .git
// stripped, or "" when the remote is unavailable (optional metadata).
func RemoteURL(remote string) string {
	out, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
}

// RemoteURLAt returns the URL of the named remote in dir, or "" when the
// remote is unavailable.
func RemoteURLAt(dir, remote string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", remote).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(string(out)), ".git")
}
