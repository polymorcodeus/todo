// Package cmd defines the todo CLI command tree and wiring.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	validation "github.com/urfave/cli-validation"
	"github.com/urfave/cli/v3"

	"gitlab.com/fuzzyporpoise/todo/internal/fs"
	"gitlab.com/fuzzyporpoise/todo/internal/todo"
)

var (
	version   = "0.0.0"
	buildTime string
)

// SetVersion wires the build-time version into the command tree.
func SetVersion(v string) { version = v }

// SetBuildTime wires the build timestamp into the command tree.
func SetBuildTime(bt string) { buildTime = bt }

func buildVersion() string {
	v := version
	if buildTime != "" {
		v += " (" + buildTime + ")"
	}
	return v
}

// exitError returns a plain, uncolored error for non-interactive use.
func exitError(err error) error {
	return cli.Exit(err.Error(), 1)
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
	ID       string `json:"id"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Opened   string `json:"opened"`
	Claimed  string `json:"claimed,omitempty"`
	AgeDays  *int   `json:"age_days,omitempty"`
	Summary  string `json:"summary"`
}

// writeJSON emits tasks as a JSON array of stable, documented fields.
func writeJSON(tasks []todo.Task) error {
	out := make([]jsonTask, 0, len(tasks))
	for _, t := range tasks {
		jt := jsonTask{
			ID:       t.ID,
			Status:   string(t.Status),
			Priority: t.Priority,
			Opened:   t.Opened,
			Claimed:  t.Claimed,
			Summary:  t.Summary,
		}
		if age := t.AgeDays(); age >= 0 {
			jt.AgeDays = &age
		}
		out = append(out, jt)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Run runs the root command.
func Run() {
	var (
		repoRoot           string
		todoPath, notesDir string
	)

	app := &cli.Command{
		Name:                  "todo",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Usage:                 "manage ad-hoc tasks in .todo/todo.md",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			var err error
			repoRoot, err = fs.FindGitRepoRoot()
			if err != nil {
				return ctx, exitError(err)
			}

			todoPath = filepath.Join(repoRoot, ".todo", "todo.md")
			notesDir = filepath.Join(repoRoot, ".todo", "notes")

			// force explicit init if missing
			if sub := cmd.Args().First(); sub != "init" {
				if _, err := os.Stat(todoPath); err != nil {
					return ctx, exitError(errors.New("repo not todo initialized - run `todo init`"))
				}
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			func() *cli.Command {
				var (
					summary     string
					priority    string
					create      bool
					noteContent string
					noteFile    string
					dryRun      bool
				)
				noteContentFlag := &cli.StringFlag{
					Name:        "note-content",
					Destination: &noteContent,
					Usage:       "content to write into the note; use '-' to read from stdin",
				}
				noteFileFlag := &cli.StringFlag{
					Name:        "note-file",
					Destination: &noteFile,
					Usage:       "copy an existing file into the note (copy, not move)",
				}
				return &cli.Command{
					Name:      "add",
					Usage:     "Add a new todo to the todo list",
					ArgsUsage: "[summary]",
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:        "priority",
							Aliases:     []string{"p"},
							Value:       "med",
							Destination: &priority,
							Usage:       "priority: low, med, high",
							Validator:   validation.Enum("low", "med", "high"),
						},
						&cli.StringFlag{
							Name:        "summary",
							Aliases:     []string{"s"},
							Destination: &summary,
							Usage:       "summary text (alternative to positional arg)",
						},
						&cli.BoolFlag{
							Name:        "create-note",
							Aliases:     []string{"n"},
							Destination: &create,
							Usage:       "create a companion note file (reads content from stdin unless --note-content/--note-file given)",
						},
						noteContentFlag,
						noteFileFlag,
						&cli.BoolFlag{
							Name:        "dry-run",
							Destination: &dryRun,
							Usage:       "preview the would-be task line and note without writing",
						},
					},
					MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
						{
							Category: "note source",
							// note-content and note-file are alternative note
							// sources; each in its own path so at most one may
							// be set.
							Flags: [][]cli.Flag{{noteContentFlag}, {noteFileFlag}},
						},
					},
					Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
						if summary == "" {
							summary = strings.TrimSpace(cmd.Args().First())
						}
						if summary == "" {
							return ctx, exitError(errors.New("task summary required: provide as first argument or --summary"))
						}
						// Since MutualExclusiveFlags matches by the flag's declared
						// Name, ensure they're wired to the same bool/state by
						// reading them back from the command (they aren't bound
						// to vars in the exclusive set).
						_ = cmd
						return ctx, nil
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						// Resolve note content: --note-content (or '-') takes
						// precedence, then --note-file copy (handled internally
						// by Add), else read note content from stdin.
						content := cmd.String("note-content")
						if content == "-" || (content == "" && create && cmd.String("note-file") == "") {
							data, err := io.ReadAll(os.Stdin)
							if err != nil {
								return exitError(fmt.Errorf("read note stdin: %w", err))
							}
							content = string(data)
						}

						result, err := todo.Add(todo.AddOptions{
							TodoPath:    todoPath,
							NotesDir:    notesDir,
							Priority:    priority,
							Summary:     summary,
							CreateNote:  create,
							NoteContent: content,
							NoteFile:    cmd.String("note-file"),
							DryRun:      dryRun,
						})
						if err != nil {
							return exitError(err)
						}
						if dryRun {
							fmt.Printf("would add:         %s\n", result.Line)
							if result.NotePath != "" {
								fmt.Printf("would create note: %s\n", result.NotePath)
								if result.NoteContent != "" {
									fmt.Println("staged note content:")
									fmt.Print(result.NoteContent)
									if !strings.HasSuffix(result.NoteContent, "\n") {
										fmt.Println()
									}
								}
							}
							return nil
						}
						noteMsg := ""
						if result.NotePath != "" {
							noteMsg = fmt.Sprintf(" + note %s", result.NotePath)
						}
						fmt.Printf("Created %s [priority:%s]%s\n", result.ID, result.Priority, noteMsg)
						return nil
					},
				}
			}(),
			{
				Name:  "init",
				Usage: "creates todo.md if missing",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					exists, err := fs.VerifyExists(todoPath)

					if err != nil {
						return exitError(err)
					}
					if !exists {
						if err := todo.Init(todoPath); err != nil {
							return exitError(err)
						}
						return nil
					}
					return nil
				},
			},
			func() *cli.Command {
				var (
					asJSON bool
					state  string
					stale  int
				)
				return &cli.Command{
					Name:    "list",
					Aliases: []string{"ls"},
					Usage:   "lists existing todos in tabular format",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:        "json",
							Destination: &asJSON,
							Usage:       "output machine-readable JSON",
						},
						&cli.IntFlag{
							Name:        "stale",
							Destination: &stale,
							Usage:       "only show claimed tasks older than N days",
						},
						&cli.StringFlag{
							Name:        "state",
							Destination: &state,
							Usage:       "filter by status: open, progress, done",
							Validator:   validation.Enum("open", "progress", "done"),
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						filter := todo.ListFilter{StaleDays: stale}
						if state != "" {
							s := parseState(state)
							filter.State = &s
						}
						tasks, err := todo.List(todoPath, filter)
						if err != nil {
							return exitError(err)
						}
						if asJSON {
							return writeJSON(tasks)
						}
						if len(tasks) == 0 {
							fmt.Println("No tasks found.")
							return nil
						}
						w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
						// tabwriter write errors surface on Flush below.
						_, _ = fmt.Fprintln(w, "\tID\tPRIORITY\tOPENED\tCLAIMED\tAGE\tSUMMARY")
						_, _ = fmt.Fprintln(w, "\t--\t--------\t------\t-------\t---\t-------")
						for _, t := range tasks {
							age := ""
							if t.AgeDays() >= 0 {
								age = fmt.Sprintf("%dd", t.AgeDays())
							} else {
								age = "-"
							}
							_, _ = fmt.Fprintf(w, "[%s]\t%s\t%s\t%s\t%s\t%s\t%s\n",
								t.Status, t.ID, t.Priority, t.Opened, dashIfEmpty(t.Claimed), age, t.Summary)
						}
						if err := w.Flush(); err != nil {
							return exitError(err)
						}
						return nil
					},
				}
			}(),
			{
				Name:      "pickup",
				Usage:     "pick up a task (mark as in progress)",
				ArgsUsage: "<task>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					ref := strings.TrimSpace(cmd.Args().First())
					if ref == "" {
						return exitError(errors.New("task number required: e.g. todo pickup TSK-001"))
					}
					line, note, err := todo.Pickup(todoPath, notesDir, ref)
					if err != nil {
						return exitError(err)
					}
					fmt.Println(line)
					if note != "" {
						fmt.Println("note:", note)
					}
					return nil
				},
			},
			func() *cli.Command {
				var clear, park bool
				return &cli.Command{
					Name:      "complete",
					Aliases:   []string{"done"},
					Usage:     "complete a task (mark done, optionally clear the line or park its note)",
					ArgsUsage: "<task>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:        "clear",
							Destination: &clear,
							Usage:       "remove the task line instead of marking it done",
						},
						&cli.BoolFlag{
							Name:        "park",
							Destination: &park,
							Usage:       "print the companion note file name",
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						ref := strings.TrimSpace(cmd.Args().First())
						if ref == "" {
							return exitError(errors.New("task number required: e.g. todo complete TSK-001"))
						}
						line, note, err := todo.Complete(todoPath, notesDir, ref, clear)
						if err != nil {
							return exitError(err)
						}
						fmt.Println(line)
						if park {
							if note != "" {
								fmt.Println("note:", note)
							} else {
								fmt.Printf("no note to park for %s\n", ref)
							}
						}
						return nil
					},
				}
			}(),
		},
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
