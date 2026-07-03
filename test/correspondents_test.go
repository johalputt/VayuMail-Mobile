package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

func TestCorrespondentEmails(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	ctx := t.Context()

	// Three senders, one repeated; newest date should sort first.
	senders := []struct {
		uid  uint32
		from string
		min  int
	}{
		{1, "carol@johal.in", 1},
		{2, "bob@johal.in", 2},
		{3, "carol@johal.in", 3}, // repeat, newest
		{4, "dave@johal.in", 4},
	}
	for _, s := range senders {
		m := store.Message{
			AccountID: a.ID, FolderID: f.ID, UID: s.uid,
			MessageID: fmt.Sprintf("<%d@x>", s.uid),
			FromAddr:  s.from, ToAddrs: a.EmailAddress,
			Subject: "m", Date: time.Date(2026, 6, 1, 10, s.min, 0, 0, time.UTC),
		}
		if _, err := db.UpsertMessage(ctx, &m); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.CorrespondentEmails(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct addresses only.
	if len(got) != 3 {
		t.Fatalf("got %d distinct senders, want 3: %v", len(got), got)
	}
	// Most-recent sender (dave, minute 4) first; carol (last at minute 3) second.
	if got[0] != "dave@johal.in" || got[1] != "carol@johal.in" {
		t.Fatalf("order = %v, want dave, carol, bob", got)
	}
}
