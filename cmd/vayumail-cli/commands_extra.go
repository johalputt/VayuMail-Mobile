package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"github.com/emersion/go-imap/v2/imapclient"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// cmdPin connects to the account's IMAP server, prints the SPKI hash of
// its TLS key, and (with -save) records it as the pinned key: future
// connections then refuse any other key (ADR-0008).
func cmdPin(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("pin", flag.ContinueOnError)
	save := fs.Bool("save", false, "persist the observed pin for this account")
	clear := fs.Bool("clear", false, "remove the stored pin")
	acct, cfg, code := loadAccount(ctx, db, fs, args)
	if code != 0 {
		return code
	}
	if *clear {
		if err := db.SetPinnedSPKI(ctx, acct.ID, ""); err != nil {
			fmt.Fprintf(os.Stderr, "clear pin: %v\n", err)
			return 1
		}
		fmt.Println("pin cleared")
		return 0
	}

	conn, err := tls.Dial("tcp", cfg.IMAPAddr(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial: %v\n", err)
		return 1
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		fmt.Fprintln(os.Stderr, "server presented no certificate")
		return 1
	}
	pin := account.SPKIHash(certs[0])
	fmt.Printf("server: %s\nspki-sha256: %s\nsubject: %s\n",
		cfg.IMAPAddr(), pin, certs[0].Subject)
	if *save {
		if err := db.SetPinnedSPKI(ctx, acct.ID, pin); err != nil {
			fmt.Fprintf(os.Stderr, "save pin: %v\n", err)
			return 1
		}
		fmt.Println("pin saved — connections now require this key")
	}
	return 0
}

// syncKey reads the 32-byte settings-sync key from VAYUMAIL_SYNC_KEY
// (standard base64). The same key on every device gives multi-device
// settings sync through the mailbox with zero cloud accounts.
func syncKey() ([]byte, error) {
	encoded := os.Getenv("VAYUMAIL_SYNC_KEY")
	if encoded == "" {
		return nil, fmt.Errorf("VAYUMAIL_SYNC_KEY is not set (32 bytes, base64)")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("VAYUMAIL_SYNC_KEY must be 32 base64-encoded bytes")
	}
	return key, nil
}

// cmdSettingsPush seals a settings file and stores it in the account's
// VayuMail.Meta mailbox.
func cmdSettingsPush(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("settings-push", flag.ContinueOnError)
	file := fs.String("file", "", "settings JSON file to push")
	acct, cfg, code := loadAccount(ctx, db, fs, args)
	if code != 0 {
		return code
	}
	_ = acct
	plaintext, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read settings: %v\n", err)
		return 1
	}
	key, err := syncKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealed, err := appcrypto.SealBlob(key, plaintext, "vayumail-settings")
	if err != nil {
		fmt.Fprintf(os.Stderr, "seal: %v\n", err)
		return 1
	}
	err = imapsync.WithConnection(ctx, cfg, envCred,
		func(client *imapclient.Client) error {
			return imapsync.SaveSettingsBlob(client, sealed)
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "push: %v\n", err)
		return 1
	}
	fmt.Println("settings pushed (sealed) to", imapsync.MetaFolder)
	return 0
}

// cmdSettingsPull fetches and opens the newest settings blob.
func cmdSettingsPull(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("settings-pull", flag.ContinueOnError)
	acct, cfg, code := loadAccount(ctx, db, fs, args)
	if code != 0 {
		return code
	}
	_ = acct
	key, err := syncKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var sealed []byte
	err = imapsync.WithConnection(ctx, cfg, envCred,
		func(client *imapclient.Client) error {
			var lerr error
			sealed, lerr = imapsync.LoadSettingsBlob(client)
			return lerr
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pull: %v\n", err)
		return 1
	}
	if sealed == nil {
		fmt.Println("no settings blob on the server yet")
		return 0
	}
	plaintext, err := appcrypto.OpenBlob(key, sealed, "vayumail-settings")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open blob (wrong key?): %v\n", err)
		return 1
	}
	os.Stdout.Write(plaintext)
	return 0
}
