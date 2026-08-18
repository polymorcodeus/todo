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
	"text/tabwriter"
	"time"
)

type header struct {
	project      string
	repo         string
	lastUpdated  string
	configured   string
	legacySource string
	otherLines   []string
}

var taskRegex = regexp.MustCompile(`^- \[(x|X|o|O|\s+)\] \[(TSK-\d{3})\]\[priority:(high|med|low)\]\[opened:(\d{4}-\d{2}-\d{2})\] (.+)$`)

var checkboxRE = regexp.MustCompile(`^-\s*\[([^]]*)\]`)

func Add(todoPath, notesDir, priority, summary string, create bool) error {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return fmt.Errorf("read todo file: %w", err)
	}

	nextID := nextTaskID(tasks)

	opened := time.Now().Format("2006-01-02")
	taskLine := fmt.Sprintf("- [ ] [%s][priority:%s][opened:%s] %s",
		nextID, priority, opened, summary)

	header.lastUpdated = time.Now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, header, append(tasks, taskLine)); err != nil {
		return fmt.Errorf("write todo file: %w", err)
	}

	notePath := ""
	if create {
		if err := os.MkdirAll(notesDir, 0755); err != nil {
			return fmt.Errorf("create notes dir: %w", err)
		}
		notePath = filepath.Join(notesDir, nextID+".md")
		if err := os.WriteFile(notePath, []byte{}, 0644); err != nil {
			return fmt.Errorf("write note: %w", err)
		}
	}

	noteMsg := ""
	if notePath != "" {
		noteMsg = fmt.Sprintf(" + note %s", notePath)
	}
	fmt.Printf("Created %s [priority:%s]%s\n", nextID, priority, noteMsg)
	return nil
}

func Init(todoPath string) error {
	if _, err := os.Stat(todoPath); err == nil {
		return errors.New("todo already initialized - no action taken")
	}

	dir := filepath.Dir(todoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	repo := getRepoRemote()

	now := time.Now()
	h := header{
		project:      filepath.Base(repo),
		repo:         repo,
		configured:   now.Format("2006-01-02"),
		lastUpdated:  now.Format("2006-01-02T15:04"),
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

func Pickup(todoPath, notesDir, ref string) (string, string, error) {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return "", "", fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, ref)
	if err != nil {
		return "", "", err
	}

	line, err := setCheckbox(tasks[idx], "o")
	if err != nil {
		return "", "", err
	}
	tasks[idx] = line

	header.lastUpdated = time.Now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, header, tasks); err != nil {
		return "", "", fmt.Errorf("write todo file: %w", err)
	}

	return line, notePathFor(taskID(line), notesDir), nil
}

func Complete(todoPath, notesDir, ref string, clear bool) (string, string, error) {
	tasks, header, err := parseTodoFile(todoPath)
	if err != nil {
		return "", "", fmt.Errorf("read todo file: %w", err)
	}

	idx, err := findTaskIndex(tasks, ref)
	if err != nil {
		return "", "", err
	}

	line := tasks[idx]
	note := notePathFor(taskID(line), notesDir)

	if clear {
		tasks = append(tasks[:idx], tasks[idx+1:]...)
	} else {
		line, err = setCheckbox(line, "x")
		if err != nil {
			return "", "", err
		}
		tasks[idx] = line
	}

	header.lastUpdated = time.Now().Format("2006-01-02T15:04")
	if err := writeTodoFile(todoPath, header, tasks); err != nil {
		return "", "", fmt.Errorf("write todo file: %w", err)
	}

	return line, note, nil
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

func writeTodoFile(path string, h header, tasks []string) error {
	var b strings.Builder

	b.WriteString(renderHeader(h))

	for _, t := range tasks {
		b.WriteString(t)
		b.WriteString("\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func List(todoPath string) error {
	tasks, _, err := parseTodoFile(todoPath)
	if err != nil {
		return fmt.Errorf("read todo file: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\tID\tPRIORITY\tOPENED\tSUMMARY")
	fmt.Fprintln(w, "\t--\t--------\t------\t-------")

	for _, t := range tasks {
		id, pri, opened, status, sum := parseTaskFields(t)
		fmt.Fprintf(w, "[%s]\t%s\t%s\t%s\t%s\n", status, id, pri, opened, sum)
	}
	return w.Flush()
}

func parseTodoFile(path string) ([]string, header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, header{}, err
	}
	defer f.Close()

	var tasks []string
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

		if taskRegex.MatchString(line) {
			tasks = append(tasks, line)
		}
	}

	return tasks, h, scanner.Err()
}

func parseTaskFields(line string) (id, priority, opened, status, summary string) {
	m := taskRegex.FindStringSubmatch(line)
	if len(m) >= 6 {
		return m[2], m[3], m[4], m[1], m[5]
	}
	return "-", "-", "-", "-", line
}

func nextTaskID(tasks []string) string {
	max := 0
	for _, t := range tasks {
		numStr := strings.TrimPrefix(taskID(t), "TSK-")
		n, _ := strconv.Atoi(numStr)
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("TSK-%03d", max+1)
}

func taskID(line string) string {
	m := taskRegex.FindStringSubmatch(line)
	if len(m) < 3 {
		return ""
	}
	return m[2]
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

func findTaskIndex(tasks []string, ref string) (int, error) {
	id, err := normalizeTaskRef(ref)
	if err != nil {
		return -1, err
	}
	for i, t := range tasks {
		if taskID(t) == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("task %s not found", id)
}

func setCheckbox(line, state string) (string, error) {
	switch state {
	case "o", "x":
	default:
		return "", fmt.Errorf("invalid checkbox state %q", state)
	}
	loc := checkboxRE.FindStringIndex(line)
	if loc == nil {
		return "", fmt.Errorf("no checkbox found in task line")
	}
	return line[:loc[0]] + "- [" + state + "]" + line[loc[1]:], nil
}

func notePathFor(id, notesDir string) string {
	path := filepath.Join(notesDir, id+".md")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return filepath.Base(path)
}
