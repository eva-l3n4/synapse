package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/swiftj/synapse/pkg/types"
)

// runIn executes the CLI inside dir with the given args (excluding argv[0]).
// Returns exit code, stdout, stderr.
func runIn(t *testing.T, dir string, argv ...string) (int, string, string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	full := append([]string{"synapse"}, argv...)
	code := Run(full, stdout, stderr)
	return code, stdout.String(), stderr.String()
}

// initStore creates a fresh .synapse directory in a temp dir and returns its path.
func initStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	code, _, stderr := runIn(t, dir, "init")
	if code != 0 {
		t.Fatalf("init failed (code %d): %s", code, stderr)
	}
	return dir
}

// TestHelpFlagOnEverySubcommand verifies that --help works on every command
// and subcommand without erroring. This is the regression test for the
// reported "synapse add --help → unknown flag" bug.
func TestHelpFlagOnEverySubcommand(t *testing.T) {
	dir := initStore(t)

	// command, args-after-cmd
	cases := [][]string{
		{"init"},
		{"add"},
		{"list"},
		{"ls"},
		{"ready"},
		{"get"},
		{"claim"},
		{"update"},
		{"edit"},
		{"block"},
		{"unblock"},
		{"note"},
		{"done"},
		{"all-done"},
		{"delete"},
		{"rm"},
		{"breadcrumb"},
		{"bc"},
		{"breadcrumb", "set"},
		{"breadcrumb", "get"},
		{"breadcrumb", "list"},
		{"breadcrumb", "delete"},
		{"skill"},
		{"skill", "install"},
		{"skill", "uninstall"},
		{"skill", "list"},
		{"skill", "update"},
		{"skill", "show"},
		{"doctor"},
		{"serve"},
		{"view"},
	}

	for _, flag := range []string{"--help", "-h"} {
		for _, c := range cases {
			name := strings.Join(c, " ") + " " + flag
			t.Run(name, func(t *testing.T) {
				args := append(append([]string{}, c...), flag)
				code, stdout, stderr := runIn(t, dir, args...)
				if code != 0 {
					t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr)
				}
				if stdout == "" {
					t.Errorf("expected non-empty stdout for help text")
				}
				if strings.Contains(strings.ToLower(stderr), "error") {
					t.Errorf("expected no error in stderr, got: %s", stderr)
				}
				// Help text should mention "Usage:" or describe the command
				if !strings.Contains(stdout, "Usage") && !strings.Contains(stdout, "usage") {
					t.Errorf("help output missing 'Usage' section: %s", stdout)
				}
			})
		}
	}
}

