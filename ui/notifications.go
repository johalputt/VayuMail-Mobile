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

// notifyChat posts a deliberately content-free notification for an
// incoming VayuTalk message. It carries no sender and no text — VayuTalk
// messages are ephemeral and end-to-end encrypted, so nothing about them
// belongs on a lock screen. Gated by the same notifications setting and
// startup grace as mail. Safe to call from any goroutine.
func (n *mailNotifier) notifyChat() {
	if n.notifier == nil {
		return
	}
	if time.Since(n.startedAt) < notifyStartupGrace {
		return
	}
	if n.enabled != nil && !n.enabled() {
		return
	}
	if _, err := n.notifier.CreateNotification("VayuTalk", "New VayuTalk message"); err != nil {
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

// post renders one or a summary notification for the batch.
func (n *mailNotifier) post(ctx context.Context, batch []syncmanager.NewMessageEvent) {
	title, body := "New mail", ""
	if n.preview != nil && !n.preview() {
		// Privacy mode: never put sender or subject on the lock screen.
		if len(batch) > 1 {
			body = fmt.Sprintf("%d new messages", len(batch))
		}
		if _, err := n.notifier.CreateNotification(title, body); err != nil {
			slog.Debug("post notification", "err", err)
		}
		return
	}
	if len(batch) == 1 {
		ev := batch[0]
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		msg, err := n.db.GetMessageByUID(lookupCtx, ev.FolderID, ev.UID)
		cancel()
		if err == nil {
			sender := msg.FromName
			if sender == "" {
				sender = msg.FromAddr
			}
			title = sender
			body = msg.Subject
			if body == "" {
				body = "(no subject)"
			}
		}
	} else {
		body = fmt.Sprintf("%d new messages", len(batch))
	}
	if _, err := n.notifier.CreateNotification(title, body); err != nil {
		slog.Debug("post notification", "err", err)
	}
}
