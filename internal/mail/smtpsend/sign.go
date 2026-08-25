package smtpsend

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/emersion/go-message"
)

// CanonicalSignedPart renders the draft body as the exact text/plain MIME
// entity that gets signed for an RFC 3156 multipart/signed message: part
// headers, blank line, and the body in CRLF canonical form ending with a
// final CRLF. Sign THESE bytes, pass them here again, and BuildPGPSigned
// will embed them verbatim — any other split risks signing bytes that are
// not byte-identical to what travels on the wire, which every receiver
// would reject as a bad signature.
func CanonicalSignedPart(d *Draft) ([]byte, error) {
	if strings.TrimSpace(d.TextBody) == "" {
		return nil, fmt.Errorf("smtpsign: empty body")
	}
	var b bytes.Buffer
	// Mime-Version must be part of the SIGNED bytes: go-message's entity
	// writer emits it whenever it serializes a MIME part, so leaving it out
	// would make every receiver parse back different bytes than we signed
	// (test/signedsend_test.go pins this round-trip).
	b.WriteString("Mime-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	// CRLF canonicalization (RFC 3156 §3): every bare LF becomes CRLF so
	// the signature covers one deterministic representation regardless of
	// what the composer held in memory.
	body := strings.ReplaceAll(d.TextBody, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}
	return b.Bytes(), nil
}

// BuildPGPSigned wraps an already-signed draft into an RFC 3156
// multipart/signed message. canonical is the exact entity bytes that
// armoredSig covers (see CanonicalSignedPart); it is embedded verbatim as
// the first part, because a signature is only valid over byte-identical
// content. The second part carries the ASCII-armored detached signature.
//
// The multipart framing is assembled by hand rather than through go-message's
// part writer for one reason: RFC 3156 requires the first part's trailing
// CRLF inside the signature, while MIME boundary parsing normally eats the
// CRLF that precedes a delimiter. Emitting canonical plus one extra CRLF
// before the boundary means parsers recover exactly the signed bytes —
// test/signedsend_test.go proves the round-trip rather than trusting it.
func BuildPGPSigned(d *Draft, canonical, armoredSig []byte) ([]byte, error) {
	if len(canonical) == 0 {
		return nil, fmt.Errorf("smtpsign: empty canonical part")
	}
	if !strings.Contains(string(armoredSig), "-----BEGIN PGP SIGNATURE-----") {
		return nil, fmt.Errorf("smtpsign: missing armored PGP signature")
	}
	header, err := draftHeader(d)
	if err != nil {
		return nil, err
	}
	boundary, err := randomBoundary()
	if err != nil {
		return nil, err
	}
	header.SetContentType("multipart/signed", map[string]string{
		"protocol": `application/pgp-signature`,
		"micalg":   `pgp-sha256`,
		"boundary": boundary,
	})

	sig := strings.ReplaceAll(string(armoredSig), "\r\n", "\n")
	sig = strings.ReplaceAll(sig, "\n", "\r\n")

	var buf bytes.Buffer
	w, err := message.CreateWriter(&buf, header.Header)
	if err != nil {
		return nil, fmt.Errorf("smtpsend: create signed header: %w", err)
	}
	// CreateWriter flushes the top-level headers immediately; Write passes
	// our hand-framed multipart body through untouched.
	var b bytes.Buffer
	b.WriteString("--" + boundary + "\r\n")
	b.Write(canonical)
	b.WriteString("\r\n") // keeps the signed trailing CRLF out of the delimiter
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(sig)
	if !strings.HasSuffix(sig, "\r\n") {
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	if _, err := w.Write(b.Bytes()); err != nil {
		return nil, fmt.Errorf("smtpsend: write signed body: %w", err)
	}
	return buf.Bytes(), nil
}

// randomBoundary returns 128 bits of hex entropy framed as a legal MIME
// boundary. crypto/rand because a predictable boundary is a spoofing tool:
// attacker-chosen text containing the boundary string could splice parts.
func randomBoundary() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("smtpsign: boundary: %w", err)
	}
	return "vayu-" + hex.EncodeToString(raw), nil
}
