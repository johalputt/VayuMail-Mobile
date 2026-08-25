# Reproducible builds

What it takes to rebuild VayuMail's Android artifacts from source and
trust that nothing crept in — and what is still missing before an
independent party (F-Droid or a curious user) can verify a published APK
byte-for-byte against their own build.

## Already deterministic

- **Toolchains are pinned, not floated.** CI and release both run
  go1.25.13 exactly (`go-version` in every workflow, matching
  `go.mod`'s `toolchain` line); every third-party action is referenced by
  commit SHA; gogio itself is installed at one pinned version by
  `scripts/gogio-hardened.sh`, which also fails the build if its manifest
  patches stop applying — a tool that changed under us fails loudly
  instead of shipping a quietly different APK.
- **No local paths in binaries.** The release build sets `GOFLAGS=-trimpath`.
- **Versioning has one source.** Both workflows read Semantic/Code from
  `internal/version/version.go`; nothing derives versions from clocks or
  git state.
- **The Java helpers cannot drift.** `platform/android/*.java` are
  compiled fresh from committed sources inside the release job (against
  android-35) rather than stored as jars, and the workflow asserts the
  expected classes exist in the jar before building.
- **The icon set is generated from committed Go code** (pure stdlib),
  so no binary asset can differ between machines.
- **Unsigned/test builds share one committed keystore**, so consecutive
  test builds update over each other instead of Android silently refusing
  the install. This is a throwaway debug key, never the Play upload key;
  release signing comes from repository secrets.

## Not yet byte-identical

An APK is a zip, and today the archive metadata (entry timestamps) varies
per run even when every input matches. Two builds from the same commit
produce functionally identical packages whose hashes differ. To close
that last gap:

1. Normalize archive timestamps after `gogio` (e.g. re-zip with fixed
   mtimes) and re-sign with `apksigner`, which is deterministic given the
   same key and input.
2. Pin the runner's SDK component versions — already explicit in
   `release.yml`'s setup-android step — AND record the runner image tag
   in the release notes, since NDK/clang updates can change object bytes
   inside `.so` files.
3. Publish the exact `go version`, gogio version, and SDK list alongside
   each release artifact.

## F-Droid readiness

The prerequisites F-Droid cares about that are already satisfied: no
Gradle (gogio builds directly, so no wrapper-provenance question), all
dependencies resolved from pinned module versions with a committed
`go.sum`, no proprietary blobs, and metadata-friendly versioning
(`Semantic`/`Code` in one file an `fdroid metadata` checker can parse).

Remaining for submission:

- An F-Droid metadata YAML describing the build recipe (it mirrors
  `release.yml`: go 1.25.13, gogio hardened, javac step, sdk 35/ndk
  26.3.11579264).
- The timestamp normalization above — F-Droid's verifier compares its own
  build hash to the published one, so byte-identity is required, not
  optional.
- A decision on signing: F-Droid can ship its own signature; if the Play
  upload key signs releases too, the private key must never touch
  F-Droid's builders, so the two distribution channels should expect
  different signatures by design.
