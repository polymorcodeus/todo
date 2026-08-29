// Package todo models and persists the ad-hoc task list in a Markdown file.
package todo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitlab.com/fuzzyporpoise/todo/internal/fs"
	"gitlab.com/fuzzyporpoise/todo/internal/git"
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

// describeStatus returns a human-readable description of a task's status for
// error messages.
func describeStatus(s Status) string {
	switch s {
	case StatusOpen:
		return "open"
	case StatusInProgress:
		return "in progress"
	case StatusDone:
		return "done"
	default:
		return "unknown"
	}
}

// Description returns a human-readable description of the status.
func (s Status) Description() string { return describeStatus(s) }

// StatusName returns the canonical status designation used in machine-readable
// output: "open", "in progress", or "complete".
func (s Status) StatusName() string {
	switch s {
	case StatusOpen:
		return "open"
	case StatusInProgress:
		return "in progress"
	case StatusDone:
		return "complete"
	default:
		return "unknown"
	}
}

// Priority is the tri-level importance of a task.
type Priority string

const (
	PriorityLow  Priority = "low"
	PriorityMed  Priority = "med"
	PriorityHigh Priority = "high"
)

// parsePriority validates a priority string.
func parsePriority(s string) (Priority, error) {
	switch Priority(s) {
	case PriorityLow, PriorityMed, PriorityHigh:
		return Priority(s), nil
	default:
		return "", fmt.Errorf("invalid priority %q: want low, med, or high", s)
	}
}

// Task is a single todo modeled by the fields in its serialized line.
type Task struct {
	ID       string
	Priority Priority
	Opened   string
	Status   Status
	Summary  string
	Claimed  string // date the task was picked up; empty until pickup
}

// String serializes a task back into its on-disk line format.
func (t Task) String() string {
	claimed := ""
	if t.Claimed != "" {
		claimed = "[claimed:" + t.Claimed + "]"
	}
	return fmt.Sprintf("- [%s] [%s][priority:%s][opened:%s]%s %s",
		t.Status, t.ID, t.Priority, t.Opened, claimed, t.Summary)
}

type header struct {
	project      string
	repo         string
	lastUpdated  string
	configured   string
	legacySource string
	nextID       string // monotonic high-water mark (TSK-NNN); lazy-backfilled on write
	otherLines   []string
}

// taskRegex matches a serialized task line. The claimed field is optional so
// files written before claimed existed still parse.
var taskRegex = regexp.MustCompile(`^- \[(x|X|o|O|\s+)\] \[(TSK-\d{3,})\]\[priority:(high|med|low)\]\[opened:(\d{4}-\d{2}-\d{2})\](?:\[claimed:(\d{4}-\d{2}-\d{2})\])? (.+)$`)

// parseTask parses a serialized task line. It returns false for lines that do
// not match the task format.
func parseTask(line string) (Task, bool) {
	m := taskRegex.FindStringSubmatch(line)
	if len(m) < 6 {
		return Task{}, false
	}
	claimed := ""
	if m[5] != "" {
		claimed = m[5]
	}
	return Task{
		ID:       m[2],
		Priority: Priority(m[3]),
		Opened:   m[4],
		Status:   parseStatus(m[1]),
		Summary:  m[6],
		Claimed:  claimed,
	}, true
}

// AddResult describes a task created by Add.
type AddResult struct {
	ID          string
	Priority    Priority
	Line        string // serialized task line (also the would-be line in dry-run)
	NotePath    string
	NoteContent string
}

// Result describes the outcome of a task mutation: the affected task's
// serialized line and the full path of its companion note, if any.
type Result struct {
	Line string
	Note string
}

// RefOptions configures a single-task mutation (Pickup, Release, Remove).
type RefOptions struct {
	TodoPath string
	NotesDir string
	Ref      string // any form accepted by normalizeTaskRef
}

// CompleteOptions configures Complete. Clear removes the task line instead of
// marking it done.
type CompleteOptions struct {
	TodoPath string
	NotesDir string
	Ref      string
	Clear    bool
}

