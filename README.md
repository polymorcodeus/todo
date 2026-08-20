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
todo list --stale 5                         # only claimed tasks older than 5 day/s
todo list --json                            # machine-readable JSON output
todo detail TSK-001                         # show task details + 20-line note preview
todo detail --lines 5 TSK-001               # preview first 5 lines of the note
todo detail --no-note TSK-001               # show task details without note preview
todo detail --json TSK-001                  # machine-readable detail output
todo pickup TSK-001                         # mark a task in progress (adds claimed date)
todo release TSK-001                        # release a picked-up task back to open (drop claim)
todo complete TSK-001                       # mark a task done (drops claimed)
todo complete --clear TSK-001               # remove the task line entirely
todo complete --park TSK-001                # done + print the companion note path
todo remove TSK-001                         # remove a task line by ref (any status, e.g. [x])
```

Task references accept `TSK-001`, `TSK-001`, `001`, `1`, or `#1`.

State rules:

- `pickup` only works on an open `[ ]` task.
- `complete` only works on an in-progress `[o]` task (i.e. one you picked up).
- `release` also only works on an in-progress `[o]` task; it returns it to `[ ]` and drops the claim.
- `remove` works on any status line (including `[x]` done lines) and deletes it.
- `pickup` records the `claimed:` date; `complete`/`release` drop it.

## Data layout

Tasks are stored as lines under a YAML-like frontmatter header in `.todo/todo.md`:

```
---
project: todo
last_updated: 2026-08-18T10:37
configured: 2026-08-18
legacy_source: none
next_id: TSK-005
---

- [ ] [TSK-001][priority:high][opened:2026-08-01] fix the thing
- [o] [TSK-002][priority:med][opened:2026-08-02][claimed:2026-08-10] refactor parser
- [x] [TSK-003][priority:low][opened:2026-08-03] write docs
```

Status is `[ ]` open, `[o]` in progress, `[x]` done. `claimed:` records when a task was picked up (present only on in-progress tasks). Optional companion notes live at `.todo/notes/<ID>.md`.

`next_id:` is a monotonic high-water mark: `add` never reuses it, so removing all tasks still lets new tasks resume at the next ID rather than restarting at TSK-001. It is backfilled automatically on any write, so files created before this field existed converge without manual action.

## JSON output

Both `todo list --json` and `todo detail --json` emit stable, machine-readable JSON.

- `status`: canonical status designation (`open`, `in progress`, `complete`)
- `status_symbol`: raw checkbox character (` `, `o`, `x`)

`todo list --json` returns an array of tasks with fields: `id`, `status`, `status_symbol`, `priority`, `opened`, `claimed`, `age_days`, `summary`.

`todo detail --json` returns a single object with fields: `id`, `status`, `status_symbol`, `priority`, `opened`, `opened_days`, `claimed`, `age_days`, `summary`, `note_path`, `note_exists`, `note_preview`, `note_preview_truncated`.

`claimed`, `age_days`, `note_preview`, and `note_preview_truncated` are omitted when empty or not applicable.

## Development

```sh
make check       # fmt + vet + lint + test
make build       # build ./todo binary
make test        # run tests
make lint        # golangci-lint (only if installed)
```

## Versioning

Version comes from the `VERSION` file for local/`go install` builds, and is overridden by `-ldflags` at release build time.