func TestAddPriorityFlag(t *testing.T) {
	dir := initStore(t)

	code, _, stderr := runIn(t, dir, "--json", "add", "Big task", "--priority", "7")
	if code != 0 {
		t.Fatalf("add failed (code %d): %s", code, stderr)
	}

	// Read back via list --json
	code, stdout, _ := runIn(t, dir, "--json", "list")
	if code != 0 {
		t.Fatalf("list failed (code %d)", code)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Priority != 7 {
		t.Errorf("priority = %d, want 7", tasks[0].Priority)
	}
	if tasks[0].Title != "Big task" {
		t.Errorf("title = %q, want %q", tasks[0].Title, "Big task")
	}
}

func TestAddLabelFlagRepeatable(t *testing.T) {
	dir := initStore(t)

	code, _, stderr := runIn(t, dir,
		"--json", "add", "Tagged task",
		"--label", "bug",
		"--label", "security",
		"--label", "auth",
	)
	if code != 0 {
		t.Fatalf("add failed (code %d): %s", code, stderr)
	}

	code, stdout, _ := runIn(t, dir, "--json", "list")
	if code != 0 {
		t.Fatalf("list failed (code %d)", code)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	want := []string{"bug", "security", "auth"}
	if len(tasks[0].Labels) != len(want) {
		t.Fatalf("labels = %v, want %v", tasks[0].Labels, want)
	}
	for i, label := range want {
		if tasks[0].Labels[i] != label {
			t.Errorf("label[%d] = %q, want %q", i, tasks[0].Labels[i], label)
		}
	}
}

func TestAddCombinedNewFlags(t *testing.T) {
	dir := initStore(t)

	code, stdout, stderr := runIn(t, dir,
		"--json", "add", "Combined",
		"--priority", "5",
		"--label", "feature",
		"--assignee", "@coder",
	)
	if code != 0 {
		t.Fatalf("add failed (code %d): %s", code, stderr)
	}

	var task types.Synapse
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if task.Priority != 5 {
		t.Errorf("priority = %d, want 5", task.Priority)
	}
	if len(task.Labels) != 1 || task.Labels[0] != "feature" {
		t.Errorf("labels = %v, want [feature]", task.Labels)
	}
	if task.Assignee != "@coder" {
		t.Errorf("assignee = %q, want @coder", task.Assignee)
	}
}

func TestList_LabelFilter(t *testing.T) {
	dir := initStore(t)
	bug1 := addTask(t, dir, "Bug 1", "--label", "bug")
	addTask(t, dir, "Feature", "--label", "feature")
	bug2 := addTask(t, dir, "Bug 2", "--label", "bug")

	code, stdout, stderr := runIn(t, dir, "--json", "list", "--label", "bug")
	if code != 0 {
		t.Fatalf("list failed (code %d): %s", code, stderr)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	got := map[int]bool{}
	for _, task := range tasks {
		if !containsString(task.Labels, "bug") {
			t.Fatalf("task #%d labels = %v, want bug label", task.ID, task.Labels)
		}
		got[task.ID] = true
	}
	if !got[bug1] || !got[bug2] {
		t.Errorf("filtered tasks = %v, want IDs %d and %d", got, bug1, bug2)
	}
}

func TestList_AssigneeFilter(t *testing.T) {
	dir := initStore(t)
	qa1 := addTask(t, dir, "QA 1", "--assignee", "@qa")
	addTask(t, dir, "Coder", "--assignee", "@coder")
	qa2 := addTask(t, dir, "QA 2", "--assignee", "@qa")

	code, stdout, stderr := runIn(t, dir, "--json", "list", "--assignee", "@qa")
	if code != 0 {
		t.Fatalf("list failed (code %d): %s", code, stderr)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	got := map[int]bool{}
	for _, task := range tasks {
		if task.Assignee != "@qa" {
			t.Fatalf("task #%d assignee = %q, want @qa", task.ID, task.Assignee)
		}
		got[task.ID] = true
	}
	if !got[qa1] || !got[qa2] {
		t.Errorf("filtered tasks = %v, want IDs %d and %d", got, qa1, qa2)
	}
}

func TestList_UnknownFlag(t *testing.T) {
	dir := initStore(t)
	code, _, stderr := runIn(t, dir, "list", "--bogus")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--bogus") || !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected unknown flag error for --bogus, got: %s", stderr)
	}
}

func TestClaim_DoneTask(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Done claim")
	if code, _, stderr := runIn(t, dir, "done", strconv.Itoa(id)); code != 0 {
		t.Fatalf("done failed: %s", stderr)
	}

	code, _, stderr := runIn(t, dir, "claim", strconv.Itoa(id))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "already done") {
		t.Errorf("expected already done error, got: %s", stderr)
	}
}

func TestClaim_BlockedTask(t *testing.T) {
	dir := initStore(t)
	blocker := addTask(t, dir, "Blocker")
	target := addTask(t, dir, "Blocked", "--blocks", strconv.Itoa(blocker))

	code, _, stderr := runIn(t, dir, "claim", strconv.Itoa(target))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "blocked") {
		t.Errorf("expected blocked error, got: %s", stderr)
	}
}

