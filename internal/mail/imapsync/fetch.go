package imapsync

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/mime"
	"github.com/johalputt/VayuMail-Mobile/internal/store"
)

// maxInlineBodyBytes caps how much of a message body is downloaded during
// sync. Larger messages are cached envelope-first; their full body loads
// when the user opens them (via FetchBody).
const maxInlineBodyBytes = 512 * 1024

// SyncFolder performs an incremental sync of the selected folder: it
// fetches every UID above the highest one cached locally, parses and
// stores each message, and reports progress through ev. The folder must
// already be selected on the client.
func SyncFolder(ctx context.Context, client *imapclient.Client, db *store.DB, ev Events, accountID int64, folder store.Folder, selected *imap.SelectData) error {
	// A UIDVALIDITY change invalidates every cached UID (RFC 3501).
	if folder.UIDValidity != 0 && selected.UIDValidity != folder.UIDValidity {
		if err := db.ClearFolderMessages(ctx, folder.ID); err != nil {
			return err
		}
	}
	if err := db.SetFolderSyncState(ctx, folder.ID, selected.UIDValidity,
		selected.HighestModSeq); err != nil {
		return err
	}

	highest, err := db.HighestUID(ctx, folder.ID)
	if err != nil {
		return err
	}
	if selected.NumMessages == 0 {
		return nil
	}

	var uidSet imap.UIDSet
	uidSet.AddRange(imap.UID(highest+1), 0) // (highest, *]

	fetchOptions := &imap.FetchOptions{
		UID:        true,
		Envelope:   true,
		Flags:      true,
		RFC822Size: true,
	}
	envelopes, err := client.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return fmt.Errorf("imapsync: fetch envelopes: %w", err)
	}

	total := len(envelopes)
	for done, buf := range envelopes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Servers answer "UID FETCH n+1:*" with the last message when the
		// range is empty; skip UIDs we already have.
		if uint32(buf.UID) <= highest {
			continue
		}
		msg := messageFromBuffer(accountID, folder.ID, buf)
		if buf.RFC822Size <= maxInlineBodyBytes {
			if err := fetchAndAttachBody(client, buf.UID, &msg); err != nil {
				// Body fetch failure degrades to envelope-only cache.
				msg.Snippet = ""
			}
		}
		if _, err := db.UpsertMessage(ctx, &msg); err != nil {
			return err
		}
		if ev.NewMessage != nil {
			ev.NewMessage(folder.ID, uint32(buf.UID))
		}
		if ev.SyncProgress != nil {
			ev.SyncProgress(done+1, total)
		}
	}
	return nil
}

// fetchAndAttachBody downloads the full body for one UID and fills the
// body, snippet, attachment, and PGP fields of msg.
func fetchAndAttachBody(client *imapclient.Client, uid imap.UID, msg *store.Message) error {
	bodySection := &imap.FetchItemBodySection{Peek: true}
	full, err := client.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}).Collect()
	if err != nil {
		return fmt.Errorf("imapsync: fetch body uid %d: %w", uid, err)
	}
	if len(full) == 0 || len(full[0].BodySection) == 0 {
		return fmt.Errorf("imapsync: empty body response for uid %d", uid)
	}
	raw := full[0].BodySection[0].Bytes
	parsed, err := mime.Parse(raw)
	if err != nil {
		return fmt.Errorf("imapsync: parse body uid %d: %w", uid, err)
	}
	msg.BodyText = parsed.Text
	msg.BodyHTML = parsed.HTML
	msg.Snippet = parsed.Snippet
	msg.HasAttachments = len(parsed.Attachments) > 0
	msg.PGPStatus = parsed.PGPStatus
	msg.HasTrackers = parsed.HasTrackers
	msg.IsList = parsed.ListID != ""
	msg.ListUnsubscribe = parsed.ListUnsubscribe
	if len(parsed.Attachments) > 0 {
		if encoded, err := json.Marshal(parsed.Attachments); err == nil {
			msg.Attachments = string(encoded)
		}
	}
	return nil
}

