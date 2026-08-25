package pgp

// generate.go — on-device PGP identity creation (plan Phase 4.1). New
// accounts generate their keypair here instead of fetching a
// server-generated private key, so the secret material never exists
// anywhere but this device's sealed store.

import (
	"bytes"
	"crypto"
	"fmt"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// GenerateKey creates a fresh RSA-3072 identity for the given address and
// returns the armored public and private key blocks. RSA was chosen over
// modern ECC deliberately: every OpenPGP implementation the user will
// exchange mail with — including older servers — understands it, and the
// key outlives the app that made it.
func GenerateKey(name, email string) (armoredPublic, armoredPrivate []byte, err error) {
	if email == "" {
		return nil, nil, fmt.Errorf("pgp: generate key: empty email")
	}
	cfg := &packet.Config{
		Algorithm:     packet.PubKeyAlgoRSA,
		RSABits:       3072,
		DefaultHash:   crypto.SHA256,
		DefaultCipher: packet.CipherAES256,
	}
	entity, err := openpgp.NewEntity(name, "", email, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("pgp: generate key: %w", err)
	}

	if armoredPrivate, err = serializeArmor(entity, true); err != nil {
		return nil, nil, err
	}
	if armoredPublic, err = serializeArmor(entity, false); err != nil {
		return nil, nil, err
	}
	return armoredPublic, armoredPrivate, nil
}

func serializeArmor(e *openpgp.Entity, private bool) ([]byte, error) {
	blockType := "PGP PUBLIC KEY BLOCK"
	serialize := func(w io.Writer) error { return e.Serialize(w) }
	if private {
		blockType = "PGP PRIVATE KEY BLOCK"
		serialize = func(w io.Writer) error { return e.SerializePrivate(w, nil) }
	}
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, blockType, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: armor encode: %w", err)
	}
	if err := serialize(w); err != nil {
		return nil, fmt.Errorf("pgp: serialize: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("pgp: armor close: %w", err)
	}
	return buf.Bytes(), nil
}