func TestDone_AlreadyDone(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Already done")
	if code, _, stderr := runIn(t, dir, "done", strconv.Itoa(id)); code != 0 {
		t.Fatalf("first done failed: %s", stderr)
	}

	code, _, stderr := runIn(t, dir, "done", strconv.Itoa(id))
	if code != 0 {
		t.Fatalf("expected exit 0 on repeated done, got %d: %s", code, stderr)
	}
	if task := getTask(t, dir, id); task.Status != types.StatusDone {
		t.Fatalf("status = %q, want done", task.Status)
	}
}

func TestAdd_CommaBlocks(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "A")
	b := addTask(t, dir, "B")

	code, stdout, stderr := runIn(t, dir, "--json", "add", "Blocked", "--blocks", strconv.Itoa(a)+","+strconv.Itoa(b))
	if code != 0 {
		t.Fatalf("add failed (code %d): %s", code, stderr)
	}

	var task types.Synapse
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(task.BlockedBy) != 2 || task.BlockedBy[0] != a || task.BlockedBy[1] != b {
		t.Fatalf("blocked_by = %v, want [%d %d]", task.BlockedBy, a, b)
	}
	if task.Status != types.StatusBlocked {
		t.Fatalf("status = %q, want blocked", task.Status)
	}
}

func TestReady_AssigneeFilter(t *testing.T) {
	dir := initStore(t)
	qa1 := addTask(t, dir, "QA 1", "--assignee", "@qa")
	addTask(t, dir, "Coder", "--assignee", "@coder")
	qa2 := addTask(t, dir, "QA 2", "--assignee", "@qa")

	code, stdout, stderr := runIn(t, dir, "--json", "ready", "--assignee", "@qa")
	if code != 0 {
		t.Fatalf("ready failed (code %d): %s", code, stderr)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	got := map[int]bool{}
	for _, task := range tasks {
		if task.Assignee != "@qa" {
			t.Fatalf("task #%d assignee = %q, want @qa", task.ID, task.Assignee)
		}
		got[task.ID] = true
	}
	if !got[qa1] || !got[qa2] {
		t.Errorf("filtered tasks = %v, want IDs %d and %d", got, qa1, qa2)
	}
}

func TestReady_LabelFilter(t *testing.T) {
	dir := initStore(t)
	bug1 := addTask(t, dir, "Bug 1", "--label", "bug")
	addTask(t, dir, "Feature", "--label", "feature")
	bug2 := addTask(t, dir, "Bug 2", "--label", "bug")

	code, stdout, stderr := runIn(t, dir, "--json", "ready", "--label", "bug")
	if code != 0 {
		t.Fatalf("ready failed (code %d): %s", code, stderr)
	}

	var tasks []types.Synapse
	if err := json.Unmarshal([]byte(stdout), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	got := map[int]bool{}
	for _, task := range tasks {
		if !containsString(task.Labels, "bug") {
			t.Fatalf("task #%d labels = %v, want bug label", task.ID, task.Labels)
		}
		got[task.ID] = true
	}
	if !got[bug1] || !got[bug2] {
		t.Errorf("filtered tasks = %v, want IDs %d and %d", got, bug1, bug2)
	}
}

func TestReady_UnknownFlag(t *testing.T) {
	dir := initStore(t)
	code, _, stderr := runIn(t, dir, "ready", "--bogus")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--bogus") || !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected unknown flag error for --bogus, got: %s", stderr)
	}
}

func TestUnknownCommandSuggestsHelp(t *testing.T) {
	dir := initStore(t)

	code, _, stderr := runIn(t, dir, "frobnicate")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown command")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "--help") {
		t.Errorf("expected stderr to suggest --help, got: %s", stderr)
	}
}

