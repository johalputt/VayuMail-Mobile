package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OutboxEntry is one queued outbound message. RawMessage holds the full
// RFC 5322 bytes exactly as they will be handed to the SMTP server, so a
// send survives app restarts without recomposition.
type OutboxEntry struct {
	ID          int64
	AccountID   int64
	RawMessage  []byte
	RetryCount  int
	MaxRetries  int
	NextAttempt time.Time
	LastError   string
	QueuedAt    time.Time
}

// EnqueueOutbox queues a raw message for sending and returns its ID.
func (db *DB) EnqueueOutbox(ctx context.Context, accountID int64, raw []byte) (int64, error) {
	now := time.Now().UTC()
	res, err := db.sql.ExecContext(ctx, `
		INSERT INTO outbox (account_id, raw_message, next_attempt, queued_at)
		VALUES (?,?,?,?)`,
		accountID, raw, now.Unix(), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("store: enqueue outbox: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: enqueue outbox id: %w", err)
	}
	return id, nil
}

// GetOutboxEntry returns one queued entry by ID.
func (db *DB) GetOutboxEntry(ctx context.Context, id int64) (OutboxEntry, error) {
	row := db.sql.QueryRowContext(ctx, `
		SELECT id, account_id, raw_message, retry_count, max_retries,
			next_attempt, COALESCE(last_error,''), queued_at
		FROM outbox WHERE id = ?`, id)
	e, err := scanOutbox(row)
	if errors.Is(err, sql.ErrNoRows) {
		return OutboxEntry{}, ErrNotFound
	}
	if err != nil {
		return OutboxEntry{}, fmt.Errorf("store: get outbox %d: %w", id, err)
	}
	return e, nil
}

// DueOutbox returns entries whose next attempt time has passed and whose
// retry budget is not exhausted, oldest first.
func (db *DB) DueOutbox(ctx context.Context, now time.Time) ([]OutboxEntry, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, account_id, raw_message, retry_count, max_retries,
			next_attempt, COALESCE(last_error,''), queued_at
		FROM outbox
		WHERE next_attempt <= ? AND retry_count < max_retries
		ORDER BY queued_at`, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("store: due outbox: %w", err)
	}
	defer rows.Close()

	var out []OutboxEntry
	for rows.Next() {
		e, err := scanOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan outbox: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: due outbox: %w", err)
	}
	return out, nil
}

func scanOutbox(row interface{ Scan(...any) error }) (OutboxEntry, error) {
	var e OutboxEntry
	var next, queued int64
	err := row.Scan(&e.ID, &e.AccountID, &e.RawMessage, &e.RetryCount,
		&e.MaxRetries, &next, &e.LastError, &queued)
	if err != nil {
		return OutboxEntry{}, err
	}
	e.NextAttempt = time.Unix(next, 0).UTC()
	e.QueuedAt = time.Unix(queued, 0).UTC()
	return e, nil
}

// MarkOutboxSent removes a successfully sent entry from the queue.
func (db *DB) MarkOutboxSent(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: mark outbox sent: %w", err)
	}
	return nil
}

// MarkOutboxFailed records a failed attempt: increments the retry count,
// stores the error, and schedules the next attempt. Entries whose retry
// count reaches max_retries become dead letters — they stay queryable but
// DueOutbox never returns them again.
func (db *DB) MarkOutboxFailed(ctx context.Context, id int64, sendErr error, nextAttempt time.Time) error {
	_, err := db.sql.ExecContext(ctx, `
		UPDATE outbox SET retry_count = retry_count + 1,
			last_error = ?, next_attempt = ?
		WHERE id = ?`, sendErr.Error(), nextAttempt.Unix(), id)
	if err != nil {
		return fmt.Errorf("store: mark outbox failed: %w", err)
	}
	return nil
}

// DeadLetters returns entries that exhausted their retry budget. The UI
// surfaces these so a send never fails silently.
func (db *DB) DeadLetters(ctx context.Context, accountID int64) ([]OutboxEntry, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, account_id, raw_message, retry_count, max_retries,
			next_attempt, COALESCE(last_error,''), queued_at
		FROM outbox
		WHERE account_id = ? AND retry_count >= max_retries
		ORDER BY queued_at`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: dead letters: %w", err)
	}
	defer rows.Close()

	var out []OutboxEntry
	for rows.Next() {
		e, err := scanOutbox(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan dead letter: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: dead letters: %w", err)
	}
	return out, nil
}
