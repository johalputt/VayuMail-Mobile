package syncmanager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
)

// maxCmdIdle bounds how long a pooled command connection may sit unused
// before the next command redials. Servers commonly drop idle sessions
// after ~30 minutes; 90 seconds stays far inside that while still
// amortising the burst that actually matters — archive-then-delete,
// move-then-read, a mark sweep — where today every tap pays TCP + TLS +
// LOGIN again (audit M4).
const maxCmdIdle = 90 * time.Second

// dialFunc is imapsync.Dial's shape. It is a Manager field so tests can
// attach the loopback plaintext server below the TLS seam, exactly like
// the other IMAP tests do.
type dialFunc = func(ctx context.Context, cfg account.Config, password string) (*imapclient.Client, <-chan imapsync.Notification, error)

// cmdConn is one pooled, authenticated command connection.
type cmdConn struct {
	client *imapclient.Client
	usedAt time.Time
}

// withCommandConn runs fn over a per-account pooled connection, redialing
// only when no healthy one exists.
//
// # Why retry-once, and why the cache cannot mask a real outage
//
// A cached socket can die between commands (server-side idle timeout, NAT
// teardown). The failure surfaces as fn's error, so the first failure on a
// cached connection is treated as "socket suspect": drop it, dial fresh,
// run fn once more. If the fresh attempt fails too, that error is real and
// is returned — the pool retries exactly once, never in a loop. Failures
// on a FRESH connection are returned immediately: they just happened on a
// socket we know is new.
//
// # Concurrency
//
// Every caller runs on the single commandLoop goroutine, and Shutdown
// drains after it exits, so pool access is effectively serial; poolMu
// exists to keep that invariant honest rather than load-bearing.
func (m *Manager) withCommandConn(
	ctx context.Context,
	cfg account.Config,
	cred func() (string, error),
	fn func(*imapclient.Client) error,
) error {
	key := cfg.KeystoreAlias

	m.poolMu.Lock()
	cc := m.cmdConns[key]
	if cc != nil && time.Since(cc.usedAt) > maxCmdIdle {
		_ = cc.client.Close()
		delete(m.cmdConns, key)
		cc = nil
	}
	if cc != nil {
		cc.usedAt = time.Now()
		client := cc.client
		m.poolMu.Unlock()

		if err := fn(client); err == nil {
			return nil
		}
		// Suspect socket: drop it (only if nothing replaced it meanwhile)
		// and fall through to a fresh dial for the one retry.
		m.poolMu.Lock()
		if cur := m.cmdConns[key]; cur != nil && cur.client == client {
			delete(m.cmdConns, key)
		}
		m.poolMu.Unlock()
		_ = client.Close()
		slog.Debug("pooled command connection failed; redialing",
			"account", key)
	} else {
		m.poolMu.Unlock()
	}

	password, err := cred()
	if err != nil {
		return fmt.Errorf("syncmanager: fetch credential: %w", err)
	}
	client, _, err := m.dial(ctx, cfg, password)
	if err != nil {
		return err
	}
	if err := fn(client); err != nil {
		_ = client.Close()
		return err
	}

	// Success: park the connection instead of logging out. Any previous
	// entry is stale by construction (serial callers), but close rather
	// than leak if that ever changes.
	m.poolMu.Lock()
	if old := m.cmdConns[key]; old != nil && old.client != client {
		_ = old.client.Close()
	}
	m.cmdConns[key] = &cmdConn{client: client, usedAt: time.Now()}
	m.poolMu.Unlock()
	return nil
}

// dropCommandConn releases one account's pooled connection — used when an
// account is removed so its socket does not outlive its credentials.
func (m *Manager) dropCommandConn(alias string) {
	m.poolMu.Lock()
	defer m.poolMu.Unlock()
	if cc, ok := m.cmdConns[alias]; ok {
		_ = cc.client.Close()
		delete(m.cmdConns, alias)
	}
}
