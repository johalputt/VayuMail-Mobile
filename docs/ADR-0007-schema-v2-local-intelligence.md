# ADR-0007 — Schema v2 and on-device intelligence

## Status

Accepted — v1.1.0.

## Decision

Schema migration v2 extends the store for features computed **entirely
on-device** (the app learns nothing about the user and sends nothing
anywhere):

- `messages.has_trackers` — tracking pixels / tracker-hosted resources
  detected at parse time (`internal/mail/mime/track.go`). VayuMail never
  fetches remote content regardless; this powers the *"this sender
  tracks you"* indicator.
- `messages.is_list`, `messages.list_unsubscribe` — List-Id detection
  and RFC 2369/8058 unsubscribe support (mailto targets are executed
  end-to-end via `UnsubscribeCmd`; https targets are copied for the
  user).
- `messages.snooze_until` — local snooze; hidden from every list until
  the deadline (`SnoozeCmd`).
- `messages.attachments` — attachment metadata (JSON) captured at parse
  time, feeding the per-file download chips (`FetchAttachmentCmd` →
  `AttachmentSavedEvent`).
- `accounts.pinned_spki` — per-account TLS key pin (ADR-0008).
- `pgp_keys` table — persisted OpenPGP keys with trust levels, loaded
  into the keyring at startup; WKD-discovered and pasted keys both land
  here.
- `messages_fts` rebuilt with `body_text` — full-body search, plus the
  query-operator parser (`from:`, `subject:`, `has:attachment`,
  `is:unread`, `before:`, `after:`).

New commands (`FetchAttachmentCmd`, `SaveDraftCmd`, `SnoozeCmd`,
`UnsubscribeCmd`) and one new event (`AttachmentSavedEvent`) extend the
Section-3 channel contract; buffer sizes and overflow rules are
unchanged. Undo-send is implemented UI-side: the outbox row is written
immediately but `SendCmd` is dispatched only after a 10-second window,
during which Undo deletes the row.

## Context

Tier-2 differentiators ("intelligence") must not become a privacy
liability. Every signal here is derived from bytes the client already
possesses, stored locally, and queryable offline.

## Consequences

- Migrations are append-only; v1 databases upgrade in place (the FTS
  index is rebuilt once).
- The tracker-host list is a curated constant — updates ship with app
  releases, never fetched at runtime.
- Sent mail is filed to the Sent folder after a successful SMTP
  delivery; drafts append to the Drafts folder with `\Draft`.
