package todo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sampleTasks = []string{
	"- [ ] [TSK-001][priority:high][opened:2026-08-01] fix the thing",
	"- [o] [TSK-002][priority:med][opened:2026-08-02] refactor parser",
	"- [x] [TSK-003][priority:low][opened:2026-08-03] write docs",
}

func writeTestTodo(t *testing.T, tasks []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	todoPath := filepath.Join(dir, ".todo", "todo.md")
	notesDir := filepath.Join(dir, ".todo", "notes")
	h := header{
		project:      "test",
		repo:         "http://example.com/repo",
		lastUpdated:  "2026-01-01T00:00",
		configured:   "2026-01-01",
		legacySource: "none",
	}
	if err := os.MkdirAll(filepath.Dir(todoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTodoFile(todoPath, h, tasks); err != nil {
		t.Fatal(err)
	}
	return todoPath, notesDir
}

func readTasks(t *testing.T, todoPath string) []string {
	t.Helper()
	tasks, _, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

func TestNormalizeTaskRef(t *testing.T) {
	for _, tc := range []struct {
		ref  string
		want string
	}{
		{"TSK-001", "TSK-001"},
		{"tsk-001", "TSK-001"},
		{"001", "TSK-001"},
		{"1", "TSK-001"},
		{"#1", "TSK-001"},
		{"TSK-042", "TSK-042"},
	} {
		got, err := normalizeTaskRef(tc.ref)
		if err != nil {
			t.Fatalf("normalizeTaskRef(%q): %v", tc.ref, err)
		}
		if got != tc.want {
			t.Errorf("normalizeTaskRef(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}

	for _, bad := range []string{"", "abc", "TSK-abc", "-1", "TSK-"} {
		if _, err := normalizeTaskRef(bad); err == nil {
			t.Errorf("normalizeTaskRef(%q): expected error", bad)
		}
	}
}

func TestSetCheckbox(t *testing.T) {
	line := "- [ ] [TSK-001][priority:high][opened:2026-08-01] fix the thing"
	for _, tc := range []struct {
		state string
		want  string
	}{
		{"o", "- [o] [TSK-001][priority:high][opened:2026-08-01] fix the thing"},
		{"x", "- [x] [TSK-001][priority:high][opened:2026-08-01] fix the thing"},
	} {
		got, err := setCheckbox(line, tc.state)
		if err != nil {
			t.Fatalf("setCheckbox(%q): %v", tc.state, err)
		}
		if got != tc.want {
			t.Errorf("setCheckbox(%q) = %q, want %q", tc.state, got, tc.want)
		}
	}

	if _, err := setCheckbox(line, "z"); err == nil {
		t.Error("setCheckbox with invalid state: expected error")
	}
	if _, err := setCheckbox("no checkbox here", "o"); err == nil {
		t.Error("setCheckbox on line without checkbox: expected error")
	}
}

func TestParseTaskFieldsAndNextID(t *testing.T) {
	id, pri, opened, status, sum := parseTaskFields(sampleTasks[0])
	if id != "TSK-001" || pri != "high" || opened != "2026-08-01" || status != " " || sum != "fix the thing" {
		t.Errorf("parseTaskFields got %q %q %q %q %q", id, pri, opened, status, sum)
	}
	if got := nextTaskID(sampleTasks); got != "TSK-004" {
		t.Errorf("nextTaskID = %q, want TSK-004", got)
	}
}

func TestPickup(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	line, note, err := Pickup(todoPath, notesDir, "#1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [o] [TSK-001]") {
		t.Errorf("pickup line = %q, want [o] TSK-001", line)
	}
	if note != "" {
		t.Errorf("note = %q, want empty (no notes dir)", note)
	}

	tasks := readTasks(t, todoPath)
	if !strings.HasPrefix(tasks[0], "- [o] [TSK-001]") {
		t.Errorf("task after pickup = %q", tasks[0])
	}
	if tasks[1] != sampleTasks[1] {
		t.Errorf("unrelated task changed: %q", tasks[1])
	}

	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.lastUpdated == "2026-01-01T00:00" {
		t.Error("last_updated not updated by pickup")
	}
}

func TestPickupWithNote(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "TSK-001.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, note, err := Pickup(todoPath, notesDir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if note != "TSK-001.md" {
		t.Errorf("note = %q, want TSK-001.md", note)
	}
}

func TestPickupNotFound(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if _, _, err := Pickup(todoPath, notesDir, "999"); err == nil {
		t.Error("pickup of missing task: expected error")
	}
}

func TestComplete(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	line, note, err := Complete(todoPath, notesDir, "TSK-002", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [x] [TSK-002]") {
		t.Errorf("complete line = %q, want [x] TSK-002", line)
	}
	if note != "" {
		t.Errorf("note = %q, want empty", note)
	}

	tasks := readTasks(t, todoPath)
	if !strings.HasPrefix(tasks[1], "- [x] [TSK-002]") {
		t.Errorf("task after complete = %q", tasks[1])
	}

	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.lastUpdated == "2026-01-01T00:00" {
		t.Error("last_updated not updated by complete")
	}
}

func TestCompleteClear(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	line, note, err := Complete(todoPath, notesDir, "3", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [x] [TSK-003]") {
		t.Errorf("cleared line = %q, want original TSK-003 line", line)
	}
	if note != "" {
		t.Errorf("note = %q, want empty", note)
	}

	tasks := readTasks(t, todoPath)
	if len(tasks) != 2 {
		t.Fatalf("tasks after clear = %d, want 2", len(tasks))
	}
	if strings.Contains(tasks[0], "TSK-003") || strings.Contains(tasks[1], "TSK-003") {
		t.Errorf("TSK-003 still present after clear: %v", tasks)
	}
}

func TestCompleteParkNote(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "TSK-003.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	line, note, err := Complete(todoPath, notesDir, "TSK-003", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [x] [TSK-003]") {
		t.Errorf("cleared line = %q, want original TSK-003 line", line)
	}
	if note != "TSK-003.md" {
		t.Errorf("note = %q, want TSK-003.md", note)
	}
}
