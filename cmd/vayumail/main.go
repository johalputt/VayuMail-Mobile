// Command vayumail is the production entrypoint.
//
// Startup order matters on Android: the window must exist and present its
// first frame immediately, or the OS keeps showing the splash forever.
// Everything that can block — data-dir resolution, SQLite open, keystore,
// sync manager, the dark-mode probe — runs in a background goroutine and
// is handed to the UI when ready. The boot loop (ui.Boot) renders an
// animated brand frame until then. See docs/ARCHITECTURE.md ("Startup").
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/x/explorer"
	xtheme "gioui.org/x/pref/theme"

	"github.com/johalputt/VayuMail-Mobile/internal/biometric"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/ui"
)

func main() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("VayuMail"))
		os.Exit(run(window))
	}()
	app.Main()
}

// run pumps frames from the very first event; the engine attaches when
// its background initialization completes.
func run(window *app.Window) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Platform file picker for composer attachments (SAF on Android, native
	// dialogs elsewhere). It must observe every window event, so it is wired to
	// the boot loop before Run starts.
	exp := explorer.NewExplorer(window)

	boot := ui.NewBoot(ctx, window)
	// Both the file picker and the biometric backend need to observe the
	// Android view lifecycle (BiometricPrompt needs the Activity behind the
	// current view), so the boot loop fans every event out to both.
	boot.SetEventListener(func(e event.Event) {
		exp.ListenEvents(e)
		biometric.HandleViewEvent(e)
	})
	go initEngine(ctx, window, boot, func() (io.ReadCloser, error) { return exp.ChooseFile() })

	err := boot.Run()
	cancel()
	boot.Shutdown()
	if err != nil {
		slog.Error("window", "err", err)
		return 1
	}
	return 0
}

// initEngine performs every blocking startup step off the UI thread and
// hands the result to the boot screen. Any failure is reported on screen
// rather than freezing the splash.
func initEngine(ctx context.Context, window *app.Window, boot *ui.Boot, pickFile func() (io.ReadCloser, error)) {
	dark := probeDarkMode()

	dbPath, err := databasePath()
	if err != nil {
		boot.Fail(err, "resolving the data directory")
		return
	}
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		boot.Fail(err, "opening the local store")
		return
	}

	// One keystore instance serves both the sync engine (credentials) and
	// the UI's app lock (PIN verifier): two instances over the same sealed
	// file could lose writes to each other.
	ks := keystore()
	mgr := syncmanager.New(db, ks)
	mgr.SetAttachmentsDir(filepath.Join(filepath.Dir(dbPath), "attachments"))
	if err := mgr.Start(ctx); err != nil {
		boot.Fail(err, "starting the sync engine")
		if cerr := db.Close(); cerr != nil {
			slog.Error("close store", "err", cerr)
		}
		return
	}

	boot.Attach(ui.New(ctx, window, db, mgr, ks, dark, pickFile), db, mgr)
}

// probeDarkMode asks the platform for the theme preference with a hard
// timeout: a wedged JNI call must never delay startup.
func probeDarkMode() bool {
	result := make(chan bool, 1)
	go func() {
		dark, err := xtheme.IsDarkMode()
		if err != nil {
			slog.Debug("dark mode preference unavailable", "err", err)
		}
		result <- dark
	}()
	select {
	case dark := <-result:
		return dark
	case <-time.After(2 * time.Second):
		slog.Warn("dark mode probe timed out; defaulting to light")
		return false
	}
}

// keystore selects the platform keystore when a gomobile bridge is
// registered, else the sealed AES-GCM store in the app-private data
// directory: credentials are encrypted at rest and survive restarts, and
// raw secrets never touch disk (Rule 6, ADR-0004). The in-memory store is
// the last-resort fallback if the data directory is unavailable.
func keystore() appcrypto.Keystore {
	p := appcrypto.NewPlatformKeystore()
	if _, err := p.Fetch("vayumail-probe"); err != appcrypto.ErrNoPlatformKeystore {
		return p
	}
	dir, err := app.DataDir()
	if err == nil {
		sealed, serr := appcrypto.NewSealedKeystore(filepath.Join(dir, "vayumail", "keys"))
		if serr == nil {
			return sealed
		}
		err = serr
	}
	slog.Warn("sealed keystore unavailable; credentials last one session", "err", err)
	return appcrypto.NewMemoryKeystore()
}

// databasePath places vayumail.db inside the platform data directory.
// app.DataDir may block until the OS context is ready — callers run it
// off the UI thread.
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
