package syncmanager

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"go.uber.org/goleak"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
)

// startCountingIMAPServer is startIMAPServer's loopback stand-in plus a
// dial counter: every TCP session increments dials, so a reuse claim has
// something hard to contradict.
func startCountingIMAPServer(t *testing.T, dials *atomic.Int64) (addr string, closeSrv func()) {
	t.Helper()
	user := imapmemserver.NewUser("t@example.com", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem := imapmemserver.New()
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			dials.Add(1)
			return mem.NewSession(), nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: {},
			imap.CapIdle:      {},
		},
		InsecureAuth: true, // loopback test only; production is TLS-only
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Close() }
}

// plainDial mirrors imapsync.Dial minus TLS: the loopback memserver speaks
// plaintext, and the pool takes its dialer as a field precisely so tests
// can attach below the TLS seam.
func plainDial(addr string) dialFunc {
	return func(ctx context.Context, cfg account.Config, password string) (*imapclient.Client, <-chan imapsync.Notification, error) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, nil, err
		}
		client := imapclient.New(conn, nil)
		if err := client.Login("t@example.com", password).Wait(); err != nil {
			_ = client.Close()
			return nil, nil, err
		}
		return client, make(chan imapsync.Notification, 8), nil
	}
}

func poolTestConfig(host string, port int) account.Config {
	cfg := account.Config{
		DisplayName:   "Test",
		EmailAddress:  "t@example.com",
		IMAPHost:      host,
		IMAPPort:      port,
		IMAPTLS:       account.TLSModeImplicit,
		SMTPHost:      host,
		SMTPPort:      587,
		SMTPTLS:       account.TLSModeSTARTTLS,
		Username:      "t@example.com",
		KeystoreAlias: "test-alias",
	}
	return cfg
}

// The M4 property: two commands back-to-back hit the server once. The old
// behaviour — WithConnection per command — would show two dials here.
func TestCommandConnectionsAreReused(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))

	var dials atomic.Int64
	addr, closeSrv := startCountingIMAPServer(t, &dials)
	defer closeSrv()

	m := New(nil, appcrypto.NewMemoryKeystore())
	m.dial = plainDial(addr)
	defer m.Shutdown()

	cfg := poolTestConfig("127.0.0.1", 143)
	cred := func() (string, error) { return "secret", nil }
	noop := func(c *imapclient.Client) error {
		return c.Noop().Wait()
	}

	ctx := context.Background()
	for i := range 3 {
		if err := m.withCommandConn(ctx, cfg, cred, noop); err != nil {
			t.Fatalf("command %d: %v", i, err)
		}
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("three commands caused %d dials; reuse broken", got)
	}

	// A different account must not share the first one's socket.
	other := cfg
	other.KeystoreAlias = "second-alias"
	if err := m.withCommandConn(ctx, other, cred, noop); err != nil {
		t.Fatalf("second account: %v", err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("accounts shared a pooled connection: %d dials", got)
	}
}

// A dead cached socket costs exactly one extra dial, not a failure and not
// a loop: the first command after the drop redials and succeeds.
func TestDeadCachedSocketIsReplaced(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))

	var dials atomic.Int64
	addr, closeSrv := startCountingIMAPServer(t, &dials)
	defer closeSrv()

	m := New(nil, appcrypto.NewMemoryKeystore())
	m.dial = plainDial(addr)
	defer m.Shutdown()

	cfg := poolTestConfig("127.0.0.1", 143)
	cred := func() (string, error) { return "secret", nil }
	noop := func(c *imapclient.Client) error {
		return c.Noop().Wait()
	}
	ctx := context.Background()

	if err := m.withCommandConn(ctx, cfg, cred, noop); err != nil {
		t.Fatalf("prime: %v", err)
	}

	// Sabotage: kill the pooled socket behind the manager's back, the way
	// a server idle-timeout or NAT teardown would.
	m.poolMu.Lock()
	cc := m.cmdConns[cfg.KeystoreAlias]
	m.poolMu.Unlock()
	if cc == nil {
		t.Fatal("no pooled connection after prime")
	}
	_ = cc.client.Close()

	if err := m.withCommandConn(ctx, cfg, cred, noop); err != nil {
		t.Fatalf("after socket death: %v", err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("recovery took %d total dials, want 2", got)
	}
}

// An expired entry is discarded without being trusted: age past maxCmdIdle
// forces a fresh dial even though the map still holds a client.
func TestIdleEvictionRedials(t *testing.T) {
	var dials atomic.Int64
	addr, closeSrv := startCountingIMAPServer(t, &dials)
	defer closeSrv()

	m := New(nil, appcrypto.NewMemoryKeystore())
	m.dial = plainDial(addr)
	defer m.Shutdown()

	cfg := poolTestConfig("127.0.0.1", 143)
	cred := func() (string, error) { return "secret", nil }
	noop := func(c *imapclient.Client) error {
		return c.Noop().Wait()
	}
	ctx := context.Background()

	if err := m.withCommandConn(ctx, cfg, cred, noop); err != nil {
		t.Fatalf("prime: %v", err)
	}
	m.poolMu.Lock()
	m.cmdConns[cfg.KeystoreAlias].usedAt = time.Now().Add(-maxCmdIdle - time.Second)
	m.poolMu.Unlock()

	if err := m.withCommandConn(ctx, cfg, cred, noop); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("expired entry was reused: %d dials", got)
	}
}

// A real failure on a FRESH connection propagates — the retry path exists
// for stale sockets only, never to paper over a live outage or bad
// credentials.
func TestFreshDialFailurePropagates(t *testing.T) {
	m := New(nil, appcrypto.NewMemoryKeystore())
	m.dial = func(ctx context.Context, cfg account.Config, password string) (*imapclient.Client, <-chan imapsync.Notification, error) {
		return nil, nil, errors.New("connection refused")
	}
	defer m.Shutdown()

	calls := 0
	err := m.withCommandConn(context.Background(), poolTestConfig("127.0.0.1", 143),
		func() (string, error) { calls++; return "secret", nil },
		func(*imapclient.Client) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected dial error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("credential fetched %d times, want 1", calls)
	}
}
