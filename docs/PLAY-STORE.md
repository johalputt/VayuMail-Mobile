# Google Play — listing, declarations and automated publishing

Everything needed to get VayuMail onto Google Play, and to keep it there without
touching the console again after the first release.

Package name: **`com.vayu.mail`** (set in `Makefile` and `release.yml`; it can
never be changed once published). Current version: **2.2.13**, version code
**42** (`internal/version/version.go`).

---

## 0. Read this first — the two things that actually block a launch

**Screenshots must be of the real app.** Play's Store Listing and Metadata
policy requires screenshots to accurately represent the app. Rendered mockups of
a UI that does not exist are a policy violation and a common rejection reason,
so
this document does not supply invented ones — section 3 has the capture
procedure instead. The icon and feature graphic *are* supplied, because those
are
brand assets rather than depictions of the running app.

**A mail client needs reviewer credentials.** VayuMail cannot show anything
until an account is connected, so a reviewer who opens it sees a sign-in screen
and nothing else. Apps that gate their whole UI behind a login and do not supply
working credentials in **App access** are rejected. Create a real, disposable
mailbox before submitting — see section 4.1. This is the single most likely
reason a first submission fails.

---

## 1. Assets

| Asset | Spec | Status |
| --- | --- | --- |
| App icon | 512×512, **32-bit PNG with alpha** | `assets/play/icon-512.png` ✔ |
| Feature graphic | 1024×500 PNG, no alpha | `assets/play/feature-graphic-1024x500.png` |
| Phone screenshots | 2–8, 16:9 or 9:16, 320–3840 px/side | capture — §3 |
| 7-inch tablet | **required**, 2–8, 16:9 or 9:16, 320–3840 px/side | capture — §3 |
| 10-inch tablet | **required**, 2–8, 16:9 or 9:16, **1080**–7680 px/side | capture — §3 |
| Promo video | optional, YouTube URL | skip for v1 |

`cmd/vayumail/appicon.png` is 512×512 but **RGB without alpha**, which Play
rejects. `assets/play/icon-512.png` is the same artwork re-encoded as RGBA —
upload that one.

---

## 2. Store listing fields — copy to paste

### App name (30 characters max)

```text
VayuMail
```

Alternative if you want keywords in the title (24 chars):

```text
VayuMail — Private Email
```

Keep the plain one unless you specifically want the keyword. Play's metadata
policy forbids performance claims, rankings, prices and emoji in the title.

### Short description (80 characters max — this one is 67)

```text
Sovereign email and encrypted chat. No telemetry, no tracking, ever.
```

### Full description (4000 characters max)

```text
VayuMail is a mail client for people who would rather own their mail than rent it.

It connects to YOUR mail server over IMAP and SMTP. There is no VayuMail account, no
VayuMail cloud, and nothing in the middle. Your mail goes from your server to your
device and stops there.

NO TELEMETRY, AND YOU CAN CHECK
There is no analytics SDK, no crash reporter and no advertising identifier in this app.
Nothing is sent anywhere except to the mail server you configured. The source is public,
so this is verifiable rather than a promise.

BUILT-IN ENCRYPTION
PGP is part of the app, not a plugin. VayuMail finds your contacts' public keys
automatically from their own mail servers and encrypts to them without being asked.
Private keys are sealed in the device keystore and never written to the database.

TRACKING PIXELS DO NOT LOAD
Remote content is never fetched automatically. When a message carries a tracking pixel
you are told the sender tracks you, instead of quietly confirming you read it.

SET UP BY TYPING YOUR ADDRESS
Enter your email address and app password. Server names, ports and encryption settings
are discovered for you. No manual IMAP configuration.

MAIL THAT ARRIVES WHEN IT ARRIVES
One held IMAP IDLE connection delivers mail the moment it lands, without the battery
cost of polling every few minutes.

EVERYTHING HAPPENS ON YOUR DEVICE
Threading, unified inbox, newsletter detection, one-tap unsubscribe, snooze, undo send
and full-text search with from:, subject:, has:attachment, is:unread, before: and after:
are all computed locally. Nothing about your mail is uploaded to be indexed.

PRIVATE MESSAGING TOO
VayuTalk is end-to-end encrypted, ephemeral and read-once, running through your own
server rather than a separate service and a separate account.

LOCKED DOWN BY DEFAULT
Credentials never touch disk in plaintext — they live in the platform keystore or a
sealed AES-256-GCM store. An optional PIN lock with idle auto-lock and an offline
brute-force throttle gates the whole app. Optional per-account certificate pinning.
Signing out closes every connection, wipes the credential from the keystore and removes
the local mail.

WHAT YOU NEED
An existing email account with IMAP and SMTP access — your own server, VayuPress, or any
standards-compliant provider. VayuMail does not provide mailboxes.

Open source under Apache-2.0: github.com/johalputt/VayuMail-Mobile
```