func TestUnknownAddFlagSuggestsHelp(t *testing.T) {
	dir := initStore(t)

	code, _, stderr := runIn(t, dir, "add", "Title", "--bogus")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown flag")
	}
	if !strings.Contains(stderr, "--bogus") {
		t.Errorf("stderr should mention the offending flag: %s", stderr)
	}
	if !strings.Contains(stderr, "--help") {
		t.Errorf("stderr should point to --help: %s", stderr)
	}
}

func TestDoctorJSONReport(t *testing.T) {
	dir := initStore(t)

	// Add a task and a breadcrumb so the report has interesting numbers
	if code, _, _ := runIn(t, dir, "add", "Doctor task"); code != 0 {
		t.Fatalf("add failed")
	}
	if code, _, _ := runIn(t, dir, "bc", "set", "session.test", "value"); code != 0 {
		t.Fatalf("bc set failed")
	}

	code, stdout, stderr := runIn(t, dir, "--json", "doctor")
	if code != 0 {
		t.Fatalf("doctor failed (code %d): %s", code, stderr)
	}

	var report DoctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}

	if report.Version != Version {
		t.Errorf("version = %q, want %q", report.Version, Version)
	}
	if !report.StoreInitialized {
		t.Error("store should be marked initialized")
	}
	if report.TaskCount != 1 {
		t.Errorf("task_count = %d, want 1", report.TaskCount)
	}
	if report.BreadcrumbCount != 1 {
		t.Errorf("breadcrumb_count = %d, want 1", report.BreadcrumbCount)
	}
}

func TestDoctorTextOutput(t *testing.T) {
	dir := initStore(t)

	code, stdout, stderr := runIn(t, dir, "doctor")
	if code != 0 {
		t.Fatalf("doctor failed (code %d): %s", code, stderr)
	}

	mustContain := []string{
		"Synapse v",
		"Aliases:",
		"Project store:",
		"MCP server:",
	}
	for _, want := range mustContain {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q\n%s", want, stdout)
		}
	}
}

func TestDoctorBeforeInit(t *testing.T) {
	dir := t.TempDir()

	code, stdout, stderr := runIn(t, dir, "doctor")
	if code != 0 {
		t.Fatalf("doctor should still succeed when uninitialized (code %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "not initialized") {
		t.Errorf("expected 'not initialized' message, got: %s", stdout)
	}
}

func TestVersionCommand(t *testing.T) {
	dir := initStore(t)
	for _, cmd := range []string{"version", "--version", "-v"} {
		t.Run(cmd, func(t *testing.T) {
			code, stdout, _ := runIn(t, dir, cmd)
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			if !strings.Contains(stdout, Version) {
				t.Errorf("expected version %s in output: %s", Version, stdout)
			}
		})
	}
}

func TestVersionJSON(t *testing.T) {
	dir := initStore(t)
	code, stdout, _ := runIn(t, dir, "--json", "version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var v map[string]string
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v["version"] != Version {
		t.Errorf("version = %q, want %q", v["version"], Version)
	}
}

func TestBinaryNameDetection(t *testing.T) {
	cases := []struct {
		argv0    string
		wantName string
	}{
		{"synapse", "synapse"},
		{"syn", "syn"},
		{"/usr/local/bin/synapse", "synapse"},
		{"/Users/foo/go/bin/syn", "syn"},
		{"weird-name", "synapse"}, // fallback
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.argv0), func(t *testing.T) {
			r := &runner{argv: []string{tc.argv0}}
			if got := r.binaryName(); got != tc.wantName {
				t.Errorf("binaryName(%q) = %q, want %q", tc.argv0, got, tc.wantName)
			}
		})
	}
}

