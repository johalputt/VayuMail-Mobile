package test

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// ATTACK: two things reach for a credential at the same moment, on a device
// that has never sealed one before.
//
// FileKeyProvider.MasterKey() reads the key file, and on ENOENT generates a
// fresh 32-byte key and writes it. There is no lock across that read-then-write,
// and SealedKeystore calls it OUTSIDE its own mutex — Store() calls cipher()
// before taking ks.mu, and Fetch() releases ks.mu before calling it.
//
// So on first use, concurrent callers each observe "no key file", each generate
// a DIFFERENT key, and each write it. One rename wins. Every secret sealed with
// a losing key is then unopenable forever: the next read of master.key returns
// the winner's bytes and GCM refuses the ciphertext.
//
// The consequence on a phone is not a crash. It is an account whose password
// cannot be unsealed — the user is signed out of their own mail with no way
// back except wiping app data, and nothing anywhere says why.
func TestMasterKeyIsStableUnderConcurrentFirstUse(t *testing.T) {
	dir := t.TempDir()
	p := &appcrypto.FileKeyProvider{Path: filepath.Join(dir, "master.key")}

	const n = 16
	keys := make([][]byte, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			k, err := p.MasterKey()
			if err != nil {
				t.Errorf("MasterKey: %v", err)
				return
			}
			keys[i] = k
		}(i)
	}
	close(start)
	wg.Wait()

	onDisk, err := os.ReadFile(p.Path)
	if err != nil {
		t.Fatalf("no master key was persisted at all: %v", err)
	}
	for i, k := range keys {
		if k == nil {
			continue
		}
		if !bytes.Equal(k, onDisk) {
			t.Fatalf("caller %d was handed a master key that is NOT the one on disk. Anything it "+
				"seals with that key can never be opened again: the next process reads the file and "+
				"GCM refuses the ciphertext. On a phone that is an account whose password cannot be "+
				"unsealed — signed out of your own mail, no way back but wiping app data, and "+
				"nothing says why.", i)
		}
	}
}

// The same race, seen through the keystore rather than the provider: seal a
// secret from several goroutines at once and then open each one.
func TestConcurrentFirstSealsRemainReadable(t *testing.T) {
	dir := t.TempDir()

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ks, err := appcrypto.NewSealedKeystore(dir)
			if err != nil {
				t.Errorf("open keystore: %v", err)
				return
			}
			<-start
			if err := ks.Store(aliasFor(i), []byte(secretFor(i))); err != nil {
				t.Errorf("store %d: %v", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	// A fresh process opening the same directory must be able to read back
	// every secret that reported success.
	ks, err := appcrypto.NewSealedKeystore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		got, err := ks.Fetch(aliasFor(i))
		if err != nil {
			// A lost WRITE is survivable — the last writer wins the file and an
			// earlier alias may simply be absent. A write that SUCCEEDED and is
			// now undecryptable is not.
			if err == appcrypto.ErrKeyNotFound {
				continue
			}
			t.Fatalf("alias %q was sealed successfully and cannot be opened (%v). The master key it "+
				"was sealed with lost the race and no longer exists.", aliasFor(i), err)
		}
		if string(got) != secretFor(i) {
			t.Fatalf("alias %q opened to the wrong secret", aliasFor(i))
		}
	}
}

func aliasFor(i int) string  { return "acct-" + string(rune('a'+i)) }
func secretFor(i int) string { return "password-" + string(rune('a'+i)) }