### Other listing fields

| Field | Value |
| --- | --- |
| App category | **Communication** |
| Tags | Email, Privacy, Productivity (choose up to 5) |
| Contact email | `ankushchoudharyjohal@gmail.com` |
| Contact website | `https://vayupress.com` |
| Contact phone | optional — leave blank |
| Privacy policy URL | **required** — see 4.2 |
| External marketing | leave off unless you run Play-linked campaigns |

---

## 3. Screenshots

`scripts/screenshots.sh` captures **real** ones by running the actual binary
under a virtual display — no mockups, so nothing here risks the Metadata policy.
It is also why the standing note about Gio being unbuildable in a sandbox
is out of date: with `libxkbcommon`, `libx11-xcb`, `libvulkan` and the Mesa dev
packages installed, `go build ./cmd/vayumail` succeeds and the app runs headless
under Xvfb.

```bash
bash scripts/screenshots.sh
```

Nine captured and committed — three per form factor:

| File | Size | Screen |
| --- | --- | --- |
| `phone-1-signin.png` | 1080×1920 | sign-in |
| `phone-2-setupcode.png` | 1080×1920 | setup code |
| `phone-3-manual.png` | 1080×1920 | manual setup |
| `tablet7-1-signin.png` | 1080×1920 | sign-in |
| `tablet7-2-setupcode.png` | 1080×1920 | setup code |
| `tablet7-3-manual.png` | 1080×1920 | manual setup |
| `tablet10-1-signin.png` | 1920×1080 | sign-in |
| `tablet10-2-setupcode.png` | 1920×1080 | setup code |
| `tablet10-3-manual.png` | 1920×1080 | manual setup |

All under `assets/play/screenshots/`. Upload each trio to its matching slot.

### Why these exact sizes

Play enforces the ratio to the pixel and applies a per-slot floor:

| Slot | Ratio | Each side |
| --- | --- | --- |
| Phone | 16:9 or 9:16 | 320–3840 px |
| 7-inch | 16:9 or 9:16 | 320–3840 px |
| 10-inch | 16:9 or 9:16 | **1080**–7680 px |

Real device dimensions do not satisfy this — a Pixel is 412×915, which is
neither 16:9 nor 9:16 — so the capture sizes are exact 9:16 / 16:9 pairs
instead. Everything ships at ≥1080 px per side, which the 10-inch slot demands
outright and which is also Play's bar for a listing to be **eligible for
promotion**.

Scaling has to happen after capture. Gio takes its UI scale solely from the
`Xft.dpi` X resource (`app/os_x11.go`: `scale = Xft.dpi/96`), and setting
`RESOURCE_MANAGER` on this Xvfb leaves the render unchanged — so 1 dp is 1 px
and the window size *is* the logical size. Asking for a 1080-wide window does
not enlarge the UI, it strands a phone-width form in a field of whitespace. The
script therefore captures small and upsamples with Lanczos.

