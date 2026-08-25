// Package push holds the platform hooks that keep sync alive when the app
// is backgrounded. The Android foreground-service hook is wired through
// platform/android (COMPLIANCE-TRACKER.md, "Android foreground service");
// the iOS APNs path remains pending.
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

// fgController is installed by platform/android once the engine's sync
// goroutines exist (cmd/vayumail initEngine). Without a registration —
// desktop, CI, or an APK whose jar predates the service class — every call
// below stays a logged no-op and sync runs only while foregrounded.
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
