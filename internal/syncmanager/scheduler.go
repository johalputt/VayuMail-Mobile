package syncmanager

import (
	"context"
	"log/slog"
	"time"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
	"github.com/johalputt/VayuMail-Mobile/internal/mail/smtpsend"
)

const (
	// schedulerBase is the normal outbox/maintenance cadence.
	schedulerBase = 5 * time.Minute
	// schedulerMax caps the battery-aware backoff: repeated failures
	// stretch the cadence so a dead network never burns radio time.
	schedulerMax = 30 * time.Minute
)

// runScheduler drives periodic work for one account: flushing due outbox
// entries and retrying failed sends. The cadence is battery-aware — every
// consecutive failing pass doubles the interval up to schedulerMax, and
// the first successful pass snaps it back to schedulerBase.
func (m *Manager) runScheduler(ctx context.Context, accountID int64, cfg account.Config, cred func() (string, error)) {
	interval := schedulerBase
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		failed := false
		err := smtpsend.ProcessOutbox(ctx, m.db, cfg, cred, accountID,
			func(outboxID int64, sendErr error) {
				if sendErr != nil {
					failed = true
				}
				m.emit(SendResultEvent{OutboxID: outboxID, Err: sendErr})
			})
		if err != nil && ctx.Err() == nil {
			slog.Warn("outbox pass failed", "account", accountID, "err", err)
			failed = true
		}

		if failed {
			interval *= 2
			if interval > schedulerMax {
				interval = schedulerMax
			}
		} else {
			interval = schedulerBase
		}
		timer.Reset(interval)
	}
}
