// Package fs provides filesystem utilities for the todo CLI.
package fs

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// VerifyExists reports whether a path exists and any non-not-exist error.
func VerifyExists(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if err == nil {
		return true, nil // path exists
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil // path does not exist
	}
	return false, err // remaining errors
}

// FindGitRepoRoot resolves the root of the current git repo.
func FindGitRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("git root resolved to empty string")
	}
	return root, nil
}
