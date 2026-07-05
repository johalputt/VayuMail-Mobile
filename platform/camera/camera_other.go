//go:build !android || !cgo

package camera

import "image"

// New returns a no-op camera on every build without the Android NDK bridge
// (desktop, CI, and any non-cgo build). It always reports "no frame", so
// the QR scanner shows its "Camera unavailable — use Paste setup code"
// fallback and behaviour is identical to before the bridge existed.
func New() Camera { return noCamera{} }

type noCamera struct{}

func (noCamera) Frame() image.Image { return nil }
func (noCamera) Stop()              {}
