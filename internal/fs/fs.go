// Package fs provides filesystem utilities for the todo CLI.
package fs

import (
	"errors"
	"os"
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
