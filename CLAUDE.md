# CLAUDE.md — standing instructions for every session in this repo

Auto-loaded at the start of every Claude Code session in VayuMail-Mobile.
Follow it without being asked.

## 1. Releases & versioning

- **Roll-at-99 semver, micro by default.** `internal/version/version.go` holds
  `Semantic` (human `X.Y.Z`) and `Code` (monotonic integer, bumped every
  release). Each segment counts **0–99** and never exceeds 99: normally bump the
  micro (`2.2.12 → 2.2.13`); at micro **99** bump minor and reset micro to 0
  (`2.2.99 → 2.3.0`); at minor **99** + micro rollover bump major
  (`2.99.99 → 3.0.0`). Never bump minor/major early — only the 99 rollover does.
- **A release is cut by pushing a `v*` git tag** (e.g. `v2.2.13`), which triggers
  `.github/workflows/release.yml` (builds the signed APK + AAB and the GitHub
  Release). CI (`ci.yml`) must be green on `main` first.
- **Release only after the WHOLE plan is complete — never per step.** When you
  are working through a multi-step plan (e.g. a security-audit remediation
  track), do NOT cut a release after each individual fix. Keep every change
  accumulating under the `## [Unreleased]` heading in `CHANGELOG.md`, and leave
  `version.go` at the last released version. Only when the entire plan is done do
  you: rename `[Unreleased]` → `[X.Y.Z] — <date>`, bump `version.go`
  (`Semantic` + `Code`), commit, and push the `vX.Y.Z` tag.
- A release commit keeps `version.go` and the `CHANGELOG.md` top section
  consistent. The tag name is `v` + `Semantic`.

## 2. Branch, push & attribution — hard rule

- **Push directly to `origin/main`** as
  **`johalputt <ankushchoudharyjohal@gmail.com>`** (author *and* committer). No
  feature branches / PRs unless the user explicitly asks. Never a `claude/…`
  branch. `git push -u origin HEAD:main`; on network failure retry up to 4× with
  exponential backoff (2s, 4s, 8s, 16s); never force-push `main`.
- **Never** put "Claude", the model name, or any model identifier in commit
  messages, code comments, changelog, or any other pushed artifact. Keep AI
  attribution out of the git history entirely (chat replies are fine).

## 3. Architecture constitution (do not violate)

- **Rule 4:** only `ui/` and `platform/` may import Gio (`gioui.org`).
  `internal/**` and `ui/state/**` must stay UI-framework-free.
- **Rule 5:** layout code never touches SQLite or the network; every mutation
  goes through the syncmanager command channel or an async loader in `ui/state`,
  and wakes the window via invalidate.
- **Rule 6:** secrets (credentials, app-lock verifier, **PGP private keys**)
  live in the platform keystore (`internal/crypto`), **never in SQLite**. Only
  public PGP material + metadata belongs in the `pgp_keys` table.

## 4. Gates before every push (mirror CI in `ci.yml`)

```
gofmt -l <changed .go files>     # must be empty
go build ./...                   # Gio native deps (xkbcommon/wayland) fail to
                                 # build in this sandbox — that is expected;
                                 # build the cgo-free packages you touched
                                 # (internal/…, ui/state) to verify your code.
go vet ./ui/state/ ./internal/…
go test ./…                      # at least the packages you touched
markdownlint-cli2 <changed .md>  # MD004: a wrapped line starting with */+ reads
                                 # as a bullet — reword. MD024: no dup headings
                                 # within one section.
```

## 5. Security-audit remediation track (in progress)

Findings are being fixed one per commit, all landing on `main`, **held under
`[Unreleased]`** until the whole track is done (then one release per §1).
Shipped: **H7** (VayuTalk sender authentication), **H6** (PGP private keys sealed
in the platform keystore, not SQLite). Remaining: M14/M15 (setup-code
SSRF/https/domain-binding), M16 (sealed-keystore master key), M17 (PGP "signed"
indicator without `VerifyDetached`), L12 (`allowBackup=false`), L13
(notification-tap intent hardening).
