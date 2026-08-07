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

	// mu guards the read-then-create sequence in MasterKey, and key caches
	// the result. See MasterKey for why both are load-bearing.
	mu  sync.Mutex
	key []byte
}

// MasterKey loads or creates the key file.
//
// # The corruption this ordering prevents
//
// The first version read the file and, on ENOENT, generated a key and wrote it
// — with no lock across the two steps, and called from OUTSIDE SealedKeystore's
// own mutex (Store takes the lock after cipher(); Fetch releases it before).
//
// So on a device that had never sealed anything, two concurrent callers each
// saw "no key file", each generated a DIFFERENT key, and each wrote it. One
// won. Every secret sealed with a losing key was then unopenable forever: the
// next read returns the winner's bytes and GCM refuses the ciphertext.
//
// On a phone that is not a crash. It is an account whose password cannot be
// unsealed — signed out of your own mail, no way back but wiping app data, and
// nothing anywhere saying why.
//
// Three things close it, and each covers a case the others do not:
//
//   - the mutex serialises callers inside this process;
//   - the cached key means the file is read once rather than on every seal;
//   - O_CREATE|O_EXCL makes the creation atomic against another PROCESS, which
//     a mutex cannot reach. Losing that race is not an error — it means somebody
//     else created the key first, so we adopt theirs rather than overwrite it.
//
// The key is never written through a shared temp path. See writeFileAtomic.
func (p *FileKeyProvider) MasterKey() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.key) == 32 {
		return p.key, nil
	}

	key, err := os.ReadFile(p.Path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("keystore: master key file corrupt")
		}
		p.key = key
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

	// Write the key COMPLETE, then publish it atomically.
	//
	// The obvious version — O_CREATE|O_EXCL then write — is wrong, and the
	// audit's own test caught it: O_EXCL creates the file EMPTY, so a
	// concurrent reader that arrives between the create and the write reads
	// zero bytes and declares the key corrupt. The file must never exist in a
	// half-written state.
	//
	// os.Link is the primitive that gives both properties at once: the target
	// appears complete or not at all, and linking onto an existing name fails
	// rather than overwriting — so a caller that loses the race adopts the
	// winner's key instead of replacing it. os.Rename cannot be used here,
	// because rename overwrites, which is exactly the lost-key bug.
	tmp, err := os.CreateTemp(filepath.Dir(p.Path), filepath.Base(p.Path)+".new-")
	if err != nil {
		return nil, fmt.Errorf("keystore: write master key: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("keystore: write master key: %w", err)
	}
	if _, err := tmp.Write(key); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("keystore: write master key: %w", err)
	}
	// Durability matters more here than anywhere else in the app: a master key
	// lost to a crash locks the user out of every credential permanently.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("keystore: sync master key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("keystore: close master key: %w", err)
	}

	if err := os.Link(tmpName, p.Path); err != nil {
		if linkUnsupported(err) {
			// Android refuses link(2) in app-private storage: SELinux policy
			// forbids hardlinks under /data/user/0/<pkg>/files, so the primitive
			// chosen above for correctness is the one the ship target denies —
			// on every device, every time. The app could never create a master
			// key there; installs only worked while one already existed.
			//
			// The fallback keeps both properties the link gave us. O_EXCL keeps
			// create-if-not-exists, so a loser still adopts the winner's key
			// rather than overwriting it. The empty-file window O_EXCL opens —
			// a reader arriving between create and write sees zero bytes and
			// calls the key corrupt — is closed by publishMu, which is sound
			// here precisely because this is a single-process app: the race the
			// link protected against is between goroutines, not processes.
			if perr := p.publishExcl(key); perr != nil {
				return nil, perr
			}
			return key, nil
		}
		// Somebody else published first. Adopt theirs — overwriting it would
		// orphan every secret already sealed under it.
		existing, rerr := os.ReadFile(p.Path)
		if rerr != nil {
			return nil, fmt.Errorf("keystore: write master key: %w", err)
		}
		if len(existing) != 32 {
			return nil, fmt.Errorf("keystore: master key file corrupt")
		}
		p.key = existing
		return existing, nil
	}
	syncDir(filepath.Dir(p.Path))
	p.key = key
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
//
// # Why the temp name is unique
//
// It used to be a FIXED path — `path + ".tmp"`. Two writers therefore shared
// one temp file: both wrote it, the first renamed it into place, and the
// second's rename found nothing left to rename and failed with
//
//	rename …/credentials.sealed.tmp …/credentials.sealed: no such file or directory
//
// which surfaced as a credential that reported it could not be saved. Worse on
// the master key, where the same collision made key creation fail outright.
//
// A unique temp file per write cannot collide. The fsync before the rename is
// what makes "atomic" true rather than merely likely: rename is atomic in the
// directory, but without the sync the bytes may not have reached the disk when
// the power goes, leaving a correctly-named file full of nothing.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Any failure past this point leaves a temp file behind; remove it rather
	// than leaving 0600 secrets scattered through the data directory.
	defer func() { _ = os.Remove(tmp) }()

	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir flushes a directory entry so a rename survives a power loss.
//
// Best-effort: some platforms refuse to open a directory for sync, and failing
// the whole write because the durability step is unavailable would be worse
// than the durability gap it is guarding against.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
