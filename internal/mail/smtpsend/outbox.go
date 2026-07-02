package smtpsend

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// retryBase is the first retry delay; attempt n waits retryBase * 2^n.
// With the schema's default max_retries of 5 the ladder is
// 1m, 2m, 4m, 8m, 16m, after which the entry becomes a dead letter.
const retryBase = time.Minute

// ProcessOutbox sends every due outbox entry for one account. Each
// result — success or failure — is reported through onResult so the
// syncmanager can emit SendResultEvent. Errors sending one entry never
// block the rest of the queue.
func ProcessOutbox(ctx context.Context, db *store.DB, cfg account.Config, cred func() (string, error), accountID int64, onResult func(outboxID int64, err error)) error {
	entries, err := db.DueOutbox(ctx, time.Now())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.AccountID != accountID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		sendErr := SendEntry(ctx, cfg, cred, entry)
		if sendErr == nil {
			if err := db.MarkOutboxSent(ctx, entry.ID); err != nil {
				return err
			}
		} else {
			delay := retryBase << uint(entry.RetryCount)
			next := time.Now().Add(delay)
			slog.Warn("outbox send failed",
				"outbox_id", entry.ID, "retry", entry.RetryCount,
				"next_attempt", next, "err", sendErr)
			if err := db.MarkOutboxFailed(ctx, entry.ID, sendErr, next); err != nil {
				return err
			}
		}
		if onResult != nil {
			onResult(entry.ID, sendErr)
		}
	}
	return nil
}

// SendEntry delivers one stored message: it derives the envelope from the
// stored headers, strips Bcc from the wire bytes, and hands the result to
// Send.
func SendEntry(ctx context.Context, cfg account.Config, cred func() (string, error), entry store.OutboxEntry) error {
	from, recipients, err := envelopeFromRaw(entry.RawMessage)
	if err != nil {
		return err
	}
	wire, err := stripBcc(entry.RawMessage)
	if err != nil {
		return err
	}
	password, err := cred()
	if err != nil {
		return fmt.Errorf("smtpsend: fetch credential: %w", err)
	}
	return Send(ctx, cfg, password, from, recipients, wire)
}

// envelopeFromRaw reads From/To/Cc/Bcc headers out of a stored message to
// rebuild the SMTP envelope after a restart.
func envelopeFromRaw(raw []byte) (from string, recipients []string, err error) {
	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownCharset(err) {
		return "", nil, fmt.Errorf("smtpsend: parse outbox message: %w", err)
	}
	header := mail.Header{Header: entity.Header}

	fromList, err := header.AddressList("From")
	if err != nil || len(fromList) == 0 {
		return "", nil, fmt.Errorf("smtpsend: outbox message has no From")
	}
	from = fromList[0].Address

	for _, key := range []string{"To", "Cc", "Bcc"} {
		list, err := header.AddressList(key)
		if err != nil {
			continue
		}
		for _, a := range list {
			recipients = append(recipients, a.Address)
		}
	}
	if len(recipients) == 0 {
		return "", nil, fmt.Errorf("smtpsend: outbox message has no recipients")
	}
	return from, recipients, nil
}

// stripBcc removes the Bcc header line(s) from the wire bytes so blind
// recipients stay blind. Only the header block is touched.
func stripBcc(raw []byte) ([]byte, error) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inHeader := true
	skipping := false
	for scanner.Scan() {
		line := scanner.Text()
		if inHeader {
			if line == "" {
				inHeader = false
				skipping = false
			} else if skipping && (len(line) > 0 && (line[0] == ' ' || line[0] == '\t')) {
				continue // folded continuation of the Bcc header
			} else {
				skipping = len(line) >= 4 && equalFoldPrefix(line, "bcc:")
				if skipping {
					continue
				}
			}
		}
		out.WriteString(line)
		out.WriteString("\r\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("smtpsend: strip bcc: %w", err)
	}
	return out.Bytes(), nil
}

func equalFoldPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return bytes.EqualFold([]byte(s[:len(prefix)]), []byte(prefix))
}
