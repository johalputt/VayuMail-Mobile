package test

// transport_security_test.go — invariants the transport must never lose.
//
// Each of these is currently TRUE. That is exactly why they are written down:
// an invariant nothing checks is one a future change can spend without anybody
// noticing, and the ones here are the difference between mail that cannot be
// read in transit and mail that can.
//
// They are static checks over production source. A behavioural test would be
// better where one is possible, but "no code anywhere disables certificate
// verification" is a property of the whole tree, not of a function.

import (
	"crypto/tls"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// productionGoFiles walks the repo for non-test .go files.
func productionGoFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	skip := map[string]bool{".git": true, "test": true, "assets": true, "docs": true}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			// The walk root is ".." — its base name starts with a dot, so a
			// naive dotfile filter skips the entire repository and every check
			// below passes on zero files. The "this guard is blind" fatal is
			// what caught that; a guard that cannot see anything must fail, not
			// report success.
			if path == ".." {
				return nil
			}
			if skip[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no production Go files found; this guard is blind")
	}
	return out
}

// THE one that matters most. Certificate verification is what stands between a
// user's mail password and anyone on the same network.
func TestNothingEverDisablesCertificateVerification(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), "InsecureSkipVerify") {
			t.Errorf("%s mentions InsecureSkipVerify. Turning off certificate verification in a "+
				"mail client hands every password and every message to anyone on the path — and it "+
				"is the single most common 'temporary' change that ships.", f)
		}
	}
}

// Every tls.Config this app builds must state its floor. Inheriting the
// toolchain's default means the floor moves when the toolchain does.
func TestEveryTLSConfigStatesAMinimumVersion(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range productionGoFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Config" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "tls" {
				return true
			}
			for _, el := range lit.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "MinVersion" {
						return true
					}
				}
			}
			t.Errorf("%s:%d builds a tls.Config with no MinVersion. The floor must be stated, not "+
				"inherited from whichever Go release happens to compile this.",
				path, fset.Position(lit.Pos()).Line)
			return true
		})
	}
}

// A custom certificate check that lives only in VerifyPeerCertificate is a
// check that stops running. Go calls it during certificate verification, and a
// resumed session does not verify certificates — the chain comes back out of
// the ticket. So the callback is skipped and whatever it was enforcing quietly
// applies to first handshakes only.
//
// tls_resumption_audit_test.go demonstrates this against a live listener, with
// the wrong pin accepted on a resumed connection. This is the static half: it
// stops the shape coming back anywhere in the tree, including in a config no
// behavioural test happens to reach.
func TestACustomCertificateCheckAlsoRunsOnResumedSessions(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0
	for _, path := range productionGoFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Config" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "tls" {
				return true
			}
			var hasPeer, hasConn bool
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch k.Name {
				case "VerifyPeerCertificate":
					hasPeer = true
				case "VerifyConnection":
					hasConn = true
				}
			}
			if hasPeer {
				checked++
			}
			if hasPeer && !hasConn {
				t.Errorf("%s:%d sets VerifyPeerCertificate without VerifyConnection. The callback "+
					"is not invoked on a resumed session, so this check runs on the first handshake "+
					"and never again for the life of the cached session — a pin that cannot be "+
					"rotated or revoked. Put the same check in VerifyConnection, which Go calls on "+
					"every handshake.",
					path, fset.Position(lit.Pos()).Line)
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("found no tls.Config with a custom certificate check; this guard is blind. " +
			"If the pinning config moved, move this check with it.")
	}
}

// And the pinned config's floor is actually 1.2 or better — the static check
// above proves the field exists, this proves the value.
func TestThePinnedTLSConfigFloorIsTLS12OrBetter(t *testing.T) {
	cfg := account.Config{IMAPHost: "mail.example", PinnedSPKI: "sha256/AAAA"}
	tc := cfg.TLSConfig()
	if tc == nil {
		t.Fatal("a pinned account produced no TLS config, so the pin cannot be enforced")
	}
	if tc.MinVersion < tls.VersionTLS12 {
		t.Errorf("pinned TLS floor is 0x%04x, below TLS 1.2", tc.MinVersion)
	}
	if tc.VerifyPeerCertificate == nil {
		t.Error("a pinned account produced a config with no pin check; the SPKI pin is decoration")
	}
}

// An unpinned account must still get real verification — nil means "use the
// library's verified defaults", never "skip".
func TestAnUnpinnedAccountStillVerifiesNormally(t *testing.T) {
	cfg := account.Config{IMAPHost: "mail.example"}
	if tc := cfg.TLSConfig(); tc != nil && tc.InsecureSkipVerify {
		t.Fatal("an account without a pin disables verification entirely")
	}
}

// There must be no plaintext transport mode. The dial paths accept implicit
// TLS and STARTTLS and REFUSE anything else; a third branch that dialled in
// the clear would be the whole product's promise gone, quietly.
func TestTheDialPathsRefuseAnyModeThatIsNotTLS(t *testing.T) {
	for _, f := range []string{
		"../internal/mail/imapsync/client.go",
		"../internal/mail/smtpsend/client.go",
	} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		if !strings.Contains(src, "TLSModeImplicit") || !strings.Contains(src, "TLSModeSTARTTLS") {
			t.Errorf("%s no longer dials both TLS modes", f)
		}
		if !strings.Contains(src, "unsupported TLS mode") {
			t.Errorf("%s does not REFUSE an unrecognised TLS mode. A default branch that dials "+
				"anyway is a plaintext fallback with no name.", f)
		}
		// A bare net.Dial in a transport file is the shape a downgrade takes.
		if strings.Contains(src, "net.Dial(") {
			t.Errorf("%s dials a raw socket; every connection this app makes is TLS from the "+
				"first byte or STARTTLS-upgraded by the library", f)
		}
	}
}

// Secrets must never reach a log line. A password in logcat is readable by
// anything with log access and survives in bug reports.
func TestSecretsAreNeverLogged(t *testing.T) {
	suspicious := []string{
		"log.Printf(\"%s\", pass", "log.Println(pass", "log.Printf(\"%s\", secret",
		"log.Println(secret", "log.Printf(\"%s\", token", "log.Println(token",
		"fmt.Println(pass", "fmt.Println(secret",
	}
	for _, f := range productionGoFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(b)
		for _, pat := range suspicious {
			if strings.Contains(src, pat) {
				t.Errorf("%s appears to log a secret (%q). A credential in the device log is "+
					"readable by anything with log access and travels in every bug report.", f, pat)
			}
		}
	}
}

// Anything generating a secret, token or nonce uses crypto/rand. math/rand is
// predictable and has been the root of real key-recovery bugs.
func TestProductionCodeNeverUsesMathRand(t *testing.T) {
	for _, f := range productionGoFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(b)
		if strings.Contains(src, `"math/rand"`) || strings.Contains(src, `"math/rand/v2"`) {
			t.Errorf("%s imports math/rand. Anything seeded that way is predictable; secrets, "+
				"nonces and tokens must come from crypto/rand.", f)
		}
	}
}
