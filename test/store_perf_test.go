package test

import (
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// TestListMessagesHeaderProjection verifies the list read path returns the
// snippet (what the row renders) but not the full body — the projection
// that stops every 200-row inbox reload from dragging up to 512 KB per row
// into the snapshot. The full body is still reachable via GetMessage.
func TestListMessagesHeaderProjection(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	m := seedMessage(t, db, a, f, 1, "Hello", false)

	// Give the row a real body so the projection has something to omit.
	m.BodyText = "the full message body that the list must not load"
	if _, err := db.UpsertMessage(t.Context(), &m); err != nil {
		t.Fatalf("set body: %v", err)
	}

	list, err := db.ListMessages(t.Context(), f.ID, 0, 200)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if list[0].Snippet == "" {
		t.Error("snippet must be present for the row line")
	}
	if list[0].BodyText != "" {
		t.Errorf("list projection leaked the body: %q", list[0].BodyText)
	}

	full, err := db.GetMessage(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if full.BodyText == "" {
		t.Error("GetMessage must still return the full body")
	}
}

// TestFolderFlagsDiff verifies the flag-refresh helper reads back exactly
// the flag state it stored, so the diff-aware refresh only updates rows
// whose flags actually changed.
func TestFolderFlagsDiff(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	m := seedMessage(t, db, a, f, 7, "Flagged", false)

	flags, err := db.FolderFlags(t.Context(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	fs, ok := flags[m.UID]
	if !ok {
		t.Fatalf("uid %d missing from folder flags", m.UID)
	}
	if fs.IsRead {
		t.Error("freshly seeded message should be unread")
	}

	// Mark read via the server-flag path, then confirm the diff sees it.
	m.IsRead = true
	m.Flags = "\\Seen"
	if err := db.UpdateMessageFlags(t.Context(), &m); err != nil {
		t.Fatal(err)
	}
	flags, err = db.FolderFlags(t.Context(), f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !flags[m.UID].IsRead || flags[m.UID].Flags != "\\Seen" {
		t.Errorf("flag update not reflected: %+v", flags[m.UID])
	}

	// A no-op re-apply of the same flags must compare equal (the diff would
	// skip the UPDATE and the FTS rewrite).
	same := store.FlagState{Flags: "\\Seen", IsRead: true, IsFlagged: false}
	if flags[m.UID] != same {
		t.Errorf("expected stable flag state, got %+v", flags[m.UID])
	}
}
