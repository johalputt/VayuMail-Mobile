# Release & rollout policy

How VayuMail moves from a green `main` to users' phones, and the rules
that keep a bad build from reaching everyone at once. The mechanics live
in `.github/workflows/release.yml` (Android APK/AAB + Play upload) and
`.github/workflows/release-ios.yml` (IPA).

## Channels

| Channel | Where | Who gets it | Promotion rule |
|---|---|---|---|
| CI (every push to main) | GitHub Checks | nobody — gates only | all required jobs green |
| Internal (Play) | `internal` track, auto-uploaded on tag builds | the team's opted-in testers | ≥ 24 h soak with normal use |
| Closed beta (Play) | `closed` track | external opt-in testers | internal clean; release notes written |
| Production (Play) | `production` track, staged | everyone | closed beta clean; staged per policy below |
| iOS | unsigned/dev-signed IPA on the GitHub Release | sideloaders only until the keystore bridge lands | not store-distributed yet |

## Staged production rollout

Production releases go out in stages, never all-at-once:

1. **10%** — hold ≥ 12 h.
2. **50%** — hold ≥ 12 h.
3. **100%** — only after both earlier holds were quiet.

Halt criteria — any one of these freezes the current stage and halts
promotion until a fix is rolling:

- Crash-free sessions < 99.5% over the stage window.
- ANR rate visibly above the previous release's baseline.
- A cluster of 1-star reviews naming the update within hours.

Halting is a Play-console action (halt staged rollout); it stops NEW
devices from receiving the build but does not recall installed ones, so
the hotfix path below matters.

## Hotfix path

1. Branch from the released tag (`hotfix/vX.Y.Z`).
2. Fix + test; bump **both** `Semantic` and `Code` in
   `internal/version/version.go` — never reuse a version code, Play will
   reject the upload.
3. Tag → release workflow → straight to internal for a smoke pass, then
   production staged from 50% (a hotfix has already passed CI as source).

## Invariants

- **Signature continuity**: every Android build signs with the same
  upload key (repository secret). Losing it means losing the install
  base — back it up outside CI.
- **Version codes are monotonic** and derived from one file; nothing
  derives versions from clocks or git state.
- Release notes ship with every channel promotion; users should be able
  to tell what changed without reading commits.
- The debug/test keystore is for sideload continuity only and is never a
  distribution identity.
