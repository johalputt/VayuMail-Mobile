package test

// constitution_test.go — the architecture rules, enforced instead of asserted.
//
// The repository's standing instructions carry an "architecture constitution"
// with three rules and the words "do not violate". Nothing checked any of them.
//
// A rule with no test is a rule that drifts, and this audit found the drift by
// grepping rather than by a failing build: two packages under internal/ import
// Gio, and a layout package reaches the network directly. Both turned out to be
// defensible — one is a JNI platform seam that genuinely needs the Android app
// handle, the other is a background goroutine with a deadline — but nobody
// decided that, because nothing ever asked.
//
// So these tests do two jobs. They pin the rules, and they force every
// exception to be WRITTEN DOWN with its reason. An allowlist entry is a
// decision somebody made on purpose; an unlisted violation is one nobody did.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gioAllowedOutsideUI are packages that import Gio despite living outside ui/
// and platform/, each with the reason it is not a violation.
//
// Both are JNI bridges. gogio produces no Gradle project, so a platform seam
// reaches Android through gioui.org/app's JVM handle and receives its callbacks
// as gioui.org/io/event values. There is no way to write the bridge without
// naming those types, and moving the packages under platform/ would move the
// seam without removing the import.
var gioAllowedOutsideUI = map[string]string{
	"internal/biometric":  "JNI bridge to the framework BiometricPrompt; needs gioui.org/app's JVM handle",
	"internal/pushnotify": "JNI bridge for tappable notifications; same Android app handle and event types",
}

// networkAllowedInLayout are layout packages that touch the network, with the
// reason. Rule 5 wants this routed through ui/state or the syncmanager; account
// setup runs before either exists for the account being created.
var networkAllowedInLayout = map[string]string{
	"ui/screens": "account setup discovers autoconfig and redeems a setup code " +
		"before any account, syncmanager or state loader exists for it; both calls " +
		"run on a background goroutine under a context deadline",
}

// goPackagesUnder returns every directory below root that holds .go files,
// as a slash-separated path relative to the repository root.
func goPackagesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join("..", root), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || strings.Contains(path, "/.git") {
			return nil
		}
		ents, rerr := os.ReadDir(path)
		if rerr != nil {
			return nil
		}
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".go") {
				rel := strings.TrimPrefix(filepath.ToSlash(path), "../")
				out = append(out, rel)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// importsOf returns every import path in pkgDir, across all build tags.
//
// Parsed rather than grepped. A grep for "gioui.org" matches the word inside a
// comment — and this file's own allowlist explains the exception in prose that
// names the package, so a grep-based check would report a violation in the
// sentence describing why it is not one.
func importsOf(t *testing.T, pkgDir string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	ents, err := os.ReadDir(filepath.Join("..", pkgDir))
	if err != nil {
		return out
	}
	fset := token.NewFileSet()
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		full := filepath.Join("..", pkgDir, e.Name())
		f, err := parser.ParseFile(fset, full, nil, parser.ImportsOnly)
		if err != nil {
			continue // a file that does not parse is the compiler's problem
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			out[p] = append(out[p], e.Name())
		}
	}
	return out
}

// RULE 4 — only ui/ and platform/ may import Gio. internal/** and ui/state/**
// stay UI-framework-free, so the engine can be driven headless, tested without
// a display, and re-targeted at another toolkit without touching its logic.
func TestRule4OnlyUIAndPlatformImportGio(t *testing.T) {
	var pkgs []string
	pkgs = append(pkgs, goPackagesUnder(t, "internal")...)
	pkgs = append(pkgs, goPackagesUnder(t, "ui/state")...)

	if len(pkgs) == 0 {
		t.Fatal("found no packages to check; this guard is blind")
	}

	for _, pkg := range pkgs {
		imports := importsOf(t, pkg)
		var gio []string
		for path, files := range imports {
			if strings.HasPrefix(path, "gioui.org") {
				gio = append(gio, path+" ("+strings.Join(files, ", ")+")")
			}
		}
		if len(gio) == 0 {
			continue
		}
		if why, ok := gioAllowedOutsideUI[pkg]; ok {
			t.Logf("Rule 4 exception: %s imports Gio — %s", pkg, why)
			continue
		}
		t.Errorf("Rule 4 violation: %s imports Gio: %s\n"+
			"Only ui/ and platform/ may depend on the UI toolkit. The engine has to stay "+
			"drivable headless and testable without a display. If this really is a platform "+
			"seam, add it to gioAllowedOutsideUI WITH its reason — an exception somebody wrote "+
			"down is a decision; an unlisted one is an accident.", pkg, strings.Join(gio, "; "))
	}
}

// The allowlist must not outlive its subjects. An entry for a package that no
// longer imports Gio is a permission nobody needs and the next reader trusts.
func TestRule4AllowlistHasNoStaleEntries(t *testing.T) {
	for pkg, why := range gioAllowedOutsideUI {
		if why == "" {
			t.Errorf("%s is allowlisted with no reason; an exception without a justification is "+
				"indistinguishable from an oversight", pkg)
		}
		imports := importsOf(t, pkg)
		found := false
		for path := range imports {
			if strings.HasPrefix(path, "gioui.org") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is allowlisted for Gio and no longer imports it. Remove the entry — a "+
				"standing exception nobody needs is one the next change quietly relies on", pkg)
		}
	}
}

