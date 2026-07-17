package store

// Sync-side message helpers: the write and reconciliation surface the IMAP
// engine drives (flag refresh, expunge reconciliation, UID anchors). Split
// from messages.go so each file stays within the constitution's size cap
// (Rule 10) as the read surface grew its header projection.

import (
	"context"
	"database/sql"
	"fmt"
)

// FlagState is the flag-bearing subset of a cached message row, keyed by
// server UID. refreshFlags diffs the server's answer against it so only
// rows whose flags actually changed pay an UPDATE.
type FlagState struct {
	Flags     string
	IsRead    bool
	IsFlagged bool
}

// FolderFlags returns the cached flag state of every message in a folder,
// keyed by UID. One indexed scan of three small columns — cheap even for
// large folders.
func (db *DB) FolderFlags(ctx context.Context, folderID int64) (map[uint32]FlagState, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT uid, COALESCE(flags,''), is_read, is_flagged
		FROM messages WHERE folder_id = ?`, folderID)
	if err != nil {
		return nil, fmt.Errorf("store: folder flags: %w", err)
	}
	defer rows.Close()
	out := make(map[uint32]FlagState)
	for rows.Next() {
		var uid uint32
		var fs FlagState
		if err := rows.Scan(&uid, &fs.Flags, &fs.IsRead, &fs.IsFlagged); err != nil {
			return nil, fmt.Errorf("store: scan folder flags: %w", err)
		}
		out[uid] = fs
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: folder flags: %w", err)
	}
	return out, nil
}

// DeleteMessageByUID removes the cached row for a server-expunged UID.
func (db *DB) DeleteMessageByUID(ctx context.Context, folderID int64, uid uint32) error {
	_, err := db.sql.ExecContext(ctx,
		`DELETE FROM messages WHERE folder_id = ? AND uid = ?`, folderID, uid)
	if err != nil {
		return fmt.Errorf("store: delete message uid %d: %w", uid, err)
	}
	return nil
}

// UpdateMessageFlags applies server-reported flag state to a cached row,
// keyed by (folder, UID). Rows not yet cached are ignored.
func (db *DB) UpdateMessageFlags(ctx context.Context, m *Message) error {
	_, err := db.sql.ExecContext(ctx, `
		UPDATE messages SET flags = ?, is_read = ?, is_flagged = ?
		WHERE folder_id = ? AND uid = ?`,
		m.Flags, m.IsRead, m.IsFlagged, m.FolderID, m.UID)
	if err != nil {
		return fmt.Errorf("store: update flags uid %d: %w", m.UID, err)
	}
	return nil
}

// DeleteMessagesNotIn removes cached rows for a folder whose UIDs are not
// in live — the reconciliation step after a server expunge.
func (db *DB) DeleteMessagesNotIn(ctx context.Context, folderID int64, live map[uint32]bool) error {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT id, uid FROM messages WHERE folder_id = ? AND uid > 0`, folderID)
	if err != nil {
		return fmt.Errorf("store: list uids: %w", err)
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var uid uint32
		if err := rows.Scan(&id, &uid); err != nil {
			rows.Close()
			return fmt.Errorf("store: scan uid: %w", err)
		}
		if !live[uid] {
			stale = append(stale, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("store: list uids: %w", err)
	}
	rows.Close()
	if len(stale) == 0 {
		return nil
	}
	// One transaction for the whole reconciliation: after a large server-side
	// expunge, per-row DELETEs each paid their own implicit transaction (and
	// fsync) on mobile storage.
	return db.tx(ctx, func(t *sql.Tx) error {
		for _, id := range stale {
			if _, err := t.ExecContext(ctx,
				`DELETE FROM messages WHERE id = ?`, id); err != nil {
				return fmt.Errorf("store: delete stale message %d: %w", id, err)
			}
		}
		return nil
	})
}

// HighestUID returns the largest cached UID in a folder, or 0 when the
// folder is empty. The sync engine fetches everything above it.
func (db *DB) HighestUID(ctx context.Context, folderID int64) (uint32, error) {
	var uid uint32
	err := db.sql.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(uid),0) FROM messages WHERE folder_id = ?`,
		folderID).Scan(&uid)
	if err != nil {
		return 0, fmt.Errorf("store: highest uid: %w", err)
	}
	return uid, nil
}
