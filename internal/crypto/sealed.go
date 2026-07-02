package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// KeyProvider supplies the 32-byte master key that seals credentials at
// rest. The provider abstraction exists so the key source can be
// upgraded — a hardware-backed Android Keystore / iOS Keychain wrapping
// provider slots in here without changing the sealed-file format
// (COMPLIANCE-TRACKER.md: "Hardware-backed key wrapping").
type KeyProvider interface {
	// MasterKey returns the 32-byte sealing key, generating it on first
	// use.
	MasterKey() ([]byte, error)
}

// FileKeyProvider keeps the master key in a 0600 file inside the
// app-private data directory. On Android/iOS that directory is sandboxed
// per-app; this provides encrypted-at-rest credentials whose protection
// equals the platform sandbox. Hardware wrapping strengthens it later
// without a format change.
type FileKeyProvider struct {
	Path string
}

// MasterKey loads or creates the key file.
func (p *FileKeyProvider) MasterKey() ([]byte, error) {
	key, err := os.ReadFile(p.Path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("keystore: master key file corrupt")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("keystore: read master key: %w", err)
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("keystore: generate master key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
		return nil, fmt.Errorf("keystore: create key dir: %w", err)
	}
	if err := writeFileAtomic(p.Path, key, 0o600); err != nil {
		return nil, fmt.Errorf("keystore: write master key: %w", err)
	}
	return key, nil
}

// SealedKeystore is a Keystore that persists credentials encrypted with
// AES-256-GCM. Raw credentials never touch disk (Rule 6): only sealed
// ciphertext is written, keyed per alias with a fresh nonce per write.
type SealedKeystore struct {
	mu       sync.Mutex
	path     string
	provider KeyProvider
	entries  map[string]string // alias -> base64(nonce||ciphertext)
}

// NewSealedKeystore opens (or creates) the sealed store at
// dir/credentials.sealed with the master key at dir/master.key.
func NewSealedKeystore(dir string) (*SealedKeystore, error) {
	return NewSealedKeystoreWithProvider(
		filepath.Join(dir, "credentials.sealed"),
		&FileKeyProvider{Path: filepath.Join(dir, "master.key")})
}

// NewSealedKeystoreWithProvider opens a sealed store with an explicit
// key provider (used by tests and future hardware-backed providers).
func NewSealedKeystoreWithProvider(path string, provider KeyProvider) (*SealedKeystore, error) {
	ks := &SealedKeystore{
		path:     path,
		provider: provider,
		entries:  map[string]string{},
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ks, nil
	}
	if err != nil {
		return nil, fmt.Errorf("keystore: read sealed store: %w", err)
	}
	if err := json.Unmarshal(raw, &ks.entries); err != nil {
		return nil, fmt.Errorf("keystore: parse sealed store: %w", err)
	}
	return ks, nil
}

// Store seals the secret and persists it atomically.
func (ks *SealedKeystore) Store(alias string, secret []byte) error {
	gcm, err := ks.cipher()
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("keystore: nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, secret, []byte(alias))

	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.entries[alias] = base64.StdEncoding.EncodeToString(sealed)
	return ks.persistLocked()
}

// Fetch opens the sealed secret for alias.
func (ks *SealedKeystore) Fetch(alias string) ([]byte, error) {
	ks.mu.Lock()
	encoded, ok := ks.entries[alias]
	ks.mu.Unlock()
	if !ok {
		return nil, ErrKeyNotFound
	}
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("keystore: corrupt entry %q: %w", alias, err)
	}
	gcm, err := ks.cipher()
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("keystore: corrupt entry %q", alias)
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	secret, err := gcm.Open(nil, nonce, ciphertext, []byte(alias))
	if err != nil {
		return nil, fmt.Errorf("keystore: unseal %q: %w", alias, err)
	}
	return secret, nil
}

// Delete removes the sealed entry for alias. Missing aliases are not an
// error.
func (ks *SealedKeystore) Delete(alias string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if _, ok := ks.entries[alias]; !ok {
		return nil
	}
	delete(ks.entries, alias)
	return ks.persistLocked()
}

func (ks *SealedKeystore) cipher() (cipher.AEAD, error) {
	key, err := ks.provider.MasterKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("keystore: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("keystore: gcm: %w", err)
	}
	return gcm, nil
}

func (ks *SealedKeystore) persistLocked() error {
	raw, err := json.Marshal(ks.entries)
	if err != nil {
		return fmt.Errorf("keystore: encode sealed store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(ks.path), 0o700); err != nil {
		return fmt.Errorf("keystore: create store dir: %w", err)
	}
	if err := writeFileAtomic(ks.path, raw, 0o600); err != nil {
		return fmt.Errorf("keystore: write sealed store: %w", err)
	}
	return nil
}

// writeFileAtomic writes via a temp file + rename so a crash never
// leaves a truncated store.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
