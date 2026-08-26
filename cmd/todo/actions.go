package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"gitlab.com/fuzzyporpoise/todo/internal/fs"
	"gitlab.com/fuzzyporpoise/todo/internal/todo"
)

// appConfig carries the resolved per-run dependencies for all commands.
type appConfig struct {
	todoPath string
	notesDir string
}

// addOptions carries the flag values for the add command.
type addOptions struct {
	summary     string
	priority    string
	create      bool
	noteContent string
	noteFile    string
	dryRun      bool
}

// listOptions carries the flag values for the list command.
type listOptions struct {
	asJSON bool
	state  string
	stale  int
}

// completeOptions carries the flag values for the complete command.
type completeOptions struct {
	clear bool
	park  bool
}

// removeOptions carries the flag values for the remove command.
type removeOptions struct {
	deleteNote bool
}

func outWriter(cmd *cli.Command) io.Writer {
	if cmd.Root().Writer != nil {
		return cmd.Root().Writer
	}
	return os.Stdout
}

func inReader(cmd *cli.Command) io.Reader {
	if cmd.Root().Reader != nil {
		return cmd.Root().Reader
	}
	return os.Stdin
}

func runAdd(cmd *cli.Command, cfg appConfig, opts addOptions) error {
	if opts.summary == "" {
		opts.summary = strings.TrimSpace(cmd.Args().First())
	}
	if opts.summary == "" {
		return exitError(errors.New("task summary required: provide as first argument or --summary"))
	}

	// Resolve note content: --note-content (or '-') takes precedence, then
	// --note-file copy (handled internally by Add), else read note content
	// from stdin.
	content := opts.noteContent
	if content == "-" || (content == "" && opts.create && opts.noteFile == "") {
		data, err := io.ReadAll(inReader(cmd))
		if err != nil {
			return exitError(fmt.Errorf("read note stdin: %w", err))
		}
		content = string(data)
	}

	result, err := todo.Add(todo.AddOptions{
		TodoPath:    cfg.todoPath,
		NotesDir:    cfg.notesDir,
		Priority:    opts.priority,
		Summary:     opts.summary,
		CreateNote:  opts.create,
		NoteContent: content,
		NoteFile:    opts.noteFile,
		DryRun:      opts.dryRun,
	})
	if err != nil {
		return exitError(err)
	}

	out := outWriter(cmd)
	if opts.dryRun {
		_, _ = fmt.Fprintf(out, "would add:         %s\n", result.Line)
		if result.NotePath != "" {
			_, _ = fmt.Fprintf(out, "would create note: %s\n", result.NotePath)
			if result.NoteContent != "" {
				_, _ = fmt.Fprintln(out, "staged note content:")
				_, _ = fmt.Fprint(out, result.NoteContent)
				if !strings.HasSuffix(result.NoteContent, "\n") {
					_, _ = fmt.Fprintln(out)
				}
			}
		}
		return nil
	}

	noteMsg := ""
	if result.NotePath != "" {
		noteMsg = fmt.Sprintf(" + note %s", result.NotePath)
	}
	_, _ = fmt.Fprintf(out, "Created %s [priority:%s]%s\n", result.ID, result.Priority, noteMsg)
	return nil
}

func runInit(cfg appConfig) error {
	exists, err := fs.VerifyExists(cfg.todoPath)
	if err != nil {
		return exitError(err)
	}
	if exists {
		return nil
	}
	if err := todo.Init(cfg.todoPath); err != nil {
		return exitError(err)
	}
	return nil
}

