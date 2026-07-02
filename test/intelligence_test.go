package test

import (
	"strings"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
)

func TestDetectTrackers(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{"clean html", `<p>Hello <img src="https://example.com/logo.png" width="200" height="80"></p>`, false},
		{"1x1 pixel", `<img src="https://cdn.example.com/o.gif" width="1" height="1">`, true},
		{"hidden image", `<img src="https://x.example.com/p.png" style="display:none">`, true},
		{"tracker host image", `<img src="https://click.list-manage.com/track/open.php?u=1">`, true},
		{"tracker host link", `<a href="https://mandrillapp.com/track/click/123">read</a>`, true},
		{"subdomain of tracker", `<img src="https://open.sendgrid.net/wf/open?upn=x">`, true},
		{"plain text", "", false},
		{"lookalike domain not flagged", `<img src="https://notsendgrid.example.org/x.png" width="400" height="300">`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mime.DetectTrackers(tt.html); got != tt.want {
				t.Errorf("DetectTrackers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstUnsubscribeTarget(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantMailto string
		wantURL    string
	}{
		{"mailto only", "<mailto:leave@list.example.com>", "leave@list.example.com", ""},
		{"mailto with subject", "<mailto:u@x.com?subject=unsub>", "u@x.com", ""},
		{"https only", "<https://list.example.com/unsub/123>", "", "https://list.example.com/unsub/123"},
		{"both prefers mailto", "<https://x.example/u>, <mailto:bye@x.example>", "bye@x.example", "https://x.example/u"},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailto, url := mime.FirstUnsubscribeTarget(tt.header)
			if mailto != tt.wantMailto || url != tt.wantURL {
				t.Errorf("got (%q,%q), want (%q,%q)", mailto, url, tt.wantMailto, tt.wantURL)
			}
		})
	}
}

func TestSearchOperators(t *testing.T) {
	db := openStore(t)
	a := seedAccount(t, db)
	f := seedFolder(t, db, a.ID)
	ctx := t.Context()

	old := seedMessage(t, db, a, f, 1, "yearly budget", true)
	_ = old
	m2 := seedMessage(t, db, a, f, 2, "wind turbine specs", false)
	_ = m2
	// Give message 2 an attachment flag for has:attachment.
	msg, err := db.GetMessageByUID(ctx, f.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	msg.HasAttachments = true
	if _, err := db.UpsertMessage(ctx, &msg); err != nil {
		t.Fatal(err)
	}

	run := func(q string) int {
		t.Helper()
		res, err := db.Search(ctx, a.ID, q, 20)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return len(res)
	}

	if got := run("from:alice"); got != 2 {
		t.Errorf("from:alice = %d, want 2", got)
	}
	if got := run("subject:budget"); got != 1 {
		t.Errorf("subject:budget = %d, want 1", got)
	}
	if got := run("has:attachment"); got != 1 {
		t.Errorf("has:attachment = %d, want 1", got)
	}
	if got := run("is:unread wind"); got != 1 {
		t.Errorf("is:unread wind = %d, want 1", got)
	}
	if got := run("before:2020-01-01"); got != 0 {
		t.Errorf("before:2020 = %d, want 0", got)
	}
	if got := run("after:2020-01-01 budget"); got != 1 {
		t.Errorf("after+term = %d, want 1", got)
	}
	// Body search: the FTS index now covers body_text.
	msg.BodyText = "the flux capacitor hums"
	if _, err := db.UpsertMessage(ctx, &msg); err != nil {
		t.Fatal(err)
	}
	if got := run("capacitor"); got != 1 {
		t.Errorf("body search = %d, want 1", got)
	}
}

func TestUnifiedInboxAndSnooze(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()

	a1 := seedAccount(t, db)
	f1 := seedFolder(t, db, a1.ID)
	a2 := a1
	a2.ID = 0
	a2.EmailAddress = "second@example.com"
	a2.KeystoreAlias = "alias-2"
	if _, err := db.InsertAccount(ctx, &a2); err != nil {
		t.Fatal(err)
	}
	f2 := seedFolder(t, db, a2.ID)

	seedMessage(t, db, a1, f1, 1, "first inbox", false)
	m := seedMessage(t, db, a2, f2, 1, "second inbox", false)

	all, err := db.ListUnifiedInbox(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("unified inbox = %d messages", len(all))
	}
	n, err := db.UnifiedUnreadCount(ctx)
	if err != nil || n != 2 {
		t.Fatalf("unified unread = %d, %v", n, err)
	}

	// Snoozing hides from both views until the deadline passes.
	if err := db.SetSnooze(ctx, m.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	all, err = db.ListUnifiedInbox(ctx, 0, 10)
	if err != nil || len(all) != 1 {
		t.Fatalf("after snooze = %d, %v", len(all), err)
	}
	if err := db.SetSnooze(ctx, m.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	all, err = db.ListUnifiedInbox(ctx, 0, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("after unsnooze = %d, %v", len(all), err)
	}
}

func TestExtractAttachment(t *testing.T) {
	raw := readMIMEFixture(t, "multipart.eml")
	ref, data, err := mime.ExtractAttachment(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Filename != "report.pdf" || ref.ContentType != "application/pdf" {
		t.Errorf("ref = %+v", ref)
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Errorf("decoded data = %q", data)
	}
	if _, _, err := mime.ExtractAttachment(raw, 5); err == nil {
		t.Error("out-of-range attachment must error")
	}
}
