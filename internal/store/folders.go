package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Folder is one IMAP mailbox as known locally, including the sync anchors
// (UIDVALIDITY, HIGHESTMODSEQ) that the sync engine uses to resume
// incrementally after a disconnect.
type Folder struct {
	ID            int64
	AccountID     int64
	Name          string // display name, last path segment
	FullName      string // full IMAP path, e.g. "Archive/2026"
	Delimiter     string
	UIDValidity   uint32
	HighestModSeq uint64
	IsInbox       bool
	IsSent        bool
	IsDrafts      bool
	IsTrash       bool
	IsArchive     bool
}

const folderCols = `id, account_id, name, full_name, COALESCE(delimiter,''),
	COALESCE(uid_validity,0), COALESCE(highest_modseq,0),
	is_inbox, is_sent, is_drafts, is_trash, is_archive`

func scanFolder(row interface{ Scan(...any) error }) (Folder, error) {
	var f Folder
	err := row.Scan(&f.ID, &f.AccountID, &f.Name, &f.FullName, &f.Delimiter,
		&f.UIDValidity, &f.HighestModSeq, &f.IsInbox, &f.IsSent, &f.IsDrafts,
		&f.IsTrash, &f.IsArchive)
	return f, err
}

// UpsertFolder inserts or updates a folder keyed by (account, full name)
// and returns its local ID.
func (db *DB) UpsertFolder(ctx context.Context, f *Folder) (int64, error) {
	res := db.sql.QueryRowContext(ctx, `
		INSERT INTO folders (account_id, name, full_name, delimiter,
			uid_validity, highest_modseq, is_inbox, is_sent, is_drafts,
			is_trash, is_archive)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id, full_name) DO UPDATE SET
			name = excluded.name,
			delimiter = excluded.delimiter,
			is_inbox = excluded.is_inbox,
			is_sent = excluded.is_sent,
			is_drafts = excluded.is_drafts,
			is_trash = excluded.is_trash,
			is_archive = excluded.is_archive
		RETURNING id`,
		f.AccountID, f.Name, f.FullName, f.Delimiter, f.UIDValidity,
		f.HighestModSeq, f.IsInbox, f.IsSent, f.IsDrafts, f.IsTrash,
		f.IsArchive)
	var id int64
	if err := res.Scan(&id); err != nil {
		return 0, fmt.Errorf("store: upsert folder %q: %w", f.FullName, err)
	}
	f.ID = id
	return id, nil
}

// ListFolders returns all folders for an account, inbox first, then
// alphabetically by full name.
func (db *DB) ListFolders(ctx context.Context, accountID int64) ([]Folder, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+folderCols+` FROM folders WHERE account_id = ?
		ORDER BY is_inbox DESC, full_name`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list folders: %w", err)
	}
	defer rows.Close()

	var out []Folder
	for rows.Next() {
		f, err := scanFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan folder: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list folders: %w", err)
	}
	return out, nil
}

// GetFolderByFullName returns the folder for (accountID, fullName).
func (db *DB) GetFolderByFullName(ctx context.Context, accountID int64, fullName string) (Folder, error) {
	row := db.sql.QueryRowContext(ctx, `
		SELECT `+folderCols+` FROM folders
		WHERE account_id = ? AND full_name = ?`, accountID, fullName)
	f, err := scanFolder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Folder{}, ErrNotFound
	}
	if err != nil {
		return Folder{}, fmt.Errorf("store: get folder %q: %w", fullName, err)
	}
	return f, nil
}

// SetFolderSyncState records the UIDVALIDITY and HIGHESTMODSEQ anchors
// after a sync pass. If UIDVALIDITY changed on the server, the caller must
// clear cached messages first (see ClearFolderMessages).
func (db *DB) SetFolderSyncState(ctx context.Context, folderID int64, uidValidity uint32, highestModSeq uint64) error {
	_, err := db.sql.ExecContext(ctx, `
		UPDATE folders SET uid_validity = ?, highest_modseq = ?
		WHERE id = ?`, uidValidity, highestModSeq, folderID)
	if err != nil {
		return fmt.Errorf("store: set folder sync state: %w", err)
	}
	return nil
}

// ClearFolderMessages deletes all cached messages in a folder. Used when
// the server reports a new UIDVALIDITY, which invalidates every cached UID.
func (db *DB) ClearFolderMessages(ctx context.Context, folderID int64) error {
	_, err := db.sql.ExecContext(ctx,
		`DELETE FROM messages WHERE folder_id = ?`, folderID)
	if err != nil {
		return fmt.Errorf("store: clear folder messages: %w", err)
	}
	return nil
}
