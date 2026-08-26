// Package fs provides filesystem utilities for the todo CLI.
package fs

import (
	"errors"
	"os"
	"path/filepath"
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

// RemoveFollowingSymlink removes the file at path. When path is a symlink, it
// deletes the resolved target first and then the link itself, so removing a
// symlinked note leaves neither an orphaned target file nor a dangling link.
// A missing path is treated as a successful no-op.
func RemoveFollowingSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return os.Remove(path)
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Dangling or otherwise unresolvable link: drop the link itself.
		return os.Remove(path)
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Remove(path)
}
