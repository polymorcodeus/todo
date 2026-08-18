package todo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var sampleTasks = []Task{
	{ID: "TSK-001", Priority: "high", Opened: "2026-08-01", Status: StatusOpen, Summary: "fix the thing"},
	{ID: "TSK-002", Priority: "med", Opened: "2026-08-02", Status: StatusInProgress, Summary: "refactor parser"},
	{ID: "TSK-003", Priority: "low", Opened: "2026-08-03", Status: StatusDone, Summary: "write docs"},
}

func writeTestTodo(t *testing.T, tasks []Task) (string, string) {
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

func readTasks(t *testing.T, todoPath string) []Task {
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

func TestParseStatus(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Status
	}{
		{" ", StatusOpen},
		{"o", StatusInProgress},
		{"x", StatusDone},
		{"O", StatusInProgress},
		{"X", StatusDone},
		{"unknown", StatusOpen},
	} {
		if got := parseStatus(tc.in); got != tc.want {
			t.Errorf("parseStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTaskRoundTrip(t *testing.T) {
	for _, tt := range sampleTasks {
		line := tt.String()
		got, ok := parseTask(line)
		if !ok {
			t.Fatalf("parseTask(%q): not recognized", line)
		}
		if got != tt {
			t.Errorf("round trip = %+v, want %+v", got, tt)
		}
	}
}

func TestParseTaskFieldsAndNextID(t *testing.T) {
	id, pri, opened, status, sum := parseTaskFields(sampleTasks[0].String())
	if id != "TSK-001" || pri != "high" || opened != "2026-08-01" || status != " " || sum != "fix the thing" {
		t.Errorf("parseTaskFields got %q %q %q %q %q", id, pri, opened, status, sum)
	}
	if got := nextTaskID(sampleTasks); got != "TSK-004" {
		t.Errorf("nextTaskID = %q, want TSK-004", got)
	}
}

func TestAdd(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, nil)

	result, err := Add(todoPath, notesDir, "high", "new task", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "TSK-001" {
		t.Errorf("result.ID = %q, want TSK-001", result.ID)
	}
	if result.Priority != "high" {
		t.Errorf("result.Priority = %q, want high", result.Priority)
	}
	if result.NotePath != "" {
		t.Errorf("result.NotePath = %q, want empty", result.NotePath)
	}

	tasks := readTasks(t, todoPath)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if tasks[0].Opened != "2026-08-18" {
		t.Errorf("opened = %q, want 2026-08-18", tasks[0].Opened)
	}
	if tasks[0].Summary != "new task" {
		t.Errorf("summary = %q, want new task", tasks[0].Summary)
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
	if !strings.HasPrefix(tasks[0].String(), "- [o] [TSK-001]") {
		t.Errorf("task after pickup = %q", tasks[0].String())
	}
	if tasks[1] != sampleTasks[1] {
		t.Errorf("unrelated task changed: %q", tasks[1].String())
	}

	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.lastUpdated == "2026-01-01T00:00" {
		t.Error("last_updated not updated by pickup")
	}
}

func TestNotePath(t *testing.T) {
	dir := t.TempDir()
	notesDir := filepath.Join(dir, ".todo", "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path, exists := NotePath(notesDir, "TSK-001")
	if want := filepath.Join(notesDir, "TSK-001.md"); path != want {
		t.Errorf("NotePath path = %q, want %q", path, want)
	}
	if exists {
		t.Error("NotePath exists = true before note created")
	}

	if err := os.WriteFile(path, []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, exists := NotePath(notesDir, "TSK-001"); !exists {
		t.Error("NotePath exists = false after note created")
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
	if want := filepath.Join(notesDir, "TSK-001.md"); note != want {
		t.Errorf("note = %q, want %q", note, want)
	}
}

func TestPickupNotFound(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if _, _, err := Pickup(todoPath, notesDir, "999"); err == nil {
		t.Error("pickup of missing task: expected error")
	}
}

func TestPickupAlreadyClaimed(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if _, _, err := Pickup(todoPath, notesDir, "2"); err == nil {
		t.Error("pickup of in-progress task: expected error")
	}
	if _, _, err := Pickup(todoPath, notesDir, "3"); err == nil {
		t.Error("pickup of done task: expected error")
	}

	tasks := readTasks(t, todoPath)
	if tasks[1].Status != StatusInProgress || tasks[2].Status != StatusDone {
		t.Error("failed pickup mutated task state")
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
	if !strings.HasPrefix(tasks[1].String(), "- [x] [TSK-002]") {
		t.Errorf("task after complete = %q", tasks[1].String())
	}

	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.lastUpdated == "2026-01-01T00:00" {
		t.Error("last_updated not updated by complete")
	}
}

func TestCompleteNotPickedUp(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if _, _, err := Complete(todoPath, notesDir, "1", false); err == nil {
		t.Error("complete of open task: expected error")
	}
	if _, _, err := Complete(todoPath, notesDir, "3", false); err == nil {
		t.Error("complete of done task: expected error")
	}

	tasks := readTasks(t, todoPath)
	if tasks[0].Status != StatusOpen || tasks[2].Status != StatusDone {
		t.Error("failed complete mutated task state")
	}
}

func TestCompleteClear(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	line, note, err := Complete(todoPath, notesDir, "2", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [o] [TSK-002]") {
		t.Errorf("cleared line = %q, want original TSK-002 line", line)
	}
	if note != "" {
		t.Errorf("note = %q, want empty", note)
	}

	tasks := readTasks(t, todoPath)
	if len(tasks) != 2 {
		t.Fatalf("tasks after clear = %d, want 2", len(tasks))
	}
	if strings.Contains(tasks[0].String(), "TSK-002") || strings.Contains(tasks[1].String(), "TSK-002") {
		t.Errorf("TSK-002 still present after clear: %v", tasks)
	}
}

func TestCompleteParkNote(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "TSK-002.md"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	line, note, err := Complete(todoPath, notesDir, "TSK-002", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [o] [TSK-002]") {
		t.Errorf("cleared line = %q, want original TSK-002 line", line)
	}
	if want := filepath.Join(notesDir, "TSK-002.md"); note != want {
		t.Errorf("note = %q, want %q", note, want)
	}
}
