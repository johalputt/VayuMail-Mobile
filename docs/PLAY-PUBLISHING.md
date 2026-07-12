# Automated Google Play publishing

Every VayuMail release (`git` tag `v*`, or a manual **Run workflow** on
`release.yml`) builds a signed AAB and, once the one-time setup below is done,
**uploads it to Google Play automatically**. After that a release on GitHub is a
release on Play with no manual step — and Play then updates the app on every
user's device in the background, so there is nothing to sideload.

The upload step is **gated**: it runs only when the `PLAY_SERVICE_ACCOUNT_JSON`
secret is present *and* the build used the real upload keystore. Until then it is
a no-op, so nothing breaks.

## One-time setup (≈20 minutes, Google side)

You do these once in your own Google accounts — they can't be automated for you.

### 1. Create the app in Play Console and upload the first build MANUALLY
Google requires the **very first** AAB for a new app to be uploaded by hand
through the Play Console UI (Play API uploads are rejected until an app exists
with at least one release and a completed listing).

- Play Console → **Create app** → package name `com.vayu.mail`.
- Complete the store listing, content rating, data safety, and privacy policy
  (privacy policy is already hosted at <https://vayupress.com/vayumail/privacy>).
- Upload `vayumail-<version>.aab` (from a GitHub release) to the **Internal
  testing** track and roll it out once, manually.
- Keep **Play App Signing** enabled (recommended): you upload with your *upload*
  key and Google re-signs with the *app signing* key. The release workflow
  already signs the AAB with your upload keystore.

### 2. Create a service account with Play API access
- Google Cloud Console → **IAM & Admin → Service Accounts → Create** (any
  project). Name it e.g. `play-publisher`.
- Create a **JSON key** for it and download the file (this is the secret).
- Google Play Console → **Users & permissions → Invite new user** → paste the
  service account's email (`...@...iam.gserviceaccount.com`).
- Grant it, for **this app**, at least: *Release to testing tracks* (and
  *Release to production* if you'll push straight to production). Save.
- Make sure the **Google Play Android Developer API** is enabled for the project
  (Cloud Console → APIs & Services → Enable APIs → search for it).

### 3. Add the secret (and, optionally, the track) to GitHub
In the `VayuMail-Mobile` repo → **Settings → Secrets and variables → Actions**:

- **Secret** `PLAY_SERVICE_ACCOUNT_JSON` = the *entire contents* of the JSON key
  file from step 2.
- (Also required for a Play-signable build, if not already set:
  `ANDROID_KEYSTORE_B64` and `ANDROID_KEYSTORE_PASS` — your upload keystore,
  base64-encoded, and its password.)
- **Variable** (optional) `PLAY_TRACK` = `internal` (default), `alpha`, `beta`,
  or `production`. Leave unset to use `internal` while iterating.

That's it. The next release uploads to Play automatically.

## What each track means

| Track        | Who sees it            | Review        | Speed        |
| ------------ | ---------------------- | ------------- | ------------ |
| `internal`   | up to 100 testers you invite | none    | minutes      |
| `alpha`/`beta` | your test groups     | light         | minutes–hours|
| `production` | everyone on Play       | full review   | hours–days   |

Recommended while iterating: **`internal`** — near-instant, no review, and your
testers get automatic background updates. Promote a build to production from the
Console (one click) or set `PLAY_TRACK=production` when you're ready for the
public, accepting Google's review time.

## Rules the automation still obeys (Google's, not ours)

- **`versionCode` must increase every upload.** It comes from
  `internal/version/version.go` (`Code`); bump it each release (we already do).
- **Same signing** — Play App Signing handles this; keep the upload keystore
  stable.
- **`targetSdk`** must meet Play's current minimum (we build `-targetsdk 35`).
- Policy/content checks apply to production; internal testing is exempt from the
  full review.

## Effort recap

- **Workflow automation:** done — the upload step is in `release.yml`.
- **Your one-time setup:** steps 1–3 above (≈20 min, once).
- **Per release after that:** nothing — `git` release → Play upload → automatic
  device updates.
