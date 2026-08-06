package test

// tls_resumption_audit_test.go — the certificate pin must hold on every
// connection, not just the first one of a session's life.
//
// The finding, in the attacker's voice: I do not need to beat the pin. I only
// need the client to stop asking. Go calls VerifyPeerCertificate during
// certificate verification, and a resumed TLS session skips certificate
// verification entirely — the peer's chain is restored from the ticket. So a
// pinned account checks its pin on the first handshake and never again for as
// long as that session can be resumed. Rotate the pin, revoke trust in the key,
// re-pin the account after a suspected interception: none of it takes effect.
//
// Today nothing sets a ClientSessionCache, so Go's client does not resume and
// the gap is latent rather than live. That is the reason to pin it now: the
// day somebody adds a cache for reconnect latency — which is an obviously good
// idea for a mail client that reconnects constantly — the pin silently becomes
// a first-connection-only control, and no test would have said so.
//
// The fix is VerifyConnection, which Go calls on every handshake, full or
// resumed. These tests prove the pin is consulted on a genuinely resumed
// session, and the control below proves the harness really does resume — an
// assertion that cannot demonstrate its own precondition proves nothing.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

const pinTestHost = "mail.pin.example"

// tlsPinTestServer starts a TLS listener on loopback and returns its address,
// the leaf certificate a client should pin, and a root pool that trusts it.
//
// After the handshake the server writes one byte. That write matters: under TLS
// 1.3 the session ticket arrives as a post-handshake message, and Go's client
// only processes it inside Read. Without a read there is no cached session and
// nothing to resume, so the test would pass while proving nothing.
func tlsPinTestServer(t *testing.T) (addr string, leaf *x509.Certificate, roots *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: pinTestHost},
		DNSNames:              []string{pinTestHost},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots = x509.NewCertPool()
	roots.AddCert(leaf)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				if tc, ok := c.(*tls.Conn); ok {
					if err := tc.Handshake(); err != nil {
						return
					}
				}
				_, _ = c.Write([]byte("x"))
				// Hold the connection open briefly so the client's read races
				// against a live peer rather than an immediate EOF.
				time.Sleep(50 * time.Millisecond)
			}(c)
		}
	}()
	return ln.Addr().String(), leaf, roots
}

// dialPinned connects with the account's real TLS config for the given pin,
// reads one byte so any session ticket is processed, and reports whether the
// handshake resumed a previous session.
func dialPinned(t *testing.T, addr, pin string, roots *x509.CertPool, cache tls.ClientSessionCache) (resumed bool, err error) {
	t.Helper()

	cfg := account.Config{IMAPHost: pinTestHost, PinnedSPKI: pin}
	tc := cfg.TLSConfig()
	if tc == nil {
		t.Fatal("a pinned account produced a nil TLS config; the pin is not being applied at all")
	}
	tc = tc.Clone()
	tc.RootCAs = roots
	tc.ServerName = pinTestHost
	tc.ClientSessionCache = cache

	conn, err := tls.Dial("tcp", addr, tc)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	if _, rerr := conn.Read(buf); rerr != nil {
		return conn.ConnectionState().DidResume, rerr
	}
	return conn.ConnectionState().DidResume, nil
}

// The control. If this fails, every resumption assertion below is vacuous —
// the client never resumed, so nothing was ever asked to re-check a pin.
func TestTheResumptionHarnessActuallyResumes(t *testing.T) {
	addr, leaf, roots := tlsPinTestServer(t)
	pin := account.SPKIHash(leaf)
	cache := tls.NewLRUClientSessionCache(8)

	if _, err := dialPinned(t, addr, pin, roots, cache); err != nil {
		t.Fatalf("first pinned connection failed: %v", err)
	}
	resumed, err := dialPinned(t, addr, pin, roots, cache)
	if err != nil {
		t.Fatalf("second pinned connection failed: %v", err)
	}
	if !resumed {
		t.Fatal("the second connection did not resume, so this file's resumption tests " +
			"prove nothing. Fix the harness before trusting them — a test that cannot " +
			"demonstrate its own precondition is decoration.")
	}
}

// The finding itself. A session established under one pin must not carry a
// connection through once the expected pin no longer matches.
func TestThePinIsEnforcedOnResumedSessionsToo(t *testing.T) {
	addr, leaf, roots := tlsPinTestServer(t)
	goodPin := account.SPKIHash(leaf)
	cache := tls.NewLRUClientSessionCache(8)

	// Establish and cache a session the honest way.
	if _, err := dialPinned(t, addr, goodPin, roots, cache); err != nil {
		t.Fatalf("first pinned connection failed: %v", err)
	}

	// Now the expected pin changes — rotated, revoked, or re-pinned after the
	// user was told their mail was being intercepted. The next connection
	// resumes, and must still be refused.
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherDER, err := x509.CreateCertificate(rand.Reader,
		&x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "other"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)},
		&x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "other"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)},
		&otherKey.PublicKey, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	otherCert, err := x509.ParseCertificate(otherDER)
	if err != nil {
		t.Fatal(err)
	}
	wrongPin := account.SPKIHash(otherCert)
	if wrongPin == goodPin {
		t.Fatal("the two pins collided; the test cannot distinguish them")
	}

	resumed, err := dialPinned(t, addr, wrongPin, roots, cache)
	if err == nil {
		t.Fatalf("a connection whose pin does not match the server was ACCEPTED "+
			"(resumed=%v). The pin is only consulted during certificate verification, "+
			"which a resumed session skips — so the pin is a first-handshake-only "+
			"control and a rotated or revoked pin never takes effect. Enforce it in "+
			"VerifyConnection, which Go calls on every handshake.", resumed)
	}
	if !strings.Contains(err.Error(), "pin mismatch") {
		t.Fatalf("connection was refused, but not by the pin check: %v\n"+
			"The pin has to be the thing that says no, or the refusal is incidental "+
			"and the next change to the handshake removes it.", err)
	}
}

// An unpinned account must remain ordinary: normal WebPKI verification, and a
// server it does not trust is still refused. VerifyConnection must not have
// been bolted on in a way that changes the no-pin path.
func TestAnUnpinnedAccountStillRefusesAnUntrustedServer(t *testing.T) {
	addr, _, _ := tlsPinTestServer(t)

	cfg := account.Config{IMAPHost: pinTestHost}
	if cfg.TLSConfig() != nil {
		t.Fatal("an account with no pin must produce a nil TLS config so the dialer " +
			"uses ordinary verification")
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: pinTestHost, MinVersion: tls.VersionTLS12})
	if err == nil {
		_ = conn.Close()
		t.Fatal("an untrusted self-signed server was accepted with default verification")
	}
	var unknown x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if !errors.As(err, &unknown) && !errors.As(err, &hostErr) && !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}
