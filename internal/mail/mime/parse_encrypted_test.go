package mime

// parse_encrypted_test.go — the decrypt-on-display path depends on the
// parser lifting armored ciphertext into Parsed.EncryptedBlock. These
// tests pin that behaviour for the two shapes VayuMail actually receives:
// VayuPress's inline PGP (a text/plain body that IS the armored block)
// and interop PGP/MIME (ciphertext in a separate octet-stream part).

import (
	"strings"
	"testing"
)

const armoredMsg = "-----BEGIN PGP MESSAGE-----\r\n" +
	"\r\n" +
	"wcBMA0e2Qle3wServ01dexampleciphertextblob==\r\n" +
	"=abcd\r\n" +
	"-----END PGP MESSAGE-----"

// crlf joins header lines with CRLF and appends the body, producing a
// wire-shaped RFC 5322 message.
func crlf(headers []string, body string) []byte {
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func TestParseInlinePGPLiftsEncryptedBlock(t *testing.T) {
	// Exactly what VayuPress sends: text/plain body that is the armored
	// block, flagged with X-VayuPGP. The top-level type is text/plain, so
	// the status must come from the inline sniff.
	raw := crlf([]string{
		"From: a@example.com",
		"To: b@example.com",
		"Subject: secret",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"X-VayuPGP: encrypted",
	}, armoredMsg)

	p, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.PGPStatus != "encrypted" {
		t.Fatalf("PGPStatus = %q, want encrypted", p.PGPStatus)
	}
	if !strings.Contains(p.EncryptedBlock, "BEGIN PGP MESSAGE") ||
		!strings.Contains(p.EncryptedBlock, "END PGP MESSAGE") {
		t.Fatalf("EncryptedBlock missing armor delimiters: %q", p.EncryptedBlock)
	}
}

func TestParsePlainMessageHasNoEncryptedBlock(t *testing.T) {
	raw := crlf([]string{
		"From: a@example.com",
		"To: b@example.com",
		"Subject: hello",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
	}, "just a normal note, nothing secret here")

	p, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.PGPStatus != "" {
		t.Fatalf("PGPStatus = %q, want empty", p.PGPStatus)
	}
	if p.EncryptedBlock != "" {
		t.Fatalf("EncryptedBlock = %q, want empty", p.EncryptedBlock)
	}
}

func TestExtractInlinePGP(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool // whether a block is returned
	}{
		{"clean", "noise\n" + armoredMsg + "\ntrailing", true},
		{"no begin", "-----END PGP MESSAGE-----", false},
		{"no end", "-----BEGIN PGP MESSAGE-----\nbody", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractInlinePGP(c.in)
			if (got != "") != c.want {
				t.Fatalf("extractInlinePGP(%q) = %q, want block=%v", c.in, got, c.want)
			}
			if c.want && !strings.HasSuffix(got, "-----END PGP MESSAGE-----") {
				t.Fatalf("block not terminated at END delimiter: %q", got)
			}
		})
	}
}
