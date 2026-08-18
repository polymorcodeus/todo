// Package todo defines the todo CLI command tree and wiring.
package todo

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"
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

// Main runs the root command.
func Main() {
	app := &cli.Command{
		Name:                  "todo",
		Usage:                 "all things agentic todo list",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Println("todo")
			return nil
		},
	}
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