// AddOptions configures Add.
type AddOptions struct {
	TodoPath    string
	NotesDir    string
	Priority    string
	Summary     string
	CreateNote  bool
	NoteContent string // content to write into the note; empty means empty note
	NoteFile    string // path to an existing file to copy into the note (copy, not move)
	DryRun      bool   // preview what would be written without persisting
}

func Add(opts AddOptions) (AddResult, error) {
	priority, err := parsePriority(opts.Priority)
	if err != nil {
		return AddResult{}, err
	}

	tasks, header, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return AddResult{}, fmt.Errorf("read todo file: %w", err)
	}

	nextID := nextTaskID(tasks, header)

	task := Task{
		ID:       nextID,
		Priority: priority,
		Opened:   now().Format("2006-01-02"),
		Status:   StatusOpen,
		Summary:  opts.Summary,
	}

	result := AddResult{
		ID:       nextID,
		Priority: priority,
		Line:     task.String(),
	}

	// Resolve the note content once up front so dry-run can preview it.
	notePath := ""
	if opts.CreateNote {
		notePath = filepath.Join(opts.NotesDir, nextID+".md")
		result.NotePath = notePath

		content := opts.NoteContent
		if opts.NoteFile != "" {
			data, err := os.ReadFile(opts.NoteFile)
			if err != nil {
				return AddResult{}, fmt.Errorf("read note file: %w", err)
			}
			content = string(data)
		}
		result.NoteContent = content
	}

	if opts.DryRun {
		return result, nil
	}

	header.lastUpdated = now().Format("2006-01-02T15:04")
	if err := writeTodoFile(opts.TodoPath, header, append(tasks, task)); err != nil {
		return AddResult{}, fmt.Errorf("write todo file: %w", err)
	}

	if opts.CreateNote {
		if err := os.MkdirAll(opts.NotesDir, 0o755); err != nil {
			return AddResult{}, fmt.Errorf("create notes dir: %w", err)
		}
		if err := os.WriteFile(notePath, []byte(result.NoteContent), 0o644); err != nil {
			return AddResult{}, fmt.Errorf("write note: %w", err)
		}
	}

	return result, nil
}

func Init(todoPath string) error {
	exists, err := fs.VerifyExists(todoPath)
	if err != nil {
		return fmt.Errorf("stat todo file: %w", err)
	}
	if exists {
		return errors.New("todo already initialized - no action taken")
	}

	dir := filepath.Dir(todoPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create todo dir: %w", err)
	}

	repo := git.RemoteURL("origin")

	current := now()
	h := header{
		project:      filepath.Base(repo),
		repo:         repo,
		configured:   current.Format("2006-01-02"),
		lastUpdated:  current.Format("2006-01-02T15:04"),
		legacySource: "none",
	}

	if err := writeTodoFile(todoPath, h, nil); err != nil {
		return fmt.Errorf("write todo file: %w", err)
	}
	return nil
}

// Pickup marks the referenced task as in progress and returns its updated line
// plus the full path of its companion note, if any.
// Pickup marks the referenced task as in progress and returns its updated line
// plus the full path of its companion note, if any.
// Pickup marks the referenced task as in progress and returns its updated line
// plus the full path of its companion note, if any.
func Pickup(opts RefOptions) (Result, error) {
	tasks, h, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return Result{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return Result{}, err
	}

	if tasks[idx].Status != StatusOpen {
		return Result{}, fmt.Errorf("cannot pickup %s: task is %s", tasks[idx].ID, describeStatus(tasks[idx].Status))
	}

	tasks[idx].Status = StatusInProgress
	tasks[idx].Claimed = now().Format("2006-01-02")
	return writeUpdated(opts.TodoPath, h, tasks, idx, opts.NotesDir)
}

