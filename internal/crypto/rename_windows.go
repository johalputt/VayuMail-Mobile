//go:build windows

package crypto

import (
	"errors"
	"syscall"
)

// errSharingViolation is Win32 error 32 (ERROR_SHARING_VIOLATION). The
// standard library's syscall package defines ERROR_ACCESS_DENIED but not this
// one; spelling it as the errno it is keeps the package dependency-free.
const errSharingViolation syscall.Errno = 32

// renameBusy reports whether err is Windows' "the target is held open"
// rename failure — the only transient case renamePublish retries. See
// writeFileAtomic for why this seam exists at all.
func renameBusy(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) ||
		errors.Is(err, errSharingViolation)
}
