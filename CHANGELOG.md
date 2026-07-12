# Changelog

All notable changes to VayuMail-Mobile are documented here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [2.2.9] — 2026-07-12

### Fixed
- **Chat is now two-sided: your messages sit on the right, theirs on the left.**
  Sent and received bubbles were both hugging the left edge. The cause was a
  layout bug — the flexible spacer that pushes a sent bubble to the right returned
  a zero-size box, and Gio advances by a child's returned size, so the spacer
  collapsed and the bubble fell back to the left. The spacer now fills its
  allocated width, so the thread reads like a normal chat. (Also fixes the
  outgoing status — “Sending…/Sent/Queued/Read” — aligning to the far edge of the
  bubble.)

### Known limitation
- Emoji still render as empty boxes on the phone. This is a limitation of the
  pure-Go UI toolkit (Gio), which does not use Android's colour-emoji font; the
  message text itself transmits and decrypts correctly (the web shows the emoji).
  A bundled emoji font is planned. Plain text and all message delivery are
  unaffected.

## [2.2.8] — 2026-07-12

### Fixed
- **Talk-subdomain discovery no longer depends on a CDN-fronted fetch.** The app
  learned its proxy-off `talk.<domain>` relay from the main domain's autoconfig —
  but that fetch can itself be bot-challenged by the very CDN the subdomain exists
  to bypass, which made discovery circular. The app now also probes the
  conventional `talk.<domain>` directly (a proxy-off host, so it reaches the origin
  even when autoconfig can't be read), and uses it once it answers as a live
  relay. Still trust-constrained to the mail domain and still falls back to the
  mail domain when no live relay is found, so nothing regresses for servers
  without a talk subdomain.

## [2.2.7] — 2026-07-12

### Added
- **Chat automatically uses your server's proxy-off `talk.<domain>` relay when it
  exists.** If a CDN (e.g. Cloudflare) sits in front of your server, its bot
  challenge blocks the app's long-lived chat stream — so the app could send but
  never receive. When VayuPress advertises a dedicated `talk.` subdomain (set up
  automatically once you point its DNS), the app now discovers it, confirms it is
  within your mail domain and answering as a live relay, and routes its chat
  stream there — bypassing the CDN with no setting to change. If none is
  advertised (or it isn't reachable), the app keeps using the mail domain exactly
  as before, so nothing changes for servers without a talk subdomain.

### Notes
- Pairs with VayuPress 3.11.48, which provisions the `talk.<domain>` vhost + TLS
  cert and advertises the host on its own. See that release's notes: your only
  step is one DNS record.

## [2.2.6] — 2026-07-12

### Added
- **Verify screen now shows BOTH safety numbers — yours and your contact's.**
  Previously it showed only the contact's number, so there was nothing to read
  back to them. It now displays "You" (your own key) above the contact's, exactly
  like the web console, so the two of you can compare both numbers over a trusted
  channel and confirm no one is in the middle. Your own number appears as soon as
  your key has synced.

### Notes
- Pairs with VayuPress 3.11.47, which stops the web console from consuming
  messages out of a shared mailbox's queue before this app can receive them (the
  cause of "an app-to-app message only shows up in the web"). Update the server
  too, and confirm it is running the new build with
  `curl -s https://YOUR-DOMAIN/health` → `"version":"3.11.47"`.

## [2.2.5] — 2026-07-12

### Fixed
- **Faster recovery when a key re-sync briefly fails.** The self-heal that
  re-fetches your key on a decrypt failure now waits only a few seconds before
  retrying after a *failed* fetch (instead of the full 30-second cooldown used
  after a successful one), so a momentary network/server blip no longer delays a
  web-sent message — while a persistent failure still can't hammer the server.

## [2.2.4] — 2026-07-12

### Fixed
- **Messages sent from the web now reliably decrypt on the phone.** If an
  incoming message can't be opened — because this device's key had drifted from
  the one the server encrypted to — the app now automatically re-fetches its
  authoritative key from the server and retries, instead of silently dropping the
  message. This self-heals a stale key without any manual "sync" step. The
  re-fetch is rate-limited so a burst of unreadable messages can't hammer the
  server. (`TestHandleEnvelopeResyncsOnDecryptFailure`.)

### Added
- **Automated Google Play publishing.** The release workflow now uploads the
  signed AAB to Google Play on every release (gated on a service-account secret;
  a no-op until configured). Once set up, a GitHub release becomes a Play release
  with no manual step, and Play updates the app on every device automatically —
  no sideloading. One-time setup is documented in `docs/PLAY-PUBLISHING.md`.

## [2.2.3] — 2026-07-12

### Added
- **Message clock time.** Each chat bubble now shows the wall-clock time it was
  sent (the server's send time, in your local zone), matching what the web
  shows for the same message — alongside the existing disappear-countdown.

### Fixed
- **Incoming messages show when they were sent, not when the phone received
  them.** A message that waited in the queue while you were offline now displays
  its real send time (from the server), instead of the moment it arrived.

## [2.2.2] — 2026-07-12

### Fixed
- **Messages sent from the VayuPress web now arrive on the phone.** The app
  automatically syncs this mailbox's authoritative private key from your
  VayuPress server whenever VayuTalk starts, so the device can always decrypt a
  message the web composed against the server-held key. Previously, if the
  device's key had drifted from the server's (or was never synced), web→app
  messages silently failed to open. The synced key also improves on-device mail
  decryption.

### Changed
- **One delivery mode, always reliable.** The Live/Store toggle is removed —
  every message is store-and-forward: delivered live when the other side is
  connected, otherwise queued and delivered on their next connect. This matches
  the web and removes the “Live” setting that could silently drop a message to an
  offline contact.

## [2.2.1] — 2026-07-12

### Fixed
- **VayuTalk now delivers in real time and never drops a message.** Two
  changes make chat feel seamless with the web and other devices:
  - **Store-and-forward is the default.** A message is delivered live if
    the other side is connected right now, and otherwise queued and
    delivered the moment they next connect — nothing is dropped for being
    offline. (Previously the composer defaulted to “Live”, which silently
    dropped a message whenever the peer wasn’t connected at that instant.)
  - **VayuTalk stays connected in the background.** The app now keeps its
    encrypted VayuTalk stream open for the active account the whole time
    it’s running — not only while the chat screen is open — so incoming
    messages arrive immediately and anything queued while you were away
    drains as soon as you’re back. Reconnects no longer duplicate an
    unread message.

## [2.2.0] — 2026-07-11

### Added
- **VayuTalk — ephemeral, end-to-end-encrypted messaging.** A new
  private chat built on your own VayuPress server. Messages are PGP
  encrypted to the recipient before they leave the device; the server is
  a blind relay that only ever holds opaque ciphertext, in memory, with a
  strict time-to-live. Open a chat from the drawer, verify a
  correspondent once by comparing safety numbers (Signal-style), and
  talk. Messages are destroyed on the server the moment they are read,
  expire automatically after their TTL, and a server restart purges
  everything. Reuses your existing mailbox identity, PGP keypair and
  approved-device credential — no new account, no phone number.

## [2.1.9] — 2026-07-11

### Changed
- **Target SDK is now Android 15 (API level 35).** Google Play requires
  new apps and updates to target API 35; the release build now sets
  `-targetsdk 35` (min SDK unchanged at 24) and installs the android-35
  platform. No app behaviour changes.

## [2.1.8] — 2026-07-11

### Changed
- **Release bundles are now signed with the project's own upload key**
  (via the `ANDROID_KEYSTORE_B64` / `ANDROID_KEYSTORE_PASS` repository
  secrets) instead of the shared debug key. The `.aab` is Play
  Console-ready: on first upload Google registers this key as the app's
  upload key under Play App Signing. Package ID is `com.vayu.mail`.

## [2.1.7] — 2026-07-11

### Changed
- **Android application ID is now `com.vayu.mail`** (was
  `org.vayumail.mobile`). This is the app's permanent identity on Google
  Play. Because the package name is a new identity, this build installs
  as a separate app rather than updating an `org.vayumail.mobile` install
  — uninstall the old sideloaded build first. The APK and AAB are
  otherwise unchanged and built from the same signed inputs.

## [2.1.6] — 2026-07-11

### Fixed
- **Encrypted mail no longer sticks on "fetching the locked copy…".** The
  repair loop had no terminal state: a broken cached row whose re-download
  produced the same bytes re-fetched forever. The repair now runs exactly
  once per session; armored bodies decrypt on-device, and mail the server
  already opened (VayuPress 3.11.33's transparent decryption) displays
  directly — with the Security row still reading "PGP end-to-end
  encrypted" via the server's X-VayuPGP marker.
- **The keyboard no longer covers the sign-in fields.** When the soft
  keyboard opens, the connect card switches to a compact scrollable
  layout (smaller logo, tighter spacing), keeping the email and password
  fields visible above the keyboard.

### Added
- **Device-approval onboarding (ADR-0011).** Connecting an account now
  registers this install as a named device with the account's VayuPress
  server (pairs with VayuPress ADR-0129). Approved devices sync
  immediately with a per-device password; when approval is required, the
  connect card waits with clear guidance ("open your VayuPress webmail →
  Mail accounts → Devices"), polling every 5 seconds for up to 10
  minutes and cancellable at any point. Servers without the endpoint get
  exactly the previous connect behavior — nothing changes for older
  VayuPress installs. The granted device ID is kept per account for
  future display.

### Security
- Each install now authenticates to IMAP/SMTP with its own approved
  device password instead of the shared mailbox password (on servers
  that enforce device approval), so a lost phone is revoked from the
  web console without rotating the mailbox credential. The device
  endpoints are called with the same transport discipline as the
  private-key fetch: HTTPS only, SSRF domain guard, refused redirects,
  size-capped responses. The device password lives only in the platform
  keystore as the account credential; SQLite stores just the public
  device ID (Rule 6).

## [2.1.5] — 2026-07-11

### Fixed
- **PGP/MIME encrypted mail no longer shows "Version: 1".** Mail sent by
  the app's own composer (RFC 3156 multipart/encrypted) was displayed as
  the structure's control part instead of being decrypted: the capture
  matched the first part whose type mentioned "pgp", which is the
  `application/pgp-encrypted` version marker, not the ciphertext. Only a
  part carrying a real `-----BEGIN PGP MESSAGE-----` armor block is now
  accepted, and the control part is dropped from the attachment list.
- **Broken encrypted messages repair themselves.** Encrypted mail cached
  by an earlier version (with "Version: 1" or an empty body stored) is
  automatically re-downloaded from the server the first time you open it,
  then decrypted — no need to clear data or re-add the account. While the
  re-download runs the message shows a short "fetching the locked copy"
  notice instead of raw structure text.
- **Pull-to-refresh now visibly refreshes.** A manual sync that found
  nothing new emitted no events, so the list never reloaded and the
  spinner never appeared — the swipe looked dead even though the sync
  ran. The engine now brackets every user-requested sync with
  started/finished events: the indicator spins for the whole sync and the
  list reloads when it completes, whether or not new mail arrived.

## [2.1.4] — 2026-07-11

### Fixed
- **Encrypted mail is now readable.** Received PGP mail (VayuPress sends
  it inline as an armored `-----BEGIN PGP MESSAGE-----` body) previously
  showed a blank message with no detail. The parser now lifts the armored
  ciphertext out of both inline-PGP and PGP/MIME messages, keeps it
  on-disk as ciphertext (never decrypted at rest), and the thread view
  decrypts it in memory when you open it. When your private key isn't on
  the device yet, the message shows a clear notice pointing to
  Settings → Encryption instead of a blank body.
- **Pull-to-refresh now works.** The gesture was being swallowed by the
  list's own scroll handler, which claims a touch-drag after only 3dp.
  The refresh control now registers a pass-through observer above the
  list and claims the pointer the instant a downward drag begins at the
  top of the list, so a swipe-down reliably triggers a sync while normal
  scrolling, row taps, and swipe actions are untouched.

### Added
- **Your private key syncs from VayuPress.** A new
  "Sync my key from VayuPress" action (Settings → Encryption) fetches
  this mailbox's own PGP private key from your VayuPress server over TLS,
  authenticated with the account credential you already hold, so encrypted
  mail opens on this device. It also runs automatically the first time you
  connect an account. Servers that don't serve a key are ignored silently.

## [2.1.3] — 2026-07-11

### Changed
- **Security is now the first section in Settings**, and **Two-factor
  unlock is always listed** — previously it appeared only after App lock
  was turned on, so it read as missing. When App lock is off, the
  Two-factor row shows a hint ("Turn on App lock first") instead of
  being hidden. The App lock subtitle now names the second factor too.
  All of this makes the 2.1.x security additions visible the moment you
  open Settings, without scrolling or first setting a PIN.

## [2.1.2] — 2026-07-11

### Added
- **The version is now shown on the login screen.** Every feature past
  onboarding (2FA, message details, pull-to-refresh, in-app password
  change, the encryption flow) only appears after you connect an
  account — the connect screen itself only carries the logo and the
  two fields. A small "VayuMail vX.Y.Z" now sits at the bottom of that
  screen so the installed build is identifiable before signing in.

## [2.1.1] — 2026-07-11

### Fixed
- **Sideload updates now install over the previous build.** Each test
  build was signed with a freshly generated key, and Android refuses to
  update an app when the signature changes — so downloading a newer APK
  appeared to do nothing (the update silently failed and the old version
  stayed). Test builds now sign with one committed debug key, so every
  build updates in place. (First move to this key still needs a one-time
  uninstall of any earlier build; after that, updates are seamless.)

### Changed
- **Every release now ships a Play Store bundle too.** The release
  workflow builds both `vayumail-<version>.apk` (sideload) and
  `vayumail-<version>.aab` (Google Play Console) from the same signed
  source and attaches both to the GitHub Release — so each version's APK
  and AAB always update together.

## [2.1.0] — 2026-07-11

Encryption that just works, a second unlock factor, and the polish
round from the first on-device testing.

### Added
- **Two-factor unlock (TOTP).** The app lock gains an optional RFC 6238
  second factor: enroll the same base32 authenticator secret your
  VayuPress 2FA uses (Settings → Security → Two-factor unlock), and
  unlocking asks PIN, then the 6-digit code — auto-submitting on the
  sixth digit. The secret lives in the keystore next to the PIN
  verifier, never in SQLite; wrong codes feed the same doubling lockout
  ladder as wrong PINs; the HOTP core is tested against the published
  RFC 4226 vectors. Enrollment is atomic — a mistyped secret can never
  lock you out, because a failed confirmation code removes it again.
- **In-app password update.** Each account row in Settings gained
  **Password**: type the new password or app password, Save, and the
  engine stops sync, overwrites the keystore entry in place, and
  reconnects — no more sign-out/sign-in dance after a server-side
  password change. Clears the sign-in-failed banner on success.
- **Message details, Gmail-style.** Tap any message header in a
  conversation to unfold the full record: From/To/Cc, exact date, a
  security line that tells the truth ("PGP end-to-end encrypted (+
  transport TLS)" vs "Transport TLS only"), tracking honesty
  ("Tracking pixels detected — blocked"), and size.
- **Pull-to-refresh.** Drag down from the top of the inbox: a
  rubber-banded indicator follows your finger, arms past the threshold,
  and spins while the sync runs. Implemented as a passive gesture
  observer, so row taps and swipe-to-archive/delete are untouched.
- **Notification preview toggle.** Settings → Sync & notifications →
  "Show message preview": off replaces sender/subject with a generic
  "New mail" line — nothing sensitive on the lock screen.
- **`make icon`** regenerates the launcher icon from the committed
  brand artwork (tools/appicon).

### Changed
- **Encryption no longer says no — it goes and gets the key.** Turning
  the shield on immediately fetches missing recipient keys from each
  recipient's own server (WKD), with a live "N recipient(s) missing a
  key" readout in the compose bar; if a key genuinely isn't published,
  the send is held with a message naming exactly who. Outbound
  encrypted mail now also **encrypts to your own key**, so your Sent
  copy stays readable — and every connected account's own key leads the
  WKD sweep, fetched automatically from your VayuPress server.
- **The connect screen now shows the VayuMail logo** — the real
  artwork, theme-aware (dark art in dark mode), replacing the text
  wordmark.
- **The launcher icon is full-bleed**: the V mark large on the brand
  deep-navy field, edge to edge, so it fills the launcher tile instead
  of floating in a white box.
- Every top-bar icon button now shows a pressed halo — taps answer
  instantly everywhere.

### Security
- The TOTP secret is keystore-resident (Rule 6), verification is
  constant-time across all accepted time windows, and both unlock
  factors share one persistent brute-force throttle (ADR-0010,
  amended).
- Disabling two-factor requires a current code; disabling the app lock
  removes both factors together.

## [2.0.0] — 2026-07-10

The enterprise redesign: a new design language, an app lock, real
sign-out, one-tap onboarding against any VayuPress server — and the
camera gone for good.

### Added
- **Direct connect onboarding.** Type your email and an app password;
  the app discovers your server's settings from its signed-over-HTTPS
  autoconfig document (`/.well-known/vayumail/autoconfig.json`, served
  by VayuPress and contract-tested in both repos) and connects. Signed
  setup codes (the ADR-0003 payload, pasted) and manual entry remain as
  fallbacks. VayuPress operators mint app passwords in the new VayuOS
  console card (VayuPress ADR-0126).
- **App lock.** Optional 4–12 digit PIN gating the whole UI: stdlib
  PBKDF2-SHA-256 verifier stored in the keystore (never SQLite, never
  plaintext — Rule 6), constant-time comparison, five free attempts
  then a doubling lockout capped at 15 minutes, idle auto-lock
  (30 seconds / 1 / 5 / 15 minutes, default 1 minute) driven by
  frame-gap detection, and
  a lock screen with an animated PIN pad and shake-on-error. While
  locked, no mail pixels render — app switchers screenshot nothing.
  (ADR-0010)
- **Sign out.** Per-account sign-out from Settings (with confirm
  dialog): the engine stops the account's sync goroutines with a
  bounded wait, wipes its credential from the keystore, and removes its
  local mail; `AccountRemovedEvent` closes the loop. Removing the last
  account returns to onboarding. Goroutine-leak-tested.
- **Account switcher.** The drawer gained an identity header: switch
  between accounts with two taps, or add another account from the
  drawer and Settings.
- **Thread action bar.** Reply, **Forward** (quoted plain text),
  Archive, and Delete (with confirmation) — thumb-reachable at the
  bottom of every conversation.
- **Notification toggle.** New-mail notifications can be switched off
  in Settings; the setting gates the notifier before it posts.
- **Motion system.** A new `ui/anim` package: press-scale buttons and
  FAB with gradient fills, staggered message-list entrance, parallax
  screen transitions with dim, animated switches, an animated confirm
  dialog, drawer account-switcher reveal, and a spinning sync
  indicator. Every animation is time-interpolated and requests frames
  only while running — an idle screen renders zero frames, so the
  motion system costs no battery at rest.

### Changed
- **New design language.** "Wind at night": deep blue-black surfaces
  with one electric indigo→cyan accent sweep, raised-surface elevation
  with soft shadows, gradient duotone avatars, pill buttons, a refined
  light palette, and richer message rows (unread accent bar,
  subject + snippet preview line, attachment and PGP indicators, folder
  icons and pill badges in the drawer).
- **PGP keys now sync themselves.** Auto-discovery of contacts' keys
  via WKD (their server's key directory — VayuPress publishes one for
  every mailbox) is **on by default**, throttled to one sweep per 10
  minutes; composing to key-holding recipients still auto-encrypts.
  Turn it off in Settings → Encryption.
- **Settings rebuilt** around sections: Accounts (sync now / sign out /
  add), Security (app lock, change PIN, auto-lock window, lock now),
  Sync & notifications (sync-all, notification toggle), Encryption
  (auto-fetch toggle, keys, tools, VayuPress key directory),
  Appearance, About.
- Auth failures now surface as an inline banner on the inbox instead
  of being silently recorded.
- `cmd/vayumail-provision` serves setup codes at `GET /code` (legacy
  `/qr` path kept, PNG endpoint removed).

### Removed
- **QR scanning, the camera, and the CAMERA permission** (ADR-0009,
  Constitution v1.2). VayuPress removed QR generation in v3.9.16; the
  scanner was the app's only camera use and its heaviest platform
  surface (device-only cgo bridge, un-CI-testable). The Ed25519
  provisioning protocol itself is unchanged — payloads arrive as
  pasted setup codes through the identical verifier and all rejection
  fixtures. Dependencies dropped: `gozxing`, `golang.org/x/xerrors`.

### Security
- The app-lock verifier design (PBKDF2 600k iterations, per-verifier
  random salt, keystore residency, lockout ladder) is documented in
  ADR-0010; a test scans the data directory to prove the literal PIN
  never touches disk.
- Onboarding no longer trusts screen-borne payloads by default: direct
  connect authenticates the channel with WebPKI before any credential
  leaves the device.
- Pinned the Go toolchain to 1.25.12, which fixes GO-2026-5856
  (Encrypted Client Hello privacy leak in `crypto/tls`) reached through
  every IMAP/SMTP/HTTPS handshake; picked up the dependency security
  bumps from main (`golang.org/x/net` 0.57.0, `golang.org/x/crypto`
  0.54.0, `gioui.org/x` 0.10.1). `govulncheck` is clean.

## [1.5.0] — 2026-07-06

### Added
- **Attachments in the composer — real file picking.** The compose attach button
  was a stub that showed "arrive in a later release"; it now opens the platform
  file picker (Android Storage Access Framework, native dialogs elsewhere) via
  `gioui.org/x/explorer`. Chosen files appear as chips above the send bar with
  name and size — **tap a chip to remove it** — and are sent through the
  composer's existing MIME attachment path (which already supported
  `multipart/mixed`). Each file is read up to a generous **50 MB** cap (matching
  the VayuPress server default). The picker is wired to the window event loop so
  it can observe the Android activity result. On platforms with no picker the
  button reports that cleanly instead of pretending. (Android/iOS file picking is
  verifiable only on-device; where the OS content stream carries no filename, a
  type-derived name is used.)

## [1.4.3] — 2026-07-06

### Fixed
- **QR scanner now shows the live camera preview.** The camera was opening and
  streaming (permission granted), but the scanner only fed frames to the decoder
  and never drew them — so the screen stayed black and looked broken. The most
  recent frame is now painted to fill the surface behind the framing overlay, so
  you can see what the camera sees and aim at the code. (Preview is drawn in the
  sensor's orientation; QR decoding is rotation-tolerant, so a code still reads
  even if the image looks turned — an orientation-correct transform can follow.)
- **Camera frames are now copied per frame** so the live GPU preview cannot tear
  or freeze on a stale texture from the shared decode buffer.

### Changed
- **QR decoding is throttled to ~8×/second instead of every rendered frame.**
  Decoding a full frame at 60fps needlessly pegged the CPU (lag + heat); the
  scanner still catches a code the instant it is framed but the UI stays smooth.

## [1.4.2] — 2026-07-06

### Fixed
- **CAMERA permission is now actually in the APK — the scanner can finally get
  the camera.** The manifest never declared `android.permission.CAMERA`, so
  Android showed **no camera permission to grant and no request dialog could ever
  appear** (the app's only listed capability was network access). Root cause:
  gogio only adds a permission when the app imports the matching
  `gioui.org/app/permission/*` package, and the camera one was never imported.
  Added a blank import of `gioui.org/app/permission/camera`, which injects
  `<uses-permission android:name="android.permission.CAMERA"/>` and the camera
  hardware feature. After installing this build the Camera permission appears
  under Settings → Apps → VayuMail → Permissions — grant it and the QR scanner
  works. (Gio's app context is the Application, not an Activity, so the app
  cannot pop the request dialog itself yet; granting once in Settings is the
  path until an Activity-based request is added.)

## [1.4.1] — 2026-07-06

### Changed
- **Camera bridge now logs why it fails to start.** The NDK Camera2 bridge emits
  diagnostic lines under the `vayumail-camera` logcat tag: the CAMERA permission
  decision (granted / not granted / whether a request dialog can even be shown),
  and, on failure to open, the exact step and status code (e.g. an
  `ACAMERA_ERROR_PERMISSION_DENIED` at `openCamera`). This turns a silent black
  preview into an actionable log. Inspect on-device with
  `adb logcat -s vayumail-camera`. In particular, if the app context is not an
  Activity the bridge cannot pop the permission dialog and says so — grant Camera
  once in Settings → Apps → VayuMail → Permissions and scanning will work.

## [1.4.0] — 2026-07-05

Live camera QR scanning on Android — the onboarding scanner now sees through the
phone's camera instead of only accepting a pasted setup code.

### Added
- **Android camera QR scanning — NDK Camera2 bridge.** The QR scanner's
  `widgets.FrameSource` is now backed by a real camera feed on Android via a new
  `internal/camera` package. The Android implementation
  (`camera_android.go`) is a **pure-cgo bridge over the NDK Camera2 API** — no
  Java or Kotlin source, so it builds with the existing `make android` (gogio)
  flow and adds no new manifest permission (CAMERA is already declared,
  ADR-0005). It opens the back camera, streams `YUV_420_888`, and hands the
  **luminance (Y) plane** straight to the gozxing decoder (QR decoding needs only
  luminance, so no YUV→RGB conversion). The camera powers on lazily when the
  scanner opens and is released automatically after a short idle period, so
  leaving the scan screen frees the device with no extra wiring. Runtime CAMERA
  permission is checked and requested over JNI on scanner open (ADR-0005); until
  it is granted — or on any camera error — the scanner cleanly falls back to
  "Paste setup code", so the app is never left in a broken state.

### Notes
- The camera bridge is compiled only by the Android toolchain and can only be
  exercised on a physical device; the desktop/CI build uses the no-op source
  (`camera_other.go`) via build tags, so the scanner UI, decode pipeline, and
  payload verification remain fully covered by the existing tests. iOS camera
  capture remains pending (COMPLIANCE-TRACKER.md: "Camera preview bridge").

## [1.3.0] — 2026-07-04

Email-only onboarding via autoconfig discovery, plus PGP/WKD interop hardening
(address-verified key discovery, SSRF-safe discovery) and the shared WKD
contract with the VayuPress server.

### Added
- **Onboard by email — VayuMail autoconfig discovery.** The manual account
  screen gains an **"Auto-detect from email"** button: enter your address, tap
  it, and the server/IMAP-port/SMTP-port fields fill themselves from the
  domain's published autoconfig — you only add your password. Backed by new
  `account.DiscoverAutoconfig`, which fetches the server's first-party document
  at `https://<domain>/.well-known/vayumail/autoconfig.json` (with an
  `autoconfig.<domain>` fallback) and returns an account `Config` prefilled with
  the IMAP/SMTP hosts, ports, TLS modes and username — so a VayuPress mailbox can
  be set up from just an email address, alongside the existing QR and paste-code
  onboarding. The lookup runs off the UI thread and applies its result on the
  next frame (no cross-thread widget mutation). User-initiated only (no
  phone-home); SSRF-guarded (public domains only, no IP literals/loopback),
  size-capped, and it rejects any document whose `schema` it does not recognise.
  The document shape is locked to the VayuPress server by a shared contract test
  (`test/autoconfig_contract_test.go`). Discovery also honours the server's
  declared `auth` field, mapping it to the account's `AuthMech`: `password`
  (VayuPress's default) stays password auth, while `oauthbearer` / `xoauth2`
  select the matching bearer-token mechanism, so a token-minting server is
  configured from discovery alone; an unrecognised value is rejected rather than
  silently mis-configuring the account.

### Security
- **WKD key discovery verifies the address matches.** `DiscoverWKD` now imports
  a fetched key only if it carries a User ID for the exact address that was
  requested. A misconfigured or hostile WKD endpoint could otherwise return a
  key for a *different* identity, which would be silently mis-associated with the
  contact; such a mismatch is now rejected and discovery falls through to the
  next candidate URL (or fails) instead of trusting the wrong key.
- **Autoconfig discovery is hardened against SSRF (CWE-918).** The discovery
  fetch no longer follows HTTP redirects, so a mail domain that 3xx-redirects to
  a private, loopback or cloud-metadata address can never be chased. The domain
  guard now requires a clean multi-label DNS hostname (rejecting IP literals,
  `localhost`, and any value carrying a port or path such as `evil.com:9999`),
  so a crafted address cannot inject a port or path into the lookup URL.

- **WKD interop contract test (shared with VayuPress).** Froze an expanded
  set of WKD address-hash known-answer vectors (`test/wkd_contract_test.go`)
  that is kept byte-for-byte identical to the matching table in the VayuPress
  server. The app computes the lookup hash and VayuPress computes the publish
  hash with two independent z-base-32/SHA-1 implementations; the shared table
  makes any drift between them fail CI on whichever side moved, instead of
  silently breaking key discovery (server publishes at a path the app never
  requests). Also pins case-folding of the local part.

## [1.2.7] — 2026-07-03

### Added
- **Auto-fetch keys on new mail (opt-in).** Settings → PGP → "Auto-fetch
  keys on new mail (WKD)". When on, new mail triggers a throttled WKD
  sweep (at most once per 10 minutes) that imports keys for any
  correspondent still missing one — so VayuPress contacts' keys stay
  current with no taps. Off by default and user-enabled, honouring the
  no-phone-home rule; the setting persists (`auto_wkd`).

## [1.2.6] — 2026-07-03

### Added
- **One-tap contact key discovery via WKD.** Settings → PGP →
  "Fetch contacts’ keys (WKD)" looks up a public key for every address you
  correspond with through Web Key Directory and imports the ones it finds.
  Because VayuPress publishes its users' keys over WKD, this
  pulls your VayuPress contacts' keys with no separate key server and no
  VayuPress change — the two systems interoperate over the open WKD
  standard. User-initiated, per the no-phone-home rule.
  - New `store.CorrespondentEmails` (distinct senders, newest first);
    reuses the existing WKD client.

## [1.2.5] — 2026-07-03

### Added
- **Token-based authentication (modern auth / 2FA).** Accounts can now log
  in with a bearer token instead of a mail password, via SASL
  **OAUTHBEARER** (RFC 7628) or **XOAUTH2** (Google/Microsoft style), for
  both IMAP and SMTP. The mechanism is stored per account (`auth_mech`,
  schema v4); the token itself stays in the platform keystore (Rule 6).
  When a provisioned account returns an OAuth token, VayuMail selects the
  token mechanism automatically (XOAUTH2 when the token type says so, else
  OAUTHBEARER). Interactive 2FA is not part of IMAP/SMTP — this is the
  standards path a 2FA-protected account uses to authenticate.
  - New `internal/mail/account/oauth.go` with a tested XOAUTH2 SASL client.

## [1.2.4] — 2026-07-03

### Added
- **Paste setup code onboarding.** When the camera is unavailable, you can
  now add an account by pasting the QR's setup code (its base64url
  payload, served by the provisioning tool's text endpoint). It runs the
  identical Ed25519-verified provisioning path as a live scan — no field
  of an unverified payload is ever used. The scanner screen and the
  welcome screen both point to this path.

## [1.2.3] — 2026-07-03

### Added
- **PGP key sync from VayuPress.** A new Settings section, "PGP key
  directory (VayuPress)", lets you point VayuMail at a key-directory URL
  on your own site and pull every contact's public key in one tap ("Sync
  keys"). Keys are imported into the keyring and persisted. The client is
  HTTPS-only and never contacts a directory you have not configured.
  - Per-address form: `GET {url}?email={addr}` → armored/binary public key.
  - Bulk form: `GET {url}` → `{"keys":[{"email","armored"}]}`.
  - New `internal/mail/pgp/keydir.go`, `settings` table (schema v3),
    end-to-end tests over an HTTPS test server.

## [1.2.2] — 2026-07-03

### Fixed
- **Tapping a message did nothing.** The row's swipe gesture sat above its
  Clickable and swallowed taps, so messages never opened. The swipeable
  now recognises a press-and-release-in-place as a tap and opens the
  conversation; horizontal drags still archive/delete.
- **Sent mail (and other folders) not showing.** "Sync now" only synced
  INBOX, so messages sent from another client or filed server-side never
  appeared. It now discovers and syncs every folder. Opening a folder
  (Sent, Archive, …) also triggers a one-shot sync of just that folder.

### Added
- **Refresh button** in the inbox toolbar — fetches new mail across all
  folders on demand (`SyncFolderCmd` / `SyncNow`).

## [1.2.1] — 2026-07-03

### Fixed
- **First-launch "disk I/O error" crash.** On Android the app failed at
  startup with `store: migrate: apply migration v2: disk I/O error
  (6410)`. Extended code 6410 is `SQLITE_IOERR_GETTEMPPATH`: the
  migration-v2 FTS5 index rebuild needed an on-disk temp directory, which
  Android does not expose to SQLite. The store now opens with
  `temp_store=MEMORY`, keeping transient b-trees in memory so no OS temp
  path is required. The transaction helper also no longer masks the real
  error with a spurious "cannot rollback" message when SQLite has already
  auto-aborted the transaction.
- **Supply-chain vulnerabilities.** Bumped `golang.org/x/image` to v0.41.0
  and `github.com/cloudflare/circl` to v1.6.3, clearing five reachable
  advisories `govulncheck` flagged (TIFF OOM/panic and a secp384r1
  miscalculation).

### Changed
- **Logo — original artwork.** The app now uses the supplied master logo
  PNGs verbatim (`assets/logo/vayumail.png` / `vayumail-dark.png`) instead
  of a redrawn vector. The launcher icon is the mark cropped from that
  master on an opaque white square, and the in-app splash paints the
  embedded master directly (`ui/logo-light.png`). The redrawn SVGs and the
  `tools/genicon` rasterizer were removed.
- **Static splash.** The launch splash shows the logo statically — no
  draw-on or breathing animation.

## [1.2.0] — 2026-07-03

### Fixed
- **Android startup freeze.** The app opened to a frozen brand mark and
  never progressed. Root cause: `app.DataDir()` and the SQLite open ran
  before the window pumped its first frame, and on Android `app.DataDir()`
  blocks until a window event arrives — a startup deadlock. The window
  now presents frames immediately via a boot loop (`ui/boot.go`), and all
  blocking initialization runs off the UI thread with on-screen error
  reporting instead of an eternal splash.
- **Archive/move UID collision.** Moving a second message into the same
  folder failed on the `UNIQUE(account, folder, uid)` constraint (moved
  rows reused `uid = 0`), so the message was never moved server-side. The
  move now deletes the local row after a successful server move and lets
  the next sync re-add it with its real UID; the server move runs first so
  a failure never loses the message locally.
- **HIGHESTMODSEQ reset.** The IDLE delta-sync path reset the stored
  CONDSTORE modseq to zero on every pass; it is now preserved.
- **Provisioning-server token leak.** Unredeemed one-time tokens are now
  pruned on expiry instead of accumulating forever.

### Changed
- **New logo.** Redrawn as a single confident rounded "V" (a wind-drawn
  mark). New `vayumail-icon/​wordmark/​dark` SVGs, an **animated** wordmark
  that draws itself on, a regenerated launcher icon, and an **animated
  in-app splash** that renders the mark live on every launch.

### Added
- End-to-end provisioning tests proving the reference server's signed
  payload verifies with the client's own verifier (canonical JSON parity),
  plus single-use and expiry coverage.
- Regression test for the archive/move UID collision.
- Constitution **v1.1**: mechanical enforcement extended (channel-buffer
  invariants, `math/rand` ban, QR rejection-path completeness, ADR
  cross-reference integrity) and a documented Enforcement map.
- `SECURITY.md` (threat model + disclosure), `CHANGELOG.md`, issue/PR
  templates, `CODEOWNERS`, Dependabot, and `govulncheck` in CI.

## [1.1.0] — 2026-07-02

### Added
- **Intelligence (on-device):** unified "All inboxes" view, tracking-pixel
  detection with a "sender tracks you" indicator, newsletter detection and
  RFC 2369/8058 one-tap unsubscribe, snooze, 10-second undo-send, search
  operators (`from:`/`subject:`/`has:attachment`/`is:unread`/`before:`/
  `after:`) and full-body FTS.
- **PGP UX:** persisted keys, WKD discovery, in-app key management, and
  auto-encrypt when every recipient has a key.
- **Sovereignty:** optional per-account TLS SPKI pinning, encrypted
  multi-device settings sync through the user's own mailbox, and a
  reference provisioning server (`cmd/vayumail-provision`).
- Schema v2, ADR-0007 and ADR-0008, parser fuzzing, coverage and
  binary-size CI gates.

## [1.0.0] — 2026-07-02

### Added
- New-mail system notifications (Android tray / desktop DBus).
- Sealed AES-256-GCM credential store surviving restarts (ADR-0004
  amendment).
- Android hardware back-button navigation.
- Signed-APK release pipeline (GitHub Actions) and launcher icon.

## [0.1.0] — 2026-07-02

### Added
- Initial pure-Go mobile email client: IMAP IDLE sync, SMTP outbox, MIME
  parse/render, OpenPGP, SQLite + FTS5, Ed25519 QR provisioning, hand-
  rolled Gio UI, the ten-rule constitution, ADR-0001…0006, and an offline
  test suite.

[Unreleased]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/johalputt/VayuMail-Mobile/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/johalputt/VayuMail-Mobile/releases/tag/v0.1.0