// TestHelpFunctionsNonEmpty verifies every helpXxx() returns non-empty text
// that mentions "Usage". Pure-function test, no IO.
func TestHelpFunctionsNonEmpty(t *testing.T) {
	helpers := map[string]func(string) string{
		"helpInit":              helpInit,
		"helpAdd":               helpAdd,
		"helpList":              helpList,
		"helpReady":             helpReady,
		"helpGet":               helpGet,
		"helpClaim":             helpClaim,
		"helpUpdate":            helpUpdate,
		"helpBlock":             helpBlock,
		"helpUnblock":           helpUnblock,
		"helpNote":              helpNote,
		"helpDone":              helpDone,
		"helpAllDone":           helpAllDone,
		"helpDelete":            helpDelete,
		"helpBreadcrumb":        helpBreadcrumb,
		"helpBreadcrumbSet":     helpBreadcrumbSet,
		"helpBreadcrumbGet":     helpBreadcrumbGet,
		"helpBreadcrumbList":    helpBreadcrumbList,
		"helpBreadcrumbDelete":  helpBreadcrumbDelete,
		"helpSkill":             helpSkill,
		"helpSkillInstall":      helpSkillInstall,
		"helpSkillUninstall":    helpSkillUninstall,
		"helpSkillList":         helpSkillList,
		"helpSkillUpdate":       helpSkillUpdate,
		"helpSkillShow":         helpSkillShow,
		"helpDoctor":            helpDoctor,
		"helpServe":             helpServe,
		"helpView":              helpView,
	}
	for name, fn := range helpers {
		t.Run(name, func(t *testing.T) {
			out := fn("synapse")
			if out == "" {
				t.Error("returned empty string")
			}
			if !strings.Contains(out, "Usage") {
				t.Errorf("missing Usage section: %s", out)
			}
		})
	}
}

// addTask is a tiny test helper that creates a task and returns its ID.
func addTask(t *testing.T, dir, title string, extra ...string) int {
	t.Helper()
	args := append([]string{"--json", "add", title}, extra...)
	code, stdout, stderr := runIn(t, dir, args...)
	if code != 0 {
		t.Fatalf("add %q failed (code %d): %s", title, code, stderr)
	}
	var task types.Synapse
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("unmarshal add output: %v", err)
	}
	return task.ID
}

// getTask reads a task back via `synapse get --json`.
func getTask(t *testing.T, dir string, id int) types.Synapse {
	t.Helper()
	code, stdout, stderr := runIn(t, dir, "--json", "get", strconv.Itoa(id))
	if code != 0 {
		t.Fatalf("get %d failed: %s", id, stderr)
	}
	var task types.Synapse
	if err := json.Unmarshal([]byte(stdout), &task); err != nil {
		t.Fatalf("unmarshal get output: %v", err)
	}
	return task
}

func TestUpdate_PatchesScalarFields(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Original")

	code, _, stderr := runIn(t, dir,
		"update", strconv.Itoa(id),
		"--title", "Renamed",
		"--description", "details here",
		"--priority", "8",
		"--assignee", "@qa",
		"--status", "in-progress",
	)
	if code != 0 {
		t.Fatalf("update failed (code %d): %s", code, stderr)
	}

	task := getTask(t, dir, id)
	if task.Title != "Renamed" {
		t.Errorf("title = %q, want Renamed", task.Title)
	}
	if task.Description != "details here" {
		t.Errorf("description = %q", task.Description)
	}
	if task.Priority != 8 {
		t.Errorf("priority = %d, want 8", task.Priority)
	}
	if task.Assignee != "@qa" {
		t.Errorf("assignee = %q, want @qa", task.Assignee)
	}
	if task.Status != types.StatusInProgress {
		t.Errorf("status = %q, want in-progress", task.Status)
	}
}

func TestUpdate_EditAlias(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Edit me")

	code, _, stderr := runIn(t, dir, "edit", strconv.Itoa(id), "--priority", "3")
	if code != 0 {
		t.Fatalf("edit alias failed: %s", stderr)
	}
	if getTask(t, dir, id).Priority != 3 {
		t.Errorf("edit alias did not apply priority")
	}
}

