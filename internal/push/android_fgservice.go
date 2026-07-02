// Package push holds the platform hooks that keep sync alive when the app
// is backgrounded. Both hooks are stubs at v0.1.0 — see
// COMPLIANCE-TRACKER.md ("Android foreground service", "iOS APNs").
package push

import (
	"context"
	"log/slog"
)

// ForegroundServiceController is implemented by gomobile-bound Android
// code to start and stop the foreground service that hosts the IMAP IDLE
// connections (ADR-0005: FOREGROUND_SERVICE permission).
type ForegroundServiceController interface {
	// StartService shows the persistent notification and pins the
	// process; the syncmanager goroutines keep running inside it.
	StartService() error
	// StopService removes the notification and releases the pin.
	StopService() error
}

// STUB: the gomobile binding that implements
// ForegroundServiceController on Android is not wired to an OS service
// yet. RegisterForegroundService accepts the controller so the engine
// side is complete; until platform code calls it, StartBackgroundSync
// logs and returns. Tracked in COMPLIANCE-TRACKER.md.
var fgController ForegroundServiceController

// RegisterForegroundService installs the Android controller. Called once
// by platform code at process start.
func RegisterForegroundService(c ForegroundServiceController) {
	fgController = c
}

// StartBackgroundSync asks the platform to keep sync alive while the app
// is backgrounded. On platforms without a registered controller it is a
// logged no-op — sync then runs only while the app is foregrounded.
func StartBackgroundSync(ctx context.Context) error {
	_ = ctx
	if fgController == nil {
		slog.Info("no foreground service controller registered; background sync inactive")
		return nil
	}
	return fgController.StartService()
}

// StopBackgroundSync releases the foreground service if one is running.
func StopBackgroundSync() error {
	if fgController == nil {
		return nil
	}
	return fgController.StopService()
}
