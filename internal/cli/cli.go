// Package cli implements the Synapse command-line interface.
//
// Both the `synapse` and `syn` binaries are thin shims that call Run().
// All command parsing, dispatch, help text, and output live here so the two
// binaries always stay in lock-step.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/swiftj/synapse/internal/mcp"
	"github.com/swiftj/synapse/internal/skill"
	"github.com/swiftj/synapse/internal/storage"
	"github.com/swiftj/synapse/internal/view"
	"github.com/swiftj/synapse/pkg/types"
)

// Version is the canonical synapse version string.
// The post-commit Git hook auto-bumps cmd/synapse/main.go's `const version`;
// keep this in sync with that constant.
const Version = "1.0.10"

// hasHelpFlag returns true if args contains -h, --help, or help.
// Used by every subcommand so users can run `synapse <cmd> --help`.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "help" {
			return true
		}
	}
	return false
}

// Run executes the synapse CLI with the given arguments and IO streams.
// argv[0] is the program name (used for binary detection); argv[1:] are the
// arguments. Returns the process exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	r := &runner{
		argv:   argv,
		stdout: stdout,
		stderr: stderr,
	}
	return r.run()
}

// RunMain is the entry point used by the cmd/synapse and cmd/syn shims.
// It wires Run to os.Args / os.Stdout / os.Stderr and exits the process.
func RunMain() {
	os.Exit(Run(os.Args, os.Stdout, os.Stderr))
}

// runner holds per-invocation state. It exists so tests can construct one
// with custom IO without touching package-level globals.
type runner struct {
	argv       []string
	stdout     io.Writer
	stderr     io.Writer
	jsonOutput bool
}

func (r *runner) printf(format string, a ...any) {
	fmt.Fprintf(r.stdout, format, a...)
}

func (r *runner) println(a ...any) {
	fmt.Fprintln(r.stdout, a...)
}

func (r *runner) errorf(format string, a ...any) {
	fmt.Fprintf(r.stderr, format, a...)
}

func (r *runner) errorln(a ...any) {
	fmt.Fprintln(r.stderr, a...)
}

