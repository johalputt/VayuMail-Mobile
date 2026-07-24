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
	// PGPSigVerified is a TRANSIENT (never persisted) flag set at
	// decrypt-on-display time when the message's OpenPGP signature actually
	// verified against a known key (audit M17). It is the ONLY basis for
	// claiming the sender is cryptographically authenticated: a bare
	// PGPStatus of "signed" comes from the MIME structure alone and is NOT
	// verified, so it must never be shown as trusted on its own.
	PGPSigVerified bool
	IsRead         bool
	IsFlagged      bool
	IsDeleted      bool
	Date           time.Time
	SizeBytes      int64
	Flags          string // space-separated raw IMAP flags
	// HasTrackers marks detected tracking pixels/links (ADR-0007).
	HasTrackers bool
	// IsList marks mailing-list/newsletter traffic (List-Id present).
	IsList bool
	// ListUnsubscribe is the raw List-Unsubscribe header value.
	ListUnsubscribe string
	// SnoozeUntil hides the message from lists until this time (zero =
	// not snoozed).
	SnoozeUntil time.Time
	// Attachments is a JSON array of {"filename","contentType"} entries
	// captured at parse time, in part order.
	Attachments string
}

const messageCols = `id, account_id, folder_id, uid, COALESCE(thread_id,''),
	COALESCE(message_id,''), COALESCE(in_reply_to,''), from_addr,
	COALESCE(from_name,''), to_addrs, COALESCE(cc_addrs,''),
	COALESCE(subject,''), COALESCE(snippet,''), COALESCE(body_text,''),
	COALESCE(body_html,''), has_attachments, COALESCE(pgp_status,''),
	is_read, is_flagged, is_deleted, date, COALESCE(size_bytes,0),
	COALESCE(flags,''), has_trackers, is_list,
	COALESCE(list_unsubscribe,''), snooze_until, COALESCE(attachments,'')`

// messageHeaderCols mirrors messageCols but projects empty bodies. List
// surfaces render only the snippet, yet the shared column list used to drag
// every row's full body_text/body_html (up to 512 KB each) into each
// 200-row snapshot rebuild. Same column count and order, so scanMessage
// works unchanged; the thread/detail paths keep the full projection.
const messageHeaderCols = `id, account_id, folder_id, uid,
	COALESCE(thread_id,''), COALESCE(message_id,''),
	COALESCE(in_reply_to,''), from_addr, COALESCE(from_name,''), to_addrs,
	COALESCE(cc_addrs,''), COALESCE(subject,''), COALESCE(snippet,''),
	'', '', has_attachments, COALESCE(pgp_status,''),
	is_read, is_flagged, is_deleted, date, COALESCE(size_bytes,0),
	COALESCE(flags,''), has_trackers, is_list,
	COALESCE(list_unsubscribe,''), snooze_until, COALESCE(attachments,'')`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var date, snooze int64
	err := row.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.ThreadID,
		&m.MessageID, &m.InReplyTo, &m.FromAddr, &m.FromName, &m.ToAddrs,
		&m.CcAddrs, &m.Subject, &m.Snippet, &m.BodyText, &m.BodyHTML,
		&m.HasAttachments, &m.PGPStatus, &m.IsRead, &m.IsFlagged,
		&m.IsDeleted, &date, &m.SizeBytes, &m.Flags, &m.HasTrackers,
		&m.IsList, &m.ListUnsubscribe, &snooze, &m.Attachments)
	if err != nil {
		return Message{}, err
	}
	m.Date = time.Unix(date, 0).UTC()
	if snooze > 0 {
		m.SnoozeUntil = time.Unix(snooze, 0).UTC()
	}
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
			date, size_bytes, flags, has_trackers, is_list,
			list_unsubscribe)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
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
			flags = excluded.flags,
			has_trackers = excluded.has_trackers,
			is_list = excluded.is_list,
			list_unsubscribe = excluded.list_unsubscribe,
			attachments = CASE WHEN excluded.attachments IS NOT NULL
				THEN excluded.attachments ELSE messages.attachments END
		RETURNING id`,
		m.AccountID, m.FolderID, m.UID, nullable(m.ThreadID),
		nullable(m.MessageID), nullable(m.InReplyTo), m.FromAddr,
		nullable(m.FromName), m.ToAddrs, nullable(m.CcAddrs),
		nullable(m.Subject), nullable(m.Snippet), m.BodyText, m.BodyHTML,
		m.HasAttachments, nullable(m.PGPStatus), m.IsRead, m.IsFlagged,
		m.IsDeleted, m.Date.Unix(), m.SizeBytes, nullable(m.Flags),
		m.HasTrackers, m.IsList, nullable(m.ListUnsubscribe),
		nullable(m.Attachments))
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
// list. Rows carry header data only (empty bodies) — the list renders the
// snippet, and the thread view loads full bodies via ListThread/GetMessage.
func (db *DB) ListMessages(ctx context.Context, folderID int64, offset, limit int) ([]Message, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+messageHeaderCols+` FROM messages
		WHERE folder_id = ? AND is_deleted = 0
			AND snooze_until <= unixepoch()
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

// CorrespondentEmails returns the distinct sender addresses seen in cached
// mail (most-recent first), up to limit. Used to bulk-discover PGP keys for
// people you actually correspond with.
func (db *DB) CorrespondentEmails(ctx context.Context, limit int) ([]string, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT from_addr FROM messages
		WHERE from_addr != ''
		GROUP BY from_addr
		ORDER BY MAX(date) DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: correspondent emails: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, fmt.Errorf("store: scan correspondent: %w", err)
		}
		out = append(out, addr)
	}
	return out, rows.Err()
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

// DeleteLocalMessage hard-deletes one cached message by local ID. Used
// after a server-side move or permanent delete succeeds: the message
// leaves the source folder locally, and the next sync of the destination
// folder brings it back with its real server UID. This avoids reusing a
// placeholder UID, which would collide with the UNIQUE(account, folder,
// uid) constraint when several messages move to the same folder.
func (db *DB) DeleteLocalMessage(ctx context.Context, id int64) error {
	res, err := db.sql.ExecContext(ctx,
		`DELETE FROM messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete local message %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete local message %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
