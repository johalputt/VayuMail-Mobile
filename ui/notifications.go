package ui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gioui.org/x/notify"

	"github.com/johalputt/VayuMail-Mobile/internal/store"
	"github.com/johalputt/VayuMail-Mobile/internal/syncmanager"
)

const (
	// notifyStartupGrace suppresses notifications right after launch so
	// the initial sync of an existing mailbox does not flood the tray.
	notifyStartupGrace = 45 * time.Second
	// notifyCoalesce batches messages arriving close together into one
	// summary notification.
	notifyCoalesce = 2 * time.Second
)

// mailNotifier turns NewMessageEvents into system notifications
// (Android tray, desktop DBus). It runs on its own goroutine; the frame
// loop hands it events without ever blocking (Rule 5).
type mailNotifier struct {
	db        *store.DB
	notifier  notify.Notifier
	events    chan syncmanager.NewMessageEvent
	startedAt time.Time
	// enabled gates posting on the user's notifications setting; it must
	// be a cheap, non-blocking read (a snapshot field). Nil means on.
	enabled func() bool
	// preview controls whether notifications carry sender and subject or
	// only a generic line — the lock-screen privacy option. Nil means on.
	preview func() bool
}

// newMailNotifier starts the notifier. On platforms without a
// notification backend it degrades to a silent no-op.
func newMailNotifier(ctx context.Context, db *store.DB) *mailNotifier {
	n := &mailNotifier{
		db:        db,
		events:    make(chan syncmanager.NewMessageEvent, 64),
		startedAt: time.Now(),
	}
	notifier, err := notify.NewNotifier()
	if err != nil {
		slog.Info("system notifications unavailable", "err", err)
		return n
	}
	n.notifier = notifier
	go n.loop(ctx)
	return n
}

// observe is called from the frame loop's event drain; it never blocks.
func (n *mailNotifier) observe(ev syncmanager.Event) {
	if n.notifier == nil {
		return
	}
	msg, ok := ev.(syncmanager.NewMessageEvent)
	if !ok {
		return
	}
	if time.Since(n.startedAt) < notifyStartupGrace {
		return
	}
	if n.enabled != nil && !n.enabled() {
		return
	}
	select {
	case n.events <- msg:
	default:
		// Full buffer means a sync burst; the summary path covers it.
	}
}

// notifyChat posts a notification for an incoming VayuTalk message. It names the
// sender only when the lock-screen preview option is on (the same privacy gate as
// mail); otherwise it stays content-free. It never carries the message text —
// VayuTalk messages are ephemeral and end-to-end encrypted, so their content never
// belongs on a lock screen. Gated by the notifications setting and startup grace.
// Safe to call from any goroutine.
func (n *mailNotifier) notifyChat(peer string) {
	if n.notifier == nil {
		return
	}
	if time.Since(n.startedAt) < notifyStartupGrace {
		return
	}
	if n.enabled != nil && !n.enabled() {
		return
	}
	body := "New VayuTalk message"
	if peer != "" && (n.preview == nil || n.preview()) {
		body = "New message from " + peer
	}
	if _, err := n.notifier.CreateNotification("VayuTalk", body); err != nil {
		slog.Debug("post chat notification", "err", err)
	}
}

// loop coalesces bursts and posts notifications.
func (n *mailNotifier) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case first := <-n.events:
			batch := []syncmanager.NewMessageEvent{first}
			timer := time.NewTimer(notifyCoalesce)
		coalesce:
			for {
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case ev := <-n.events:
					batch = append(batch, ev)
				case <-timer.C:
					break coalesce
				}
			}
			n.post(ctx, batch)
		}
	}
}

// post renders one or a summary notification for the batch and hands it to the
// system notifier.
func (n *mailNotifier) post(ctx context.Context, batch []syncmanager.NewMessageEvent) {
	if n.notifier == nil {
		return
	}
	title, body := n.render(ctx, batch)
	if _, err := n.notifier.CreateNotification(title, body); err != nil {
		slog.Debug("post notification", "err", err)
	}
}

// render composes the notification title and body for a batch. It is split out
// from post so it can be unit-tested without a system notifier. Preview-off keeps
// it content-free (lock-screen privacy); otherwise it names the sender, the
// subject, and — so a user with several mailboxes knows WHICH inbox got mail —
// the mailbox it landed in.
func (n *mailNotifier) render(ctx context.Context, batch []syncmanager.NewMessageEvent) (title, body string) {
	if len(batch) == 0 {
		return "New mail", ""
	}
	if n.preview != nil && !n.preview() {
		// Privacy mode: never put sender, subject or mailbox on the lock screen.
		if len(batch) > 1 {
			return "New mail", fmt.Sprintf("%d new messages", len(batch))
		}
		return "New mail", ""
	}
	if len(batch) == 1 {
		ev := batch[0]
		title = "New mail"
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		msg, err := n.db.GetMessageByUID(lookupCtx, ev.FolderID, ev.UID)
		cancel()
		if err == nil {
			sender := msg.FromName
			if sender == "" {
				sender = msg.FromAddr
			}
			if sender != "" {
				title = sender
			}
			body = msg.Subject
			if body == "" {
				body = "(no subject)"
			}
		}
		if label := n.mailboxLabel(ctx, ev.AccountID, ev.FolderID); label != "" {
			if body != "" {
				body += "  ·  " + label
			} else {
				body = label
			}
		}
		return title, body
	}
	// Summary: name the mailbox when every message landed in the same one, so a
	// burst into one inbox still reads "N new messages in you@domain".
	body = fmt.Sprintf("%d new messages", len(batch))
	sameAccount := batch[0].AccountID
	for _, ev := range batch[1:] {
		if ev.AccountID != sameAccount {
			sameAccount = 0
			break
		}
	}
	if sameAccount != 0 {
		if label := n.mailboxLabel(ctx, sameAccount, 0); label != "" {
			body = fmt.Sprintf("%d new messages in %s", len(batch), label)
		}
	}
	return "New mail", body
}

// mailboxLabel names the mailbox a new message landed in — the account's address,
// with a non-Inbox folder appended (e.g. "you@example.com/Archive"). folderID 0
// skips the folder lookup (account-level label). Best-effort: an unknown account
// yields "".
func (n *mailNotifier) mailboxLabel(ctx context.Context, accountID, folderID int64) string {
	if n.db == nil || accountID == 0 {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	acct, err := n.db.GetAccount(lookupCtx, accountID)
	if err != nil {
		return ""
	}
	label := acct.EmailAddress
	if label == "" {
		label = acct.DisplayName
	}
	if folderID != 0 {
		if f, ferr := n.db.GetFolder(lookupCtx, folderID); ferr == nil && !f.IsInbox && f.Name != "" {
			label += "/" + f.Name
		}
	}
	return label
}
