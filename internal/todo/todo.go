// Package todo models and persists the ad-hoc task list in a Markdown file.
package todo

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// now is the clock; tests override it via setNow to make behavior deterministic.
var now = time.Now

func setNow(f func() time.Time) { now = f }

// Status is the on-disk checkbox state of a task.
type Status string

const (
	StatusOpen       Status = " "
	StatusInProgress Status = "o"
	StatusDone       Status = "x"
)

// parseStatus normalizes a checkbox char (accepting uppercase legacy forms)
// into a canonical Status.
func parseStatus(s string) Status {
	switch strings.ToLower(s) {
	case "o":
		return StatusInProgress
	case "x":
		return StatusDone
	default:
		return StatusOpen
	}
}

// Task is a single todo modeled by the fields in its serialized line.
type Task struct {
	ID       string
	Priority string
	Opened   string
	Status   Status
	Summary  string
}

// String serializes a task back into its on-disk line format.
func (t Task) String() string {
	return fmt.Sprintf("- [%s] [%s][priority:%s][opened:%s] %s",
		t.Status, t.ID, t.Priority, t.Opened, t.Summary)
}

type header struct {
	project      string
	repo         string
	lastUpdated  string
	configured   string
	legacySource string
	otherLines   []string
}

var taskRegex = regexp.MustCompile(`^- \[(x|X|o|O|\s+)\] \[(TSK-\d{3})\]\[priority:(high|med|low)\]\[opened:(\d{4}-\d{2}-\d{2})\] (.+)$`)

// parseTask parses a serialized task line. It returns false for lines that do
// not match the task format.
func parseTask(line string) (Task, bool) {
	m := taskRegex.FindStringSubmatch(line)
	if len(m) < 6 {
		return Task{}, false
	}
	return Task{
		ID:       m[2],
		Priority: m[3],
		Opened:   m[4],
		Status:   parseStatus(m[1]),
		Summary:  m[5],
	}, true
}

// parseTaskFields returns the display fields of a task line, using placeholders
// when the line is not a recognized task.
func parseTaskFields(line string) (id, priority, opened, status, summary string) {
	t, ok := parseTask(line)
	if !ok {
		return "-", "-", "-", "-", line
	}
	return t.ID, t.Priority, t.Opened, string(t.Status), t.Summary
}

// AddResult describes a task created by Add.
type AddResult struct {
	ID       string
	Priority string
	NotePath string
}

func Add(todoPath, notesDir, priority, summary string, create bool) (AddResult, error) {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return AddResult{}, fmt.Errorf("read todo file: %w", err)
	}

	nextID := nextTaskID(tasks)

	task := Task{
		ID:       nextID,
		Priority: priority,
		Opened:   now().Format("2006-01-02"),
		Status:   StatusOpen,
		Summary:  summary,
	}

	header.lastUpdated = now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, header, append(tasks, task)); err != nil {
		return AddResult{}, fmt.Errorf("write todo file: %w", err)
	}

	notePath := ""
	if create {
		if err := os.MkdirAll(notesDir, 0755); err != nil {
			return AddResult{}, fmt.Errorf("create notes dir: %w", err)
		}
		notePath = filepath.Join(notesDir, nextID+".md")
		if err := os.WriteFile(notePath, []byte{}, 0644); err != nil {
			return AddResult{}, fmt.Errorf("write note: %w", err)
		}
	}

	return AddResult{ID: nextID, Priority: priority, NotePath: notePath}, nil
}

func Init(todoPath string) error {
	if _, err := os.Stat(todoPath); err == nil {
		return errors.New("todo already initialized - no action taken")
	}

	dir := filepath.Dir(todoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create todo dir: %w", err)
	}

	repo := getRepoRemote()

	current := now()
	h := header{
		project:      filepath.Base(repo),
		repo:         repo,
		configured:   current.Format("2006-01-02"),
		lastUpdated:  current.Format("2006-01-02T15:04"),
		legacySource: "none",
	}

	return writeTodoFile(todoPath, h, nil)
}

func getRepoRemote() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	url = strings.TrimSuffix(url, ".git")
	return url
}

// Pickup marks the referenced task as in progress and returns its updated line
// plus the full path of its companion note, if any.
func Pickup(todoPath, notesDir, ref string) (string, string, error) {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return "", "", fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, ref)
	if err != nil {
		return "", "", err
	}

	tasks[idx].Status = StatusInProgress
	return writeUpdated(todoPath, header, tasks, idx, notesDir)
}

