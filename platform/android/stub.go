//go:build !android

package android

import (
	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/push"
)

// HardwareKeystore returns nil everywhere but Android, so main() falls
// through to exactly the keystore selection it had before this package
// existed. Desktop and CI never see a JNI call.
func HardwareKeystore(string) appcrypto.KeyProvider { return nil }

// ForegroundSyncController returns nil off Android; push.StartBackgroundSync
// stays a logged no-op without one.
func ForegroundSyncController() push.ForegroundServiceController { return nil }
