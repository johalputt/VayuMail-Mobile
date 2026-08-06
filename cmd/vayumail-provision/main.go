// Command vayumail-provision is the reference implementation of the
// VayuMail setup-code provisioning service (ADR-0003, amended by
// ADR-0009) — the server-side counterpart to internal/mail/account.
// VayuPress embeds this logic; any mail server operator can also run it
// standalone.
//
// It generates Ed25519-signed provisioning payloads (setup codes the
// user pastes into the app) and serves the one-time token exchange
// endpoint:
//
//	vayumail-provision -addr :8448 -server mail.example.com \
//	    -users users.txt [-key ed25519.seed]
//
// users.txt holds one "email:password" per line. Passwords are handed
// out exactly once per generated token, over the exchange endpoint.
//
// Endpoints:
//
//	GET  /code?user=email    base64url setup code (text/plain)
//	GET  /qr?user=email      same payload (legacy path, kept for old tooling)
//	POST /provision          {"token","username"} -> credentials JSON
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

func main() {
	// Loopback by default. The exchange endpoint dispenses mail passwords, so
	// the out-of-the-box posture should be "reachable by this machine", with
	// exposure an explicit decision the operator makes by passing -addr.
	addr := flag.String("addr", "127.0.0.1:8448", "listen address")
	server := flag.String("server", "", "mail server host (IMAP+SMTP)")
	imapPort := flag.Int("imap-port", 993, "IMAP port (implicit TLS)")
	smtpPort := flag.Int("smtp-port", 587, "SMTP port (STARTTLS)")
	usersFile := flag.String("users", "", "file of email:password lines")
	keyFile := flag.String("key", "", "Ed25519 seed file (32 bytes; generated when absent)")
	ttl := flag.Int("ttl", 900, "payload validity in seconds")
	adminToken := flag.String("admin-token", "",
		"bearer token required by GET /code (generated and logged when empty)")
	flag.Parse()

	if *server == "" || *usersFile == "" {
		fmt.Fprintln(os.Stderr, "usage: vayumail-provision -server HOST -users FILE [flags]")
		os.Exit(2)
	}

	users, err := loadUsers(*usersFile)
	if err != nil {
		slog.Error("load users", "err", err)
		os.Exit(1)
	}
	priv, err := loadOrCreateKey(*keyFile)
	if err != nil {
		slog.Error("load key", "err", err)
		os.Exit(1)
	}

	// Generated rather than defaulted to empty-means-open: a reference
	// implementation that starts unauthenticated is the one operators copy.
	token := *adminToken
	if token == "" {
		token, err = randomToken()
		if err != nil {
			slog.Error("generate admin token", "err", err)
			os.Exit(1)
		}
		slog.Warn("no -admin-token given; generated one for this run",
			"admin_token", token)
	}

	svc := newService(serviceConfig{
		Server:     *server,
		IMAPPort:   *imapPort,
		SMTPPort:   *smtpPort,
		TTL:        *ttl,
		Users:      users,
		Key:        priv,
		AdminToken: token,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /code", svc.handleSetupCode)
	mux.HandleFunc("GET /qr", svc.handleSetupCode) // legacy path
	mux.HandleFunc("POST /provision", svc.handleExchange)

	slog.Info("vayumail-provision listening",
		"addr", *addr, "server", *server, "users", len(users))
	// The old line here said only "serve this behind TLS", which named the
	// wrong control. TLS stops somebody reading the exchange off the wire; it
	// does nothing about anybody being able to ask for a setup code and redeem
	// it for a mail password. Authentication is what stops that, and TLS is
	// what keeps the redeemed credential private in transit. Both, in order.
	slog.Warn("GET /code requires the admin token; POST /provision hands out mail " +
		"credentials to whoever holds a valid one-time token — serve both behind TLS")
	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

// loadUsers parses "email:password" lines; blank lines and #comments are
// skipped.
func loadUsers(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	users := map[string]string{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		email, password, ok := strings.Cut(line, ":")
		if !ok || email == "" || password == "" {
			return nil, fmt.Errorf("line %d: want email:password", i+1)
		}
		users[strings.ToLower(email)] = password
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("no users in %s", path)
	}
	return users, nil
}

// loadOrCreateKey reads a 32-byte Ed25519 seed, generating and saving one
// when the file does not exist yet (0600).
func loadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		slog.Warn("ephemeral signing key — pass -key FILE to persist across restarts")
		return priv, nil
	}
	seed, err := os.ReadFile(path)
	if err == nil {
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("seed file must be exactly %d bytes", ed25519.SeedSize)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	seed = make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		return nil, err
	}
	slog.Info("generated new signing key", "path", path)
	return ed25519.NewKeyFromSeed(seed), nil
}
