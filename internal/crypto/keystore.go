// Package crypto defines the platform keystore abstraction that keeps
// every credential out of SQLite and off the filesystem (Constitutional
// Rule 6). The concrete secure storage is provided by the platform layer
// (Android Keystore / iOS Keychain via gomobile bind); the engine only
// ever sees this interface.
package crypto

import (
	"errors"
	"sync"
)

// ErrKeyNotFound is returned when no secret exists under an alias.
var ErrKeyNotFound = errors.New("keystore: key not found")

// ErrNoPlatformKeystore is returned when the platform bridge has not been
// registered — for example when engine code runs headless without a
// device keystore and no explicit in-memory store was chosen.
var ErrNoPlatformKeystore = errors.New("keystore: no platform keystore registered")

// Keystore stores secrets by alias. Implementations must never write
// secrets to application-readable disk.
type Keystore interface {
	// Store saves a secret under an alias, replacing any previous value.
	Store(alias string, secret []byte) error
	// Fetch returns the secret stored under alias, or ErrKeyNotFound.
	Fetch(alias string) ([]byte, error)
	// Delete removes the secret stored under alias. Deleting a missing
	// alias is not an error.
	Delete(alias string) error
}

// MemoryKeystore is a process-lifetime Keystore for tests and the
// headless CLI. Secrets vanish when the process exits; nothing touches
// disk.
type MemoryKeystore struct {
	mu      sync.RWMutex
	secrets map[string][]byte
}

// NewMemoryKeystore returns an empty in-memory keystore.
func NewMemoryKeystore() *MemoryKeystore {
	return &MemoryKeystore{secrets: make(map[string][]byte)}
}

// Store implements Keystore.
func (m *MemoryKeystore) Store(alias string, secret []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(secret))
	copy(cp, secret)
	m.secrets[alias] = cp
	return nil
}

// Fetch implements Keystore.
func (m *MemoryKeystore) Fetch(alias string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	secret, ok := m.secrets[alias]
	if !ok {
		return nil, ErrKeyNotFound
	}
	cp := make([]byte, len(secret))
	copy(cp, secret)
	return cp, nil
}

// Delete implements Keystore.
func (m *MemoryKeystore) Delete(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.secrets, alias)
	return nil
}

// PlatformBridge is implemented by gomobile-bound platform code: Android
// Keystore on Android, Keychain on iOS. It mirrors Keystore with a
// gomobile-friendly signature.
type PlatformBridge interface {
	Store(alias string, secret []byte) error
	Fetch(alias string) ([]byte, error)
	Delete(alias string) error
}

var (
	platformMu     sync.RWMutex
	platformBridge PlatformBridge
)

// RegisterPlatform installs the platform keystore bridge. The platform
// layer calls this exactly once at process start, before the engine runs.
func RegisterPlatform(bridge PlatformBridge) {
	platformMu.Lock()
	defer platformMu.Unlock()
	platformBridge = bridge
}

// PlatformKeystore is the Keystore used in production builds: it
// delegates to the registered gomobile bridge.
type PlatformKeystore struct{}

// NewPlatformKeystore returns the platform-backed keystore.
func NewPlatformKeystore() *PlatformKeystore { return &PlatformKeystore{} }

func bridge() (PlatformBridge, error) {
	platformMu.RLock()
	defer platformMu.RUnlock()
	if platformBridge == nil {
		return nil, ErrNoPlatformKeystore
	}
	return platformBridge, nil
}

// Store implements Keystore via the platform bridge.
func (p *PlatformKeystore) Store(alias string, secret []byte) error {
	b, err := bridge()
	if err != nil {
		return err
	}
	return b.Store(alias, secret)
}

// Fetch implements Keystore via the platform bridge.
func (p *PlatformKeystore) Fetch(alias string) ([]byte, error) {
	b, err := bridge()
	if err != nil {
		return nil, err
	}
	return b.Fetch(alias)
}

// Delete implements Keystore via the platform bridge.
func (p *PlatformKeystore) Delete(alias string) error {
	b, err := bridge()
	if err != nil {
		return err
	}
	return b.Delete(alias)
}
