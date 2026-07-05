// Package camera provides the platform camera frame source that feeds the
// QR scanner (ui/widgets.FrameSource). It lives under platform/ because the
// Android implementation uses cgo + Gio's JNI handles, which the engine
// (internal/) is forbidden to touch (Constitution Rule 1 "pure Go engine",
// Rule 4 "engine never imports Gio", ADR-0001).
//
// On Android it is a pure-cgo bridge over the NDK Camera2 API
// (camera_android.go + camera_android.c) — no Java or Kotlin source, so it
// compiles with an unchanged gogio build and adds no new manifest
// permission (CAMERA is already declared, ADR-0005). On every other
// platform, and in CI (which compiles the !android tag), New returns a
// no-op that reports no frame, so the scanner shows its "Camera
// unavailable — use Paste setup code" fallback and nothing else changes.
//
// REVIEWER / ON-DEVICE NOTE: the Android implementation links
// libcamera2ndk + libmediandk and can only be compiled by the Android
// toolchain (`make android` → gogio) and only exercised on a physical
// device with a camera. It is deliberately excluded from the desktop/CI
// build by build tags and therefore cannot be verified in CI — it must be
// built and tested on-device (see platform/android/README.md). Every
// failure path (no permission, no camera, driver error) degrades to "no
// frame", which the scanner already handles, so the app is never left in a
// broken state.
package camera

import "image"

// Camera is the platform camera used for QR scanning.
type Camera interface {
	// Frame returns the most recent camera frame as a luminance image
	// suitable for QR decoding, or nil when no frame is available yet (or
	// the platform has no camera). It never blocks and is safe to call
	// from the UI thread on every frame; the first call lazily powers the
	// camera on.
	Frame() image.Image

	// Stop releases the camera device. It is safe to call repeatedly; a
	// later Frame call powers the camera back on. The scanner also
	// releases the device automatically after a short idle period, so
	// leaving the scan screen frees the camera without explicit wiring.
	Stop()
}
