package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	t.Chdir(dir)
	return dir
}

func runApp(t *testing.T, args []string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := newApp()
	app.Writer = &stdout
	app.ErrWriter = &stderr
	app.Reader = strings.NewReader("")
	// Prevent ExitCoder from calling os.Exit so tests can inspect the error.
	app.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	err := app.Run(context.Background(), append([]string{"todo"}, args...))
	return stdout.String(), stderr.String(), err
}

func TestInitCreatesTodoFile(t *testing.T) {
	setupGitRepo(t)

	if _, _, err := runApp(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, err := os.Stat(".todo/todo.md"); err != nil {
		t.Fatalf(".todo/todo.md not created: %v", err)
	}
}

func TestAddAndList(t *testing.T) {
	setupGitRepo(t)

	if _, _, err := runApp(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, _, err := runApp(t, []string{"add", "test task"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "Created TSK-001") {
		t.Errorf("add output = %q, want Created TSK-001", out)
	}

	out, _, err = runApp(t, []string{"list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "test task") {
		t.Errorf("list output = %q, want task summary", out)
	}
}

func TestAddWithoutSummaryErrors(t *testing.T) {
	setupGitRepo(t)

	if _, _, err := runApp(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}

	_, _, err := runApp(t, []string{"add"})
	if err == nil {
		t.Fatal("add with no summary: expected error")
	}
}

func TestListJSON(t *testing.T) {
	setupGitRepo(t)

	if _, _, err := runApp(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runApp(t, []string{"add", "json task"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	out, _, err := runApp(t, []string{"list", "--json"})
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	if !strings.Contains(out, "json task") {
		t.Errorf("json output = %q, want task summary", out)
	}
}
