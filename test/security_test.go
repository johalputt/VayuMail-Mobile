package test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"go.uber.org/goleak"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// selfSignedCert builds a throwaway certificate for pin tests.
func selfSignedCert(t *testing.T) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "mail.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestSPKIPinning(t *testing.T) {
	certA := selfSignedCert(t)
	certB := selfSignedCert(t)

	pinA := account.SPKIHash(certA)
	if pinA == "" || pinA == account.SPKIHash(certB) {
		t.Fatal("SPKI hashes must be distinct and non-empty")
	}

	cfg := account.Config{IMAPHost: "mail.example.com", PinnedSPKI: pinA}
	tlsCfg := cfg.TLSConfig()
	if tlsCfg == nil {
		t.Fatal("pinned config must produce a TLS config")
	}
	verify := tlsCfg.VerifyPeerCertificate

	// Matching chain passes.
	if err := verify(nil, [][]*x509.Certificate{{certA}}); err != nil {
		t.Errorf("matching pin rejected: %v", err)
	}
	// Any other key is refused — the MITM case.
	if err := verify(nil, [][]*x509.Certificate{{certB}}); err == nil {
		t.Error("mismatched pin accepted — pinning is ineffective")
	}

	// Unpinned config keeps standard verification (nil TLS config).
	unpinned := account.Config{IMAPHost: "mail.example.com"}
	if unpinned.TLSConfig() != nil {
		t.Error("unpinned account must use default verification")
	}
}

func TestWKDAddressHashing(t *testing.T) {
	// Known-answer test from the WKD draft: the local part "Joe.Doe"
	// hashes to this z-base-32 string.
	got := pgp.WKDHash("Joe.Doe")
	want := "iy9q119eutrkn8s1mk4r39qejnbu3n5q"
	if got != want {
		t.Errorf("WKDHash = %q, want %q", got, want)
	}
}

func TestSealedBlobRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plaintext := []byte(`{"theme":"system","cadence":300}`)

	sealed, err := appcrypto.SealBlob(key, plaintext, "vayumail-settings")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("blob is not encrypted")
	}
	opened, err := appcrypto.OpenBlob(key, sealed, "vayumail-settings")
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("open = %q, %v", opened, err)
	}
	// Wrong key or wrong context must fail.
	otherKey := bytes.Repeat([]byte{8}, 32)
	if _, err := appcrypto.OpenBlob(otherKey, sealed, "vayumail-settings"); err == nil {
		t.Error("wrong key opened the blob")
	}
	if _, err := appcrypto.OpenBlob(key, sealed, "other-context"); err == nil {
		t.Error("wrong context opened the blob")
	}
}

// TestSettingsSyncViaIMAP proves the multi-device settings path end to
// end against the in-memory IMAP server: seal, push, pull, open.
func TestSettingsSyncViaIMAP(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"))

	addr, closeSrv := startIMAPServer(t)
	defer closeSrv()
	client := dialTestClient(t, addr)
	defer func() { _ = client.Close() }()

	key := bytes.Repeat([]byte{3}, 32)
	settings := []byte(`{"signature":"sent from vayumail"}`)
	sealed, err := appcrypto.SealBlob(key, settings, "vayumail-settings")
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SaveSettingsBlob(client, sealed); err != nil {
		t.Fatal(err)
	}

	got, err := imapsync.LoadSettingsBlob(client)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := appcrypto.OpenBlob(key, got, "vayumail-settings")
	if err != nil || !bytes.Equal(opened, settings) {
		t.Fatalf("round trip = %q, %v", opened, err)
	}

	// A second push wins: newest blob is returned.
	settings2 := []byte(`{"signature":"v2"}`)
	sealed2, err := appcrypto.SealBlob(key, settings2, "vayumail-settings")
	if err != nil {
		t.Fatal(err)
	}
	if err := imapsync.SaveSettingsBlob(client, sealed2); err != nil {
		t.Fatal(err)
	}
	got2, err := imapsync.LoadSettingsBlob(client)
	if err != nil {
		t.Fatal(err)
	}
	opened2, err := appcrypto.OpenBlob(key, got2, "vayumail-settings")
	if err != nil || !bytes.Equal(opened2, settings2) {
		t.Fatalf("second round trip = %q, %v", opened2, err)
	}
	if err := client.Logout().Wait(); err != nil {
		t.Logf("logout: %v", err)
	}
}