All three screens are pre-login, so no account was needed and no real address
appears in any of them. That is also their limit — they show onboarding, not
mail. Add inbox and message shots when you have a demo mailbox; the store
listing is stronger for showing what the app does with mail in it.

**Everything past sign-in needs an account.** The app shows a login and nothing
else until one is connected, so:

```bash
VAYUMAIL_DEMO_EMAIL=playreview@johal.in \
VAYUMAIL_DEMO_PASSWORD=… \
  bash scripts/screenshots.sh
```

Use the same throwaway mailbox you give Play under **App access** (§4.1) — one
account then covers both the reviewer and the store images. Check every capture
before uploading: a screenshot showing a real address is both a privacy problem
and a rejection risk.

### Capturing on a real device instead

Play wants 2–8 phone screenshots; supply **6**, in this order. The first two are
what most people see, so lead with the thing that distinguishes the app.

1. Inbox — unified, a few threads, clean
2. A message showing the **tracking-pixel warning**
3. A message showing **PGP encrypted / signed** state
4. Compose, with the encrypt indicator visible
5. Search with an operator typed (`from:` or `has:attachment`)
6. Settings showing **app lock** and account security

```bash
# Sideload the APK from the latest GitHub Release, then:
adb shell screencap -p /sdcard/s1.png && adb pull /sdcard/s1.png
```

This is the better route for the screens that show real mail, because the
Android build is what users actually run.

### Then frame them

Raw captures are accepted as-is. If you want the framed-with-caption style,
`assets/play/frame.html` (section 7) takes a raw capture and renders a 1080×1920
Play-ready image with a caption. Whatever you do, the app content inside the
frame must stay the real screenshot.

---

## 4. App content — the declarations that gate publishing

All under **Policy → App content**. Every one must be complete before the
Production track will accept a release.

### 4.1 App access — DO NOT SKIP

Choose **"All or some functionality is restricted"** and add:

| Field | Value |
| --- | --- |
| Name | Mail account sign-in |
| Username | a real mailbox you create, e.g. `playreview@johal.in` |
| Password | its app password |
| Any other instructions | see the note below this table |

For **Any other instructions**, paste:

```text
Enter the address and app password on the sign-in screen. Server settings
are discovered automatically. The account has a few sample messages,
including one with a tracking pixel and one PGP-encrypted.
```

Create that mailbox in VayuPress → VayuMail, seed it with a handful of harmless
messages, and keep it alive. If it stops working a future update gets rejected.

### 4.2 Privacy policy

Required, and it must be a public URL that loads without a login. Publish one at
`https://vayupress.com/vayumail-privacy` covering: what is collected (nothing),
what is stored on device, what leaves the device (only traffic to the user's own
mail server), and how to delete it (sign out / uninstall).

### 4.3 Ads

**No**, this app contains no ads.

### 4.4 Content rating

Complete the questionnaire. For a mail client with user-to-user communication:

| Question | Answer |
| --- | --- |
| Category | Utility, Productivity, Communication, Other |
| Violence, sexuality, language, controlled substances | No to all |
| Does the app allow users to interact or exchange content? | **Yes** |
| Can users communicate with users they do not know? | **Yes** (email) |
| Does the app share the user's current location? | No |
| Does the app allow purchase of digital goods? | No |
| Does the app contain user-generated content? | **Yes** — email is UGC |

Expect **PEGI 3 / ESRB Everyone / IARC 3+** with an interactive-elements notice.

### 4.5 Target audience

**18 and over.** Do not tick any child age band — a mail client that can receive
anything from anyone should not be in the Designed for Families programme, and
saying otherwise triggers a much stricter review.

### 4.6 Data safety

This is the section most likely to be filled in wrongly. Answer it from what the
code actually does.

**Does your app collect or share any of the required user data types?**
→ **No.**

That answer is correct here because Play defines "collect" as transmitting data
off the device to a server *you or a third party* control. VayuMail transmits
mail only to the user's own mail server, which Play treats as the user's own
service, not collection by you. There is no analytics, no crash reporting and no
advertising ID in the build.

