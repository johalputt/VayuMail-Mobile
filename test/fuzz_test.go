package test

import (
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
)

// FuzzMIMEParse hammers the message parser — the largest
// attacker-controlled surface in the app (every byte comes from the
// network). The parser must never panic; any input either parses
// best-effort or returns an error.
func FuzzMIMEParse(f *testing.F) {
	for _, fixture := range []string{"simple.eml", "multipart.eml", "pgp.eml", "htmlonly.eml"} {
		raw, err := fixtureBytes(fixture)
		if err == nil {
			f.Add(raw)
		}
	}
	f.Add([]byte("From: a@b\r\n\r\nbody"))
	f.Add([]byte("Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\n--x--"))
	f.Add([]byte{0x00, 0xFF, 0x0D, 0x0A})

	f.Fuzz(func(t *testing.T, raw []byte) {
		parsed, err := mime.Parse(raw)
		if err == nil && parsed != nil {
			// Derived renderers must also hold up on whatever parsed.
			_ = mime.DisplayText(parsed.Text, parsed.HTML)
			_ = mime.DetectTrackers(parsed.HTML)
			_ = mime.Snippet(parsed.Text, parsed.HTML)
		}
	})
}

// FuzzHTMLToText covers the sanitizing renderer with adversarial HTML.
func FuzzHTMLToText(f *testing.F) {
	f.Add("<script>x</script><p>ok</p>")
	f.Add("<img src=x width=1 height=1>")
	f.Add("<<<>>><style>*{}</style>")
	f.Fuzz(func(t *testing.T, src string) {
		_ = mime.HTMLToText(src)
		_ = mime.DetectTrackers(src)
	})
}
