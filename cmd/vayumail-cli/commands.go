package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/imapsync"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// cmdAccounts lists configured accounts.
func cmdAccounts(ctx context.Context, db *store.DB) int {
	accounts, err := db.ListAccounts(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list accounts: %v\n", err)
		return 1
	}
	if len(accounts) == 0 {
		fmt.Println("no accounts configured")
		return 0
	}
	for _, a := range accounts {
		fmt.Printf("%d\t%s\tIMAP %s:%d (%s)\tSMTP %s:%d (%s)\n",
			a.ID, a.EmailAddress, a.IMAPHost, a.IMAPPort, a.IMAPTLS,
			a.SMTPHost, a.SMTPPort, a.SMTPTLS)
	}
	return 0
}

// cmdAddAccount persists an account row. The credential is deliberately
// NOT accepted here: the CLI reads VAYUMAIL_PASSWORD at connect time, so
// no secret ever reaches the database or shell history via flags.
func cmdAddAccount(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("add-account", flag.ContinueOnError)
	email := fs.String("email", "", "email address (also the username)")
	host := fs.String("host", "", "IMAP and SMTP host")
	imapPort := fs.Int("imap-port", 993, "IMAP port (implicit TLS)")
	smtpPort := fs.Int("smtp-port", 587, "SMTP port (STARTTLS)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg := account.Config{
		DisplayName:   *email,
		EmailAddress:  *email,
		IMAPHost:      *host,
		IMAPPort:      *imapPort,
		IMAPTLS:       account.TLSModeImplicit,
		SMTPHost:      *host,
		SMTPPort:      *smtpPort,
		SMTPTLS:       account.TLSModeSTARTTLS,
		Username:      *email,
		KeystoreAlias: "cli-env", // resolved to VAYUMAIL_PASSWORD at runtime
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid account: %v\n", err)
		return 2
	}
	row := store.Account{
		DisplayName: cfg.DisplayName, EmailAddress: cfg.EmailAddress,
		IMAPHost: cfg.IMAPHost, IMAPPort: cfg.IMAPPort, IMAPTLS: string(cfg.IMAPTLS),
		SMTPHost: cfg.SMTPHost, SMTPPort: cfg.SMTPPort, SMTPTLS: string(cfg.SMTPTLS),
		Username: cfg.Username, KeystoreAlias: cfg.KeystoreAlias,
	}
	id, err := db.InsertAccount(ctx, &row)
	if err != nil {
		fmt.Fprintf(os.Stderr, "insert account: %v\n", err)
		return 1
	}
	fmt.Printf("account %d added\n", id)
	return 0
}

// envCred fetches the password from the environment at connection time.
func envCred() (string, error) {
	pw := os.Getenv("VAYUMAIL_PASSWORD")
	if pw == "" {
		return "", fmt.Errorf("VAYUMAIL_PASSWORD is not set")
	}
	return pw, nil
}

// loadAccount resolves -account ID into config.
func loadAccount(ctx context.Context, db *store.DB, fs *flag.FlagSet, args []string) (store.Account, account.Config, int) {
	id := fs.Int64("account", 0, "account ID (see: accounts)")
	rest := fs
	if err := rest.Parse(args); err != nil {
		return store.Account{}, account.Config{}, 2
	}
	acct, err := db.GetAccount(ctx, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "account %d: %v\n", *id, err)
		return store.Account{}, account.Config{}, 1
	}
	cfg := account.Config{
		DisplayName: acct.DisplayName, EmailAddress: acct.EmailAddress,
		IMAPHost: acct.IMAPHost, IMAPPort: acct.IMAPPort,
		IMAPTLS:  account.TLSMode(acct.IMAPTLS),
		SMTPHost: acct.SMTPHost, SMTPPort: acct.SMTPPort,
		SMTPTLS:  account.TLSMode(acct.SMTPTLS),
		Username: acct.Username, KeystoreAlias: acct.KeystoreAlias,
	}
	return acct, cfg, 0
}

// cmdSync runs one full folder + INBOX sync.
func cmdSync(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	acct, cfg, code := loadAccount(ctx, db, fs, args)
	if code != 0 {
		return code
	}
	events := imapsync.Events{
		NewMessage: func(folderID int64, uid uint32) {
			fmt.Printf("new message: folder=%d uid=%d\n", folderID, uid)
		},
		SyncProgress: func(done, total int) {
			fmt.Printf("\rsync %d/%d", done, total)
			if done == total {
				fmt.Println()
			}
		},
	}
	err := imapsync.WithConnection(ctx, cfg, envCred, func(client *imapclient.Client) error {
		if _, err := imapsync.SyncFolders(ctx, client, db, acct.ID); err != nil {
			return err
		}
		folder, err := db.GetFolderByFullName(ctx, acct.ID, "INBOX")
		if err != nil {
			return err
		}
		selected, err := client.Select("INBOX", nil).Wait()
		if err != nil {
			return err
		}
		return imapsync.SyncFolder(ctx, client, db, events, acct.ID, folder, selected)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync: %v\n", err)
		return 1
	}
	fmt.Println("sync complete")
	return 0
}

// cmdWatch holds an IDLE connection and prints events until interrupted.
func cmdWatch(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	acct, cfg, code := loadAccount(ctx, db, fs, args)
	if code != 0 {
		return code
	}
	events := imapsync.Events{
		NewMessage: func(folderID int64, uid uint32) {
			fmt.Printf("%s new message: folder=%d uid=%d\n",
				time.Now().Format(time.TimeOnly), folderID, uid)
		},
		Connection: func(online bool) {
			fmt.Printf("%s online=%v\n", time.Now().Format(time.TimeOnly), online)
		},
		AuthError: func(err error) {
			fmt.Fprintf(os.Stderr, "auth error: %v\n", err)
		},
	}
	fmt.Println("watching INBOX (Ctrl-C to stop)")
	err := imapsync.RunIDLE(ctx, cfg, envCred, "INBOX", db, events, acct.ID)
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		return 1
	}
	return 0
}

// cmdSearch runs an FTS5 query.
func cmdSearch(ctx context.Context, db *store.DB, args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	id := fs.Int64("account", 0, "account ID")
	query := fs.String("query", "", "search text")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	results, err := db.Search(ctx, *id, *query, 50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "search: %v\n", err)
		return 1
	}
	for _, r := range results {
		m := r.Message
		fmt.Printf("%s\t%s\t%s\n", m.Date.Format(time.DateOnly), m.FromAddr, m.Subject)
	}
	fmt.Printf("%d result(s)\n", len(results))
	return 0
}

// cmdCodeVerify parses and verifies a provisioning setup-code file,
// printing the outcome. It never contacts the network.
func cmdCodeVerify(args []string) int {
	fs := flag.NewFlagSet("code-verify", flag.ContinueOnError)
	file := fs.String("file", "", "file containing the base64url setup code")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read payload: %v\n", err)
		return 1
	}
	payload, err := account.ParseAndVerify(raw, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "REJECTED: %v\n", err)
		return 1
	}
	fmt.Printf("VERIFIED\nserver: %s\nimap: %d/%s\nsmtp: %d/%s\nuser: %s\nexpires: %s\n",
		payload.Server, payload.IMAPPort, payload.IMAPTLS,
		payload.SMTPPort, payload.SMTPTLS, payload.Username,
		time.Unix(payload.ExpiresAt, 0).Format(time.RFC3339))
	return 0
}