// Complete marks the referenced task done (or removes its line when clear is
// true) and returns the resulting line plus the full path of its companion
// note, if any.
func Complete(todoPath, notesDir, ref string, clear bool) (string, string, error) {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return "", "", fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, ref)
	if err != nil {
		return "", "", err
	}

	note, exists := NotePath(notesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}
	if clear {
		line := tasks[idx].String()
		tasks = append(tasks[:idx], tasks[idx+1:]...)
		if err := writeTodoFile(todoPath, header, tasks); err != nil {
			return "", note, fmt.Errorf("write todo file: %w", err)
		}
		return line, note, nil
	}

	tasks[idx].Status = StatusDone
	return writeUpdated(todoPath, header, tasks, idx, notesDir)
}

// writeUpdated persists tasks after a status change and returns the updated
// line and note for the task at idx.
func writeUpdated(todoPath string, header header, tasks []Task, idx int, notesDir string) (string, string, error) {
	header.lastUpdated = now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, header, tasks); err != nil {
		return "", "", fmt.Errorf("write todo file: %w", err)
	}
	note, exists := NotePath(notesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}
	return tasks[idx].String(), note, nil
}

func renderHeader(h header) string {
	var b strings.Builder

	b.WriteString("---\n")
	if h.project != "" {
		fmt.Fprintf(&b, "project: %s\n", h.project)
	}
	if h.repo != "" {
		fmt.Fprintf(&b, "repo: %s\n", h.repo)
	}
	fmt.Fprintf(&b, "last_updated: %s\n", h.lastUpdated)
	if h.configured != "" {
		fmt.Fprintf(&b, "configured: %s\n", h.configured)
	}
	if h.legacySource != "" {
		fmt.Fprintf(&b, "legacy_source: %s\n", h.legacySource)
	}
	for _, ol := range h.otherLines {
		b.WriteString(ol)
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")

	return b.String()
}

func writeTodoFile(path string, h header, tasks []Task) error {
	var b strings.Builder

	b.WriteString(renderHeader(h))

	for _, t := range tasks {
		b.WriteString(t.String())
		b.WriteString("\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// List returns the tasks in a todo file, in file order.
func List(todoPath string) ([]Task, error) {
	tasks, _, err := parseTodoFile(todoPath)
	if err != nil {
		return nil, fmt.Errorf("read todo file: %w", err)
	}
	return tasks, nil
}

func parseTodoFile(path string) ([]Task, header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, header{}, err
	}
	//nolint:errcheck // read-only file; close error not meaningful here
	defer f.Close()

	var tasks []Task
	var h header
	inHeader := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "---" {
			inHeader = !inHeader
			continue
		}

		if inHeader {
			if after, ok := strings.CutPrefix(line, "project:"); ok {
				h.project = strings.TrimSpace(after)
			} else if after, ok := strings.CutPrefix(line, "repo:"); ok {
				h.repo = strings.TrimSpace(after)
			} else if after0, ok0 := strings.CutPrefix(line, "configured:"); ok0 {
				h.configured = strings.TrimSpace(after0)
			} else if after1, ok1 := strings.CutPrefix(line, "legacy_source:"); ok1 {
				h.legacySource = strings.TrimSpace(after1)
			} else if after2, ok2 := strings.CutPrefix(line, "last_updated:"); ok2 {
				h.lastUpdated = strings.TrimSpace(after2)
			} else {
				h.otherLines = append(h.otherLines, line)
			}
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		if t, ok := parseTask(line); ok {
			tasks = append(tasks, t)
		}
	}

	return tasks, h, scanner.Err()
}

func nextTaskID(tasks []Task) string {
	max := 0
	for _, t := range tasks {
		numStr := strings.TrimPrefix(t.ID, "TSK-")
		n, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("TSK-%03d", max+1)
}

func normalizeTaskRef(ref string) (string, error) {
	s := strings.TrimSpace(ref)
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimPrefix(strings.ToUpper(s), "TSK-")
	if s == "" {
		return "", fmt.Errorf("invalid task reference %q", ref)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 999999 {
		return "", fmt.Errorf("invalid task reference %q: want TSK-NNN, NNN, N, or #N", ref)
	}
	return fmt.Sprintf("TSK-%03d", n), nil
}

func findTaskIndex(tasks []Task, ref string) (int, error) {
	id, err := normalizeTaskRef(ref)
	if err != nil {
		return -1, err
	}
	for i, t := range tasks {
		if t.ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("task %s not found", id)
}

// NotePath returns the full path of a task's companion note in .todo/notes and
// whether it currently exists on disk. It is empty when absent, so callers that
// only need the path can discard the existence flag.
func NotePath(notesDir, id string) (string, bool) {
	path := filepath.Join(notesDir, id+".md")
	if _, err := os.Stat(path); err != nil {
		return path, false
	}
	return path, true
}