// jsonOut writes v as indented JSON to stdout.
// Nil slices are coerced to empty arrays for agent-friendly output.
func (r *runner) jsonOut(v any) {
	if v == nil {
		v = []any{}
	} else if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.IsNil() {
		v = []any{}
	}
	enc := json.NewEncoder(r.stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// extractGlobalFlags scans args for --json, sets r.jsonOutput, and returns
// the args with --json removed so per-command parsers don't see it.
func (r *runner) extractGlobalFlags(args []string) []string {
	out := args[:0]
	for _, arg := range args {
		if arg == "--json" {
			r.jsonOutput = true
		} else {
			out = append(out, arg)
		}
	}
	return out
}

// binaryName returns the short name of the running binary (synapse or syn).
// Used in help text so the examples match what the user typed.
func (r *runner) binaryName() string {
	if len(r.argv) == 0 {
		return "synapse"
	}
	base := filepath.Base(r.argv[0])
	if base == "syn" || base == "synapse" {
		return base
	}
	return "synapse"
}

func (r *runner) run() int {
	args := r.extractGlobalFlags(append([]string{}, r.argv...))

	if len(args) < 2 {
		r.printUsage()
		return 1
	}

	cmd := args[1]
	cmdArgs := args[2:]

	switch cmd {
	case "init":
		return r.cmdInit(cmdArgs)
	case "add":
		return r.cmdAdd(cmdArgs)
	case "list", "ls":
		return r.cmdList(cmdArgs)
	case "ready":
		return r.cmdReady(cmdArgs)
	case "get":
		return r.cmdGet(cmdArgs)
	case "claim":
		return r.cmdClaim(cmdArgs)
	case "update", "edit":
		return r.cmdUpdate(cmdArgs)
	case "block":
		return r.cmdBlock(cmdArgs)
	case "unblock":
		return r.cmdUnblock(cmdArgs)
	case "note":
		return r.cmdNote(cmdArgs)
	case "done":
		return r.cmdDone(cmdArgs)
	case "all-done":
		return r.cmdDoneAll(cmdArgs)
	case "delete", "rm":
		return r.cmdDelete(cmdArgs)
	case "breadcrumb", "bc":
		return r.cmdBreadcrumb(cmdArgs)
	case "skill":
		return r.cmdSkill(cmdArgs)
	case "doctor":
		return r.cmdDoctor(cmdArgs)
	case "serve":
		return r.cmdServe(cmdArgs)
	case "view":
		return r.cmdView(cmdArgs)
	case "version", "-v", "--version":
		if r.jsonOutput {
			r.jsonOut(map[string]string{"version": Version})
			return 0
		}
		r.printf("%s v%s\n", r.binaryName(), Version)
		return 0
	case "help", "-h", "--help":
		r.printUsage()
		return 0
	default:
		r.errorf("error: unknown command: %s\n", cmd)
		r.errorf("run '%s --help' to see available commands\n", r.binaryName())
		return 1
	}
}

// ── Help text ───────────────────────────────────────────────────────────────

func (r *runner) printUsage() {
	bin := r.binaryName()
	r.println(`Synapse - The shared nervous system for Vibe Coders and their Agents.

Usage:
  ` + bin + ` [--json] <command> [arguments]

Both 'synapse' and 'syn' are valid binary names — they behave identically.
Every command supports --help (or -h) for full per-command documentation.

Global Flags:
  --json            Output structured JSON (works with any command)

Commands:
  init              Initialize .synapse directory in current project
  add <title>       Create a new synapse task
  list, ls          List all synapses
  ready             List ready (unblocked, open) tasks
  get <id>          Get details of a specific synapse
  claim <id>        Mark synapse as in-progress
  update, edit <id> Patch any field on an existing task
  block <id>        Add blocker(s) to an existing task
  unblock <id>      Remove blocker(s) from an existing task
  note <id>         Append a note to a task
  done <id>         Mark synapse as done
  all-done          Mark all tasks as done (cleanup command)
  delete, rm <id>   Delete a synapse task (or --all / --done for bulk)
  breadcrumb, bc    Manage breadcrumbs (persistent key-value storage)
  skill             Manage agentic skill installations
  doctor            Check installation health and project state
  serve             Start MCP server (JSON-RPC over stdio)
  view              Start visualization web server
  version           Print version
  help              Print this help message

Examples:
  ` + bin + ` init
  ` + bin + ` --json add "Fix login bug" --priority 5 --label bug --blocks 4
  ` + bin + ` --json ready
  ` + bin + ` claim 5
  ` + bin + ` --json done 5
  ` + bin + ` add --help          # per-command documentation`)
}

// helpInit, helpAdd, ... return the help text for each subcommand.
// They are pure functions so tests can verify them in isolation.

func helpInit(bin string) string {
	return `Initialize a new .synapse directory in the current project.

Usage:
  ` + bin + ` init [--git]

Flags:
  --git             Also stage .synapse/memory.jsonl for commit (if in a Git repo)
  -h, --help        Show this help

Creates:
  .synapse/memory.jsonl       — task store (JSONL, Git-tracked)
  .synapse/breadcrumbs.jsonl  — breadcrumb store (JSONL, Git-tracked)

Example:
  ` + bin + ` init --git`
}

func helpAdd(bin string) string {
	return `Create a new synapse task.

Usage:
  ` + bin + ` add <title> [flags]

Flags:
  --blocks N        Block on synapse N (repeatable)
  --parent N        Set parent synapse ID
  --assignee X      Assign to role (e.g., @qa, @architect, @coder)
  --priority N      Priority (higher = more important; default 0)
  --label X         Tag the task (repeatable; e.g., bug, feature, security)
  -h, --help        Show this help

Examples:
  ` + bin + ` add "Implement JWT refresh" --priority 5 --label feature --label auth
  ` + bin + ` add "Fix null check" --parent 12 --label bug
  ` + bin + ` add "Deploy to staging" --blocks 3 --blocks 7`
}

func helpList(bin string) string {
	return `List synapse tasks.

Usage:
  ` + bin + ` list [flags]
  ` + bin + ` ls   [flags]

Flags:
  --status X        Filter by status (open, in-progress, blocked, review, done)
  --limit N         Limit output to N tasks (default 20, 0 for unlimited)
  --summary         Condensed output (default)
  --full            Show all fields for each task
  -h, --help        Show this help

Examples:
  ` + bin + ` list
  ` + bin + ` list --status open --limit 10
  ` + bin + ` --json list --full`
}

func helpReady(bin string) string {
	return `List ready (unblocked, open) tasks — the next things any agent can pick up.

Usage:
  ` + bin + ` ready

Flags:
  -h, --help        Show this help

A task is "ready" when its status is open and all of its blockers are done.

Example:
  ` + bin + ` --json ready`
}

func helpGet(bin string) string {
	return `Get full details of a specific synapse task.

Usage:
  ` + bin + ` get <id>

Flags:
  -h, --help        Show this help

Example:
  ` + bin + ` --json get 5`
}

func helpClaim(bin string) string {
	return `Claim a synapse task and mark it as in-progress.

Usage:
  ` + bin + ` claim <id>

Flags:
  -h, --help        Show this help

Example:
  ` + bin + ` claim 5`
}

func helpDone(bin string) string {
	return `Mark a synapse task as done.

Usage:
  ` + bin + ` done <id>

Flags:
  -h, --help        Show this help

Example:
  ` + bin + ` done 5`
}

func helpUpdate(bin string) string {
	return `Patch fields on an existing synapse task.

Usage:
  ` + bin + ` update <id> [flags]
  ` + bin + ` edit   <id> [flags]   (alias)

Flags:
  --title X              Set title
  --description X        Set description (use "" to clear)
  --status X             Set status (open, in-progress, blocked, review, done)
  --priority N           Set priority
  --assignee X           Set assignee (use "" to clear)
  --parent N             Set parent ID (use 0 to clear)

  --add-blocker N        Add a blocker (repeatable)
  --remove-blocker N     Remove a blocker (repeatable)
  --set-blockers LIST    Replace blocker list (comma-separated, e.g. 1,2,3 or "" to clear)

  --add-label X          Add a label (repeatable)
  --remove-label X       Remove a label (repeatable)
  --set-labels LIST      Replace labels (comma-separated, "" to clear)

  -h, --help             Show this help

Examples:
  ` + bin + ` update 12 --priority 9 --add-label security
  ` + bin + ` edit 12 --add-blocker 4 --add-blocker 7
  ` + bin + ` update 12 --status review --assignee @qa
  ` + bin + ` update 12 --set-blockers ""           # clear all blockers
  ` + bin + ` update 12 --remove-blocker 4`
}

func helpBlock(bin string) string {
	return `Add one or more blockers to an existing task.

Usage:
  ` + bin + ` block <id> --by <blocker-id> [--by <blocker-id> ...]

Flags:
  --by N            Blocker task ID (repeatable)
  -h, --help        Show this help

Adding any blocker also flips the task's status to "blocked" if it was "open".

Examples:
  ` + bin + ` block 12 --by 4
  ` + bin + ` block 12 --by 4 --by 7`
}

func helpUnblock(bin string) string {
	return `Remove blocker(s) from an existing task.

Usage:
  ` + bin + ` unblock <id> [--from <blocker-id> ...]
  ` + bin + ` unblock <id> --all

Flags:
  --from N          Specific blocker to remove (repeatable). Omit to remove all.
  --all             Remove every blocker
  -h, --help        Show this help

If the task ends up with no blockers, its status flips back to "open".

Examples:
  ` + bin + ` unblock 12 --from 4
  ` + bin + ` unblock 12 --all`
}

func helpNote(bin string) string {
	return `Append a note to a task for context persistence.

Usage:
  ` + bin + ` note <id> <text>...

Flags:
  -h, --help        Show this help

Example:
  ` + bin + ` note 12 "Discovered race condition in token rotation"`
}

func helpAllDone(bin string) string {
	return `Mark all tasks as done. This is a cleanup command — use with care.

Usage:
  ` + bin + ` all-done

Flags:
  -h, --help        Show this help`
}

func helpDelete(bin string) string {
	return `Delete a synapse task (or bulk delete).

Usage:
  ` + bin + ` delete <id>
  ` + bin + ` rm     <id>
  ` + bin + ` delete --all
  ` + bin + ` delete --done

Flags:
  --all             Delete every task in the store
  --done            Delete only completed tasks (cleanup)
  -h, --help        Show this help

Examples:
  ` + bin + ` delete 5
  ` + bin + ` delete --done`
}

func helpBreadcrumb(bin string) string {
	return `Manage breadcrumbs — persistent key-value storage for cross-session memory.

Usage:
  ` + bin + ` breadcrumb <subcommand> [args]
  ` + bin + ` bc         <subcommand> [args]

Subcommands:
  set <key> <value>          Set a breadcrumb value
      --task-id N            Link to task ID
  get <key>                  Get a breadcrumb value
  list [prefix]              List breadcrumbs (optionally filter by prefix)
  delete <key>               Delete a breadcrumb

Flags:
  -h, --help                 Show this help (also works on each subcommand)

Recommended namespaces:
  session.*    Current session context        decision.*  Design decisions
  arch.*       Architecture decisions          bug.*       Bug investigations
  env.*        Environment configuration

Examples:
  ` + bin + ` bc set arch.auth "JWT with refresh tokens"
  ` + bin + ` bc set session.progress "API endpoints done" --task-id 12
  ` + bin + ` bc get arch.auth
  ` + bin + ` bc list arch.`
}

func helpBreadcrumbSet(bin string) string {
	return `Set a breadcrumb value.

Usage:
  ` + bin + ` breadcrumb set <key> <value> [--task-id N]
  ` + bin + ` bc set <key> <value> [--task-id N]

Flags:
  --task-id N       Link the breadcrumb to a task ID
  -h, --help        Show this help

Example:
  ` + bin + ` bc set arch.db "PostgreSQL with pgx"`
}

func helpBreadcrumbGet(bin string) string {
	return `Get a breadcrumb value.

Usage:
  ` + bin + ` breadcrumb get <key>
  ` + bin + ` bc get <key>

Flags:
  -h, --help        Show this help

Example:
  ` + bin + ` bc get arch.db`
}

func helpBreadcrumbList(bin string) string {
	return `List breadcrumbs (optionally filtered by key prefix).

Usage:
  ` + bin + ` breadcrumb list [prefix]
  ` + bin + ` bc list [prefix]

Flags:
  -h, --help        Show this help

Examples:
  ` + bin + ` bc list                # all breadcrumbs
  ` + bin + ` bc list session.       # only session.* keys`
}

func helpBreadcrumbDelete(bin string) string {
	return `Delete a breadcrumb by key.

Usage:
  ` + bin + ` breadcrumb delete <key>
  ` + bin + ` bc delete <key>

Flags:
  -h, --help        Show this help`
}

func helpSkill(bin string) string {
	return `Manage agentic skill installations (Claude Code, Codex, Gemini, etc.).

Usage:
  ` + bin + ` skill <subcommand> [args]

Subcommands:
  install <agent>            Install skill for an agent
      --level L              Install level: user or project (default: project)
  uninstall <agent>          Remove skill for an agent
      --level L              Install level: user or project (default: project)
  list                       Show installation status for all agents
  update [agent]             Update installed skill(s)
      --level L              Install level: user or project (default: project)
  show                       Print the embedded SKILL.md content

Flags:
  -h, --help                 Show this help (also works on each subcommand)

Example:
  ` + bin + ` skill install claude-code --level user`
}

func helpSkillInstall(bin string) string {
	return `Install the synapse skill for an agent.

Usage:
  ` + bin + ` skill install <agent> [--level user|project]

Flags:
  --level L         Install at user level (~/.) or project level (default: project)
  -h, --help        Show this help

Available agents: ` + strings.Join(skill.AgentNames(), ", ") + `

Example:
  ` + bin + ` skill install claude-code --level user`
}

func helpSkillUninstall(bin string) string {
	return `Uninstall the synapse skill for an agent.

Usage:
  ` + bin + ` skill uninstall <agent> [--level user|project]

Flags:
  --level L         Uninstall from user or project level (default: project)
  -h, --help        Show this help`
}

func helpSkillList(bin string) string {
	return `Show installation status for all agents and both levels.

Usage:
  ` + bin + ` skill list

Flags:
  -h, --help        Show this help`
}

func helpSkillUpdate(bin string) string {
	return `Update installed skill(s) to the current synapse version.

Usage:
  ` + bin + ` skill update [agent] [--level user|project]

Flags:
  --level L         Level to update (default: project)
  -h, --help        Show this help

If no agent is specified, every installed skill is updated.`
}

func helpSkillShow(bin string) string {
	return `Print the embedded SKILL.md content (with version injected).

Usage:
  ` + bin + ` skill show

Flags:
  -h, --help        Show this help`
}

func helpDoctor(bin string) string {
	return `Run an installation health check and show project state.

Usage:
  ` + bin + ` doctor

Flags:
  -h, --help        Show this help

Reports:
  - Synapse version and binary path
  - Whether both 'syn' and 'synapse' aliases are on PATH
  - Project store status (initialized? task and breadcrumb counts)
  - Git repo detection
  - MCP server availability`
}

func helpServe(bin string) string {
	return `Start the MCP server (JSON-RPC over stdio).

Usage:
  ` + bin + ` serve

Flags:
  -h, --help        Show this help

This is the entry point used by Claude Code and other MCP-aware agents.`
}

func helpView(bin string) string {
	return `Start the visualization web server.

Usage:
  ` + bin + ` view [--port N]

Flags:
  --port N          Port to listen on (default: 8080)
  -h, --help        Show this help

Example:
  ` + bin + ` view --port 3000`
}

// ── Storage helpers ─────────────────────────────────────────────────────────

func (r *runner) getStore() (*storage.JSONLStore, int) {
	store := storage.NewJSONLStore(storage.DefaultDir)
	if err := store.Load(); err != nil {
		r.errorf("error loading store: %v\n", err)
		return nil, 1
	}
	// Reconcile on load so read-only commands (get, list) see accurate status.
	if n := store.Reconcile(); n > 0 {
		// Persist the reconciled state so disk matches memory.
		_ = store.Save()
	}
	return store, 0
}

func (r *runner) saveStore(store *storage.JSONLStore) int {
	// Auto-transition blocked → open when all blockers are done.
	store.Reconcile()
	if err := store.Save(); err != nil {
		r.errorf("error saving store: %v\n", err)
		return 1
	}
	return 0
}

func (r *runner) getBreadcrumbStore() (*storage.BreadcrumbStore, int) {
	store := storage.NewBreadcrumbStore(storage.DefaultDir)
	if err := store.Load(); err != nil {
		r.errorf("error loading breadcrumbs: %v\n", err)
		return nil, 1
	}
	return store, 0
}

func (r *runner) saveBreadcrumbStore(store *storage.BreadcrumbStore) int {
	if err := store.Save(); err != nil {
		r.errorf("error saving breadcrumbs: %v\n", err)
		return 1
	}
	return 0
}

// ── Commands ────────────────────────────────────────────────────────────────

func (r *runner) cmdInit(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpInit(r.binaryName()))
		return 0
	}

	stageMemory := false
	for _, arg := range args {
		if arg == "--git" {
			stageMemory = true
		}
	}

	store := storage.NewJSONLStore(storage.DefaultDir)
	result, err := store.InitWithOptions(stageMemory)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if r.jsonOutput {
		r.jsonOut(result)
		return 0
	}

	r.println("Initialized .synapse directory")
	if result.MemoryCreated {
		r.println("  ✓ Created memory.jsonl")
	} else {
		r.println("  - memory.jsonl already exists")
	}

	if result.GitRepoDetected {
		if result.MemoryStaged {
			r.println("  ✓ Staged .synapse/memory.jsonl for commit")
		} else if stageMemory {
			r.println("  - Could not stage memory.jsonl")
		}
	} else {
		r.println("  - Not a Git repository (skipping Git integration)")
	}
	return 0
}

