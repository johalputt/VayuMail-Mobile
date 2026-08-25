package test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/emersion/go-message"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
)

// signedFixture builds a draft plus a keyring holding the sender's private
// key, and returns the canonical part, the armored signature over it, the
// fully assembled RFC 3156 message, and the sender's PUBLIC key armored so
// verifiers can import exactly who signed.
func signedFixture(t *testing.T) (*smtpsend.Draft, *pgp.Keyring, []byte, []byte, []byte, []byte) {
	t.Helper()
	draft := &smtpsend.Draft{
		FromName: "Alice", FromAddr: "alice@example.com",
		To:       []string{"bob@example.com"},
		Subject:  "signed hello",
		TextBody: "first line\r\nsecond line\r\n",
	}

	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com")); err != nil {
		t.Fatalf("import key: %v", err)
	}
	fp, err := kr.FingerprintForEmail("alice@example.com")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	pub, err := kr.ExportPublicArmored(fp)
	if err != nil {
		t.Fatalf("export public: %v", err)
	}

	canonical, err := smtpsend.CanonicalSignedPart(draft)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	sig, err := kr.Sign(canonical, draft.FromAddr)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := smtpsend.BuildPGPSigned(draft, canonical, sig)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return draft, kr, canonical, sig, raw, pub
}

// splitMultipart parses a built message's body into its two parts' raw
// bytes using the boundary from the top-level Content-Type — the same
// operation every receiver performs before verifying.
func splitMultipart(t *testing.T, raw []byte) (part1, sigPart []byte) {
	t.Helper()
	msg, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	mediaType, params, _ := msg.Header.ContentType()
	if mediaType != "multipart/signed" {
		t.Fatalf("content type = %q, want multipart/signed", mediaType)
	}
	if got := params["protocol"]; got != "application/pgp-signature" {
		t.Fatalf("protocol = %q", got)
	}
	if got := params["micalg"]; got != "pgp-sha256" {
		t.Fatalf("micalg = %q", got)
	}
	mr := msg.MultipartReader()
	for i := 0; i < 2; i++ {
		p, err := mr.NextPart()
		if err != nil {
			t.Fatalf("part %d: %v", i+1, err)
		}
		var buf bytes.Buffer
		if err := p.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			part1 = buf.Bytes()
		} else {
			sigPart = buf.Bytes()
		}
	}
	return part1, sigPart
}

// The contract that makes RFC 3156 work end-to-end: the first part as a
// receiver parses it back out of the wire format must verify against the
// signature in the second part. A builder that signs different bytes than
// it embeds fails here, not on some user's machine years later.
func TestPGPSignedRoundTripVerifies(t *testing.T) {
	_, _, canonical, _, raw, pub := signedFixture(t)

	part1, sigPart := splitMultipart(t, raw)
	if !bytes.Equal(part1, canonical) {
		t.Fatalf("embedded part differs from signed bytes:\n got %q\nwant %q", part1, canonical)
	}

	// Verify as a receiver would: a fresh keyring holding only the
	// sender's public key.
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(pub); err != nil {
		t.Fatal(err)
	}
	status, err := kr.VerifyDetached(part1, sigPart)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if status != pgp.SigValid {
		t.Fatalf("status = %v, want SigValid", status)
	}
}

// Tampering with one byte of the signed content must break verification —
// the whole point of signing.
func TestPGPSignedTamperBreaksVerification(t *testing.T) {
	_, _, canonical, sig, _, _ := signedFixture(t)

	tampered := bytes.Replace(canonical, []byte("second"), []byte("second!"), 1)
	if bytes.Equal(tampered, canonical) {
		t.Fatal("tamper did not change bytes")
	}
	kr := pgp.NewKeyring()
	if _, err := kr.ImportArmored(newTestKey(t, "Alice", "alice@example.com")); err != nil {
		t.Fatal(err)
	}
	status, err := kr.VerifyDetached(tampered, sig)
	if status != pgp.SigInvalid && !(err != nil) {
		t.Fatalf("tampered content verified: status=%v err=%v", status, err)
	}
}

// The canonical part is CRLF-canonical even when the composer held bare-LF
// text, so the same logical message produces one stable signature.
func TestCanonicalSignedPartCRLFCanonicalizes(t *testing.T) {
	draft := &smtpsend.Draft{
		FromAddr: "alice@example.com", To: []string{"b@x"},
		Subject: "lf", TextBody: "a\nb\nc\n",
	}
	got, err := smtpsend.CanonicalSignedPart(draft)
	if err != nil {
		t.Fatal(err)
	}
	bodyIdx := bytes.Index(got, []byte("\r\n\r\n"))
	if bodyIdx < 0 {
		t.Fatalf("no header/body separator: %q", got)
	}
	body := string(got[bodyIdx+4:])
	if want := "a\r\nb\r\nc\r\n"; body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	// No bare LF or CR survives: every newline is exactly a CRLF pair.
	if stripped := strings.ReplaceAll(body, "\r\n", ""); strings.ContainsAny(stripped, "\r\n") {
		t.Fatalf("bare CR/LF remains after canonicalization: %q", body)
	}
}
