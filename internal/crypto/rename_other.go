//go:build !windows

package crypto

// renameBusy is always false where rename(2) exists: a POSIX rename either
// succeeds or fails for a reason no retry fixes, so writeFileAtomic behaves
// identically to before on Linux, Android, and iOS. See renamePublish.
func renameBusy(err error) bool { return false }
