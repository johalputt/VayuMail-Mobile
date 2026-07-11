package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxSSEEvent bounds a single Server-Sent Event so a hostile or buggy
// server cannot exhaust memory with one unbounded line. A ciphertext
// envelope is at most 65536 bytes of opaque payload, base64-expanded to
// ~87 KiB, plus small JSON framing.
const maxSSEEvent = 256 << 10

// StreamEvent is the sealed set of things the live stream delivers.
type StreamEvent interface{ isStreamEvent() }

// Envelope is one encrypted message pushed to this client. Ciphertext is
// base64 of opaque bytes; only the recipient's keyring can open it.
type Envelope struct {
	ID         string
	From       string
	Ciphertext string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Mode       string
}

// Receipt reports a state change for a message this client sent. Status is
// "read" (recipient acked) or "expired" (TTL elapsed undelivered).
type Receipt struct {
	ID     string
	Status string
}

// Ping is a heartbeat; it carries no payload and only proves liveness.
type Ping struct{}

func (Envelope) isStreamEvent() {}
func (Receipt) isStreamEvent()  {}
func (Ping) isStreamEvent()     {}

// envelopeWire is the JSON shape of an envelope event. Times cross the
// wire as RFC 3339 strings — the format the VayuPress VayuTalk relay emits
// (see ADR-0131). Declaring them as int64 here silently failed every
// json.Unmarshal, so buildEvent dropped every real envelope.
type envelopeWire struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	Mode       string `json:"mode"`
}

// OpenStream GETs /stream and returns a channel of typed events. The
// channel closes when the stream ends or ctx is cancelled, so a caller can
// retry with backoff. Redirects are refused and the response is not
// size-capped as a whole (it is a long-lived stream), but every individual
// event is bounded by maxSSEEvent.
func (t *Transport) OpenStream(ctx context.Context, token, domain string) (<-chan StreamEvent, error) {
	if !publicTalkDomain(domain) {
		return nil, ErrTalk
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL(domain)+"/stream", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, ErrTalk
	}
	if err := statusErr(resp.StatusCode); err != nil {
		resp.Body.Close()
		return nil, err
	}
	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parseSSE(ctx, resp.Body, ch)
	}()
	return ch, nil
}

// parseSSE reads the event stream line by line, assembling event/data
// pairs separated by blank lines, and forwards typed events until the
// stream ends or ctx is cancelled.
func parseSSE(ctx context.Context, r io.Reader, ch chan<- StreamEvent) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 8192), maxSSEEvent)
	var name string
	var data strings.Builder
	dispatch := func() {
		ev := buildEvent(name, data.String())
		name = ""
		data.Reset()
		if ev == nil {
			return
		}
		select {
		case ch <- ev:
		case <-ctx.Done():
		}
	}
	for sc.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := sc.Text()
		switch {
		case line == "":
			dispatch()
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(line[len("data:"):]))
		default:
			// Comments (": ...") and other fields are ignored.
		}
	}
}

// buildEvent converts a completed event/data pair into a typed StreamEvent,
// or nil when the payload is unrecognized or malformed.
func buildEvent(name, data string) StreamEvent {
	switch name {
	case "envelope":
		var w envelopeWire
		if json.Unmarshal([]byte(data), &w) != nil || w.ID == "" {
			return nil
		}
		// Timestamps are non-critical for delivery/decryption: a malformed
		// one yields a zero time, never a dropped envelope.
		created, _ := time.Parse(time.RFC3339, w.CreatedAt)
		expires, _ := time.Parse(time.RFC3339, w.ExpiresAt)
		return Envelope{
			ID:         w.ID,
			From:       w.From,
			Ciphertext: w.Ciphertext,
			CreatedAt:  created,
			ExpiresAt:  expires,
			Mode:       w.Mode,
		}
	case "receipt":
		var w struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(data), &w) != nil || w.ID == "" {
			return nil
		}
		return Receipt{ID: w.ID, Status: w.Status}
	case "ping":
		return Ping{}
	default:
		return nil
	}
}
