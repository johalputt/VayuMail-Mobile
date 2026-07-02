package smtpsend

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/account"
)

// ErrAuth wraps SMTP authentication failures; the outbox does not retry
// them because retrying a rejected credential can lock the account.
var ErrAuth = errors.New("smtpsend: authentication failed")

// Send delivers one raw RFC 5322 message. The password is used for this
// connection only and discarded when the function returns (Rule 6).
// Recipients are the envelope RCPT TO addresses; from is MAIL FROM.
func Send(ctx context.Context, cfg account.Config, password, from string, recipients []string, raw []byte) error {
	if len(recipients) == 0 {
		return fmt.Errorf("smtpsend: no recipients")
	}

	var (
		client *smtp.Client
		err    error
	)
	switch cfg.SMTPTLS {
	case account.TLSModeImplicit:
		client, err = smtp.DialTLS(cfg.SMTPAddr(), nil)
	case account.TLSModeSTARTTLS:
		client, err = smtp.DialStartTLS(cfg.SMTPAddr(), nil)
	default:
		return fmt.Errorf("smtpsend: unsupported TLS mode %q", cfg.SMTPTLS)
	}
	if err != nil {
		return fmt.Errorf("smtpsend: dial %s: %w", cfg.SMTPAddr(), err)
	}
	defer func() {
		// Quit already closes; Close here is the error-path fallback.
		_ = client.Close()
	}()

	if err := client.Auth(sasl.NewPlainClient("", cfg.Username, password)); err != nil {
		if isPermanentSMTPError(err) {
			return fmt.Errorf("%w: %v", ErrAuth, err)
		}
		return fmt.Errorf("smtpsend: auth: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.SendMail(from, recipients, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("smtpsend: send: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtpsend: quit: %w", err)
	}
	return nil
}

// isPermanentSMTPError reports whether err is a 5xx SMTP status — a
// permanent rejection that retrying cannot fix.
func isPermanentSMTPError(err error) bool {
	var smtpErr *smtp.SMTPError
	if errors.As(err, &smtpErr) {
		return smtpErr.Code >= 500
	}
	return false
}