// FetchRaw downloads the complete raw RFC 5322 bytes of one message.
// Used for attachment extraction and oversized-body loads; the folder
// must already be selected.
func FetchRaw(client *imapclient.Client, uid uint32) ([]byte, error) {
	full, err := client.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{Peek: true}},
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapsync: fetch raw uid %d: %w", uid, err)
	}
	if len(full) == 0 || len(full[0].BodySection) == 0 {
		return nil, fmt.Errorf("imapsync: empty raw response for uid %d", uid)
	}
	return full[0].BodySection[0].Bytes, nil
}

// FetchBody downloads and parses the full body of one already-cached
// message, updating the stored row. Used for messages that exceeded
// maxInlineBodyBytes during sync.
func FetchBody(ctx context.Context, client *imapclient.Client, db *store.DB, msg *store.Message) error {
	if err := fetchAndAttachBody(client, imap.UID(msg.UID), msg); err != nil {
		return err
	}
	if _, err := db.UpsertMessage(ctx, msg); err != nil {
		return err
	}
	return ctx.Err()
}

// messageFromBuffer maps a FETCH response onto the store model.
func messageFromBuffer(accountID, folderID int64, buf *imapclient.FetchMessageBuffer) store.Message {
	msg := store.Message{
		AccountID: accountID,
		FolderID:  folderID,
		UID:       uint32(buf.UID),
		SizeBytes: buf.RFC822Size,
		Flags:     joinFlags(buf.Flags),
		IsRead:    hasFlag(buf.Flags, imap.FlagSeen),
		IsFlagged: hasFlag(buf.Flags, imap.FlagFlagged),
	}
	env := buf.Envelope
	if env == nil {
		return msg
	}
	msg.Subject = env.Subject
	msg.MessageID = env.MessageID
	if len(env.InReplyTo) > 0 {
		msg.InReplyTo = env.InReplyTo[0]
	}
	if !env.Date.IsZero() {
		msg.Date = env.Date.UTC()
	}
	if len(env.From) > 0 {
		msg.FromAddr = env.From[0].Addr()
		msg.FromName = env.From[0].Name
	}
	msg.ToAddrs = joinAddrs(env.To)
	msg.CcAddrs = joinAddrs(env.Cc)
	msg.ThreadID = ThreadID(env.Subject, msg.MessageID, msg.InReplyTo)
	return msg
}

var reSubjectPrefix = regexp.MustCompile(`(?i)^\s*((re|fwd?|aw|sv)\s*(\[\d+\])?\s*:\s*)+`)

// ThreadID derives a stable conversation key. Replies share the thread of
// the message they answer; otherwise messages group by normalized subject.
// This is deliberate poor-man's threading for v0.1 — no server THREAD
// extension required, fully offline.
func ThreadID(subject, messageID, inReplyTo string) string {
	normalized := strings.ToLower(strings.TrimSpace(
		reSubjectPrefix.ReplaceAllString(subject, "")))
	if normalized != "" {
		return "subj:" + normalized
	}
	if inReplyTo != "" {
		return "ref:" + inReplyTo
	}
	return "msg:" + messageID
}

func joinAddrs(addrs []imap.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.Addr())
	}
	return strings.Join(parts, ", ")
}

func joinFlags(flags []imap.Flag) string {
	parts := make([]string, 0, len(flags))
	for _, f := range flags {
		parts = append(parts, string(f))
	}
	return strings.Join(parts, " ")
}

func splitFlags(flags string) []string {
	if flags == "" {
		return nil
	}
	return strings.Fields(flags)
}

func hasFlag(flags []imap.Flag, want imap.Flag) bool {
	for _, f := range flags {
		if strings.EqualFold(string(f), string(want)) {
			return true
		}
	}
	return false
}
