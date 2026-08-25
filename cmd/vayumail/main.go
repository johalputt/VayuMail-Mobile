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
	"gioui.org/x/haptic"
	xtheme "gioui.org/x/pref/theme"

	"github.com/johalputt/VayuMail-Mobile/internal/biometric"
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/push"
	"github.com/johalputt/VayuMail-Mobile/internal/pushnotify"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
	"github.com/johalputt/VayuMail-Mobile/platform/android"
	"github.com/johalputt/VayuMail-Mobile/ui"
	"github.com/johalputt/VayuMail-Mobile/ui/widgets"
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

	// Haptics: one buzzer for the whole app (Android vibration over JNI;
	// a silent no-op off Android). Like the file picker and biometrics it
	// needs the Android view lifecycle, so it joins the event fan-out.
	buzzer := haptic.NewBuzzer(window)
	widgets.SetBuzzer(buzzer)

	boot := ui.NewBoot(ctx, window)
	// Both the file picker and the biometric backend need to observe the
	// Android view lifecycle (BiometricPrompt needs the Activity behind the
	// current view), so the boot loop fans every event out to both.
	// A tapped new-mail notification opens its mailbox: the bridge reads the
	// tapped mailbox off the (re)launch intent on a view event and hands it to the
	// UI's pending-nav (no-op off Android).
	pushnotify.SetTapHandler(ui.SetMailNavTarget)
	boot.SetEventListener(func(e event.Event) {
		exp.ListenEvents(e)
		feedView(buzzer, e)
		biometric.HandleViewEvent(e)
		pushnotify.HandleViewEvent(e)
		refreshSystemMotion() // honor animator-scale changes on resume (5.6)
	})
	go initEngine(ctx, window, boot, func() (io.ReadCloser, error) { return exp.ChooseFile() })

	err := boot.Run()
	cancel()
	buzzer.Shutdown()
	// The window is gone: release the Android foreground service pin so the
	// notification disappears with the app. No-op on platforms without a
	// registered controller.
	if serr := push.StopBackgroundSync(); serr != nil {
		slog.Debug("foreground service stop", "err", serr)
	}
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

	// Background delivery (ADR-0005): once the engine's goroutines exist,
	// pin the process inside the dataSync foreground service so IMAP IDLE
	// survives backgrounding. Off Android, or when the helper class is
	// missing from an older dev build, both calls are no-ops and sync stays
	// foreground-only as before.
	if c := android.ForegroundSyncController(); c != nil {
		push.RegisterForegroundService(c)
		if serr := push.StartBackgroundSync(ctx); serr != nil {
			slog.Warn("foreground service start failed; background sync inactive", "err", serr)
		}
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

// keystore selects secret storage, strongest first:
//
//  1. The hardware-wrapped store (audit 2026-08 H1): the master key exists
//     at rest only as ciphertext under an AndroidKeyStore key that never
//     leaves the device's secure hardware, so hardware.key beside the sealed
//     blob is useless off this device. The sealed-file FORMAT is unchanged
//     — only where the sealing key comes from (ADR-0004's wrapping slot).
//  2. The gomobile platform keystore bridge, when one is registered —
//     reserved today for a future iOS Keychain implementation.
//  3. A sealed AES-256-GCM store whose 32-byte sealing key is a sibling
//     0600 file: encrypted at rest, but only as confidential as the OS app
//     sandbox (an attacker with a one-time read of the app-private dir gets
//     both halves — audit M16). Accepted on desktop/dev; the mobile stopgap
//     step 1 replaced.
//  4. An in-memory store (credentials last one session) when the data
//     directory is unavailable.
//
// VAYUMAIL_REQUIRE_SECURE_KEYSTORE=1 makes steps 1 and 3 FAIL CLOSED: no
// hardware wrap available means in-memory, never an on-disk cleartext key.
func keystore() appcrypto.Keystore {
	dir, err := app.DataDir()
	if err == nil {
		keysDir := filepath.Join(dir, "vayumail", "keys")
		if p := android.HardwareKeystore(keysDir); p != nil {
			slog.Info("master key wrapped by device secure hardware")
			sealed, serr := appcrypto.NewSealedKeystoreWithProvider(
				filepath.Join(keysDir, "credentials.sealed"), p)
			if serr == nil {
				return sealed
			}
			slog.Warn("hardware-wrapped store unavailable; falling back", "err", serr)
		}
	}

	p := appcrypto.NewPlatformKeystore()
	if _, err := p.Fetch("vayumail-probe"); err != appcrypto.ErrNoPlatformKeystore {
		return p
	}
	if requireSecureKeystore() {
		slog.Warn("no hardware keystore and VAYUMAIL_REQUIRE_SECURE_KEYSTORE set; " +
			"failing closed to in-memory store — credentials last one session, " +
			"no sealing key is written to disk")
		return appcrypto.NewMemoryKeystore()
	}
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

// requireSecureKeystore reports whether the operator demands hardware-backed
// (or no-persistence) secret storage — set VAYUMAIL_REQUIRE_SECURE_KEYSTORE
// to 1/true/yes to refuse the on-disk-sealing-key fallback (audit M16).
func requireSecureKeystore() bool {
	switch os.Getenv("VAYUMAIL_REQUIRE_SECURE_KEYSTORE") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	}
	return false
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
