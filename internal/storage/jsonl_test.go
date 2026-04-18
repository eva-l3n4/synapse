package storage

import (
	"testing"

	"github.com/swiftj/synapse/pkg/types"
)

func setupStore(t *testing.T) (*JSONLStore, string) {
	t.Helper()

	dir := t.TempDir()
	store := NewJSONLStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	return store, dir
}

func createSynapse(t *testing.T, store *JSONLStore, title string) *types.Synapse {
	t.Helper()

	syn, err := store.Create(title)
	if err != nil {
		t.Fatalf("Create(%q) error: %v", title, err)
	}
	return syn
}

func markDoneAndUpdate(t *testing.T, store *JSONLStore, id int) {
	t.Helper()

	syn, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get(%d) error: %v", id, err)
	}
	syn.MarkDone()
	if err := store.Update(syn); err != nil {
		t.Fatalf("Update(%d) error: %v", id, err)
	}
}

func reloadStore(t *testing.T, dir string) *JSONLStore {
	t.Helper()

	store := NewJSONLStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return store
}

func TestReconcile_BlockedToOpen(t *testing.T) {
	store, dir := setupStore(t)

	a := createSynapse(t, store, "A")
	b := createSynapse(t, store, "B")
	b.Status = types.StatusBlocked
	b.BlockedBy = []int{a.ID}
	if err := store.Update(b); err != nil {
		t.Fatalf("Update(B) error: %v", err)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	store = reloadStore(t, dir)

	markDoneAndUpdate(t, store, a.ID)

	if transitioned := store.Reconcile(); transitioned != 1 {
		t.Fatalf("Reconcile() transitioned %d tasks, want 1", transitioned)
	}

	a, err := store.Get(a.ID)
	if err != nil {
		t.Fatalf("Get(A) error: %v", err)
	}
	if a.Status != types.StatusDone {
		t.Fatalf("A status = %q, want %q", a.Status, types.StatusDone)
	}

	b, err = store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get(B) error: %v", err)
	}
	if b.Status != types.StatusOpen {
		t.Fatalf("B status = %q, want %q", b.Status, types.StatusOpen)
	}
	if len(b.BlockedBy) != 0 {
		t.Fatalf("B blocked_by = %v, want empty", b.BlockedBy)
	}
	if len(b.ResolvedBlockers) != 1 || b.ResolvedBlockers[0] != a.ID {
		t.Fatalf("B resolved_blockers = %v, want [%d]", b.ResolvedBlockers, a.ID)
	}
}

func TestReconcile_PartialUnblock(t *testing.T) {
	store, dir := setupStore(t)

	a := createSynapse(t, store, "A")
	b := createSynapse(t, store, "B")
	c := createSynapse(t, store, "C")
	c.Status = types.StatusBlocked
	c.BlockedBy = []int{a.ID, b.ID}
	if err := store.Update(c); err != nil {
		t.Fatalf("Update(C) error: %v", err)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	store = reloadStore(t, dir)

	markDoneAndUpdate(t, store, a.ID)

	if transitioned := store.Reconcile(); transitioned != 0 {
		t.Fatalf("Reconcile() transitioned %d tasks, want 0", transitioned)
	}

	c, err := store.Get(c.ID)
	if err != nil {
		t.Fatalf("Get(C) error: %v", err)
	}
	if c.Status != types.StatusBlocked {
		t.Fatalf("C status = %q, want %q", c.Status, types.StatusBlocked)
	}
	if len(c.BlockedBy) != 1 || c.BlockedBy[0] != b.ID {
		t.Fatalf("C blocked_by = %v, want [%d]", c.BlockedBy, b.ID)
	}
	if len(c.ResolvedBlockers) != 1 || c.ResolvedBlockers[0] != a.ID {
		t.Fatalf("C resolved_blockers = %v, want [%d]", c.ResolvedBlockers, a.ID)
	}

	b, err = store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get(B) error: %v", err)
	}
	if b.Status != types.StatusOpen {
		t.Fatalf("B status = %q, want %q", b.Status, types.StatusOpen)
	}
}

