# ADR-0011 — Device-approval onboarding

- **Status:** Accepted (v2.1.6). Pairs with VayuPress ADR-0129
  (device registry and approval enforcement on the server side).

## Context

Until now, direct connect handed the typed mailbox password straight
to IMAP. That means one credential unlocks mail from any device,
forever, with no way for an operator to see which installs exist or to
revoke a single lost phone without rotating the password everywhere.

VayuPress ADR-0129 adds a device registry: a mail domain can enforce
device approval, in which case IMAP rejects the raw mailbox password
and any pending or blocked device password — only the per-device
password of an *approved* device syncs mail. The server exposes two
JSON endpoints on the account's own domain:

- `POST /api/v1/members/vayumail-device-register` with
  `{"email","password","device_name","platform"}` returns
  `{"device_id","device_password","status"}` where status is
  `pending` or `approved`; `401` for bad credentials, `404`/`405`
  when the server predates the feature.
- `POST /api/v1/members/vayumail-device-status` with
  `{"email","device_id","device_password"}` returns
  `{"status":"pending"|"approved"|"blocked"}`.

The app must adopt this without regressing servers that don't have it,
and within the standing constraints: engine packages stay Gio-free
(Rule 1/4), nothing blocks a frame (Rule 5), credentials never touch
SQLite (Rule 6).

## Decision

**A client the shape of the private-key fetcher.**
`internal/mail/account/device.go` mirrors `privkey.go`'s transport
discipline exactly: HTTPS only, the `publicMailDomain` SSRF guard vets
the domain before any request leaves the process, redirects are
refused (`http.ErrUseLastResponse`) so neither the mailbox credential
nor the granted device password can be replayed to another host, and
responses are size-capped. `RegisterDevice` returns a `DeviceGrant`;
`DeviceStatus` polls it.

**Absence is a typed sentinel, and absence means "behave like 2.1.5".**
`ErrNoDeviceEndpoint` covers `404`/`405` *and* a `200` whose body is
not the endpoint's JSON (a storefront or misrouting proxy). On that
error the connect flow adds the account with the typed password,
byte-for-byte the pre-approval behavior — old servers never regress. A
`401` is the distinct `ErrDeviceCredentials`: the endpoint exists and
the password is wrong, so onboarding reports it instead of deferring
the failure to a confusing IMAP auth error. Any other failure surfaces
as an error rather than silently falling back, because on an enforcing
server the fallback password cannot sync anyway.

**Onboarding waits, off the frame loop.** After autoconfig discovery,
`connect()` registers the device (`device_name` is "VayuMail on
Android/Linux/…" from `runtime.GOOS`). An `approved` grant syncs
immediately with the device password. A `pending` grant parks the
connect card in a wait state — the existing mutex-guarded status line
explains where to approve the device — while the same goroutine polls
`DeviceStatus` every 5 seconds under a context that is cancelled when
the user leaves the flow (cancel, setup-code, or manual buttons) or
after 10 minutes. Approved → account added with the device password
and a success snack; blocked → "This device was blocked from the web
console."; timeout → a gentle retry hint. Every transition reaches the
UI via the guarded fields plus `Refresh()` wake-ups (Rule 5).

**Only the public ID is persisted.** The granted `device_id` is stored
in settings under `device-id:<email>` so the install can be
cross-referenced in the web console later. The device password becomes
the account credential in the platform keystore, exactly like a
mailbox password before it — never SQLite (Rule 6, ADR-0004).

## Consequences

- A lost phone is revoked from the VayuPress console without touching
  the mailbox password; each install authenticates as itself.
- On enforcing domains, onboarding gains a human step. The wait state
  makes it legible, bounded, and cancellable.
- The typed mailbox password is still sent once, to the account's own
  domain over pinned-discipline HTTPS, to authorize registration —
  the same exposure `FetchPrivateKey` (ADR-0008 transport rules)
  already accepts.
- Polling is 12 requests/minute worst case for at most 10 minutes,
  only while the user sits on the connect card.
- If a device is deleted server-side later, IMAP starts rejecting the
  stored device password; the existing auth-error banner and
  update-password path recover, and a fresh connect re-registers.