func (r *runner) cmdAdd(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpAdd(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: title required")
		r.errorf("run '%s add --help' for usage\n", r.binaryName())
		return 1
	}

	var title string
	var blocks []int
	var parentID int
	var assignee string
	var priority int
	var labels []string

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--blocks" && i+1 < len(args):
			i++
			// Accept comma-separated IDs: --blocks 1,2,3 or repeated --blocks 1 --blocks 2
			for _, part := range strings.Split(args[i], ",") {
				part = strings.TrimSpace(part)
				id, err := strconv.Atoi(part)
				if err != nil {
					r.errorf("error: invalid blocker ID: %s\n", part)
					return 1
				}
				blocks = append(blocks, id)
			}
		case arg == "--parent" && i+1 < len(args):
			i++
			id, err := strconv.Atoi(args[i])
			if err != nil {
				r.errorf("error: invalid parent ID: %s\n", args[i])
				return 1
			}
			parentID = id
		case arg == "--assignee" && i+1 < len(args):
			i++
			assignee = args[i]
		case arg == "--priority" && i+1 < len(args):
			i++
			p, err := strconv.Atoi(args[i])
			if err != nil {
				r.errorf("error: invalid priority: %s\n", args[i])
				return 1
			}
			priority = p
		case arg == "--label" && i+1 < len(args):
			i++
			labels = append(labels, args[i])
		case !strings.HasPrefix(arg, "--"):
			if title == "" {
				title = arg
			} else {
				title = title + " " + arg
			}
		default:
			r.errorf("error: unknown flag or missing value: %s\n", arg)
			r.errorf("run '%s add --help' for usage\n", r.binaryName())
			return 1
		}
		i++
	}

	if title == "" {
		r.errorln("error: title required")
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Create(title)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	syn.BlockedBy = blocks
	syn.ParentID = parentID
	syn.Assignee = assignee
	syn.Priority = priority
	syn.Labels = labels

	if len(blocks) > 0 {
		syn.Status = types.StatusBlocked
	}

	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}

	r.printf("Created synapse #%d: %s\n", syn.ID, syn.Title)
	if priority != 0 {
		r.printf("  Priority: %d\n", priority)
	}
	if len(labels) > 0 {
		r.printf("  Labels: %s\n", strings.Join(labels, ", "))
	}
	if len(blocks) > 0 {
		r.printf("  Blocked by: %v\n", blocks)
	}
	if parentID > 0 {
		r.printf("  Parent: #%d\n", parentID)
	}
	if assignee != "" {
		r.printf("  Assignee: %s\n", assignee)
	}
	return 0
}