Then declare:

| Question | Answer |
| --- | --- |
| Is all user data encrypted in transit? | **Yes** — IMAP/SMTP over TLS |
| Way to request data deletion? | **Yes** — see note below |
| Independently security-reviewed? | **No** (do not claim otherwise) |

**Data deletion**: signing out wipes the credential from the keystore and removes
the local mail; uninstalling removes everything. Say exactly that.

If you later add crash reporting or any analytics, this section must change on
the same release. A stale data-safety declaration is a policy violation on its
own, independent of what the code does.

### 4.7 Remaining declarations

| Section | Answer |
| --- | --- |
| Government apps | No |
| Financial features | None of these |
| Health apps | No |
| News apps | No |
| COVID-19 contact tracing | No |
| Data deletion | Point at the same privacy policy URL |

---

## 5. First release — must be manual

Google requires the **first** app bundle for a new package to be uploaded
through the console. The Publishing API can only take over once the package
exists and has a release history. So:

1. Play Console → **Create app** → name `VayuMail`, English (UK), App, Free.
2. Complete every **App content** section from section 4.
3. Fill the **Store listing** from section 2 and upload the assets.
4. **Test and release → Testing → Internal testing → Create new release.**
5. Upload `vayumail-2.2.13.aab` from the GitHub Release.
6. Roll out to internal testing, install it yourself, confirm it runs.
7. Promote to Production when you are satisfied.

### Signing — get this right once

Keep **Play App Signing** enabled (the default). You upload an AAB signed with
your *upload key*; Google re-signs with the *app signing key*.

The upload key is the one in your `ANDROID_KEYSTORE_B64` /
`ANDROID_KEYSTORE_PASS` repository secrets. **Back that keystore up somewhere
off GitHub.** If you lose it you can request a reset from Google, but until that
completes you cannot ship an update at all.

---

## 6. Automated publishing — already built

The upload step is already in `.github/workflows/release.yml` and the setup is
documented in **[PLAY-PUBLISHING.md](PLAY-PUBLISHING.md)**. Nothing in this
document duplicates it. In short: set the `PLAY_SERVICE_ACCOUNT_JSON` secret,
and every `v*` tag uploads its AAB to the **internal** track.

Two guards in that workflow are worth knowing about, because they are what makes
it safe to leave switched on:

- It runs **only when the build was signed with the real upload key**. A
  test-key build can never reach Play, so a run with the keystore secrets
  missing
  degrades to a GitHub-only release instead of pushing something Play would
  reject.
- It runs **after** the GitHub release is published, so a Play outage or a
  rejected upload never costs you the release or its artifacts.

The track defaults to `internal` deliberately. Promotion to production stays a
deliberate act in the console, because "every tag goes straight to every user"
is
how a bad build reaches everyone at once. Set the `PLAY_TRACK` repository
variable to `production` when you want that.

## 7. Files in this repo

| Path | What it is |
| --- | --- |
| `assets/play/icon-512.png` | Play-spec app icon (512×512 RGBA) |
| `assets/play/feature-graphic-1024x500.png` | Feature graphic |
| `assets/play/frame.html` | Screenshot framing template — see §3 |
| `.github/workflows/release.yml` | Builds APK + AAB; uploads to Play |

---

## 8. Checklist before hitting Submit

- [ ] Reviewer mailbox created, seeded, and its credentials in **App access**
- [ ] Privacy policy live at a URL that loads without a login
- [ ] 9 screenshots captured from the real app, no real addresses visible
- [ ] every screenshot exactly 16:9 or 9:16 and ≥1080 px on each side
- [ ] Icon uploaded from `assets/play/icon-512.png` (the RGBA one)
- [ ] Data safety completed and matching what the build actually does
- [ ] Content rating questionnaire submitted
- [ ] Target audience set to 18+
- [ ] Upload keystore backed up somewhere other than GitHub
- [ ] Internal testing release installed and opened on a real device
