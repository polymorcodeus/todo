package cmd

import (
	"bytes"
	"context"
	"encoding/json"
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

	var tasks []jsonTask
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		t.Fatalf("parse list json: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	task := tasks[0]
	if task.ID != "TSK-001" {
		t.Errorf("id = %q, want TSK-001", task.ID)
	}
	if task.Status != "open" {
		t.Errorf("status = %q, want open", task.Status)
	}
	if task.StatusSymbol != " " {
		t.Errorf("status_symbol = %q, want space", task.StatusSymbol)
	}
	if task.Summary != "json task" {
		t.Errorf("summary = %q, want json task", task.Summary)
	}
}

func TestDetailJSON(t *testing.T) {
	setupGitRepo(t)

	if _, _, err := runApp(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, _, err := runApp(t, []string{"add", "detail task"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, _, err := runApp(t, []string{"pickup", "TSK-001"}); err != nil {
		t.Fatalf("pickup: %v", err)
	}

	out, _, err := runApp(t, []string{"detail", "TSK-001", "--json"})
	if err != nil {
		t.Fatalf("detail --json: %v", err)
	}

	var detail jsonDetail
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		t.Fatalf("parse detail json: %v", err)
	}
	if detail.ID != "TSK-001" {
		t.Errorf("id = %q, want TSK-001", detail.ID)
	}
	if detail.Status != "in progress" {
		t.Errorf("status = %q, want in progress", detail.Status)
	}
	if detail.StatusSymbol != "o" {
		t.Errorf("status_symbol = %q, want o", detail.StatusSymbol)
	}
	if detail.Summary != "detail task" {
		t.Errorf("summary = %q, want detail task", detail.Summary)
	}
}