func TestUpdate_AddRemoveBlockers(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "Dep A")
	b := addTask(t, dir, "Dep B")
	c := addTask(t, dir, "Dep C")
	target := addTask(t, dir, "Target")

	// Add two blockers — task should auto-flip to blocked.
	code, _, stderr := runIn(t, dir,
		"update", strconv.Itoa(target),
		"--add-blocker", strconv.Itoa(a),
		"--add-blocker", strconv.Itoa(b),
	)
	if code != 0 {
		t.Fatalf("add-blocker failed: %s", stderr)
	}
	task := getTask(t, dir, target)
	if len(task.BlockedBy) != 2 || task.BlockedBy[0] != a || task.BlockedBy[1] != b {
		t.Errorf("blocked_by = %v, want [%d %d]", task.BlockedBy, a, b)
	}
	if task.Status != types.StatusBlocked {
		t.Errorf("status = %q, want blocked (auto-flip)", task.Status)
	}

	// Add a third — should be appended without duplicating.
	runIn(t, dir,
		"update", strconv.Itoa(target),
		"--add-blocker", strconv.Itoa(b), // duplicate, should be no-op
		"--add-blocker", strconv.Itoa(c),
	)
	task = getTask(t, dir, target)
	if len(task.BlockedBy) != 3 {
		t.Errorf("blocked_by len = %d after dedup add, want 3", len(task.BlockedBy))
	}

	// Remove one.
	runIn(t, dir, "update", strconv.Itoa(target), "--remove-blocker", strconv.Itoa(a))
	task = getTask(t, dir, target)
	if len(task.BlockedBy) != 2 {
		t.Errorf("blocked_by len = %d after remove, want 2", len(task.BlockedBy))
	}

	// Set explicit list (replaces).
	runIn(t, dir, "update", strconv.Itoa(target), "--set-blockers", strconv.Itoa(c))
	task = getTask(t, dir, target)
	if len(task.BlockedBy) != 1 || task.BlockedBy[0] != c {
		t.Errorf("blocked_by = %v, want [%d]", task.BlockedBy, c)
	}

	// Clear all via empty set — status should auto-flip back to open.
	runIn(t, dir, "update", strconv.Itoa(target), "--set-blockers", "")
	task = getTask(t, dir, target)
	if len(task.BlockedBy) != 0 {
		t.Errorf("blocked_by = %v, want empty", task.BlockedBy)
	}
	if task.Status != types.StatusOpen {
		t.Errorf("status = %q, want open (auto-flip after clearing blockers)", task.Status)
	}
}

func TestUpdate_AddRemoveLabels(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Labeled", "--label", "bug")

	// Add two more.
	runIn(t, dir,
		"update", strconv.Itoa(id),
		"--add-label", "security",
		"--add-label", "auth",
	)
	task := getTask(t, dir, id)
	if len(task.Labels) != 3 {
		t.Errorf("labels = %v, want 3", task.Labels)
	}

	// Add a duplicate — should be a no-op.
	runIn(t, dir, "update", strconv.Itoa(id), "--add-label", "bug")
	task = getTask(t, dir, id)
	if len(task.Labels) != 3 {
		t.Errorf("labels = %v, duplicate add should be no-op", task.Labels)
	}

	// Remove one.
	runIn(t, dir, "update", strconv.Itoa(id), "--remove-label", "auth")
	task = getTask(t, dir, id)
	for _, l := range task.Labels {
		if l == "auth" {
			t.Errorf("auth label should have been removed")
		}
	}

	// Replace all via comma-separated.
	runIn(t, dir, "update", strconv.Itoa(id), "--set-labels", "feature,docs")
	task = getTask(t, dir, id)
	if len(task.Labels) != 2 || task.Labels[0] != "feature" || task.Labels[1] != "docs" {
		t.Errorf("labels = %v, want [feature docs]", task.Labels)
	}

	// Clear all via empty.
	runIn(t, dir, "update", strconv.Itoa(id), "--set-labels", "")
	task = getTask(t, dir, id)
	if len(task.Labels) != 0 {
		t.Errorf("labels = %v, want empty", task.Labels)
	}
}