func (r *runner) cmdList(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpList(r.binaryName()))
		return 0
	}

	var statusFilter string
	var labelFilter string
	var fullOutput bool
	limit := 20

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 < len(args) {
				i++
				statusFilter = args[i]
			}
		case "--label":
			if i+1 < len(args) {
				i++
				labelFilter = args[i]
			}
		case "--limit":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil {
					r.errorf("error: invalid limit: %s\n", args[i])
					return 1
				}
				limit = n
			}
		case "--full":
			fullOutput = true
		case "--summary":
			fullOutput = false
		}
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	var synapses []*types.Synapse

	if statusFilter != "" {
		status := types.Status(statusFilter)
		if !status.IsValid() {
			r.errorf("error: invalid status: %s\n", statusFilter)
			r.errorln("valid statuses: open, in-progress, blocked, review, done")
			return 1
		}
		synapses = store.ByStatus(status)
	} else if labelFilter != "" {
		synapses = store.ByLabel(labelFilter)
	} else {
		synapses = store.All()
	}

	totalCount := len(synapses)
	if limit > 0 && len(synapses) > limit {
		synapses = synapses[:limit]
	}

	if r.jsonOutput {
		r.jsonOut(synapses)
		return 0
	}

	if len(synapses) == 0 {
		r.println("No synapses found")
		return 0
	}

	if totalCount > len(synapses) {
		r.printf("Showing %d of %d synapse(s) (use --limit 0 for all):\n\n", len(synapses), totalCount)
	} else {
		r.printf("Found %d synapse(s):\n\n", len(synapses))
	}

	for _, syn := range synapses {
		if fullOutput {
			r.printSynapseDetailed(syn)
			r.println()
		} else {
			r.printSynapse(syn)
		}
	}
	return 0
}

func (r *runner) cmdReady(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpReady(r.binaryName()))
		return 0
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	ready := store.Ready()

	if r.jsonOutput {
		r.jsonOut(ready)
		return 0
	}

	if len(ready) == 0 {
		r.println("No ready tasks")
		return 0
	}

	r.printf("Ready tasks (%d):\n\n", len(ready))
	for _, syn := range ready {
		r.printSynapse(syn)
	}
	return 0
}

func (r *runner) cmdGet(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpGet(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}

	r.printSynapseDetailed(syn)
	return 0
}

func (r *runner) cmdClaim(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpClaim(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if syn.Status == types.StatusDone {
		r.errorf("error: synapse #%d is already done\n", id)
		return 1
	}

	syn.MarkInProgress()
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}

	r.printf("Claimed synapse #%d: %s\n", syn.ID, syn.Title)
	r.printf("Status: %s\n", syn.Status)
	return 0
}

