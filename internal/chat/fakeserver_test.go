package chat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// fakeTalk is an in-memory VayuTalk server implementing the frozen wire
// protocol for tests. It never persists anything and stores only opaque
// ciphertext in memory, mirroring the real server's invariants.
type fakeTalk struct {
	mu        sync.Mutex
	creds     map[string]string // email -> password
	tokens    map[string]string // token -> email
	pubkeys   map[string]string // email -> armored public key
	streams   map[string][]chan string
	queue     map[string][]string // recipient email -> queued SSE payloads
	envSender map[string]string   // envelope id -> sender email
	seq       int
}

func newFakeTalk() *fakeTalk {
	return &fakeTalk{
		creds:     map[string]string{},
		tokens:    map[string]string{},
		pubkeys:   map[string]string{},
		streams:   map[string][]chan string{},
		queue:     map[string][]string{},
		envSender: map[string]string{},
	}
}

func (s *fakeTalk) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/talk/connect":
		s.connect(w, r)
	case "/api/v1/talk/stream":
		s.stream(w, r)
	case "/api/v1/talk/send":
		s.send(w, r)
	case "/api/v1/talk/ack":
		s.ack(w, r)
	case "/api/v1/talk/pubkey":
		s.pubkey(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *fakeTalk) userFor(r *http.Request) string {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens[tok]
}

func (s *fakeTalk) connect(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	want, ok := s.creds[in.Email]
	if !ok || want != in.Password {
		s.mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid-credentials"}`))
		return
	}
	s.seq++
	tok := "tok-" + strconv.Itoa(s.seq)
	s.tokens[tok] = in.Email
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"token": tok, "expires_in": 43200})
}

func (s *fakeTalk) stream(w http.ResponseWriter, r *http.Request) {
	user := s.userFor(r)
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flusher", http.StatusInternalServerError)
		return
	}
	// Register the stream BEFORE flushing 200 so a sender that observes the
	// recipient online cannot race ahead of registration and lose a live
	// message.
	ch := make(chan string, 32)
	s.mu.Lock()
	s.streams[user] = append(s.streams[user], ch)
	for _, payload := range s.queue[user] { // flush store-mode backlog
		ch <- payload
	}
	delete(s.queue, user)
	s.mu.Unlock()
	defer s.removeStream(user, ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-ch:
			_, _ = w.Write([]byte(payload))
			fl.Flush()
		}
	}
}

func (s *fakeTalk) removeStream(user string, ch chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.streams[user]
	for i, c := range list {
		if c == ch {
			s.streams[user] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

func (s *fakeTalk) send(w http.ResponseWriter, r *http.Request) {
	user := s.userFor(r)
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var in struct {
		To, Ciphertext, Mode string
		TTLSeconds           int `json:"ttl_seconds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if raw, err := base64.StdEncoding.DecodeString(in.Ciphertext); err != nil || len(raw) > 65536 {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	s.mu.Lock()
	s.seq++
	id := "env-" + strconv.Itoa(s.seq)
	s.envSender[id] = user
	payload := envelopeSSE(id, user, in.Ciphertext, in.Mode)
	live := s.streams[in.To]
	delivered := false
	if len(live) > 0 {
		for _, c := range live {
			c <- payload
		}
		delivered = true
	} else if in.Mode == "store" {
		s.queue[in.To] = append(s.queue[in.To], payload)
	}
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "delivered": delivered})
}

func (s *fakeTalk) ack(w http.ResponseWriter, r *http.Request) {
	user := s.userFor(r)
	if user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var in struct{ ID string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	sender := s.envSender[in.ID]
	delete(s.envSender, in.ID)
	receipt := receiptSSE(in.ID, "read")
	for _, c := range s.streams[sender] {
		c <- receipt
	}
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{})
}

func (s *fakeTalk) pubkey(w http.ResponseWriter, r *http.Request) {
	if s.userFor(r) == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	email := r.URL.Query().Get("email")
	s.mu.Lock()
	armored, ok := s.pubkeys[email]
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"email":              email,
		"armored_public_key": armored,
		"fingerprint":        "AAAA1111BBBB2222CCCC3333DDDD4444EEEE5555",
	})
}

func envelopeSSE(id, from, ciphertext, mode string) string {
	// Mirror the real VayuPress relay's wire format: RFC 3339 timestamps
	// (see ADR-0131). Emitting these here is what exercises the client's
	// timestamp parse path in tests.
	now := time.Now().UTC()
	b, _ := json.Marshal(envelopeWire{
		ID: id, From: from, Ciphertext: ciphertext, Mode: mode,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339),
	})
	return "event: envelope\ndata: " + string(b) + "\n\n"
}

func receiptSSE(id, status string) string {
	return "event: receipt\ndata: " + fmt.Sprintf(`{"id":%q,"status":%q}`, id, status) + "\n\n"
}

// rewriteRT redirects every request to the test server's host while the
// caller keeps using a public https URL, so the SSRF guard is exercised
// with a real domain yet traffic reaches the local server.
type rewriteRT struct {
	base *url.URL
	rt   http.RoundTripper
}

func (rr rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rr.base.Scheme
	req.URL.Host = rr.base.Host
	req.Host = rr.base.Host
	return rr.rt.RoundTrip(req)
}

func rewriteClient(t *testing.T, serverURL string) *http.Client {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	return &http.Client{Transport: rewriteRT{base: u, rt: http.DefaultTransport}}
}

// genArmoredKeypair generates a fresh OpenPGP key and returns its armored
// private and public blobs.
func genArmoredKeypair(t *testing.T, name, email string) (priv, pub string) {
	t.Helper()
	ent, err := openpgp.NewEntity(name, "", email, nil)
	if err != nil {
		t.Fatalf("NewEntity(%s): %v", email, err)
	}
	var pb bytes.Buffer
	pw, err := armor.Encode(&pb, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatalf("armor priv: %v", err)
	}
	if err := ent.SerializePrivate(pw, nil); err != nil {
		t.Fatalf("serialize priv: %v", err)
	}
	_ = pw.Close()

	var ub bytes.Buffer
	uw, err := armor.Encode(&ub, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor pub: %v", err)
	}
	if err := ent.Serialize(uw); err != nil {
		t.Fatalf("serialize pub: %v", err)
	}
	_ = uw.Close()
	return pb.String(), ub.String()
}