func TestUpdate_NoFlagsIsNoOp(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Idle")

	code, stdout, _ := runIn(t, dir, "update", strconv.Itoa(id))
	if code != 0 {
		t.Fatal("update with no flags should succeed")
	}
	if !strings.Contains(stdout, "No changes") {
		t.Errorf("expected 'No changes' message, got: %s", stdout)
	}
}

func TestUpdate_InvalidStatus(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Bad status")

	code, _, stderr := runIn(t, dir, "update", strconv.Itoa(id), "--status", "frobnicated")
	if code == 0 {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(stderr, "invalid status") {
		t.Errorf("expected 'invalid status' in stderr, got: %s", stderr)
	}
}

func TestUpdate_UnknownFlag(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Test")

	code, _, stderr := runIn(t, dir, "update", strconv.Itoa(id), "--bogus", "x")
	if code == 0 {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected 'unknown flag' in stderr, got: %s", stderr)
	}
}

func TestBlockCommand(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "A")
	b := addTask(t, dir, "B")
	target := addTask(t, dir, "Target")

	code, _, stderr := runIn(t, dir, "block", strconv.Itoa(target),
		"--by", strconv.Itoa(a), "--by", strconv.Itoa(b))
	if code != 0 {
		t.Fatalf("block failed: %s", stderr)
	}

	task := getTask(t, dir, target)
	if len(task.BlockedBy) != 2 {
		t.Errorf("blocked_by = %v, want 2 entries", task.BlockedBy)
	}
	if task.Status != types.StatusBlocked {
		t.Errorf("status = %q, want blocked", task.Status)
	}
}

func TestBlockCommandRequiresBy(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Solo")

	code, _, stderr := runIn(t, dir, "block", strconv.Itoa(id))
	if code == 0 {
		t.Fatal("expected error when --by missing")
	}
	if !strings.Contains(stderr, "--by") {
		t.Errorf("expected error to mention --by, got: %s", stderr)
	}
}

func TestUnblockCommand_Specific(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "A")
	b := addTask(t, dir, "B")
	target := addTask(t, dir, "T", "--blocks", strconv.Itoa(a), "--blocks", strconv.Itoa(b))

	code, _, _ := runIn(t, dir, "unblock", strconv.Itoa(target), "--from", strconv.Itoa(a))
	if code != 0 {
		t.Fatal("unblock --from failed")
	}
	task := getTask(t, dir, target)
	if len(task.BlockedBy) != 1 || task.BlockedBy[0] != b {
		t.Errorf("blocked_by = %v, want [%d]", task.BlockedBy, b)
	}
	// Still has a blocker, so still blocked.
	if task.Status != types.StatusBlocked {
		t.Errorf("status = %q, want still blocked", task.Status)
	}

	// Remove the last one — should flip to open.
	runIn(t, dir, "unblock", strconv.Itoa(target), "--from", strconv.Itoa(b))
	task = getTask(t, dir, target)
	if len(task.BlockedBy) != 0 {
		t.Errorf("blocked_by should be empty, got %v", task.BlockedBy)
	}
	if task.Status != types.StatusOpen {
		t.Errorf("status = %q, want open", task.Status)
	}
}

func TestUnblockCommand_All(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "A")
	b := addTask(t, dir, "B")
	c := addTask(t, dir, "C")
	target := addTask(t, dir, "T",
		"--blocks", strconv.Itoa(a),
		"--blocks", strconv.Itoa(b),
		"--blocks", strconv.Itoa(c),
	)

	code, _, _ := runIn(t, dir, "unblock", strconv.Itoa(target), "--all")
	if code != 0 {
		t.Fatal("unblock --all failed")
	}
	task := getTask(t, dir, target)
	if len(task.BlockedBy) != 0 {
		t.Errorf("blocked_by = %v, want empty after --all", task.BlockedBy)
	}
	if task.Status != types.StatusOpen {
		t.Errorf("status = %q, want open", task.Status)
	}
}

