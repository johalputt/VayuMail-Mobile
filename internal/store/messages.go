package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Message is one mail message as cached locally. BodyText and BodyHTML may
// be empty when only the envelope has been fetched so far.
type Message struct {
	ID             int64
	AccountID      int64
	FolderID       int64
	UID            uint32
	ThreadID       string
	MessageID      string
	InReplyTo      string
	FromAddr       string
	FromName       string
	ToAddrs        string // comma-separated RFC 5322 addresses
	CcAddrs        string
	Subject        string
	Snippet        string
	BodyText       string
	BodyHTML       string
	HasAttachments bool
	PGPStatus      string // "", "signed", "encrypted", "signed+encrypted"
	IsRead         bool
	IsFlagged      bool
	IsDeleted      bool
	Date           time.Time
	SizeBytes      int64
	Flags          string // space-separated raw IMAP flags
}

const messageCols = `id, account_id, folder_id, uid, COALESCE(thread_id,''),
	COALESCE(message_id,''), COALESCE(in_reply_to,''), from_addr,
	COALESCE(from_name,''), to_addrs, COALESCE(cc_addrs,''),
	COALESCE(subject,''), COALESCE(snippet,''), COALESCE(body_text,''),
	COALESCE(body_html,''), has_attachments, COALESCE(pgp_status,''),
	is_read, is_flagged, is_deleted, date, COALESCE(size_bytes,0),
	COALESCE(flags,'')`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var date int64
	err := row.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.ThreadID,
		&m.MessageID, &m.InReplyTo, &m.FromAddr, &m.FromName, &m.ToAddrs,
		&m.CcAddrs, &m.Subject, &m.Snippet, &m.BodyText, &m.BodyHTML,
		&m.HasAttachments, &m.PGPStatus, &m.IsRead, &m.IsFlagged,
		&m.IsDeleted, &date, &m.SizeBytes, &m.Flags)
	if err != nil {
		return Message{}, err
	}
	m.Date = time.Unix(date, 0).UTC()
	return m, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpsertMessage inserts or updates a message keyed by
// (account, folder, UID) and returns its local ID.
func (db *DB) UpsertMessage(ctx context.Context, m *Message) (int64, error) {
	res := db.sql.QueryRowContext(ctx, `
		INSERT INTO messages (account_id, folder_id, uid, thread_id,
			message_id, in_reply_to, from_addr, from_name, to_addrs,
			cc_addrs, subject, snippet, body_text, body_html,
			has_attachments, pgp_status, is_read, is_flagged, is_deleted,
			date, size_bytes, flags)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id, folder_id, uid) DO UPDATE SET
			thread_id = excluded.thread_id,
			snippet = excluded.snippet,
			body_text = CASE WHEN excluded.body_text != ''
				THEN excluded.body_text ELSE messages.body_text END,
			body_html = CASE WHEN excluded.body_html != ''
				THEN excluded.body_html ELSE messages.body_html END,
			has_attachments = excluded.has_attachments,
			pgp_status = excluded.pgp_status,
			is_read = excluded.is_read,
			is_flagged = excluded.is_flagged,
			is_deleted = excluded.is_deleted,
			flags = excluded.flags
		RETURNING id`,
		m.AccountID, m.FolderID, m.UID, nullable(m.ThreadID),
		nullable(m.MessageID), nullable(m.InReplyTo), m.FromAddr,
		nullable(m.FromName), m.ToAddrs, nullable(m.CcAddrs),
		nullable(m.Subject), nullable(m.Snippet), m.BodyText, m.BodyHTML,
		m.HasAttachments, nullable(m.PGPStatus), m.IsRead, m.IsFlagged,
		m.IsDeleted, m.Date.Unix(), m.SizeBytes, nullable(m.Flags))
	var id int64
	if err := res.Scan(&id); err != nil {
		return 0, fmt.Errorf("store: upsert message uid %d: %w", m.UID, err)
	}
	m.ID = id
	return id, nil
}

