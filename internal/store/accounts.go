package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// Account is one configured mail account. It intentionally carries no
// credential: KeystoreAlias names the platform-keystore entry that holds
// the secret (Constitutional Rule 6).
type Account struct {
	ID            int64
	DisplayName   string
	EmailAddress  string
	IMAPHost      string
	IMAPPort      int
	IMAPTLS       string // "tls" or "starttls"
	SMTPHost      string
	SMTPPort      int
	SMTPTLS       string // "tls" or "starttls"
	Username      string
	KeystoreAlias string
	CreatedAt     time.Time
	// PinnedSPKI is an optional base64 SHA-256 hash of the server's TLS
	// public key; when set, connections require a match (ADR-0008).
	PinnedSPKI string
}

const accountCols = `id, display_name, email_address, imap_host, imap_port,
	imap_tls, smtp_host, smtp_port, smtp_tls, username, keystore_alias,
	created_at, COALESCE(pinned_spki,'')`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var created int64
	err := row.Scan(&a.ID, &a.DisplayName, &a.EmailAddress, &a.IMAPHost,
		&a.IMAPPort, &a.IMAPTLS, &a.SMTPHost, &a.SMTPPort, &a.SMTPTLS,
		&a.Username, &a.KeystoreAlias, &created, &a.PinnedSPKI)
	if err != nil {
		return Account{}, err
	}
	a.CreatedAt = time.Unix(created, 0).UTC()
	return a, nil
}

// InsertAccount stores a new account row and returns its ID. The caller
// must have already placed the credential in the platform keystore under
// a.KeystoreAlias.
func (db *DB) InsertAccount(ctx context.Context, a *Account) (int64, error) {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	res, err := db.sql.ExecContext(ctx, `
		INSERT INTO accounts (display_name, email_address, imap_host,
			imap_port, imap_tls, smtp_host, smtp_port, smtp_tls, username,
			keystore_alias, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		a.DisplayName, a.EmailAddress, a.IMAPHost, a.IMAPPort, a.IMAPTLS,
		a.SMTPHost, a.SMTPPort, a.SMTPTLS, a.Username, a.KeystoreAlias,
		a.CreatedAt.Unix())
	if err != nil {
		return 0, fmt.Errorf("store: insert account: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: insert account id: %w", err)
	}
	a.ID = id
	return id, nil
}

// GetAccount returns the account with the given ID.
func (db *DB) GetAccount(ctx context.Context, id int64) (Account, error) {
	row := db.sql.QueryRowContext(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE id = ?`, id)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("store: get account %d: %w", id, err)
	}
	return a, nil
}

// ListAccounts returns all configured accounts ordered by creation time.
func (db *DB) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := db.sql.QueryContext(ctx,
		`SELECT `+accountCols+` FROM accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan account: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	return out, nil
}

// DeleteAccount removes an account. Folders, messages, and outbox entries
// cascade via foreign keys. The caller is responsible for deleting the
// keystore entry named by the account's KeystoreAlias.
func (db *DB) DeleteAccount(ctx context.Context, id int64) error {
	res, err := db.sql.ExecContext(ctx,
		`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete account %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete account %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPinnedSPKI records (or clears, with "") the pinned TLS public-key
// hash for an account (ADR-0008).
func (db *DB) SetPinnedSPKI(ctx context.Context, accountID int64, spki string) error {
	_, err := db.sql.ExecContext(ctx,
		`UPDATE accounts SET pinned_spki = ? WHERE id = ?`,
		nullable(spki), accountID)
	if err != nil {
		return fmt.Errorf("store: set pinned spki: %w", err)
	}
	return nil
}