// Complete marks the referenced task done (or removes its line when clear is
// true) and returns the resulting line plus the full path of its companion
// note, if any.
// Complete marks the referenced task done (or removes its line when clear is
// true) and returns the resulting line plus the full path of its companion
// note, if any.
// Complete marks the referenced task done (or removes its line when Clear is
// true) and returns the resulting line plus the full path of its companion
// note, if any.
func Complete(opts CompleteOptions) (Result, error) {
	tasks, h, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return Result{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return Result{}, err
	}

	if tasks[idx].Status != StatusInProgress {
		return Result{}, fmt.Errorf("cannot complete %s: task is %s", tasks[idx].ID, describeStatus(tasks[idx].Status))
	}

	note, exists := NotePath(opts.NotesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}
	if opts.Clear {
		line := tasks[idx].String()
		tasks = append(tasks[:idx], tasks[idx+1:]...)
		if err := writeTodoFile(opts.TodoPath, h, tasks); err != nil {
			return Result{}, fmt.Errorf("write todo file: %w", err)
		}
		return Result{Line: line, Note: note}, nil
	}

	tasks[idx].Status = StatusDone
	tasks[idx].Claimed = ""
	return writeUpdated(opts.TodoPath, h, tasks, idx, opts.NotesDir)
}

// Release returns a picked-up task to open and drops its claimed date. It only
// accepts an in-progress task and returns the resulting line plus the full
// path of its companion note, if any.
// Release returns a picked-up task to open and drops its claimed date. It only
// accepts an in-progress task and returns the resulting line plus the full
// path of its companion note, if any.
// Release returns a picked-up task to open and drops its claimed date. It only
// accepts an in-progress task and returns the resulting line plus the full
// path of its companion note, if any.
func Release(opts RefOptions) (Result, error) {
	tasks, h, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return Result{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return Result{}, err
	}

	if tasks[idx].Status != StatusInProgress {
		return Result{}, fmt.Errorf("cannot release %s: task is %s", tasks[idx].ID, describeStatus(tasks[idx].Status))
	}

	tasks[idx].Status = StatusOpen
	tasks[idx].Claimed = ""
	return writeUpdated(opts.TodoPath, h, tasks, idx, opts.NotesDir)
}

// ReopenResult describes the outcome of Reopen. NoOp is true when the task
// was already open and no write was performed.
type ReopenResult struct {
	Result
	NoOp bool
}

// Reopen restores a completed task to open status. It is a no-op when the task
// is already open and an error when the task is not done (except for the
// already-open no-op case).
func Reopen(opts RefOptions) (ReopenResult, error) {
	tasks, h, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return ReopenResult{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return ReopenResult{}, err
	}

	note, exists := NotePath(opts.NotesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}

	if tasks[idx].Status == StatusOpen {
		return ReopenResult{Result: Result{Line: tasks[idx].String(), Note: note}, NoOp: true}, nil
	}
	if tasks[idx].Status != StatusDone {
		return ReopenResult{}, fmt.Errorf("cannot reopen %s: task is %s", tasks[idx].ID, describeStatus(tasks[idx].Status))
	}

	tasks[idx].Status = StatusOpen
	tasks[idx].Claimed = ""
	res, err := writeUpdated(opts.TodoPath, h, tasks, idx, opts.NotesDir)
	if err != nil {
		return ReopenResult{}, err
	}
	return ReopenResult{Result: res}, nil
}

// Remove deletes the referenced task line regardless of its status and returns
// the removed line (unchanged) plus the full path of its companion note, if any.
// Remove deletes the referenced task line regardless of its status and returns
// the removed line (unchanged) plus the full path of its companion note, if any.
// Remove deletes the referenced task line regardless of its status and returns
// the removed line (unchanged) plus the full path of its companion note, if any.
func Remove(opts RefOptions) (Result, error) {
	tasks, h, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return Result{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return Result{}, err
	}

	note, exists := NotePath(opts.NotesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}

	line := tasks[idx].String()
	tasks = append(tasks[:idx], tasks[idx+1:]...)
	h.lastUpdated = now().Format("2006-01-02T15:04")
	if err := writeTodoFile(opts.TodoPath, h, tasks); err != nil {
		return Result{}, fmt.Errorf("write todo file: %w", err)
	}
	return Result{Line: line, Note: note}, nil
}

// BumpResult describes the outcome of Bump. NoOp is true when the task was
// already at the boundary (high when bumping up, low when bumping down) and
// no write was performed.
type BumpResult struct {
	Result
	NoOp bool
}

