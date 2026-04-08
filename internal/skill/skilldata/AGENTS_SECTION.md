# Synapse — Task Tracking & Memory for AI Agents

> Version: {{VERSION}}

Synapse provides persistent task tracking and breadcrumb memory across sessions.
Data lives in `.synapse/memory.jsonl` (Git-tracked).

## Binary Names

Both `syn` and `synapse` are valid binary names — they behave **identically**.
Use whichever is installed on PATH. Every command also supports `--help` (or `-h`):

```bash
syn --help              # top-level usage
syn add --help          # per-command help works on every subcommand
syn doctor              # one-shot health check (binary, store, MCP, aliases)
```

## Available Commands

```bash
syn ready --json                       # Get actionable tasks (unblocked, open)
syn add "Title"                        # Create task
   --priority N --label X              #   (repeatable: --label, --blocks)
   --blocks N --parent N --assignee X
syn update <id>                        # Patch any field on an existing task
   --add-blocker N --remove-blocker N  #   (or --set-blockers "1,2,3")
   --add-label X   --remove-label X    #   (or --set-labels "bug,security")
   --status X --priority N --title X --description X --assignee X
syn edit <id>                          # alias for update
syn block <id> --by N                  # Quick: add a blocker (repeatable)
syn unblock <id> [--from N | --all]    # Quick: remove blocker(s)
syn note <id> "text"                   # Append a note for context
syn claim <id>                         # Mark as in-progress
syn done <id>                          # Mark as complete
syn list --json                        # List all tasks
syn get <id> --json                    # Full task details
syn delete <id>                        # Delete task
syn bc set <key> <value>               # Store breadcrumb (cross-session memory)
syn bc get <key>                       # Retrieve breadcrumb
syn bc list [prefix]                   # List breadcrumbs
```

## Workflow

1. Run `syn ready --json` to find actionable tasks
2. `syn claim <id>` before starting work
3. Discover bugs/issues? `syn add "New issue" --parent <current_id>` (or
   `spawn_task` via MCP)
4. Need to patch a task? `syn update <id> --add-blocker N --priority 5` —
   never edit `.synapse/memory.jsonl` by hand
5. `syn done <id>` when finished
6. Store context: `syn bc set session.last_file src/auth.go`

## Status Discipline

Keep Synapse accurate at all times so any agent can query it for current progress
without scanning code. For multi-step plans, create a task for each step upfront.
Claim tasks before starting, mark them done immediately when finished, and use
`syn bc set session.progress "..."` at pause points. Synapse should always
reflect reality.

## Status Values

`open` → `in-progress` → `review` → `done` | `blocked` (waiting on deps)

Adding blockers via `update`/`block` auto-flips a task to `blocked`. Removing
the last blocker auto-flips it back to `open`.

## Breadcrumb Namespaces

- `session.*` — Current session context
- `arch.*` — Architecture decisions
- `bug.*` — Bug investigation notes
- `decision.*` — Design decisions with rationale

If the Synapse MCP server is available, prefer MCP tools over CLI for richer
integration (pagination, multi-agent claims, breadcrumb filtering).
