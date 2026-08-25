// Package android hosts the JNI seams between the pure-Go engine and the
// device's own facilities: the hardware-backed keystore that wraps the
// master key (ADR-0004) and the foreground service that keeps IMAP IDLE
// alive in the background (ADR-0005). Everything device-specific sits behind
// build tags; the shared decision-making lives in host-testable files so the
// only thing a phone adds is the raw wrap/unwrap and start/stop calls.
package android

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// wrappedKeyName is the wrapped master-key blob stored beside the sealed
// store. Its bytes are ciphertext under a wrapping key generated inside the
// hardware-backed Android Keystore — useless on any other device, and
// therefore safe to persist where credentials.sealed lives (Rule 6: what is
// protected is still never plaintext at rest).
const wrappedKeyName = "hardware.key"

var errBadKeyLen = errors.New("android keystore: unwrapped key is not 32 bytes")

// WrappedKeyProvider implements appcrypto.KeyProvider by sealing the master
// key under a wrapping key that never leaves the device's secure hardware.
//
// The wrap/unwrap primitives arrive as closures so every branch below is
// exercised by ordinary unit tests; the android-tagged file supplies the
// JNI-backed pair, and nothing else in this file knows or cares how they are
// implemented.
type WrappedKeyProvider struct {
	// Dir holds hardware.key. It must live beside credentials.sealed so a
	// wiped app directory takes both halves of the story away together.
	Dir string
	// Wrap seals plaintext under the hardware key, returning base64 text.
	Wrap func(plaintext []byte) (string, error)
	// Unwrap opens a Wrap result. An error here means THIS DEVICE cannot
	// open its own blob — treated as fatal for the provider, never as a
	// signal to regenerate (regenerating would orphan every secret already
	// sealed under the old key).
	Unwrap func(sealed string) ([]byte, error)

	mu  sync.Mutex
	key []byte
}

// NewWrappedKeyProvider returns a provider over dir with the given device
// primitives. Nil wrap or unwrap panics on first use rather than silently
// degrading to something that looks like a keystore.
func NewWrappedKeyProvider(
	dir string,
	wrap func([]byte) (string, error),
	unwrap func(string) ([]byte, error),
) *WrappedKeyProvider {
	return &WrappedKeyProvider{Dir: dir, Wrap: wrap, Unwrap: unwrap}
}

// MasterKey returns the 32-byte sealing key, generating it on first use.
//
// # Ordering, and the corruption it prevents
//
// Read → unwrap → cache covers every launch after the first. The create path
// runs exactly once per install: generate, wrap, then publish the WRAPPED
// blob through crypto.WriteFileAtomic. The atomic publish matters even though
// creation is once-per-install, because a crash mid-write that left a short
// file would otherwise be read back as a corrupt blob on next boot and look
// like a hardware failure instead of an interrupted one. The process-local
// mutex serialises callers; cross-process racing is out of scope for the
// same reason it is in the sealed store — this is a single-process app.
func (p *WrappedKeyProvider) MasterKey() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.key) == 32 {
		return p.key, nil
	}
	if p.Wrap == nil || p.Unwrap == nil {
		return nil, errors.New("android keystore: provider has no wrap/unwrap primitives")
	}

	blob := filepath.Join(p.Dir, wrappedKeyName)
	raw, err := os.ReadFile(blob)
	switch {
	case err == nil:
		key, uerr := p.Unwrap(string(raw))
		if uerr != nil {
			return nil, fmt.Errorf("android keystore: unwrap %s: %w", wrappedKeyName, uerr)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("android keystore: %w (%d)", errBadKeyLen, len(key))
		}
		p.key = key
		return key, nil

	case os.IsNotExist(err):
		key := make([]byte, 32)
		if _, gerr := rand.Read(key); gerr != nil {
			return nil, fmt.Errorf("android keystore: generate master key: %w", gerr)
		}
		sealed, werr := p.Wrap(key)
		if werr != nil {
			return nil, fmt.Errorf("android keystore: wrap master key: %w", werr)
		}
		if mkerr := os.MkdirAll(p.Dir, 0o700); mkerr != nil {
			return nil, fmt.Errorf("android keystore: create key dir: %w", mkerr)
		}
		if perr := appcrypto.WriteFileAtomic(blob, []byte(sealed), 0o600); perr != nil {
			return nil, fmt.Errorf("android keystore: write %s: %w", wrappedKeyName, perr)
		}
		p.key = key
		return key, nil

	default:
		return nil, fmt.Errorf("android keystore: read %s: %w", wrappedKeyName, err)
	}
}
