// Package cmd defines the todo CLI command tree and wiring.
package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	validation "github.com/urfave/cli-validation"
	"github.com/urfave/cli/v3"

	"gitlab.com/fuzzyporpoise/todo/internal/fs"
	"gitlab.com/fuzzyporpoise/todo/internal/git"
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

// newApp builds the todo command tree. It is separate from Run so tests can
// inject writers/readers and call app.Run directly.
func newApp() *cli.Command {
	var (
		repoRoot string
		cfg      appConfig
	)

	return &cli.Command{
		Name:                  "todo",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Usage:                 "manage ad-hoc tasks in .todo/todo.md",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			var err error
			repoRoot, err = git.RepoRoot()
			if err != nil {
				return ctx, exitError(err)
			}

			cfg = appConfigFor(repoRoot)

			// Allow `init` to run on an uninitialized repo.
			if cmd.Args().First() == "init" {
				return ctx, nil
			}
			exists, err := fs.VerifyExists(cfg.todoPath)
			if err != nil {
				return ctx, exitError(err)
			}
			if !exists {
				return ctx, exitError(errors.New("repo not todo initialized - run `todo init`"))
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
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return runAdd(cmd, cfg, addOptions{
							summary:     summary,
							priority:    priority,
							create:      create,
							noteContent: noteContent,
							noteFile:    noteFile,
							dryRun:      dryRun,
						})
					},
				}
			}(),
			{
				Name:  "init",
				Usage: "creates todo.md if missing",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runInit(cfg)
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
							Usage:       "only show claimed tasks older than N day/s",
						},
						&cli.StringFlag{
							Name:        "state",
							Destination: &state,
							Usage:       "filter by status: open, progress, done",
							Validator:   validation.Enum("open", "progress", "done"),
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return runList(cmd, cfg, listOptions{
							asJSON: asJSON,
							state:  state,
							stale:  stale,
						})
					},
				}
			}(),
			{
				Name:      "pickup",
				Usage:     "pick up a task (mark as in progress)",
				ArgsUsage: "<task>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runPickup(cmd, cfg)
				},
			},
			{
				Name:      "release",
				Usage:     "release a picked-up task back to open (drop its claim)",
				ArgsUsage: "<task>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runRelease(cmd, cfg)
				},
			},
			{
				Name:      "reopen",
				Usage:     "reopen a completed task (restore it to open status)",
				ArgsUsage: "<task>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runReopen(cmd, cfg)
				},
			},
			func() *cli.Command {
				var down bool
				return &cli.Command{
					Name:      "bump",
					Usage:     "bump a task's priority up or down by one level",
					ArgsUsage: "<task>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:        "down",
							Destination: &down,
							Usage:       "bump priority down instead of up",
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return runBump(cmd, cfg, down)
					},
				}
			}(),
			func() *cli.Command {
				var deleteNote bool
				return &cli.Command{
					Name:      "remove",
					Aliases:   []string{"rm"},
					Usage:     "remove a task line by reference (any status)",
					ArgsUsage: "<task>",
					Flags: []cli.Flag{
						&cli.BoolFlag{
							Name:        "note",
							Destination: &deleteNote,
							Usage:       "also delete the companion note file",
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return runRemove(cmd, cfg, removeOptions{deleteNote: deleteNote})
					},
				}
			}(),
			func() *cli.Command {
				var (
					lines  int
					noNote bool
					asJSON bool
				)
				return &cli.Command{
					Name:      "detail",
					Usage:     "show detailed information for a task, including a note preview",
					ArgsUsage: "<task>",
					Flags: []cli.Flag{
						&cli.IntFlag{
							Name:        "lines",
							Value:       20,
							Destination: &lines,
							Usage:       "number of note lines to preview",
						},
						&cli.BoolFlag{
							Name:        "no-note",
							Destination: &noNote,
							Usage:       "skip the companion note preview",
						},
						&cli.BoolFlag{
							Name:        "json",
							Destination: &asJSON,
							Usage:       "output machine-readable JSON",
						},
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						return runDetail(cmd, cfg, detailOptions{
							lines:  lines,
							noNote: noNote,
							asJSON: asJSON,
						})
					},
				}
			}(),
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
						return runComplete(cmd, cfg, completeOptions{
							clear: clear,
							park:  park,
						})
					},
				}
			}(),
		},
	}
}

// Run runs the root command.
func Run() {
	app := newApp()
	if err := app.Run(context.Background(), os.Args); err != nil {
		// cli has already reported the error by the time Run returns:
		// HandleExitCoder prints ExitCoder messages (no timestamp) and usage
		// errors are shown as "Incorrect Usage". Only the exit code is left.
		code := 1
		var exitErr cli.ExitCoder
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
		os.Exit(code)
	}
}

// appConfigFor builds the per-run configuration from the resolved repo root.
func appConfigFor(repoRoot string) appConfig {
	return appConfig{
		todoPath: filepath.Join(repoRoot, ".todo", "todo.md"),
		notesDir: filepath.Join(repoRoot, ".todo", "notes"),
	}
}
