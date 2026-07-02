package test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-message/mail"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
)

func testDraft() smtpsend.Draft {
	return smtpsend.Draft{
		FromName: "Test User",
		FromAddr: "t@example.com",
		To:       []string{"to@example.com"},
		Cc:       []string{"cc@example.com"},
		Bcc:      []string{"hidden@example.com"},
		Subject:  "Hello wind",
		TextBody: "The body of the message.",
	}
}

func TestBuildMIMERoundTrip(t *testing.T) {
	draft := testDraft()
	raw, err := smtpsend.BuildMIME(&draft)
	if err != nil {
		t.Fatal(err)
	}

	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("re-parse built message: %v", err)
	}
	defer mr.Close()

	subject, err := mr.Header.Subject()
	if err != nil || subject != "Hello wind" {
		t.Errorf("subject = %q, %v", subject, err)
	}
	from, err := mr.Header.AddressList("From")
	if err != nil || len(from) != 1 || from[0].Address != "t@example.com" {
		t.Errorf("from = %v, %v", from, err)
	}
	if id, err := mr.Header.MessageID(); err != nil || id == "" {
		t.Errorf("message-id missing: %q, %v", id, err)
	}
	// Bcc is present in the STORED bytes (envelope recovery after
	// restart) and stripped only on the wire.
	bcc, err := mr.Header.AddressList("Bcc")
	if err != nil || len(bcc) != 1 {
		t.Errorf("stored message must keep Bcc: %v, %v", bcc, err)
	}
}

func TestBuildMIMEWithAttachment(t *testing.T) {
	draft := testDraft()
	draft.Attachments = []smtpsend.Attachment{{
		Filename:    "wind.txt",
		ContentType: "text/plain",
		Data:        []byte("gust"),
	}}
	raw, err := smtpsend.BuildMIME(&draft)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("multipart/mixed")) {
		t.Error("attachment draft must be multipart/mixed")
	}
	if !bytes.Contains(raw, []byte("wind.txt")) {
		t.Error("attachment filename missing")
	}
}

func TestBuildMIMEValidation(t *testing.T) {
	empty := smtpsend.Draft{FromAddr: "t@example.com"}
	if _, err := smtpsend.BuildMIME(&empty); err == nil {
		t.Error("draft with no recipients must fail")
	}
	noFrom := smtpsend.Draft{To: []string{"to@example.com"}}
	if _, err := smtpsend.BuildMIME(&noFrom); err == nil {
		t.Error("draft with no From must fail")
	}
}

func TestBuildPGPEncryptedStructure(t *testing.T) {
	draft := testDraft()
	raw, err := smtpsend.BuildPGPEncrypted(&draft, []byte("-----BEGIN PGP MESSAGE-----\nabc\n-----END PGP MESSAGE-----"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"multipart/encrypted",
		"application/pgp-encrypted",
		"Version: 1",
		"BEGIN PGP MESSAGE",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("PGP/MIME output missing %q", want)
		}
	}
}

func TestDraftRecipients(t *testing.T) {
	draft := testDraft()
	got := draft.Recipients()
	want := []string{"to@example.com", "cc@example.com", "hidden@example.com"}
	if len(got) != len(want) {
		t.Fatalf("recipients = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("recipients[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestOutboxRetrySchedule proves the queue behavior end to end without a
// network: sending to an unreachable server fails fast, records the
// error, schedules a retry, and eventually dead-letters.
func TestOutboxRetrySchedule(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	ctx := t.Context()

	draft := testDraft()
	raw, err := smtpsend.BuildMIME(&draft)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.EnqueueOutbox(ctx, a.ID, raw)
	if err != nil {
		t.Fatal(err)
	}

	// Port 1 on localhost refuses instantly — offline-safe failure.
	cfg := account.Config{
		DisplayName: "t", EmailAddress: "t@example.com",
		IMAPHost: "127.0.0.1", IMAPPort: 1, IMAPTLS: account.TLSModeImplicit,
		SMTPHost: "127.0.0.1", SMTPPort: 1, SMTPTLS: account.TLSModeImplicit,
		Username: "t@example.com", KeystoreAlias: "k",
	}
	cred := func() (string, error) { return "pw", nil }

	var results []error
	err = smtpsend.ProcessOutbox(ctx, db, cfg, cred, a.ID,
		func(outboxID int64, sendErr error) {
			if outboxID != id {
				t.Errorf("outbox id = %d", outboxID)
			}
			results = append(results, sendErr)
		})
	if err != nil {
		t.Fatalf("ProcessOutbox must not fail the pass: %v", err)
	}
	if len(results) != 1 || results[0] == nil {
		t.Fatalf("want one failed result, got %v", results)
	}

	entry, err := db.GetOutboxEntry(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RetryCount != 1 {
		t.Errorf("retry count = %d", entry.RetryCount)
	}
	if entry.LastError == "" {
		t.Error("last error must be recorded")
	}
	if !entry.NextAttempt.After(time.Now()) {
		t.Error("next attempt must be scheduled in the future")
	}

	// The entry is not due again until its backoff elapses.
	due, err := db.DueOutbox(ctx, time.Now())
	if err != nil || len(due) != 0 {
		t.Fatalf("entry must not be due during backoff: %v, %v", due, err)
	}
}
