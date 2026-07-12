package account

import "testing"

// TestTalkHostFor pins the trust boundary for the advertised VayuTalk host: the
// app sends the mailbox credential there, so only the mail domain or a subdomain
// of it is ever accepted — a tampered autoconfig can't redirect the credential to
// a foreign server.
func TestTalkHostFor(t *testing.T) {
	cases := []struct {
		advertised, domain, want string
	}{
		{"talk.example.com", "example.com", "talk.example.com"},
		{"example.com", "example.com", "example.com"},
		{"  TALK.Example.com ", "example.com", "talk.example.com"}, // trimmed + lowercased
		{"talk.evil.com", "example.com", ""},                       // foreign host: rejected
		{"example.com.evil.com", "example.com", ""},                // suffix-spoof: rejected
		{"notexample.com", "example.com", ""},                      // not a subdomain: rejected
		{"", "example.com", ""},                                    // none advertised
		{"talk.example.com:9999", "example.com", ""},               // host with port: rejected
	}
	for _, c := range cases {
		if got := talkHostFor(c.advertised, c.domain); got != c.want {
			t.Errorf("talkHostFor(%q, %q) = %q, want %q", c.advertised, c.domain, got, c.want)
		}
	}
}
