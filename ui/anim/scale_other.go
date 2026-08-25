//go:build !android

package anim

// Non-Android platforms have no system-wide animation scale to honor;
// report 1.0 so only the in-app reduce-motion toggle governs motion.

// SystemAnimatorScale reports the platform's animation speed, always 1
// off Android.
func SystemAnimatorScale() float32 { return 1 }