// Bump changes a task's priority up or down by one level. Bumping up when
// already high, or down when already low, is a no-op that returns the current
// line without writing.
func Bump(todoPath, notesDir, ref string, down bool) (BumpResult, error) {
	tasks, h, err := parseTodoFile(todoPath)
	if err != nil {
		return BumpResult{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, ref)
	if err != nil {
		return BumpResult{}, err
	}

	note, exists := NotePath(notesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}

	next := bumpPriority(tasks[idx].Priority, down)
	if next == tasks[idx].Priority {
		return BumpResult{Result: Result{Line: tasks[idx].String(), Note: note}, NoOp: true}, nil
	}

	tasks[idx].Priority = next
	res, err := writeUpdated(todoPath, h, tasks, idx, notesDir)
	if err != nil {
		return BumpResult{}, err
	}
	return BumpResult{Result: res}, nil
}

// bumpPriority returns the next priority level: up goes low→med→high→high,
// down goes high→med→low→low.
func bumpPriority(p Priority, down bool) Priority {
	if down {
		switch p {
		case PriorityHigh:
			return PriorityMed
		case PriorityMed:
			return PriorityLow
		default:
			return PriorityLow
		}
	}
	switch p {
	case PriorityLow:
		return PriorityMed
	case PriorityMed:
		return PriorityHigh
	default:
		return PriorityHigh
	}
}

// writeUpdated persists tasks after a status change and returns the updated
// line and note for the task at idx.
// writeUpdated persists tasks after a status change and returns the updated
// line and note for the task at idx.
func writeUpdated(todoPath string, h header, tasks []Task, idx int, notesDir string) (Result, error) {
	h.lastUpdated = now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, h, tasks); err != nil {
		return Result{}, fmt.Errorf("write todo file: %w", err)
	}
	note, exists := NotePath(notesDir, tasks[idx].ID)
	if !exists {
		note = ""
	}
	return Result{Line: tasks[idx].String(), Note: note}, nil
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
	if h.nextID != "" {
		fmt.Fprintf(&b, "next_id: %s\n", h.nextID)
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

	// Lazily backfill recognized header fields before writing so every
	// mutation converges toward a valid, self-describing header.
	normalizeHeader(&h, tasks)

	b.WriteString(renderHeader(h))

	for _, t := range tasks {
		b.WriteString(t.String())
		b.WriteString("\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// normalizeHeader ensures recognized header fields are valid before a write,
// lazily backfilling missing, malformed, or stale values. It never decrements
// next_id: missing/invalid/too-low next_id is rewritten to max(id)+1, while a
// valid high mark is preserved so removed tasks are never re-issued.
func normalizeHeader(h *header, tasks []Task) {
	if n, ok := parseIDNum(h.nextID); !ok || n <= maxTaskNum(tasks) {
		h.nextID = formatID(maxTaskNum(tasks) + 1)
	}
	if h.lastUpdated == "" {
		h.lastUpdated = now().Format("2006-01-02T15:04")
	}
}

// ListFilter filters tasks returned by List. A nil State matches all states;
// StaleDays <= 0 disables stale filtering.
type ListFilter struct {
	State     *Status
	StaleDays int
}

// List returns the tasks in a todo file, in file order, filtered by f.
func List(todoPath string, f ListFilter) ([]Task, error) {
	tasks, _, err := parseTodoFile(todoPath)
	if err != nil {
		return nil, fmt.Errorf("read todo file: %w", err)
	}

	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		age := t.AgeDays()
		if f.State != nil && t.Status != *f.State {
			continue
		}
		if f.StaleDays > 0 && (age < 0 || age < f.StaleDays) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// AgeDays returns the number of full days since the task was claimed, or -1
// when the task has no claimed date (i.e. is not in progress).
func (t Task) AgeDays() int {
	if t.Claimed == "" {
		return -1
	}
	claimed, err := time.Parse("2006-01-02", t.Claimed)
	if err != nil {
		return -1
	}
	now := now().Truncate(24 * time.Hour)
	return int(now.Sub(claimed).Hours() / 24)
}

// OpenedDays returns the number of full days since the task was opened.
func (t Task) OpenedDays() int {
	opened, err := time.Parse("2006-01-02", t.Opened)
	if err != nil {
		return -1
	}
	now := now().Truncate(24 * time.Hour)
	return int(now.Sub(opened).Hours() / 24)
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
			} else if after3, ok3 := strings.CutPrefix(line, "next_id:"); ok3 {
				h.nextID = strings.TrimSpace(after3)
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

// maxTaskNum returns the highest numeric part of a task ID, or 0 when absent.
func maxTaskNum(tasks []Task) int {
	max := 0
	for _, t := range tasks {
		if n, ok := parseIDNum(t.ID); ok && n > max {
			max = n
		}
	}
	return max
}

// parseIDNum parses a TSK-NNN (or bare number) identifier into its numeric
// part. ok is false for empty, malformed, or out-of-range values.
func parseIDNum(s string) (int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToUpper(s), "TSK-")
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 999999 {
		return 0, false
	}
	return n, true
}

// formatID renders a numeric task id as its zero-padded TSK-NNN form.
func formatID(n int) string {
	return fmt.Sprintf("TSK-%03d", n)
}

// nextTaskID returns the ID to issue for the next task: the larger of the
// header's next_id high-water mark (when valid) and max(current ids)+1.
func nextTaskID(tasks []Task, h header) string {
	candidate := maxTaskNum(tasks) + 1
	if n, ok := parseIDNum(h.nextID); ok && n > candidate {
		candidate = n
	}
	return formatID(candidate)
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
// only need the path can discard the existence flag. Stat errors other than
// "not exist" are treated as absent as well, matching the existing callers.
func NotePath(notesDir, id string) (string, bool) {
	path := filepath.Join(notesDir, id+".md")
	if _, err := os.Stat(path); err != nil {
		return path, false
	}
	return path, true
}

// DetailOptions configures Detail.
type DetailOptions struct {
	TodoPath string
	NotesDir string
	Ref      string
	Lines    int  // max note lines to preview; <=0 means unlimited
	NoNote   bool // skip note preview
}

// DetailResult is the read-only result of Detail.
type DetailResult struct {
	Task          Task
	NotePath      string
	NoteExists    bool
	NotePreview   string // first Lines lines of the note, if any
	NoteTruncated bool   // true when more lines exist beyond the preview
}

// Detail returns full information about a single task plus a preview of its
// companion note. It never mutates the todo file or header.
func Detail(opts DetailOptions) (DetailResult, error) {
	tasks, _, err := parseTodoFile(opts.TodoPath)
	if err != nil {
		return DetailResult{}, fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, opts.Ref)
	if err != nil {
		return DetailResult{}, err
	}

	task := tasks[idx]
	notePath, exists := NotePath(opts.NotesDir, task.ID)
	if !exists {
		notePath = filepath.Join(opts.NotesDir, task.ID+".md")
	}

	res := DetailResult{
		Task:       task,
		NotePath:   notePath,
		NoteExists: exists,
	}

	if !opts.NoNote && exists {
		preview, truncated, err := readNotePreview(notePath, opts.Lines)
		if err != nil {
			return DetailResult{}, fmt.Errorf("read note: %w", err)
		}
		res.NotePreview = preview
		res.NoteTruncated = truncated
	}

	return res, nil
}

// readNotePreview reads up to lines from path. A lines value <= 0 means no
// limit. It returns the preview text and whether more content follows.
func readNotePreview(path string, lines int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	//nolint:errcheck // read-only file; close error not meaningful here
	defer f.Close()

	r := bufio.NewReader(f)
	var out []string
	for i := 0; ; i++ {
		if lines > 0 && i >= lines {
			// Read one extra line to detect whether content was truncated.
			_, err := r.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", false, err
			}
			return strings.Join(out, "\n"), true, nil
		}

		line, err := r.ReadString('\n')
		if line != "" {
			out = append(out, strings.TrimSuffix(line, "\n"))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, err
		}
	}
	return strings.Join(out, "\n"), false, nil
}