func runList(cmd *cli.Command, cfg appConfig, opts listOptions) error {
	filter := todo.ListFilter{StaleDays: opts.stale}
	if opts.state != "" {
		s := parseState(opts.state)
		filter.State = &s
	}

	tasks, err := todo.List(cfg.todoPath, filter)
	if err != nil {
		return exitError(err)
	}

	out := outWriter(cmd)
	if opts.asJSON {
		return writeJSON(out, tasks)
	}

	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(out, "No tasks found.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	// tabwriter write errors surface on Flush below.
	_, _ = fmt.Fprintln(w, "\tID\tPRIORITY\tOPENED\tCLAIMED\tAGE\tSUMMARY")
	_, _ = fmt.Fprintln(w, "\t--\t--------\t------\t-------\t---\t-------")
	for _, t := range tasks {
		age := "-"
		if t.AgeDays() >= 0 {
			age = fmt.Sprintf("%d day/s", t.AgeDays())
		}
		_, _ = fmt.Fprintf(w, "[%s]\t%s\t%s\t%s\t%s\t%s\t%s\n",
			t.Status, t.ID, t.Priority, t.Opened, dashIfEmpty(t.Claimed), age, t.Summary)
	}
	if err := w.Flush(); err != nil {
		return exitError(err)
	}
	return nil
}

func runPickup(cmd *cli.Command, cfg appConfig) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}
	res, err := todo.Pickup(todo.RefOptions{TodoPath: cfg.todoPath, NotesDir: cfg.notesDir, Ref: ref})
	if err != nil {
		return exitError(err)
	}
	printTaskLine(outWriter(cmd), res.Line, res.Note)
	return nil
}

// detailOptions carries the flag values for the detail command.
type detailOptions struct {
	lines  int
	noNote bool
	asJSON bool
}

// jsonDetail is the machine-readable representation of a task detail.
type jsonDetail struct {
	ID                   string `json:"id"`
	Status               string `json:"status"`
	StatusSymbol         string `json:"status_symbol"`
	Priority             string `json:"priority"`
	Opened               string `json:"opened"`
	OpenedDays           int    `json:"opened_days"`
	Claimed              string `json:"claimed,omitempty"`
	AgeDays              *int   `json:"age_days,omitempty"`
	Summary              string `json:"summary"`
	NotePath             string `json:"note_path"`
	NoteExists           bool   `json:"note_exists"`
	NotePreview          string `json:"note_preview,omitempty"`
	NotePreviewTruncated bool   `json:"note_preview_truncated,omitempty"`
}

func runDetail(cmd *cli.Command, cfg appConfig, opts detailOptions) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}

	lines := opts.lines
	if lines <= 0 {
		lines = 20
	}

	res, err := todo.Detail(todo.DetailOptions{
		TodoPath: cfg.todoPath,
		NotesDir: cfg.notesDir,
		Ref:      ref,
		Lines:    lines,
		NoNote:   opts.noNote,
	})
	if err != nil {
		return exitError(err)
	}

	out := outWriter(cmd)
	if opts.asJSON {
		jd := jsonDetail{
			ID:           res.Task.ID,
			Status:       res.Task.Status.StatusName(),
			StatusSymbol: string(res.Task.Status),
			Priority:     string(res.Task.Priority),
			Opened:       res.Task.Opened,
			OpenedDays:   res.Task.OpenedDays(),
			Claimed:      res.Task.Claimed,
			Summary:      res.Task.Summary,
			NotePath:     res.NotePath,
			NoteExists:   res.NoteExists,
			NotePreview:  res.NotePreview,
		}
		if res.NotePreview != "" {
			jd.NotePreviewTruncated = res.NoteTruncated
		}
		if age := res.Task.AgeDays(); age >= 0 {
			jd.AgeDays = &age
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(jd)
	}

	claimed := "-"
	if res.Task.Claimed != "" {
		if age := res.Task.AgeDays(); age >= 0 {
			claimed = fmt.Sprintf("%s (%d day/s)", res.Task.Claimed, age)
		} else {
			claimed = res.Task.Claimed
		}
	}

	_, _ = fmt.Fprintf(out, "%s (priority: %s)\n", res.Task.ID, res.Task.Priority)
	_, _ = fmt.Fprintf(out, "status: %s | opened: %s (%d day/s) | claimed: %s\n",
		res.Task.Status.Description(), res.Task.Opened, res.Task.OpenedDays(), claimed)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "summary:")
	_, _ = fmt.Fprintln(out, res.Task.Summary)
	_, _ = fmt.Fprintln(out)

	if res.NoteExists {
		_, _ = fmt.Fprintf(out, "note: %s\n", res.NotePath)
		if res.NotePreview != "" {
			for i, line := range strings.Split(res.NotePreview, "\n") {
				_, _ = fmt.Fprintf(out, "  %2d | %s\n", i+1, line)
			}
			if res.NoteTruncated {
				_, _ = fmt.Fprintln(out, "       ...")
			}
		}
	} else {
		_, _ = fmt.Fprintln(out, "note: none")
	}

	return nil
}

