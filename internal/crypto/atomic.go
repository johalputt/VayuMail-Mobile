package crypto

import "os"

// WriteFileAtomic publishes data at path with writeFileAtomic's guarantees:
// a unique temp file in the same directory, fsync of the bytes before the
// rename, and a directory sync after it. It exists because platform code
// persists security-relevant blobs too — today the hardware-wrapped master
// key in platform/android — and that persistence must obey the exact crash
// and concurrent-writer discipline the sealed store holds itself to rather
// than growing a second, weaker copy of the primitive.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(path, data, perm)
}
