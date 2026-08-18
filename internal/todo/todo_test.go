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
	if got := nextTaskID(sampleTasks, header{}); got != "TSK-004" {
		t.Errorf("nextTaskID = %q, want TSK-004", got)
	}
	if got := nextTaskID(sampleTasks, header{nextID: "TSK-010"}); got != "TSK-010" {
		t.Errorf("nextTaskID with high next_id = %q, want TSK-010", got)
	}
	if got := nextTaskID(nil, header{nextID: "TSK-004"}); got != "TSK-004" {
		t.Errorf("nextTaskID empty-with-next = %q, want TSK-004", got)
	}
	if got := nextTaskID(nil, header{nextID: "garbage"}); got != "TSK-001" {
		t.Errorf("nextTaskID malformed = %q, want TSK-001", got)
	}
}

func TestAdd(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, nil)

	result, err := Add(AddOptions{
		TodoPath: todoPath,
		NotesDir: notesDir,
		Priority: "high",
		Summary:  "new task",
	})
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
	claimed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return claimed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	line, note, err := Pickup(todoPath, notesDir, "#1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [o] [TSK-001]") {
		t.Errorf("pickup line = %q, want [o] TSK-001", line)
	}
	if !strings.Contains(line, "[claimed:2026-08-18]") {
		t.Errorf("pickup line = %q, want claimed date", line)
	}
	if note != "" {
		t.Errorf("note = %q, want empty (no notes dir)", note)
	}

	tasks := readTasks(t, todoPath)
	if !strings.HasPrefix(tasks[0].String(), "- [o] [TSK-001]") {
		t.Errorf("task after pickup = %q", tasks[0].String())
	}
	if tasks[0].Claimed != "2026-08-18" {
		t.Errorf("task claimed = %q, want 2026-08-18", tasks[0].Claimed)
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

func TestCompleteDropsClaimed(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	tasks := readTasks(t, todoPath)
	tasks[1].Claimed = "2026-08-18"
	_ = writeTodoFile(todoPath, header{project: "test", repo: "http://example.com/repo", lastUpdated: "2026-01-01T00:00", configured: "2026-01-01", legacySource: "none"}, tasks)

	line, _, err := Complete(todoPath, notesDir, "TSK-002", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "claimed") {
		t.Errorf("completed line still has claimed: %q", line)
	}
	after := readTasks(t, todoPath)
	if after[1].Claimed != "" {
		t.Errorf("claimed not dropped on complete: %q", after[1].String())
	}
}

func TestAddDryRun(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, nil)

	result, err := Add(AddOptions{
		TodoPath:    todoPath,
		NotesDir:    notesDir,
		Priority:    "med",
		Summary:     "x",
		CreateNote:  true,
		NoteContent: "note body",
		DryRun:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "TSK-001" {
		t.Errorf("dry run ID = %q, want TSK-001", result.ID)
	}
	if want := "- [ ] [TSK-001][priority:med][opened:2026-08-18] x"; result.Line != want {
		t.Errorf("dry run line = %q, want %q", result.Line, want)
	}
	if want := filepath.Join(notesDir, "TSK-001.md"); result.NotePath != want {
		t.Errorf("dry run note path = %q, want %q", result.NotePath, want)
	}
	if result.NoteContent != "note body" {
		t.Errorf("dry run note content = %q, want %q", result.NoteContent, "note body")
	}

	// Nothing persisted: no todo file change and no note file on disk.
	if _, err := os.Stat(filepath.Join(notesDir, "TSK-001.md")); !os.IsNotExist(err) {
		t.Errorf("dry run wrote a note file: %v", err)
	}
	if len(readTasks(t, todoPath)) != 0 {
		t.Error("dry run wrote a task")
	}
}

func TestAddNoteContent(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, nil)

	result, err := Add(AddOptions{
		TodoPath:    todoPath,
		NotesDir:    notesDir,
		Priority:    "med",
		Summary:     "with note",
		CreateNote:  true,
		NoteContent: "hello\nworld",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(notesDir, "TSK-001.md"); result.NotePath != want {
		t.Errorf("note path = %q, want %q", result.NotePath, want)
	}
	data, err := os.ReadFile(filepath.Join(notesDir, "TSK-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\nworld" {
		t.Errorf("note content = %q, want %q", string(data), "hello\nworld")
	}
}

func TestAddNoteFileCopy(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, nil)
	src := filepath.Join(t.TempDir(), "src.md")
	if err := os.WriteFile(src, []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Add(AddOptions{
		TodoPath:   todoPath,
		NotesDir:   notesDir,
		Priority:   "med",
		Summary:    "copy note",
		CreateNote: true,
		NoteFile:   src,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(notesDir, "TSK-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from file" {
		t.Errorf("note content = %q, want %q", string(data), "from file")
	}
	// Copy, not move: source still present.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file was moved: %v", err)
	}
}

func TestListFilterState(t *testing.T) {
	todoPath, _ := writeTestTodo(t, sampleTasks)

	open := StatusOpen
	tasks, err := List(todoPath, ListFilter{State: &open})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "TSK-001" {
		t.Errorf("open filter = %+v, want only TSK-001", tasks)
	}

	// No filter returns everything in file order.
	all, err := List(todoPath, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("unfiltered = %d, want 3", len(all))
	}
}

func TestListFilterStale(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	staleTask := Task{
		ID: "TSK-010", Priority: "med", Opened: "2026-08-01",
		Status: StatusInProgress, Summary: "old", Claimed: "2026-08-10", // 8 days ago
	}
	freshTask := Task{
		ID: "TSK-011", Priority: "med", Opened: "2026-08-01",
		Status: StatusInProgress, Summary: "fresh", Claimed: "2026-08-17", // 1 day ago
	}
	todoPath, _ := writeTestTodo(t, []Task{staleTask, freshTask})

	// stale threshold that includes the 8-day task only.
	tasks, err := List(todoPath, ListFilter{StaleDays: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "TSK-010" {
		t.Errorf("stale filter = %+v, want only TSK-010", tasks)
	}
}

func TestAgeDays(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	if got := (Task{}).AgeDays(); got != -1 {
		t.Errorf("unclaimed AgeDays = %d, want -1", got)
	}
	if got := (Task{Claimed: "2026-08-10"}).AgeDays(); got != 8 {
		t.Errorf("AgeDays = %d, want 8", got)
	}
	if got := (Task{Claimed: "2026-08-18"}).AgeDays(); got != 0 {
		t.Errorf("AgeDays today = %d, want 0", got)
	}
}

func TestRoundTripWithClaimed(t *testing.T) {
	tt := Task{
		ID: "TSK-001", Priority: "high", Opened: "2026-08-01",
		Status: StatusInProgress, Summary: "claimed task", Claimed: "2026-08-10",
	}
	got, ok := parseTask(tt.String())
	if !ok {
		t.Fatalf("parseTask(%q): not recognized", tt.String())
	}
	if got != tt {
		t.Errorf("round trip = %+v, want %+v", got, tt)
	}
}

func TestRelease(t *testing.T) {
	claimed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return claimed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	// Give TSK-002 an in-progress claim to release.
	tasks := readTasks(t, todoPath)
	tasks[1].Claimed = "2026-08-10"
	_ = writeTodoFile(todoPath, header{project: "test", repo: "http://example.com/repo", lastUpdated: "2026-01-01T00:00", configured: "2026-01-01", legacySource: "none"}, tasks)

	line, _, err := Release(todoPath, notesDir, "TSK-002")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [ ] [TSK-002]") {
		t.Errorf("release line = %q, want open [ ] TSK-002", line)
	}
	if strings.Contains(line, "claimed") {
		t.Errorf("release line still has claimed: %q", line)
	}

	after := readTasks(t, todoPath)
	if after[1].Status != StatusOpen || after[1].Claimed != "" {
		t.Errorf("task after release = %q, want open with no claim", after[1].String())
	}
}

func TestReleaseInvalid(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	// Open and done tasks cannot be released.
	if _, _, err := Release(todoPath, notesDir, "1"); err == nil {
		t.Error("release of open task: expected error")
	}
	if _, _, err := Release(todoPath, notesDir, "3"); err == nil {
		t.Error("release of done task: expected error")
	}
	tasks := readTasks(t, todoPath)
	if tasks[0].Status != StatusOpen || tasks[2].Status != StatusDone {
		t.Error("failed release mutated state")
	}
}

func TestRemove(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	// Remove a done task line - the [x] -> removed path.
	line, _, err := Remove(todoPath, notesDir, "TSK-003")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "- [x] [TSK-003]") {
		t.Errorf("removed line = %q, want original TSK-003 line", line)
	}

	tasks := readTasks(t, todoPath)
	if len(tasks) != 2 {
		t.Fatalf("tasks after remove = %d, want 2", len(tasks))
	}
	for _, tt := range tasks {
		if tt.ID == "TSK-003" {
			t.Errorf("TSK-003 still present after remove: %v", tasks)
		}
	}
}

func TestRemoveAnyStatus(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	// Remove an open and an in-progress task; all statuses should work.
	for _, ref := range []string{"TSK-001", "TSK-002"} {
		if _, _, err := Remove(todoPath, notesDir, ref); err != nil {
			t.Fatalf("remove %s: %v", ref, err)
		}
	}
	tasks := readTasks(t, todoPath)
	if len(tasks) != 1 || tasks[0].ID != "TSK-003" {
		t.Errorf("after multi-remove = %+v, want only TSK-003", tasks)
	}
}

func TestRemoveNotFound(t *testing.T) {
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	if _, _, err := Remove(todoPath, notesDir, "999"); err == nil {
		t.Error("remove of missing task: expected error")
	}
}

func TestAddBackfillsNextIDOnOldFile(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	// Write a legacy file with no next_id field and three tasks.
	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	h := &header{}
	h.project = "test"
	h.repo = "http://example.com/repo"
	h.configured = "2026-01-01"
	h.lastUpdated = "2026-01-01T00:00"
	h.legacySource = "none"
	_ = writeTodoFile(todoPath, *h, sampleTasks)

	result, err := Add(AddOptions{TodoPath: todoPath, NotesDir: notesDir, Priority: "med", Summary: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "TSK-004" {
		t.Errorf("first add after migration = %q, want TSK-004", result.ID)
	}

	// Header should now carry next_id: TSK-005.
	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.nextID != "TSK-005" {
		t.Errorf("next_id after add = %q, want TSK-005", header.nextID)
	}
}

func TestAddResumesAfterEmptying(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, sampleTasks)
	// Backfill a next_id high-water mark (as a prior add would have written).
	h := &header{}
	h.project = "test"
	h.repo = "http://example.com/repo"
	h.configured = "2026-01-01"
	h.lastUpdated = "2026-01-01T00:00"
	h.legacySource = "none"
	h.nextID = "TSK-004"
	_ = writeTodoFile(todoPath, *h, sampleTasks)

	// Remove every task.
	for _, ref := range []string{"TSK-001", "TSK-002", "TSK-003"} {
		if _, _, err := Remove(todoPath, notesDir, ref); err != nil {
			t.Fatal(err)
		}
	}

	// next_id must NOT be decremented by removals.
	_, header, err := parseTodoFile(todoPath)
	if err != nil {
		t.Fatal(err)
	}
	if header.nextID != "TSK-004" {
		t.Errorf("next_id after removals = %q, want TSK-004 preserved", header.nextID)
	}

	result, err := Add(AddOptions{TodoPath: todoPath, NotesDir: notesDir, Priority: "med", Summary: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "TSK-004" {
		t.Errorf("add after emptying = %q, want TSK-004 (resume high-water mark)", result.ID)
	}
}

func TestBackfillInvalidNextID(t *testing.T) {
	fixed := time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC)
	setNow(func() time.Time { return fixed })
	defer setNow(time.Now)

	todoPath, notesDir := writeTestTodo(t, sampleTasks)

	// Malformed next_id is treated as absent and backfilled on the next write.
	h := &header{}
	h.project = "test"
	h.repo = "http://example.com/repo"
	h.configured = "2026-01-01"
	h.lastUpdated = "2026-01-01T00:00"
	h.legacySource = "none"
	h.nextID = "not-a-task"
	_ = writeTodoFile(todoPath, *h, sampleTasks)

	result, err := Add(AddOptions{TodoPath: todoPath, NotesDir: notesDir, Priority: "med", Summary: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "TSK-004" {
		t.Errorf("add with malformed next_id = %q, want TSK-004", result.ID)
	}

	// A next_id <= max(current) is likewise backfilled upward.
	h2 := &header{}
	h2.project = "test"
	h2.repo = "http://example.com/repo"
	h2.configured = "2026-01-01"
	h2.lastUpdated = "2026-01-01T00:00"
	h2.legacySource = "none"
	h2.nextID = "TSK-002" // stale: below current max of 3
	_ = writeTodoFile(todoPath, *h2, sampleTasks)

	r2, err := Add(AddOptions{TodoPath: todoPath, NotesDir: notesDir, Priority: "med", Summary: "y"})
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != "TSK-004" {
		t.Errorf("add with stale next_id = %q, want TSK-004", r2.ID)
	}
}

func TestNormalizeHeaderPreservesHighMark(t *testing.T) {
	tasks := []Task{
		{ID: "TSK-001", Status: StatusOpen},
	}
	var h header
	h.nextID = "TSK-050" // valid high mark well above current max
	normalizeHeader(&h, tasks)
	if h.nextID != "TSK-050" {
		t.Errorf("normalizeHeader lowered high mark = %q, want TSK-050", h.nextID)
	}

	h2 := header{nextID: "TSK-002"} // stale: <= max (3) -> backfilled
	highTasks := []Task{
		{ID: "TSK-001", Status: StatusOpen},
		{ID: "TSK-002", Status: StatusOpen},
		{ID: "TSK-003", Status: StatusOpen},
	}
	normalizeHeader(&h2, highTasks)
	if h2.nextID != "TSK-004" {
		t.Errorf("normalizeHeader stale next_id = %q, want TSK-004", h2.nextID)
	}
}
