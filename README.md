# todo

Manage an ad-hoc task list in a plain Markdown file, `.todo/todo.md`, stored in the root of a git repo. Designed for agents and scripts: everything is a version-controllable text file, with optional companion notes.

## Installation

Requires Go 1.26+ and `make`.

```sh
make install   # builds and copies the binary to /usr/local/bin
```

Or build/install manually:

```sh
go install .   # uses the embedded VERSION file
go build -o todo .
```

## Usage

`todo` operates on the current git repo. Run `todo init` once to create the `.todo/todo.md` file, then manage tasks:

```sh
todo init                                   # create .todo/todo.md if missing
todo add "fix the thing"                    # add a task (default priority: med)
todo add -p high "urgent issue"             # add with priority (low|med|high)
todo add -s "summaries also via flag"       # summary via flag
todo add -n --note-content "body" "task"    # create a note with content
echo "body" | todo add -n "task"            # ...or read note content from stdin
todo add -n --note-file ./draft.md "task"   # ...or copy an existing file (not move)
todo add --dry-run -s "x" -n "task"         # preview the would-be line + note, no write
todo list                                   # list tasks (alias: todo ls)
todo list --state open                      # filter by status: open|progress|done
todo list --stale 5                         # only claimed tasks older than 5 days
todo list --json                            # machine-readable JSON output
todo pickup TSK-001                         # mark a task in progress (adds claimed date)
todo complete TSK-001                       # mark a task done (drops claimed)
todo complete --clear TSK-001               # remove the task line entirely
todo complete --park TSK-001                # done + print the companion note path
```

Task references accept `TSK-001`, `TSK-001`, `001`, `1`, or `#1`.

State rules:

- `pickup` only works on an open `[ ]` task.
- `complete` only works on an in-progress `[o]` task (i.e. one you picked up).
- `pickup` records the `claimed:` date; `complete` drops it.

## Data layout

Tasks are stored as lines under a YAML-like frontmatter header in `.todo/todo.md`:

```
- [ ] [TSK-001][priority:high][opened:2026-08-01] fix the thing
- [o] [TSK-002][priority:med][opened:2026-08-02][claimed:2026-08-10] refactor parser
- [x] [TSK-003][priority:low][opened:2026-08-03] write docs
```

Status is `[ ]` open, `[o]` in progress, `[x]` done. `claimed:` records when a task was picked up (present only on in-progress tasks). Optional companion notes live at `.todo/notes/<ID>.md`.

## Development

```sh
make check       # fmt + vet + lint + test
make build       # build ./todo binary
make test        # run tests
make lint        # golangci-lint (only if installed)
```

## Versioning

Version comes from the `VERSION` file for local/`go install` builds, and is overridden by `-ldflags` at release build time.
