// Command vayumail is the production entrypoint: it opens the local
// store, starts the sync manager, and runs the Gio window until close.
//
// Startup order (docs/ARCHITECTURE.md):
//  1. open SQLite  2. create Manager  3. Start Manager (one goroutine set
//     per account)  4. run the Gio event loop  5. on close: cancel context,
//     Manager drains and exits, close DB.
package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"gioui.org/app"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui"
)

func main() {
	go func() {
		os.Exit(run())
	}()
	app.Main()
}

func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPath, err := databasePath()
	if err != nil {
		slog.Error("resolve data dir", "err", err)
		return 1
	}
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		slog.Error("open store", "err", err)
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close store", "err", err)
		}
	}()

	mgr := syncmanager.New(db, keystore())
	if err := mgr.Start(ctx); err != nil {
		slog.Error("start sync manager", "err", err)
		return 1
	}
	defer mgr.Shutdown()

	window := new(app.Window)
	window.Option(app.Title("VayuMail"))

	if err := ui.New(ctx, window, db, mgr).Run(); err != nil {
		slog.Error("window", "err", err)
		return 1
	}
	// Window closed: cancel sync before the deferred shutdown waits.
	cancel()
	return 0
}

// keystore selects the platform keystore when a gomobile bridge is
// registered, else falls back to the in-memory store (desktop test runs:
// credentials then live only for the process, never on disk — Rule 6
// holds either way).
func keystore() appcrypto.Keystore {
	p := appcrypto.NewPlatformKeystore()
	if _, err := p.Fetch("vayumail-probe"); err == appcrypto.ErrNoPlatformKeystore {
		slog.Warn("no platform keystore bridge; using in-memory keystore (credentials last one session)")
		return appcrypto.NewMemoryKeystore()
	}
	return p
}

// databasePath places vayumail.db inside the platform data directory.
func databasePath() (string, error) {
	dir, err := app.DataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "vayumail")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "vayumail.db"), nil
}