func runRelease(cmd *cli.Command, cfg appConfig) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}
	res, err := todo.Release(todo.RefOptions{TodoPath: cfg.todoPath, NotesDir: cfg.notesDir, Ref: ref})
	if err != nil {
		return exitError(err)
	}
	printTaskLine(outWriter(cmd), res.Line, res.Note)
	return nil
}

func runReopen(cmd *cli.Command, cfg appConfig) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}
	res, err := todo.Reopen(todo.RefOptions{TodoPath: cfg.todoPath, NotesDir: cfg.notesDir, Ref: ref})
	if err != nil {
		return exitError(err)
	}
	out := outWriter(cmd)
	if res.NoOp {
		_, _ = fmt.Fprintf(out, "%s is already open\n", res.Line)
		return nil
	}
	printTaskLine(out, res.Line, res.Note)
	return nil
}

func runRemove(cmd *cli.Command, cfg appConfig, opts removeOptions) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}
	res, err := todo.Remove(todo.RefOptions{TodoPath: cfg.todoPath, NotesDir: cfg.notesDir, Ref: ref})
	if err != nil {
		return exitError(err)
	}
	if opts.deleteNote && res.Note != "" {
		if err := fs.RemoveFollowingSymlink(res.Note); err != nil {
			return exitError(fmt.Errorf("delete note: %w", err))
		}
	}
	printTaskLine(outWriter(cmd), res.Line, res.Note)
	return nil
}

func runComplete(cmd *cli.Command, cfg appConfig, opts completeOptions) error {
	ref, err := requireTaskRef(cmd)
	if err != nil {
		return exitError(err)
	}
	res, err := todo.Complete(todo.CompleteOptions{TodoPath: cfg.todoPath, NotesDir: cfg.notesDir, Ref: ref, Clear: opts.clear})
	if err != nil {
		return exitError(err)
	}
	out := outWriter(cmd)
	_, _ = fmt.Fprintln(out, res.Line)
	if opts.park {
		if res.Note != "" {
			_, _ = fmt.Fprintln(out, "note:", res.Note)
		} else {
			_, _ = fmt.Fprintf(out, "no note to park for %s\n", ref)
		}
	}
	return nil
}

// requireTaskRef validates and returns the first positional argument as a task
// reference.
func requireTaskRef(cmd *cli.Command) (string, error) {
	ref := strings.TrimSpace(cmd.Args().First())
	if ref == "" {
		return "", errors.New("task number required: provide a task reference such as TSK-001")
	}
	return ref, nil
}

// printTaskLine prints the updated task line and, when present, its companion
// note path.
func printTaskLine(out io.Writer, line, note string) {
	_, _ = fmt.Fprintln(out, line)
	if note != "" {
		_, _ = fmt.Fprintln(out, "note:", note)
	}
}

// parseState maps the --state flag value to a task Status filter.
func parseState(s string) todo.Status {
	switch s {
	case "progress":
		return todo.StatusInProgress
	case "done":
		return todo.StatusDone
	default:
		return todo.StatusOpen
	}
}

// dashIfEmpty returns "-" for empty strings, used to keep columns aligned.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// jsonTask is the machine-readable representation of a task for --json.
type jsonTask struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	StatusSymbol string `json:"status_symbol"`
	Priority     string `json:"priority"`
	Opened       string `json:"opened"`
	Claimed      string `json:"claimed,omitempty"`
	AgeDays      *int   `json:"age_days,omitempty"`
	Summary      string `json:"summary"`
}

// writeJSON emits tasks as a JSON array of stable, documented fields.
func writeJSON(out io.Writer, tasks []todo.Task) error {
	outTasks := make([]jsonTask, 0, len(tasks))
	for _, t := range tasks {
		jt := jsonTask{
			ID:           t.ID,
			Status:       t.Status.StatusName(),
			StatusSymbol: string(t.Status),
			Priority:     string(t.Priority),
			Opened:       t.Opened,
			Claimed:      t.Claimed,
			Summary:      t.Summary,
		}
		if age := t.AgeDays(); age >= 0 {
			jt.AgeDays = &age
		}
		outTasks = append(outTasks, jt)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(outTasks)
}