// parseIDList parses a comma-separated list of integer IDs.
// An empty string returns an empty slice (used to clear lists).
func parseIDList(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return []int{}, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %w", p, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// parseStringList parses a comma-separated list of strings.
// An empty string returns an empty slice.
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// containsInt reports whether xs contains x.
func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// containsString reports whether xs contains x.
func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// removeInt returns xs with the first occurrence of x removed.
func removeInt(xs []int, x int) []int {
	out := make([]int, 0, len(xs))
	removed := false
	for _, v := range xs {
		if !removed && v == x {
			removed = true
			continue
		}
		out = append(out, v)
	}
	return out
}

// removeString returns xs with the first occurrence of x removed.
func removeString(xs []string, x string) []string {
	out := make([]string, 0, len(xs))
	removed := false
	for _, v := range xs {
		if !removed && v == x {
			removed = true
			continue
		}
		out = append(out, v)
	}
	return out
}

// updateOpts captures every patch flag accepted by `synapse update`.
// Pointer fields distinguish "not set" (nil) from "set to zero value" (e.g.
// description = "" should clear, not be ignored).
type updateOpts struct {
	title       *string
	description *string
	status      *string
	priority    *int
	assignee    *string
	parentID    *int

	addBlockers    []int
	removeBlockers []int
	setBlockers    *[]int

	addLabels    []string
	removeLabels []string
	setLabels    *[]string
}

// parseUpdateFlags walks args and populates an updateOpts. It is shared by
// the `update`/`edit` command and exposed for testing.
func parseUpdateFlags(args []string) (*updateOpts, error) {
	opts := &updateOpts{}

	needArg := func(i int, flag string) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("flag %s requires a value", flag)
		}
		return args[i+1], nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--title":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			opts.title = &v
			i++
		case "--description":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			opts.description = &v
			i++
		case "--status":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			if !types.Status(v).IsValid() {
				return nil, fmt.Errorf("invalid status %q (valid: open, in-progress, blocked, review, done)", v)
			}
			opts.status = &v
			i++
		case "--priority":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			n, perr := strconv.Atoi(v)
			if perr != nil {
				return nil, fmt.Errorf("invalid priority %q: %w", v, perr)
			}
			opts.priority = &n
			i++
		case "--assignee":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			opts.assignee = &v
			i++
		case "--parent":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			n, perr := strconv.Atoi(v)
			if perr != nil {
				return nil, fmt.Errorf("invalid parent %q: %w", v, perr)
			}
			opts.parentID = &n
			i++
		case "--add-blocker":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			n, perr := strconv.Atoi(v)
			if perr != nil {
				return nil, fmt.Errorf("invalid blocker %q: %w", v, perr)
			}
			opts.addBlockers = append(opts.addBlockers, n)
			i++
		case "--remove-blocker":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			n, perr := strconv.Atoi(v)
			if perr != nil {
				return nil, fmt.Errorf("invalid blocker %q: %w", v, perr)
			}
			opts.removeBlockers = append(opts.removeBlockers, n)
			i++
		case "--set-blockers":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			ids, perr := parseIDList(v)
			if perr != nil {
				return nil, perr
			}
			opts.setBlockers = &ids
			i++
		case "--add-label":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			opts.addLabels = append(opts.addLabels, v)
			i++
		case "--remove-label":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			opts.removeLabels = append(opts.removeLabels, v)
			i++
		case "--set-labels":
			v, err := needArg(i, arg)
			if err != nil {
				return nil, err
			}
			labels := parseStringList(v)
			opts.setLabels = &labels
			i++
		default:
			return nil, fmt.Errorf("unknown flag: %s", arg)
		}
	}
	return opts, nil
}

// applyUpdate mutates syn in place per opts. Returns true if anything changed.
func applyUpdate(syn *types.Synapse, opts *updateOpts) bool {
	changed := false

	if opts.title != nil {
		syn.Title = *opts.title
		changed = true
	}
	if opts.description != nil {
		syn.Description = *opts.description
		changed = true
	}
	if opts.status != nil {
		syn.Status = types.Status(*opts.status)
		changed = true
	}
	if opts.priority != nil {
		syn.Priority = *opts.priority
		changed = true
	}
	if opts.assignee != nil {
		syn.Assignee = *opts.assignee
		changed = true
	}
	if opts.parentID != nil {
		syn.ParentID = *opts.parentID
		changed = true
	}

	// Blockers — set takes precedence; otherwise apply add/remove deltas.
	if opts.setBlockers != nil {
		syn.BlockedBy = *opts.setBlockers
		changed = true
	} else {
		for _, id := range opts.addBlockers {
			if !containsInt(syn.BlockedBy, id) {
				syn.BlockedBy = append(syn.BlockedBy, id)
				changed = true
			}
		}
		for _, id := range opts.removeBlockers {
			if containsInt(syn.BlockedBy, id) {
				syn.BlockedBy = removeInt(syn.BlockedBy, id)
				changed = true
			}
		}
	}

	// Auto-flip status when blockers change and the user didn't set it explicitly.
	if opts.status == nil {
		if len(syn.BlockedBy) > 0 && syn.Status == types.StatusOpen {
			syn.Status = types.StatusBlocked
		} else if len(syn.BlockedBy) == 0 && syn.Status == types.StatusBlocked {
			syn.Status = types.StatusOpen
		}
	}

	// Labels.
	if opts.setLabels != nil {
		syn.Labels = *opts.setLabels
		changed = true
	} else {
		for _, l := range opts.addLabels {
			if !containsString(syn.Labels, l) {
				syn.Labels = append(syn.Labels, l)
				changed = true
			}
		}
		for _, l := range opts.removeLabels {
			if containsString(syn.Labels, l) {
				syn.Labels = removeString(syn.Labels, l)
				changed = true
			}
		}
	}

	return changed
}

func (r *runner) cmdUpdate(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpUpdate(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		r.errorf("run '%s update --help' for usage\n", r.binaryName())
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	opts, err := parseUpdateFlags(args[1:])
	if err != nil {
		r.errorf("error: %v\n", err)
		r.errorf("run '%s update --help' for usage\n", r.binaryName())
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if !applyUpdate(syn, opts) {
		if r.jsonOutput {
			r.jsonOut(syn)
			return 0
		}
		r.printf("No changes for synapse #%d\n", id)
		return 0
	}

	if err := store.Update(syn); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}

	r.printf("Updated synapse #%d: %s\n", syn.ID, syn.Title)
	r.printSynapseDetailed(syn)
	return 0
}

func (r *runner) cmdBlock(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpBlock(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		r.errorf("run '%s block --help' for usage\n", r.binaryName())
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	var blockers []int
	for i := 1; i < len(args); i++ {
		if args[i] == "--by" && i+1 < len(args) {
			i++
			n, perr := strconv.Atoi(args[i])
			if perr != nil {
				r.errorf("error: invalid blocker ID: %s\n", args[i])
				return 1
			}
			blockers = append(blockers, n)
		} else {
			r.errorf("error: unknown flag: %s\n", args[i])
			r.errorf("run '%s block --help' for usage\n", r.binaryName())
			return 1
		}
	}

	if len(blockers) == 0 {
		r.errorln("error: at least one --by <id> required")
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	opts := &updateOpts{addBlockers: blockers}
	applyUpdate(syn, opts)

	if err := store.Update(syn); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}
	r.printf("Synapse #%d now blocked by: %v\n", syn.ID, syn.BlockedBy)
	return 0
}

func (r *runner) cmdUnblock(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpUnblock(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		r.errorf("run '%s unblock --help' for usage\n", r.binaryName())
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	var fromIDs []int
	clearAll := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 >= len(args) {
				r.errorln("error: --from requires a value")
				return 1
			}
			i++
			n, perr := strconv.Atoi(args[i])
			if perr != nil {
				r.errorf("error: invalid blocker ID: %s\n", args[i])
				return 1
			}
			fromIDs = append(fromIDs, n)
		case "--all":
			clearAll = true
		default:
			r.errorf("error: unknown flag: %s\n", args[i])
			r.errorf("run '%s unblock --help' for usage\n", r.binaryName())
			return 1
		}
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	var opts *updateOpts
	if clearAll || (len(fromIDs) == 0 && !clearAll) {
		// No --from flags and no --all means: remove all (the implicit default,
		// since unblock with no args otherwise has no effect).
		empty := []int{}
		opts = &updateOpts{setBlockers: &empty}
	} else {
		opts = &updateOpts{removeBlockers: fromIDs}
	}
	applyUpdate(syn, opts)

	if err := store.Update(syn); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}
	if len(syn.BlockedBy) == 0 {
		r.printf("Synapse #%d unblocked (status: %s)\n", syn.ID, syn.Status)
	} else {
		r.printf("Synapse #%d still blocked by: %v\n", syn.ID, syn.BlockedBy)
	}
	return 0
}

func (r *runner) cmdNote(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpNote(r.binaryName()))
		return 0
	}

	if len(args) < 2 {
		r.errorln("error: usage: note <id> <text>...")
		r.errorf("run '%s note --help' for usage\n", r.binaryName())
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	text := strings.Join(args[1:], " ")
	if strings.TrimSpace(text) == "" {
		r.errorln("error: note text required")
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	syn.Notes = append(syn.Notes, text)
	if err := store.Update(syn); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}
	r.printf("Added note to synapse #%d (%d total)\n", syn.ID, len(syn.Notes))
	return 0
}

