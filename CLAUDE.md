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

```text
gofmt -l <changed .go files>     # must be empty
go build ./...                   # Gio needs native deps. They ARE installable:
                                 # apt-get install -y libxkbcommon-dev
                                 # libxkbcommon-x11-dev libwayland-dev libx11-dev
                                 # libx11-xcb-dev libxcursor-dev libxfixes-dev
                                 # libgles2-mesa-dev libegl1-mesa-dev libvulkan-dev
                                 # After that the full app builds and runs headless
                                 # under Xvfb — see scripts/screenshots.sh.
go vet ./...
golangci-lint run ./...          # v2, must be 0 issues
staticcheck ./...
gosec -severity high -confidence high ./...
go test ./...                    # at least the packages you touched
go mod tidy                      # must leave go.mod/go.sum unchanged
sh scripts/constitution.sh       # Rules 1–10
markdownlint-cli2                # config is committed: .markdownlint-cli2.jsonc
                                 # MD004: a wrapped line starting with */+ reads
                                 # as a bullet — reword. MD024 is scoped to
                                 # siblings, so a changelog may repeat "Fixed".
```

Cross-compilation is worth running by hand when touching `internal/`, because
the ship target is a phone and every other gate runs on host linux/amd64:

```text
PKGS=$(go list ./internal/... | grep -vE 'internal/(biometric|pushnotify)$')
CGO_ENABLED=0 GOOS=android GOARCH=arm go build $PKGS   # 32-bit is the strict one
```

`internal/biometric` and `internal/pushnotify` are excluded because they are the
JNI seams and select their cgo files on `GOOS=android` by design.

Notes that have cost time here:

- **golangci-lint stops analysing a package the moment it fails to typecheck.**
  A short report is not necessarily a clean one — if `typecheck` appears among
  the issues, every other finding in that package is still hidden. Fixing one
  compile error surfaced 37 further issues in three rounds.
- **`gosec` at high/high is quiet but not empty.** It is what found that the
  certificate pin was not enforced on resumed TLS sessions (G123).

## 5. Security-audit remediation track

Findings are fixed one per commit, all landing on `main`, **held under
`[Unreleased]`** until the whole track is done (then one release per §1).

**Shipped in 2.2.13 and earlier:** H7 (VayuTalk sender authentication), H6 (PGP
private keys sealed in the platform keystore, not SQLite), M14/M15 (setup-code
SSRF / https / domain binding), M16 (sealed-keystore master key), M17 (PGP
"signed" indicator without `VerifyDetached`), L13 (notification-tap intent
hardening).

**Currently under `[Unreleased]`:** L12 (Android `allowBackup=false`) — see
below; the resumed-session TLS pin bypass; the master-key creation race.

### L12 is the worked example: a claim is not a control

This section listed L12 as "remaining" long after `CHANGELOG.md` announced it
under **Security** as *"Android backup of the app-private data is disabled"*, and
both files in `platform/android/` stated in the present tense that the app set
`allowBackup="false"`. None of it existed. gogio compiles the manifest into
itself, its `<application>` tag had no `allowBackup` at all, and the two rule
files were referenced by nothing in the build — so every shipped APK had
Android's default `allowBackup="true"`, putting `vayumail.db` and the sealed
keystore inside `adb backup` and cloud Auto Backup.

What made it survive: **adding the artifacts felt like doing the work.** The rule
files were real, the documentation was real, and the only missing piece was the
one nothing could test. Three separate places asserted the control and no gate
could tell any of them apart from the truth.

So the standing rule, beyond this one finding:

- **A security note in `CHANGELOG.md` names the mechanism that enforces it** —
  the file, the flag, the pipeline step. "`platform/android/` now carries the
  required attributes" describes a directory, not a control.
- **If it cannot be tested, it is not done.** The fix here shipped with
  `test/android_manifest_audit_test.go`, which fails if any file in the tree
  claims backup is disabled while the pipeline does not disable it.
- **Mutation-test the claim, not just the code.** The first version of that test
  passed with the hardening removed, because it matched the YAML *comment*
  explaining the step rather than the step.
