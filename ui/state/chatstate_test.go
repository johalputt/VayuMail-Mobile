package state

import (
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/chat"
)

// applyIncoming must ignore a message it already holds, so the persistent
// VayuTalk connection can reconnect (which re-flushes un-revealed store-mode
// envelopes) without duplicating messages in the conversation.
func TestApplyIncomingDedupe(t *testing.T) {
	cs := &ChatState{convs: map[string]*chatConv{}}
	msg := chat.IncomingMessage{Peer: "bob@example.com", ID: "env-1", Plaintext: "hi", ExpiresAt: time.Now().Add(time.Hour)}
	cs.applyIncoming(msg)
	cs.applyIncoming(msg) // simulate a reconnect re-flush
	c := cs.convs["bob@example.com"]
	if c == nil {
		t.Fatal("conversation was not created")
	}
	if len(c.msgs) != 1 {
		t.Fatalf("expected 1 message after dedupe, got %d", len(c.msgs))
	}
}

func TestClampTTL(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want time.Duration
	}{
		{10 * time.Second, 60 * time.Second},     // below floor
		{5 * time.Minute, 5 * time.Minute},       // in range
		{2 * time.Hour, 3600 * time.Second},      // above ceiling
		{3600 * time.Second, 3600 * time.Second}, // exactly the ceiling
	}
	for _, c := range cases {
		if got := clampTTL(c.in); got != c.want {
			t.Errorf("clampTTL(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDomainOf(t *testing.T) {
	cases := map[string]string{
		"ankush@vayu.example": "vayu.example",
		"a@b@c.d":             "c.d", // last '@' wins
		"noat":                "",
		"":                    "",
	}
	for in, want := range cases {
		if got := domainOf(in); got != want {
			t.Errorf("domainOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafetyNumberFormatsGroups(t *testing.T) {
	// The wrapper must delegate to the engine and produce grouped output
	// (spaces every four hex characters).
	got := SafetyNumber("ABCD1234EF56")
	if got == "" {
		t.Fatal("SafetyNumber returned empty")
	}
	if got != "ABCD 1234 EF56" {
		t.Errorf("SafetyNumber grouping = %q", got)
	}
}
