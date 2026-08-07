package crypto

import (
	"path/filepath"
	"sync"
	"testing"
)

// Android refuses link(2) in app-private storage — SELinux forbids hardlinks
// under /data/user/0/<pkg>/files — so the primitive the master-key creation
// chose for its atomicity is the one the ship target denies, on every device.
// The observed failure was:
//
//	keystore: write master key: link …/master.key.new-2096712107
//	  …/master.key: permission denied
//
// The fallback must keep both properties the link provided: every racing caller
// ends up with the SAME key, and no caller ever reads a half-written one.
func TestPublishExclAgreesOnOneKeyUnderRace(t *testing.T) {
	dir := t.TempDir()
	p := &FileKeyProvider{Path: filepath.Join(dir, "master.key")}

	const racers = 16
	keys := make([][]byte, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			k := make([]byte, 32)
			for j := range k {
				k[j] = byte(i + 1) // each racer proposes a DIFFERENT key
			}
			if err := p.publishExcl(k); err != nil {
				t.Errorf("racer %d: %v", i, err)
				return
			}
			keys[i] = k
		}(i)
	}
	wg.Wait()

	for i, k := range keys {
		if len(k) != 32 {
			t.Fatalf("racer %d ended with a %d-byte key; a short read here is a permanently "+
				"unreadable credential store", i, len(k))
		}
		if string(k) != string(keys[0]) {
			t.Fatalf("racer %d holds a different key from racer 0.\n\n"+
				"Every loser must ADOPT the winner's key. Replacing it orphans every secret "+
				"already sealed under the old one — the user is locked out of every stored "+
				"credential, permanently.", i)
		}
	}
}

// The control: a second, later call must adopt what is on disk rather than
// overwrite it, which is the property that makes the race above safe.
func TestPublishExclAdoptsAnExistingKey(t *testing.T) {
	dir := t.TempDir()
	p := &FileKeyProvider{Path: filepath.Join(dir, "master.key")}

	first := make([]byte, 32)
	for i := range first {
		first[i] = 0xAB
	}
	if err := p.publishExcl(first); err != nil {
		t.Fatal(err)
	}

	second := make([]byte, 32)
	for i := range second {
		second[i] = 0xCD
	}
	if err := p.publishExcl(second); err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Error("the second caller kept its own key instead of adopting the stored one, " +
			"which would orphan everything sealed under the first")
	}
}
