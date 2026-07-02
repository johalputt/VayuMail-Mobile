# ADR-0006 — Dependency license audit

## Status

Accepted — living document. **Must be updated in the same commit as any
`go.mod` change.**

## Decision

Every dependency, direct and indirect, is confirmed permissive
(Apache-2.0 / MIT / BSD-3-Clause / Unlicense) before it enters `go.mod`.
Copyleft (GPL, AGPL, LGPL, CDDL, EPL) is a hard stop.

Note on BSD-3-Clause: the constitution names Apache-2.0 and MIT; the
`golang.org/x/*` modules and Go toolchain ecosystem are BSD-3-Clause,
which is permissive, non-copyleft, and unavoidable (Gio itself depends on
them). The maintainer accepts BSD-3-Clause and Unlicense as within the
rule's intent (no copyleft, no vendor lock); this note is the documented
rationale required by the constitution's amendment clause.

## Audit table

### Direct dependencies

| Dependency | Version | License | Permissive confirmed | Notes |
|---|---|---|---|---|
| gioui.org | v0.10.1 | MIT / Unlicense (dual) | ✅ | UI framework; only `ui/` and `platform/` may import |
| gioui.org/x | v0.10.0 | MIT / Unlicense (dual) | ✅ | `pref/theme` dark-mode detection only |
| github.com/ProtonMail/go-crypto | v1.4.1 | BSD-3-Clause | ✅ | OpenPGP (fork of golang.org/x/crypto openpgp) |
| github.com/emersion/go-imap/v2 | v2.0.0-beta.8 | MIT | ✅ | IMAP client; v2 required for reliable IDLE |
| github.com/emersion/go-message | v0.18.2 | MIT | ✅ | MIME parse/build |
| github.com/emersion/go-sasl | v0.0.0-2024… | MIT | ✅ | SASL PLAIN for SMTP auth |
| github.com/emersion/go-smtp | v0.24.0 | MIT | ✅ | SMTP client |
| github.com/makiuchi-d/gozxing | v0.1.1 | MIT | ✅ | Pure-Go QR decoder (ZXing port; LICENSE verified MIT) |
| golang.org/x/net | v0.56.0 | BSD-3-Clause | ✅ | HTML tokenizer for sanitized text rendering |
| modernc.org/sqlite | v1.53.0 | BSD-3-Clause | ✅ | Pure-Go SQLite with FTS5; the only permitted driver (Rule 1) |
| go.uber.org/goleak | v1.3.0 | MIT | ✅ | Tests only — goroutine leak detection |

### Indirect dependencies

| Dependency | License | Permissive confirmed | Pulled in by |
|---|---|---|---|
| gioui.org/shader | MIT / Unlicense | ✅ | gioui.org |
| git.wow.st/gmp/jni | BSD-3-Clause | ✅ | gioui.org/x (Android pref, notify) |
| github.com/esiqveland/notify | BSD-3-Clause | ✅ | gioui.org/x/notify (Linux DBus) |
| github.com/godbus/dbus/v5 | BSD-2-Clause | ✅ | esiqveland/notify |
| github.com/cloudflare/circl | BSD-3-Clause | ✅ | ProtonMail/go-crypto |
| github.com/dustin/go-humanize | MIT | ✅ | modernc.org/sqlite |
| github.com/go-text/typesetting | BSD-3-Clause | ✅ | gioui.org (text shaping) |
| github.com/google/uuid | BSD-3-Clause | ✅ | modernc.org/sqlite |
| github.com/mattn/go-isatty | MIT | ✅ | modernc.org/sqlite (no cgo — isatty only) |
| github.com/ncruces/go-strftime | MIT | ✅ | modernc.org/sqlite |
| github.com/remyoudompheng/bigfft | BSD-3-Clause | ✅ | modernc.org/mathutil |
| golang.org/x/crypto | BSD-3-Clause | ✅ | ProtonMail/go-crypto |
| golang.org/x/exp/shiny | BSD-3-Clause | ✅ | gioui.org |
| golang.org/x/image | BSD-3-Clause | ✅ | gioui.org |
| golang.org/x/sys | BSD-3-Clause | ✅ | multiple |
| golang.org/x/text | BSD-3-Clause | ✅ | go-message charset decoding |
| golang.org/x/xerrors | BSD-3-Clause | ✅ | gozxing |
| modernc.org/libc | BSD-3-Clause | ✅ | modernc.org/sqlite (pure-Go libc, **not** cgo) |
| modernc.org/mathutil | BSD-3-Clause | ✅ | modernc.org/sqlite |
| modernc.org/memory | BSD-3-Clause | ✅ | modernc.org/sqlite |

## Never add

- `mattn/go-sqlite3` — cgo (Rule 1).
- Any analytics or crash-reporting SDK — telemetry violation.
- Any HTTP client beyond `net/http`, JSON library beyond
  `encoding/json`, or logging library beyond `log/slog` — the standard
  library is sufficient.

## cgo status

`internal/*` and `cmd/vayumail-cli` build with `CGO_ENABLED=0` (verified
in CI). Gio's Linux/Android windowing backends bind to system windowing
libraries (X11/Wayland/EGL) at build time — system interfaces, not
vendored C dependencies; Windows and JS backends are pure Go. This is
documented here as the constitution requires for any capability without
a fully cgo-free path.
