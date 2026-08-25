//go:build !android

package android

import (
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// HardwareKeystore returns nil everywhere but Android, so main() falls
// through to exactly the keystore selection it had before this package
// existed. Desktop and CI never see a JNI call.
func HardwareKeystore(string) appcrypto.KeyProvider { return nil }
