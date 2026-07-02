// Command vayumail-cli exercises the VayuMail engine headlessly — no Gio,
// no window, 100%% cgo-free. It exists to debug the engine, to prove the
// package boundary (ADR-0001), and to script account operations.
//
// Credentials are taken from the VAYUMAIL_PASSWORD environment variable
// and held in an in-memory keystore for the life of the process; nothing
// is ever written to disk (Rule 6).
//
// Usage:
//
//	vayumail-cli [-db PATH] <command> [flags]
//
// Commands: accounts, add-account, sync, watch, search, qr-verify
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	global := flag.NewFlagSet("vayumail-cli", flag.ContinueOnError)
	dbPath := global.String("db", defaultDBPath(), "path to the SQLite database")
	if err := global.Parse(args); err != nil {
		return 2
	}
	rest := global.Args()
	if len(rest) == 0 {
		usage()
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// qr-verify needs no database.
	if rest[0] == "qr-verify" {
		return cmdQRVerify(rest[1:])
	}

	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		return 1
	}
	defer db.Close()

	switch rest[0] {
	case "accounts":
		return cmdAccounts(ctx, db)
	case "add-account":
		return cmdAddAccount(ctx, db, rest[1:])
	case "sync":
		return cmdSync(ctx, db, rest[1:])
	case "watch":
		return cmdWatch(ctx, db, rest[1:])
	case "search":
		return cmdSearch(ctx, db, rest[1:])
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `vayumail-cli — headless VayuMail engine

  vayumail-cli [-db PATH] accounts
  vayumail-cli [-db PATH] add-account -email a@b.c -host mail.b.c [-imap-port 993] [-smtp-port 587]
  vayumail-cli [-db PATH] sync   -account ID   (VAYUMAIL_PASSWORD env)
  vayumail-cli [-db PATH] watch  -account ID   (VAYUMAIL_PASSWORD env)
  vayumail-cli [-db PATH] search -account ID -query TEXT
  vayumail-cli qr-verify -file payload.b64

`)
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "vayumail.db"
	}
	return filepath.Join(home, ".vayumail", "vayumail.db")
}
