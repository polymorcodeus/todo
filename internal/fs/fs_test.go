package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFollowingSymlinkRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.md")
	if err := os.WriteFile(path, []byte("note"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := RemoveFollowingSymlink(path); err != nil {
		t.Fatalf("RemoveFollowingSymlink: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}
}

func TestRemoveFollowingSymlinkRemovesTargetAndLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "store.md")
	if err := os.WriteFile(target, []byte("note"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := RemoveFollowingSymlink(link); err != nil {
		t.Fatalf("RemoveFollowingSymlink: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target still exists: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("link still exists: %v", err)
	}
}

func TestRemoveFollowingSymlinkMissing(t *testing.T) {
	if err := RemoveFollowingSymlink(filepath.Join(t.TempDir(), "nope.md")); err != nil {
		t.Fatalf("RemoveFollowingSymlink on missing path: %v", err)
	}
}
