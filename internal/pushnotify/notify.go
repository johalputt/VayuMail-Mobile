// Package pushnotify is the platform seam for TAPPABLE new-mail notifications on
// Android. A posted notification carries the mailbox it points at (account +
// folder ids) as intent extras; when the user taps it the app is (re)launched and
// the bridge reads those extras and hands them to the registered tap handler,
// which opens that mailbox. Every non-Android platform gets the stub (Available
// reports false), so callers fall back to their existing notifier and the host /
// CGO_ENABLED=0 build stays clean — only the //go:build android file uses cgo +
// JNI, exactly mirroring internal/biometric.
//
// UNVERIFIED: the Android path (notify_android.go + VayuNotify.java) has not been
// compiled or run in this environment — there is no Android SDK/NDK here and CI
// does not build the APK. It needs an on-device APK build (see README.md in this
// package for the jar-build and AndroidManifest requirements) and will likely
// need a round of fixes on a real device.
package pushnotify

import "gioui.org/io/event"

// Available reports whether the tappable-notification backend is active (Android
// only). When false, callers post via their normal notifier.
func Available() bool { return available() }

// Post shows a new-mail notification that, when tapped, opens the given mailbox
// (accountID + folderID). id lets a later Post for the same mailbox replace an
// earlier one. Returns false if the platform could not post it (the caller should
// fall back to its own notifier). No-op returning false off Android.
func Post(id int, title, body string, accountID, folderID int64) bool {
	return post(id, title, body, accountID, folderID)
}

// SetTapHandler registers the callback invoked with the mailbox a tapped
// notification pointed at. It fires from the event pump (HandleViewEvent), so the
// handler should be cheap and thread-safe.
func SetTapHandler(fn func(accountID, folderID int64)) { setTapHandler(fn) }

// HandleViewEvent must be fed every window event so the Android backend can hold
// the current view (needed to read the launch intent's extras) and detect a tap
// on resume. No-op off Android.
func HandleViewEvent(e event.Event) { handleViewEvent(e) }