func (r *runner) cmdDone(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpDone(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required")
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if syn.Status == types.StatusDone {
		if r.jsonOutput {
			r.jsonOut(syn)
			return 0
		}
		r.printf("Synapse #%d is already done\n", syn.ID)
		return 0
	}

	syn.MarkDone()
	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(syn)
		return 0
	}

	r.printf("Completed synapse #%d: %s\n", syn.ID, syn.Title)
	return 0
}

func (r *runner) printSynapse(syn *types.Synapse) {
	statusIcon := statusToIcon(syn.Status)
	r.printf("%s [%s] #%d: %s\n", statusIcon, syn.Status, syn.ID, syn.Title)
	if syn.Priority != 0 {
		r.printf("   Priority: %d\n", syn.Priority)
	}
	if len(syn.Labels) > 0 {
		r.printf("   Labels: %s\n", strings.Join(syn.Labels, ", "))
	}
	if syn.Assignee != "" {
		r.printf("   Assignee: %s\n", syn.Assignee)
	}
	if len(syn.BlockedBy) > 0 {
		r.printf("   Blocked by: %v\n", syn.BlockedBy)
	}
	r.println()
}

func (r *runner) printSynapseDetailed(syn *types.Synapse) {
	r.printf("Synapse #%d\n", syn.ID)
	r.printf("  Title:       %s\n", syn.Title)
	r.printf("  Status:      %s %s\n", statusToIcon(syn.Status), syn.Status)
	if syn.Priority != 0 {
		r.printf("  Priority:    %d\n", syn.Priority)
	}
	if len(syn.Labels) > 0 {
		r.printf("  Labels:      %s\n", strings.Join(syn.Labels, ", "))
	}
	if syn.Description != "" {
		r.printf("  Description: %s\n", syn.Description)
	}
	if syn.Assignee != "" {
		r.printf("  Assignee:    %s\n", syn.Assignee)
	}
	if syn.ParentID > 0 {
		r.printf("  Parent:      #%d\n", syn.ParentID)
	}
	if len(syn.BlockedBy) > 0 {
		r.printf("  Blocked by:  %v\n", syn.BlockedBy)
	}
	r.printf("  Created:     %s\n", syn.CreatedAt.Format("2006-01-02 15:04:05"))
	r.printf("  Updated:     %s\n", syn.UpdatedAt.Format("2006-01-02 15:04:05"))
}

func statusToIcon(status types.Status) string {
	switch status {
	case types.StatusOpen:
		return "○"
	case types.StatusInProgress:
		return "◐"
	case types.StatusBlocked:
		return "◌"
	case types.StatusReview:
		return "◑"
	case types.StatusDone:
		return "●"
	default:
		return "?"
	}
}

func (r *runner) cmdDoneAll(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpAllDone(r.binaryName()))
		return 0
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	all := store.All()

	count := 0
	for _, syn := range all {
		if syn.Status != types.StatusDone {
			syn.MarkDone()
			count++
		}
	}

	if count == 0 {
		if r.jsonOutput {
			r.jsonOut(map[string]int{"count": 0})
			return 0
		}
		r.println("No tasks to mark as done")
		return 0
	}

	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(map[string]int{"count": count})
		return 0
	}

	r.printf("Marked %d task(s) as done\n", count)
	return 0
}

func (r *runner) cmdDelete(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpDelete(r.binaryName()))
		return 0
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}

	if len(args) > 0 && args[0] == "--all" {
		all := store.All()
		count := len(all)
		if count == 0 {
			if r.jsonOutput {
				r.jsonOut(map[string]int{"deleted": 0})
				return 0
			}
			r.println("No tasks to delete")
			return 0
		}

		if err := store.DeleteAll(); err != nil {
			r.errorf("error: %v\n", err)
			return 1
		}
		if code := r.saveStore(store); code != 0 {
			return code
		}

		if r.jsonOutput {
			r.jsonOut(map[string]int{"deleted": count})
			return 0
		}
		r.printf("Deleted all %d task(s)\n", count)
		return 0
	}

	if len(args) > 0 && args[0] == "--done" {
		count, err := store.DeleteByStatus(types.StatusDone)
		if err != nil {
			r.errorf("error: %v\n", err)
			return 1
		}

		if count == 0 {
			if r.jsonOutput {
				r.jsonOut(map[string]int{"deleted": 0})
				return 0
			}
			r.println("No completed tasks to delete")
			return 0
		}

		if code := r.saveStore(store); code != 0 {
			return code
		}

		if r.jsonOutput {
			r.jsonOut(map[string]int{"deleted": count})
			return 0
		}
		r.printf("Deleted %d completed task(s)\n", count)
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: synapse ID required (or use --all/--done to delete tasks)")
		return 1
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		r.errorf("error: invalid ID: %s\n", args[0])
		return 1
	}

	syn, err := store.Get(id)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	snapshot := *syn
	if err := store.Delete(id); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	if code := r.saveStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(&snapshot)
		return 0
	}
	r.printf("Deleted synapse #%d: %s\n", id, snapshot.Title)
	return 0
}