// Two gates enforce Rule 4 — this file and scripts/constitution.sh — and they
// carry separate allowlists. Two lists that must agree and nothing checking
// that they do is a slow leak: someone adds a package to the shell list to get
// a red build green, this file never learns about it, and the exception now
// exists in the half of the enforcement that runs first.
//
// So the lists are compared directly. Adding a Gio exception means editing both
// or failing here, which is the point.
func TestTheShellAndGoConstitutionAgreeOnRule4(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "scripts", "constitution.sh"))
	if err != nil {
		t.Fatalf("read constitution.sh: %v", err)
	}
	const marker = "GIO_ALLOWED=\""
	i := strings.Index(string(src), marker)
	if i < 0 {
		t.Fatal("scripts/constitution.sh no longer defines GIO_ALLOWED. If Rule 4 moved, " +
			"move this check with it — do not delete it and leave one gate unwitnessed.")
	}
	rest := string(src)[i+len(marker):]
	j := strings.Index(rest, "\"")
	if j < 0 {
		t.Fatal("GIO_ALLOWED is not a closed quoted string")
	}

	shell := map[string]bool{}
	for _, f := range strings.Fields(rest[:j]) {
		shell[f] = true
	}
	for pkg := range gioAllowedOutsideUI {
		if !shell[pkg] {
			t.Errorf("%s is allowlisted for Gio here but NOT in scripts/constitution.sh, so the "+
				"shell gate fails the build on a package this file considers legitimate", pkg)
		}
	}
	for pkg := range shell {
		if _, ok := gioAllowedOutsideUI[pkg]; !ok {
			t.Errorf("%s is allowlisted for Gio in scripts/constitution.sh but not here, so the "+
				"shell gate permits an import this file has never agreed to and no reason was "+
				"ever written down for it", pkg)
		}
	}
}

// RULE 5 — layout code never touches SQLite or the network. Mutations go
// through the syncmanager command channel or an async loader in ui/state, so a
// frame is never blocked on I/O and a screen is never the thing that opened a
// socket.
//
// Importing internal/store for its TYPES is not a violation: a message row has
// to be named to be drawn. The rule is about performing I/O, so this checks the
// packages that do it — database/sql and net/http.
func TestRule5LayoutDoesNotOpenSocketsOrDatabases(t *testing.T) {
	pkgs := append(goPackagesUnder(t, "ui/screens"), goPackagesUnder(t, "ui/widgets")...)
	if len(pkgs) == 0 {
		t.Fatal("found no layout packages to check; this guard is blind")
	}

	forbidden := map[string]string{
		"database/sql": "SQLite",
		"net/http":     "the network",
	}
	for _, pkg := range pkgs {
		imports := importsOf(t, pkg)
		for path, what := range forbidden {
			files, used := imports[path]
			if !used {
				continue
			}
			if why, ok := networkAllowedInLayout[pkg]; ok {
				t.Logf("Rule 5 exception: %s imports %s — %s", pkg, path, why)
				continue
			}
			t.Errorf("Rule 5 violation: %s imports %s, so layout code reaches %s directly (%s).\n"+
				"Route it through the syncmanager command channel or an async loader in ui/state. "+
				"A frame blocked on I/O is a frozen app, and a screen that opens its own socket is "+
				"one nothing can rate-limit, cancel or pin.",
				pkg, path, what, strings.Join(files, ", "))
		}
	}
}

// Same staleness rule for the layout exceptions.
func TestRule5AllowlistHasNoStaleEntries(t *testing.T) {
	for pkg, why := range networkAllowedInLayout {
		if why == "" {
			t.Errorf("%s is allowlisted with no reason", pkg)
		}
		imports := importsOf(t, pkg)
		if _, ok := imports["net/http"]; !ok {
			if _, ok := imports["database/sql"]; !ok {
				t.Errorf("%s is allowlisted for direct I/O and performs none. Remove the entry.", pkg)
			}
		}
	}
}

// RULE 6 — secrets live in the platform keystore, never in SQLite. The narrow
// mechanical half of that: the store package must not reach for the keystore,
// because a secret that passes through the storage layer is one edit away from
// being persisted by it.
func TestRule6TheStoreNeverTouchesTheKeystore(t *testing.T) {
	for _, pkg := range goPackagesUnder(t, "internal/store") {
		for path := range importsOf(t, pkg) {
			if strings.HasSuffix(path, "internal/crypto") {
				t.Errorf("Rule 6 violation: %s imports %s. Secrets belong in the platform keystore "+
					"and must never travel through the SQLite layer — a private key that reaches "+
					"this package is one line from being written to a row, which is the exact defect "+
					"audit H6 was raised for.", pkg, path)
			}
		}
	}
}