func TestReconcile_CascadeChain(t *testing.T) {
	store, dir := setupStore(t)

	a := createSynapse(t, store, "A")
	b := createSynapse(t, store, "B")
	c := createSynapse(t, store, "C")

	b.Status = types.StatusBlocked
	b.BlockedBy = []int{a.ID}
	if err := store.Update(b); err != nil {
		t.Fatalf("Update(B) error: %v", err)
	}
	c.Status = types.StatusBlocked
	c.BlockedBy = []int{b.ID}
	if err := store.Update(c); err != nil {
		t.Fatalf("Update(C) error: %v", err)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	store = reloadStore(t, dir)

	markDoneAndUpdate(t, store, a.ID)

	if transitioned := store.Reconcile(); transitioned != 1 {
		t.Fatalf("first Reconcile() transitioned %d tasks, want 1", transitioned)
	}

	b, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get(B) after first reconcile error: %v", err)
	}
	if b.Status != types.StatusOpen {
		t.Fatalf("B status after first reconcile = %q, want %q", b.Status, types.StatusOpen)
	}

	c, err = store.Get(c.ID)
	if err != nil {
		t.Fatalf("Get(C) after first reconcile error: %v", err)
	}
	if c.Status != types.StatusBlocked {
		t.Fatalf("C status after first reconcile = %q, want %q", c.Status, types.StatusBlocked)
	}
	if len(c.BlockedBy) != 1 || c.BlockedBy[0] != b.ID {
		t.Fatalf("C blocked_by after first reconcile = %v, want [%d]", c.BlockedBy, b.ID)
	}
	if len(c.ResolvedBlockers) != 0 {
		t.Fatalf("C resolved_blockers after first reconcile = %v, want empty", c.ResolvedBlockers)
	}

	markDoneAndUpdate(t, store, b.ID)

	if transitioned := store.Reconcile(); transitioned != 1 {
		t.Fatalf("second Reconcile() transitioned %d tasks, want 1", transitioned)
	}

	c, err = store.Get(c.ID)
	if err != nil {
		t.Fatalf("Get(C) after second reconcile error: %v", err)
	}
	if c.Status != types.StatusOpen {
		t.Fatalf("C status after second reconcile = %q, want %q", c.Status, types.StatusOpen)
	}
	if len(c.BlockedBy) != 0 {
		t.Fatalf("C blocked_by after second reconcile = %v, want empty", c.BlockedBy)
	}
	if len(c.ResolvedBlockers) != 1 || c.ResolvedBlockers[0] != b.ID {
		t.Fatalf("C resolved_blockers after second reconcile = %v, want [%d]", c.ResolvedBlockers, b.ID)
	}
}

func TestReconcile_NoOpOnOpen(t *testing.T) {
	store, dir := setupStore(t)

	a := createSynapse(t, store, "A")
	b := createSynapse(t, store, "B")
	b.Status = types.StatusOpen
	b.BlockedBy = []int{a.ID}
	if err := store.Update(b); err != nil {
		t.Fatalf("Update(B) error: %v", err)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	store = reloadStore(t, dir)

	if transitioned := store.Reconcile(); transitioned != 0 {
		t.Fatalf("Reconcile() transitioned %d tasks, want 0", transitioned)
	}

	b, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get(B) error: %v", err)
	}
	if b.Status != types.StatusOpen {
		t.Fatalf("B status = %q, want %q", b.Status, types.StatusOpen)
	}
	if len(b.BlockedBy) != 1 || b.BlockedBy[0] != a.ID {
		t.Fatalf("B blocked_by = %v, want [%d]", b.BlockedBy, a.ID)
	}
	if len(b.ResolvedBlockers) != 0 {
		t.Fatalf("B resolved_blockers = %v, want empty", b.ResolvedBlockers)
	}
}

func TestReconcile_NoDuplicateResolved(t *testing.T) {
	store, dir := setupStore(t)

	a := createSynapse(t, store, "A")
	b := createSynapse(t, store, "B")
	b.Status = types.StatusBlocked
	b.BlockedBy = []int{a.ID}
	if err := store.Update(b); err != nil {
		t.Fatalf("Update(B) error: %v", err)
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	store = reloadStore(t, dir)

	markDoneAndUpdate(t, store, a.ID)

	if transitioned := store.Reconcile(); transitioned != 1 {
		t.Fatalf("first Reconcile() transitioned %d tasks, want 1", transitioned)
	}
	if transitioned := store.Reconcile(); transitioned != 0 {
		t.Fatalf("second Reconcile() transitioned %d tasks, want 0", transitioned)
	}

	b, err := store.Get(b.ID)
	if err != nil {
		t.Fatalf("Get(B) error: %v", err)
	}
	if len(b.ResolvedBlockers) != 1 || b.ResolvedBlockers[0] != a.ID {
		t.Fatalf("B resolved_blockers = %v, want single [%d]", b.ResolvedBlockers, a.ID)
	}
}