func TestUnblockCommand_NoArgsClearsAll(t *testing.T) {
	dir := initStore(t)
	a := addTask(t, dir, "A")
	target := addTask(t, dir, "T", "--blocks", strconv.Itoa(a))

	// `unblock <id>` with no flags should remove all (the implicit default).
	code, _, _ := runIn(t, dir, "unblock", strconv.Itoa(target))
	if code != 0 {
		t.Fatal("unblock with no flags failed")
	}
	if len(getTask(t, dir, target).BlockedBy) != 0 {
		t.Error("expected blockers cleared with bare 'unblock'")
	}
}

func TestNoteCommand(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Notes test")

	code, _, stderr := runIn(t, dir, "note", strconv.Itoa(id),
		"Discovered race", "condition", "in", "auth")
	if code != 0 {
		t.Fatalf("note failed: %s", stderr)
	}

	task := getTask(t, dir, id)
	if len(task.Notes) != 1 {
		t.Fatalf("notes len = %d, want 1", len(task.Notes))
	}
	if task.Notes[0] != "Discovered race condition in auth" {
		t.Errorf("note text = %q", task.Notes[0])
	}

	// Append a second note.
	runIn(t, dir, "note", strconv.Itoa(id), "Second note")
	if n := len(getTask(t, dir, id).Notes); n != 2 {
		t.Errorf("notes len = %d, want 2 after append", n)
	}
}

func TestNoteCommand_RequiresText(t *testing.T) {
	dir := initStore(t)
	id := addTask(t, dir, "Test")

	code, _, stderr := runIn(t, dir, "note", strconv.Itoa(id))
	if code == 0 {
		t.Fatal("expected error when note text missing")
	}
	if !strings.Contains(stderr, "usage") && !strings.Contains(stderr, "required") {
		t.Errorf("expected helpful error, got: %s", stderr)
	}
}

func TestParseIDList(t *testing.T) {
	tests := []struct {
		input    string
		want     []int
		wantErr  bool
	}{
		{"", []int{}, false},
		{"1", []int{1}, false},
		{"1,2,3", []int{1, 2, 3}, false},
		{" 1 , 2 , 3 ", []int{1, 2, 3}, false},
		{"1,,2", []int{1, 2}, false},
		{"abc", nil, true},
		{"1,abc,3", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseIDList(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("[%d] = %d, want %d", i, got[i], v)
				}
			}
		})
	}
}

func TestParseStringList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseStringList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
			}
			for i, v := range tt.want {
				if got[i] != v {
					t.Errorf("[%d] = %q, want %q", i, got[i], v)
				}
			}
		})
	}
}

func TestApplyUpdate_ClearsAssigneeAndDescription(t *testing.T) {
	syn := &types.Synapse{
		Title:       "T",
		Status:      types.StatusOpen,
		Description: "old",
		Assignee:    "@old",
	}
	empty := ""
	opts := &updateOpts{description: &empty, assignee: &empty}
	if !applyUpdate(syn, opts) {
		t.Fatal("expected changed=true")
	}
	if syn.Description != "" || syn.Assignee != "" {
		t.Errorf("description=%q assignee=%q, both should be empty", syn.Description, syn.Assignee)
	}
}

// TestPriorityShownInListOutput verifies the human-readable list output
// includes the priority when set.
func TestPriorityShownInListOutput(t *testing.T) {
	dir := initStore(t)

	if code, _, _ := runIn(t, dir, "add", "P-task", "--priority", "9"); code != 0 {
		t.Fatal("add failed")
	}

	code, stdout, _ := runIn(t, dir, "list")
	if code != 0 {
		t.Fatalf("list failed")
	}
	if !strings.Contains(stdout, "Priority: 9") {
		t.Errorf("expected 'Priority: 9' in list output, got: %s", stdout)
	}
}
