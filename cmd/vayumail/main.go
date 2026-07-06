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
	"gioui.org/x/explorer"
	// Declares the Android CAMERA permission (+ camera hardware feature) in the
	// gogio-generated manifest, so the QR scanner can request and use the camera.
	// Without this blank import the permission is absent from the APK entirely —
	// Android then shows no camera permission to grant and no request dialog can
	// appear (gioui.org/app/permission/camera).
	_ "gioui.org/app/permission/camera"
	xtheme "gioui.org/x/pref/theme"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/platform/camera"
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

	// The camera (QR scanner frame source) is platform-selected: a real
	// NDK bridge on Android, a no-op elsewhere. Released on shutdown so the
	// device is never held past app exit.
	cam := camera.New()
	defer cam.Stop()

	// Platform file picker for composer attachments (SAF on Android, native
	// dialogs elsewhere). It must observe every window event, so it is wired to
	// the boot loop before Run starts.
	exp := explorer.NewExplorer(window)

	boot := ui.NewBoot(ctx, window)
	boot.SetEventListener(exp.ListenEvents)
	go initEngine(ctx, window, boot, cam, func() (io.ReadCloser, error) { return exp.ChooseFile() })

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
func initEngine(ctx context.Context, window *app.Window, boot *ui.Boot, cam camera.Camera, pickFile func() (io.ReadCloser, error)) {
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

	mgr := syncmanager.New(db, keystore())
	mgr.SetAttachmentsDir(filepath.Join(filepath.Dir(dbPath), "attachments"))
	if err := mgr.Start(ctx); err != nil {
		boot.Fail(err, "starting the sync engine")
		if cerr := db.Close(); cerr != nil {
			slog.Error("close store", "err", cerr)
		}
		return
	}

	boot.Attach(ui.New(ctx, window, db, mgr, dark, cam.Frame, pickFile), db, mgr)
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
