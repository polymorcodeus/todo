// Package todo defines the todo CLI command tree and wiring.
package todo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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

// Main runs the root command.
func Main() {
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
					summary  string
					priority string
					create   bool
				)
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
							Usage:       "create an empty companion note file",
						},
					},
					Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
						if summary == "" {
							summary = strings.TrimSpace(cmd.Args().First())
						}
						if summary == "" {
							return ctx, exitError(errors.New("task summary required: provide as first argument or --summary"))
						}
						return ctx, nil
					},
					Action: func(ctx context.Context, cmd *cli.Command) error {
						if err := todo.Add(todoPath, notesDir, priority, summary, create); err != nil {
							return exitError(err)
						}
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
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "lists existing todos in tabular format",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := todo.List(todoPath); err != nil {
						return exitError(err)
					}
					return nil
				},
			},
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
