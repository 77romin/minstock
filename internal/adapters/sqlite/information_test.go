package sqlite

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestInformationReadStatesPersistAndCanBeReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reads.db")
	repo, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 230)
	for i := range ids {
		ids[i] = fmt.Sprintf("%064x", i)
		if _, err := repo.SetInformationRead(t.Context(), ids[i], true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.SetInformationRead(t.Context(), "bad-id", true); err == nil {
		t.Fatal("invalid identifier saved")
	}
	repo.Close()
	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	states, err := restarted.InformationReadStates(t.Context(), ids)
	if err != nil || len(states) != 230 {
		t.Fatalf("persisted read states: %d err=%v", len(states), err)
	}
	if at, err := restarted.SetInformationRead(t.Context(), ids[0], false); err != nil || !at.IsZero() {
		t.Fatalf("mark unread: %v", err)
	}
	states, err = restarted.InformationReadStates(t.Context(), ids[:2])
	if err != nil || len(states) != 1 || states[ids[1]].IsZero() {
		t.Fatal("reset erased other article state")
	}
}
