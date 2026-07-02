package imapsync

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// SyncFolders lists all mailboxes on the server, maps special-use
// attributes onto the local folder model, and upserts every folder into
// the store. It returns the fresh folder list.
func SyncFolders(ctx context.Context, client *imapclient.Client, db *store.DB, accountID int64) ([]store.Folder, error) {
	options := &imap.ListOptions{}
	if client.Caps().Has(imap.CapSpecialUse) {
		options.ReturnSpecialUse = true
	}
	list, err := client.List("", "*", options).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapsync: list folders: %w", err)
	}

	for _, data := range list {
		if hasAttr(data.Attrs, imap.MailboxAttrNoSelect) {
			continue
		}
		f := folderFromList(accountID, data)
		if _, err := db.UpsertFolder(ctx, &f); err != nil {
			return nil, fmt.Errorf("imapsync: store folder %q: %w", f.FullName, err)
		}
	}
	folders, err := db.ListFolders(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("imapsync: reload folders: %w", err)
	}
	return folders, nil
}

// folderFromList converts one LIST response into the local folder model.
func folderFromList(accountID int64, data *imap.ListData) store.Folder {
	delim := ""
	if data.Delim != 0 {
		delim = string(data.Delim)
	}
	name := data.Mailbox
	if delim != "" {
		if idx := strings.LastIndex(name, delim); idx >= 0 {
			name = name[idx+len(delim):]
		}
	}
	f := store.Folder{
		AccountID: accountID,
		Name:      name,
		FullName:  data.Mailbox,
		Delimiter: delim,
		IsInbox:   strings.EqualFold(data.Mailbox, "INBOX"),
		IsSent:    hasAttr(data.Attrs, imap.MailboxAttrSent),
		IsDrafts:  hasAttr(data.Attrs, imap.MailboxAttrDrafts),
		IsTrash:   hasAttr(data.Attrs, imap.MailboxAttrTrash),
		IsArchive: hasAttr(data.Attrs, imap.MailboxAttrArchive),
	}
	// Fallback heuristics for servers without SPECIAL-USE.
	if !f.IsSent && !f.IsDrafts && !f.IsTrash && !f.IsArchive && !f.IsInbox {
		switch strings.ToLower(name) {
		case "sent", "sent messages", "sent items":
			f.IsSent = true
		case "drafts":
			f.IsDrafts = true
		case "trash", "deleted messages", "deleted items":
			f.IsTrash = true
		case "archive", "archives":
			f.IsArchive = true
		}
	}
	return f
}

func hasAttr(attrs []imap.MailboxAttr, want imap.MailboxAttr) bool {
	for _, a := range attrs {
		if strings.EqualFold(string(a), string(want)) {
			return true
		}
	}
	return false
}