func (r *runner) cmdBreadcrumb(args []string) int {
	if hasHelpFlag(args) && (len(args) == 0 || (args[0] == "--help" || args[0] == "-h")) {
		r.println(helpBreadcrumb(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: subcommand required (set, get, list, delete)")
		r.errorf("run '%s breadcrumb --help' for usage\n", r.binaryName())
		return 1
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "set":
		return r.cmdBreadcrumbSet(subargs)
	case "get":
		return r.cmdBreadcrumbGet(subargs)
	case "list", "ls":
		return r.cmdBreadcrumbList(subargs)
	case "delete", "rm":
		return r.cmdBreadcrumbDelete(subargs)
	default:
		r.errorf("error: unknown breadcrumb subcommand: %s\n", subcmd)
		r.errorf("run '%s breadcrumb --help' for usage\n", r.binaryName())
		return 1
	}
}

func (r *runner) cmdBreadcrumbSet(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpBreadcrumbSet(r.binaryName()))
		return 0
	}

	if len(args) < 2 {
		r.errorln("error: key and value required")
		r.errorf("run '%s breadcrumb set --help' for usage\n", r.binaryName())
		return 1
	}

	key := args[0]
	var value string
	var taskID int

	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--task-id" && i+1 < len(args) {
			i++
			id, err := strconv.Atoi(args[i])
			if err != nil {
				r.errorf("error: invalid task ID: %s\n", args[i])
				return 1
			}
			taskID = id
		} else if !strings.HasPrefix(arg, "--") {
			if value == "" {
				value = arg
			} else {
				value = value + " " + arg
			}
		} else {
			r.errorf("error: unknown flag: %s\n", arg)
			return 1
		}
		i++
	}

	if value == "" {
		r.errorln("error: value required")
		return 1
	}

	store, code := r.getBreadcrumbStore()
	if code != 0 {
		return code
	}
	_, err := store.Set(key, value, taskID)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if code := r.saveBreadcrumbStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		b, _ := store.Get(key)
		r.jsonOut(b)
		return 0
	}

	r.printf("Set breadcrumb: %s = %s\n", key, value)
	if taskID > 0 {
		r.printf("  Linked to task #%d\n", taskID)
	}
	return 0
}

func (r *runner) cmdBreadcrumbGet(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpBreadcrumbGet(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: key required")
		r.errorf("run '%s breadcrumb get --help' for usage\n", r.binaryName())
		return 1
	}

	key := args[0]
	store, code := r.getBreadcrumbStore()
	if code != 0 {
		return code
	}

	b, found := store.Get(key)
	if !found {
		r.errorf("breadcrumb not found: %s\n", key)
		return 1
	}

	if r.jsonOutput {
		r.jsonOut(b)
		return 0
	}

	r.println(b.Value)
	return 0
}

func (r *runner) cmdBreadcrumbList(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpBreadcrumbList(r.binaryName()))
		return 0
	}

	var prefix string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			prefix = arg
		}
	}

	store, code := r.getBreadcrumbStore()
	if code != 0 {
		return code
	}
	breadcrumbs := store.List(prefix)

	if r.jsonOutput {
		r.jsonOut(breadcrumbs)
		return 0
	}

	if len(breadcrumbs) == 0 {
		if prefix != "" {
			r.printf("No breadcrumbs found with prefix: %s\n", prefix)
		} else {
			r.println("No breadcrumbs found")
		}
		return 0
	}

	r.printf("Breadcrumbs (%d):\n\n", len(breadcrumbs))
	for _, b := range breadcrumbs {
		value := b.Value
		if len(value) > 50 {
			value = value[:47] + "..."
		}
		r.printf("  %s = %s\n", b.Key, value)
		if b.TaskID > 0 {
			r.printf("    Task: #%d\n", b.TaskID)
		}
	}
	return 0
}

func (r *runner) cmdBreadcrumbDelete(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpBreadcrumbDelete(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: key required")
		r.errorf("run '%s breadcrumb delete --help' for usage\n", r.binaryName())
		return 1
	}

	key := args[0]
	store, code := r.getBreadcrumbStore()
	if code != 0 {
		return code
	}

	if !store.Delete(key) {
		r.errorf("breadcrumb not found: %s\n", key)
		return 1
	}

	if code := r.saveBreadcrumbStore(store); code != 0 {
		return code
	}

	if r.jsonOutput {
		r.jsonOut(map[string]string{"deleted": key})
		return 0
	}
	r.printf("Deleted breadcrumb: %s\n", key)
	return 0
}

func (r *runner) cmdSkill(args []string) int {
	if hasHelpFlag(args) && (len(args) == 0 || (args[0] == "--help" || args[0] == "-h")) {
		r.println(helpSkill(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: subcommand required (install, uninstall, list, update, show)")
		r.errorf("run '%s skill --help' for usage\n", r.binaryName())
		return 1
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "install":
		return r.cmdSkillInstall(subargs)
	case "uninstall":
		return r.cmdSkillUninstall(subargs)
	case "list", "ls":
		return r.cmdSkillList(subargs)
	case "update":
		return r.cmdSkillUpdate(subargs)
	case "show":
		return r.cmdSkillShow(subargs)
	default:
		r.errorf("error: unknown skill subcommand: %s\n", subcmd)
		r.errorf("run '%s skill --help' for usage\n", r.binaryName())
		return 1
	}
}

func (r *runner) cmdSkillInstall(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpSkillInstall(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: agent name required")
		r.errorf("available agents: %s\n", strings.Join(skill.AgentNames(), ", "))
		return 1
	}

	agentName := args[0]
	level := skill.LevelProject

	for i := 1; i < len(args); i++ {
		if args[i] == "--level" && i+1 < len(args) {
			i++
			switch args[i] {
			case "user":
				level = skill.LevelUser
			case "project":
				level = skill.LevelProject
			default:
				r.errorf("error: invalid level: %s (must be 'user' or 'project')\n", args[i])
				return 1
			}
		}
	}

	if err := skill.Install(agentName, level, Version); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	cfg, _ := skill.GetAgent(agentName)
	target := skill.TargetPath(cfg, level)
	r.printf("Installed synapse skill for %s (%s)\n", cfg.DisplayName, level)
	r.printf("  Path: %s\n", target)
	return 0
}

func (r *runner) cmdSkillUninstall(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpSkillUninstall(r.binaryName()))
		return 0
	}

	if len(args) == 0 {
		r.errorln("error: agent name required")
		r.errorf("available agents: %s\n", strings.Join(skill.AgentNames(), ", "))
		return 1
	}

	agentName := args[0]
	level := skill.LevelProject

	for i := 1; i < len(args); i++ {
		if args[i] == "--level" && i+1 < len(args) {
			i++
			switch args[i] {
			case "user":
				level = skill.LevelUser
			case "project":
				level = skill.LevelProject
			default:
				r.errorf("error: invalid level: %s (must be 'user' or 'project')\n", args[i])
				return 1
			}
		}
	}

	if err := skill.Uninstall(agentName, level); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}

	cfg, _ := skill.GetAgent(agentName)
	r.printf("Uninstalled synapse skill for %s (%s)\n", cfg.DisplayName, level)
	return 0
}

