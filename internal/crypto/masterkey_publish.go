package crypto

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
)

// publishMu serialises master-key publication within this process. See the
// linkUnsupported branch above for why a process-local lock is sufficient.
//
// NOT independently proven, and said plainly rather than left looking tested:
// removing it fails nothing in the suite, because O_EXCL already guarantees a
// single creator and the losers adopt through the ErrExist path, which IS
// covered. What the lock closes is the narrow window where a loser reads the
// file after the creator has opened it and before the write lands, sees zero
// bytes and calls the key corrupt. Reproducing that interleaving reliably needs
// a seam this package does not have. It is kept because the cost is one
// uncontended lock on a once-per-install path, and the failure it prevents is
// a permanently unreadable credential store.
var publishMu sync.Mutex

// linkUnsupported reports whether a link(2) failure means the filesystem or
// policy refuses hardlinks, as opposed to the target already existing.
func linkUnsupported(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, syscall.EOPNOTSUPP)
}

// publishExcl creates the master key at p.Path with O_EXCL, writes and syncs
// it. If the file already exists it adopts what is there rather than replacing
// it — overwriting would orphan every secret already sealed under the old key.
func (p *FileKeyProvider) publishExcl(key []byte) error {
	publishMu.Lock()
	defer publishMu.Unlock()

	f, err := os.OpenFile(p.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("keystore: write master key: %w", err)
		}
		existing, rerr := os.ReadFile(p.Path)
		if rerr != nil {
			return fmt.Errorf("keystore: read master key: %w", rerr)
		}
		if len(existing) != 32 {
			return fmt.Errorf("keystore: master key file corrupt")
		}
		copy(key, existing)
		return nil
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		_ = os.Remove(p.Path)
		return fmt.Errorf("keystore: write master key: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(p.Path)
		return fmt.Errorf("keystore: sync master key: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("keystore: close master key: %w", err)
	}
	return nil
}