// GetMessageByUID returns the cached message with the given server UID
// in a folder. Used by the notifier to describe a freshly synced message.
func (db *DB) GetMessageByUID(ctx context.Context, folderID int64, uid uint32) (Message, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+messageCols+` FROM messages WHERE folder_id = ? AND uid = ?`,
		folderID, uid)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("store: get message uid %d: %w", uid, err)
	}
	return m, nil
}

// GetMessage returns the message with the given local ID.
func (db *DB) GetMessage(ctx context.Context, id int64) (Message, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+messageCols+` FROM messages WHERE id = ?`, id)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("store: get message %d: %w", id, err)
	}
	return m, nil
}

// ListMessages returns messages in a folder, newest first, excluding
// locally deleted rows. offset/limit page the result for the virtualized
// list.
func (db *DB) ListMessages(ctx context.Context, folderID int64, offset, limit int) ([]Message, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+messageCols+` FROM messages
		WHERE folder_id = ? AND is_deleted = 0
		ORDER BY date DESC, uid DESC LIMIT ? OFFSET ?`,
		folderID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list messages: %w", err)
	}
	return collectMessages(rows)
}

// ListThread returns all messages sharing a thread ID across folders of
// one account, oldest first — the order a conversation reads in.
func (db *DB) ListThread(ctx context.Context, accountID int64, threadID string) ([]Message, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+messageCols+` FROM messages
		WHERE account_id = ? AND thread_id = ? AND is_deleted = 0
		ORDER BY date ASC, uid ASC`, accountID, threadID)
	if err != nil {
		return nil, fmt.Errorf("store: list thread: %w", err)
	}
	return collectMessages(rows)
}

func collectMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate messages: %w", err)
	}
	return out, nil
}

// CountMessages returns the number of non-deleted messages in a folder.
func (db *DB) CountMessages(ctx context.Context, folderID int64) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM messages
		WHERE folder_id = ? AND is_deleted = 0`, folderID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count messages: %w", err)
	}
	return n, nil
}

// UnreadCount returns the number of unread, non-deleted messages in a
// folder. Feeds the drawer badges and is cheap thanks to the partial
// unread index.
func (db *DB) UnreadCount(ctx context.Context, folderID int64) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM messages
		WHERE folder_id = ? AND is_read = 0 AND is_deleted = 0`,
		folderID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: unread count: %w", err)
	}
	return n, nil
}

// SetRead sets or clears the read flag on a message.
func (db *DB) SetRead(ctx context.Context, id int64, read bool) error {
	return db.setMessageBool(ctx, id, "is_read", read)
}

// SetFlagged sets or clears the flagged (starred) state on a message.
func (db *DB) SetFlagged(ctx context.Context, id int64, flagged bool) error {
	return db.setMessageBool(ctx, id, "is_flagged", flagged)
}

// SetDeleted marks or unmarks a message as locally deleted. The row stays
// until the server confirms the expunge; unmarking implements undo.
func (db *DB) SetDeleted(ctx context.Context, id int64, deleted bool) error {
	return db.setMessageBool(ctx, id, "is_deleted", deleted)
}

func (db *DB) setMessageBool(ctx context.Context, id int64, col string, v bool) error {
	// col is one of three compile-time constants above, never user input.
	res, err := db.sql.ExecContext(ctx,
		`UPDATE messages SET `+col+` = ? WHERE id = ?`, v, id)
	if err != nil {
		return fmt.Errorf("store: set %s: %w", col, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set %s: %w", col, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveMessage reassigns a message to another folder locally. UID is set to
// 0 until the next sync learns the UID assigned by the server.
func (db *DB) MoveMessage(ctx context.Context, id, destFolderID int64) error {
	res, err := db.sql.ExecContext(ctx, `
		UPDATE messages SET folder_id = ?, uid = 0 WHERE id = ?`,
		destFolderID, id)
	if err != nil {
		return fmt.Errorf("store: move message %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: move message %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
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
	for _, id := range stale {
		if _, err := db.sql.ExecContext(ctx,
			`DELETE FROM messages WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: delete stale message %d: %w", id, err)
		}
	}
	return nil
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