func (r *runner) cmdSkillList(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpSkillList(r.binaryName()))
		return 0
	}

	infos := skill.List()

	r.println("Synapse Skill Installations:")
	r.println()

	lastAgent := ""
	for _, info := range infos {
		if info.Agent != lastAgent {
			cfg, _ := skill.GetAgent(info.Agent)
			r.printf("  %s (%s):\n", cfg.DisplayName, info.Agent)
			lastAgent = info.Agent
		}

		status := "not installed"
		if info.Installed {
			if info.Version != "" {
				status = fmt.Sprintf("v%s", info.Version)
			} else {
				status = "installed"
			}
		}

		r.printf("    %-8s %s\n", info.Level+":", status)
	}
	return 0
}

func (r *runner) cmdSkillUpdate(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpSkillUpdate(r.binaryName()))
		return 0
	}

	level := skill.LevelProject
	var agentName string

	for i := 0; i < len(args); i++ {
		if args[i] == "--level" && i+1 < len(args) {
			i++
			switch args[i] {
			case "user":
				level = skill.LevelUser
			case "project":
				level = skill.LevelProject
			default:
				r.errorf("error: invalid level: %s (must be 'user' or 'project')\n", args[i])
				return 1
			}
		} else if !strings.HasPrefix(args[i], "--") {
			agentName = args[i]
		}
	}

	if agentName != "" {
		if !skill.IsInstalled(agentName, level) {
			r.errorf("error: %s is not installed at %s level\n", agentName, level)
			return 1
		}
		if err := skill.Update(agentName, level, Version); err != nil {
			r.errorf("error: %v\n", err)
			return 1
		}
		cfg, _ := skill.GetAgent(agentName)
		r.printf("Updated synapse skill for %s (%s) to v%s\n", cfg.DisplayName, level, Version)
		return 0
	}

	updated, err := skill.UpdateAll(Version)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	if len(updated) == 0 {
		r.println("No installed skills to update")
		return 0
	}
	r.printf("Updated %d installation(s) to v%s:\n", len(updated), Version)
	for _, name := range updated {
		r.printf("  %s\n", name)
	}
	return 0
}

func (r *runner) cmdSkillShow(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpSkillShow(r.binaryName()))
		return 0
	}

	content, err := skill.ShowSkillContent(Version)
	if err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	fmt.Fprint(r.stdout, content)
	return 0
}

// DoctorReport is the structured result of `synapse doctor`.
type DoctorReport struct {
	Version           string `json:"version"`
	BinaryPath        string `json:"binary_path"`
	BinaryName        string `json:"binary_name"`
	SynOnPath         bool   `json:"syn_on_path"`
	SynapseOnPath     bool   `json:"synapse_on_path"`
	SynPath           string `json:"syn_path,omitempty"`
	SynapsePath       string `json:"synapse_path,omitempty"`
	StoreInitialized  bool   `json:"store_initialized"`
	StoreDir          string `json:"store_dir"`
	TaskCount         int    `json:"task_count"`
	ReadyCount        int    `json:"ready_count"`
	BreadcrumbCount   int    `json:"breadcrumb_count"`
	GitRepoDetected   bool   `json:"git_repo_detected"`
	MCPServerCommand  string `json:"mcp_server_command"`
}

func (r *runner) cmdDoctor(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpDoctor(r.binaryName()))
		return 0
	}

	report := DoctorReport{
		Version:    Version,
		BinaryName: r.binaryName(),
		StoreDir:   storage.DefaultDir,
	}

	// Resolve binary path
	if exe, err := os.Executable(); err == nil {
		report.BinaryPath = exe
	} else if len(r.argv) > 0 {
		report.BinaryPath = r.argv[0]
	}

	// Check both names on PATH
	if p, err := lookupOnPath("syn"); err == nil {
		report.SynOnPath = true
		report.SynPath = p
	}
	if p, err := lookupOnPath("synapse"); err == nil {
		report.SynapseOnPath = true
		report.SynapsePath = p
	}

	// Project store status
	if _, err := os.Stat(filepath.Join(storage.DefaultDir, "memory.jsonl")); err == nil {
		report.StoreInitialized = true
		store := storage.NewJSONLStore(storage.DefaultDir)
		if err := store.Load(); err == nil {
			all := store.All()
			report.TaskCount = len(all)
			report.ReadyCount = len(store.Ready())
		}
		bcStore := storage.NewBreadcrumbStore(storage.DefaultDir)
		if err := bcStore.Load(); err == nil {
			report.BreadcrumbCount = len(bcStore.List(""))
		}
	}

	// Git detection
	if _, err := os.Stat(".git"); err == nil {
		report.GitRepoDetected = true
	}

	// MCP command suggestion
	report.MCPServerCommand = report.BinaryName + " serve"

	if r.jsonOutput {
		r.jsonOut(report)
		return 0
	}

	r.printf("Synapse v%s\n", report.Version)
	r.printf("  Binary:        %s\n", report.BinaryPath)

	aliasMark := func(ok bool) string {
		if ok {
			return "✓"
		}
		return "✗"
	}
	r.printf("  Aliases:       syn (%s)  synapse (%s)\n",
		aliasMark(report.SynOnPath), aliasMark(report.SynapseOnPath))
	if !report.SynOnPath || !report.SynapseOnPath {
		r.println("                 (Tip: re-run 'go install ./...' from the synapse repo to install both)")
	}

	if report.StoreInitialized {
		r.printf("  Project store: %s/memory.jsonl (%d task(s), %d ready)\n",
			report.StoreDir, report.TaskCount, report.ReadyCount)
		r.printf("  Breadcrumbs:   %s/breadcrumbs.jsonl (%d entries)\n",
			report.StoreDir, report.BreadcrumbCount)
	} else {
		r.printf("  Project store: not initialized (run '%s init')\n", report.BinaryName)
	}

	if report.GitRepoDetected {
		r.println("  Git:           detected")
	} else {
		r.println("  Git:           not a git repository")
	}
	r.printf("  MCP server:    available via '%s'\n", report.MCPServerCommand)
	return 0
}

// lookupOnPath is a tiny indirection so we don't import os/exec just for one call.
func lookupOnPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("not found")
}

func (r *runner) cmdServe(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpServe(r.binaryName()))
		return 0
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	bcStore, code := r.getBreadcrumbStore()
	if code != 0 {
		return code
	}
	server := mcp.NewServer(store, bcStore)
	if err := server.Run(); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	return 0
}

func (r *runner) cmdView(args []string) int {
	if hasHelpFlag(args) {
		r.println(helpView(r.binaryName()))
		return 0
	}

	port := 8080
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			i++
			p, err := strconv.Atoi(args[i])
			if err != nil {
				r.errorf("error: invalid port: %s\n", args[i])
				return 1
			}
			port = p
		}
	}

	store, code := r.getStore()
	if code != 0 {
		return code
	}
	server := view.NewServer(store, port)
	r.printf("Starting visualization at http://localhost:%d\n", port)
	if err := server.Run(); err != nil {
		r.errorf("error: %v\n", err)
		return 1
	}
	return 0
}
