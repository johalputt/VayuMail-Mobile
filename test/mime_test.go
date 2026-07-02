package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
)

func readMIMEFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("fixtures", "mime", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestParseSimple(t *testing.T) {
	p, err := mime.Parse(readMIMEFixture(t, "simple.eml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Text, "The wind carries this message.") {
		t.Errorf("text = %q", p.Text)
	}
	if p.HTML != "" {
		t.Errorf("unexpected HTML part: %q", p.HTML)
	}
	if len(p.Attachments) != 0 {
		t.Errorf("attachments = %v", p.Attachments)
	}
	if p.PGPStatus != "" {
		t.Errorf("pgp status = %q", p.PGPStatus)
	}
	if !strings.HasPrefix(p.Snippet, "Hello Bob,") {
		t.Errorf("snippet = %q", p.Snippet)
	}
}

func TestParseMultipartWithAttachment(t *testing.T) {
	p, err := mime.Parse(readMIMEFixture(t, "multipart.eml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Text, "Plain body here.") {
		t.Errorf("text = %q", p.Text)
	}
	if !strings.Contains(p.HTML, "HTML body") {
		t.Errorf("html = %q", p.HTML)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("attachments = %v", p.Attachments)
	}
	if p.Attachments[0].Filename != "report.pdf" {
		t.Errorf("filename = %q", p.Attachments[0].Filename)
	}
	if p.Attachments[0].ContentType != "application/pdf" {
		t.Errorf("content type = %q", p.Attachments[0].ContentType)
	}
}

func TestParsePGPEncrypted(t *testing.T) {
	p, err := mime.Parse(readMIMEFixture(t, "pgp.eml"))
	if err != nil {
		t.Fatal(err)
	}
	if p.PGPStatus != "encrypted" {
		t.Errorf("pgp status = %q, want encrypted", p.PGPStatus)
	}
}

func TestHTMLSanitization(t *testing.T) {
	p, err := mime.Parse(readMIMEFixture(t, "htmlonly.eml"))
	if err != nil {
		t.Fatal(err)
	}
	text := mime.DisplayText(p.Text, p.HTML)
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "world") {
		t.Errorf("display text = %q", text)
	}
	if strings.Contains(text, "iframe") || strings.Contains(text, "evil.example") {
		t.Errorf("active content leaked into display text: %q", text)
	}
}

func TestHTMLToTextStripsScriptAndStyle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ban  string
	}{
		{"script dropped", `<p>safe</p><script>alert("x")</script>`, "safe", "alert"},
		{"style dropped", `<style>p{}</style><p>body</p>`, "body", "p{}"},
		{"blocks separated by line breaks", `<div>a</div><div>b</div>`, "a\n\nb", ""},
		{"entities decoded", `<p>a &amp; b</p>`, "a & b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mime.HTMLToText(tt.in)
			if !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want containing %q", got, tt.want)
			}
			if tt.ban != "" && strings.Contains(got, tt.ban) {
				t.Errorf("got %q, must not contain %q", got, tt.ban)
			}
		})
	}
}

func TestQuoteDepth(t *testing.T) {
	tests := []struct {
		line string
		want int
	}{
		{"no quote", 0},
		{"> one", 1},
		{"> > two", 2},
		{">>> three", 3},
		{"  > indented", 1},
	}
	for _, tt := range tests {
		if got := mime.QuoteDepth(tt.line); got != tt.want {
			t.Errorf("QuoteDepth(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestSnippetCapsAt160Runes(t *testing.T) {
	long := strings.Repeat("gale ", 100)
	s := mime.Snippet(long, "")
	if got := len([]rune(s)); got > 160 {
		t.Errorf("snippet rune length = %d", got)
	}
	if strings.Contains(s, "\n") {
		t.Error("snippet must be single-line")
	}
}
