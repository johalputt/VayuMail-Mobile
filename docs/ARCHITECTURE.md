# VayuMail-Mobile Architecture

This document is normative. Do not deviate from the goroutine topology or
channel types below without writing a new ADR (CONTRIBUTING.md).

## Goroutine topology

```
main goroutine (cmd/vayumail/main.go)
│
├─ 1. store.Open(...)                     SQLite, WAL, migrations
├─ 2. syncmanager.New(db, keystore)       creates eventCh(256) + cmdCh(64)
├─ 3. mgr.Start(ctx)
│      │
│      ├─ commandLoop            drains cmdCh, executes typed commands
│      │                          (short-lived IMAP/SMTP connections)
│      │
│      └─ per account:
│          ├─ idleLoop           imapsync.RunIDLE — holds the IMAP IDLE
│          │                      connection, syncs deltas, writes to
│          │                      SQLite, emits Events (dispatcher role
│          │                      folded into the Events callbacks)
│          └─ scheduler          outbox flush every 5m, battery-aware
│                                 backoff to 30m on repeated failure
│
├─ 4. Gio event loop (ui.Run)    single-threaded, never blocks
│      └─ AppState loader        the only goroutine that reads SQLite
│                                 for the UI; publishes snapshots and
│                                 calls window.Invalidate()
│
└─ 5. window close → cancel ctx → mgr.Shutdown() waits on WaitGroup
       (goleak-verified: zero leaked goroutines) → db.Close()
```

## Channels

Defined in `internal/syncmanager/events.go` and `commands.go`:

```go
eventCh chan Event   // buffered 256, flows FROM syncmanager TO ui
cmdCh   chan Cmd     // buffered 64,  flows FROM ui TO syncmanager
```

Event types: `NewMessageEvent`, `FlagChangeEvent`, `SyncProgressEvent`,
`SendResultEvent`, `AuthErrorEvent`, `ConnectionEvent`, `FolderListEvent`.

Command types: `MoveCmd`, `DeleteCmd`, `MarkCmd`, `SendCmd`, `SyncNowCmd`,
`AddAccountCmd`.

Overflow policy (Rule 5, enforced in code):

- `eventCh` full → the event is **dropped with a log line**; sync
  goroutines never block. Safe because the UI reloads from the store on
  every event, so a dropped event delays nothing permanently.
- `cmdCh` full → `Manager.Send` returns an **error immediately**; the UI
  shows a transient snackbar. `Layout()` never blocks.

The Gio frame loop drains `eventCh` non-blockingly before every frame:

```go
case app.FrameEvent:
    for {
        select {
        case ev := <-events: state.Apply(ev)
        default:              goto draw
        }
    }
```

## Package dependency graph

```
cmd/vayumail ────────► ui ────────► ui/{screens,widgets,state,theme}
cmd/vayumail-cli ─┐        │
                  │        ▼
                  └──► internal/syncmanager ──► internal/mail/{imapsync,smtpsend}
                              │                        │
                              ▼                        ▼
                        internal/store ◄──── internal/mail/mime
                              ▲
internal/mail/account ◄───────┘ (imapsync, smtpsend import account)
internal/mail/pgp      (imported by ui/state and tests)
internal/crypto        (keystore; imported by syncmanager and cmd)
internal/push          (platform hooks; imported by platform code)
```

Rules encoded in the graph:

- `internal/mail`, `internal/store`, `internal/syncmanager` never import
  Gio (Rule 4, CI-enforced).
- `internal/mail/*` never imports `internal/syncmanager`. The IDLE loop
  reports through the `imapsync.Events` callback struct and the
  syncmanager adapts those callbacks onto the typed event bus. This keeps
  the import graph acyclic and lets a CLI use `internal/mail` without
  pulling the sync layer (see ADR-0001; this is the one deliberate
  refinement of the original `RunIDLE(..., chan<- syncmanager.Event)`
  sketch, which Go's import-cycle rule cannot express).
- Only the AppState loader goroutine reads SQLite for the UI; `Layout()`
  reads an in-memory snapshot under a short mutex.

## Data flow: receiving a new email

1. The server pushes `* N EXISTS` on the idling connection.
2. go-imap's unilateral handler enqueues a `Notification` (buffered 64,
   drop-on-full — safe, any notification triggers the same delta sync).
3. `idleLoop` sends `DONE`, runs `SyncFolder`: fetches every UID above
   the highest cached one (envelope, flags, size), then the body for
   messages ≤ 512 KiB, parses MIME (`internal/mail/mime`), and upserts
   into SQLite.
4. Each stored message fires `Events.NewMessage`; the manager emits
   `NewMessageEvent` onto `eventCh` and the loop re-enters IDLE.
5. The next frame drains the event; `AppState.Apply` schedules a snapshot
   reload; the loader re-queries SQLite and invalidates the window.
6. The inbox re-renders from the new snapshot. Unread counts come from
   the partial index on `is_read = 0`.

## Data flow: sending an email

1. The composer builds a `smtpsend.Draft`; `AppState.EnqueueDraft`
   serializes it (PGP/MIME when the encrypt toggle is on) **in a
   goroutine**, writes the raw bytes into the `outbox` table, and sends
   `SendCmd{OutboxID}`.
2. `commandLoop` executes the send: envelope recovered from the stored
   headers, Bcc stripped from the wire bytes, STARTTLS/TLS connection,
   AUTH PLAIN with the password fetched from the keystore for exactly
   this connection.
3. Success deletes the outbox row; failure records the error and
   schedules a retry (1m·2ⁿ). After 5 failures the entry becomes a dead
   letter surfaced in the UI. Either way a `SendResultEvent` reaches the
   UI as a snackbar.
4. The scheduler retries due entries every 5 minutes independently of
   the UI.

## Data flow: provisioning an account via QR

1. The scanner decodes the QR into a base64url payload (gozxing,
   pure Go).
2. `account.ParseAndVerify` — version check, expiry check, Ed25519
   signature over canonical JSON, TLS-mode check, port check. Any
   failure returns a typed error and a clear user-facing message;
   **no field of an unverified payload is ever used** (Rule 7).
3. `account.ExchangeToken` POSTs the one-time token to the payload's
   endpoint and receives the mail credential. Runs in a goroutine, never
   on the frame loop.
4. `AddAccountCmd` stores the credential in the platform keystore,
   zeroes the in-memory copy, inserts the account row (which carries
   only the keystore alias — Rule 6), and starts the account's IDLE loop
   and scheduler.

## Concurrency invariants

- Zero goroutine leaks: `Manager.Shutdown()` cancels the root context
  and waits on a WaitGroup; verified by `goleak.VerifyNone` in every
  syncmanager test.
- No mutex is held across an I/O call anywhere in the codebase.
- Credentials exist in memory only between keystore fetch and connection
  teardown.
- SQLite is opened with one connection (`SetMaxOpenConns(1)`), WAL mode,
  5s busy timeout — writers from the sync side and reads from the UI
  loader serialize without `SQLITE_BUSY` surprises on mobile storage.
