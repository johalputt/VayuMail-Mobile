# ADR-0001 — Strict Gio-free package boundary

## Status

Accepted — v0.1.0.

## Decision

`internal/mail/`, `internal/store/`, and `internal/syncmanager/` carry a
strict boundary: they never import `gioui.org/*`, any platform-specific
package, or anything under `ui/` or `platform/`. Only `ui/` and
`platform/` may import Gio. CI enforces the boundary with a grep that
fails the build on any violation.

Corollary (import direction within the engine): `internal/mail/*` also
never imports `internal/syncmanager`. The IDLE loop reports state through
the `imapsync.Events` callback struct; the syncmanager adapts those
callbacks onto its typed `Event` bus. This is the one refinement over the
original design sketch (`RunIDLE(..., eventCh chan<- syncmanager.Event)`),
which would create an import cycle Go forbids
(`syncmanager → imapsync → syncmanager`) and would couple the protocol
layer to the sync layer, defeating the boundary's purpose.

## Context

The mail protocol logic must be reusable across clients for the lifetime
of the project: a CLI (`cmd/vayumail-cli` already proves this), a
VayuPress server plugin, or a desktop client should be able to import
`internal/mail` tomorrow without any modification. UI frameworks come and
go; RFC 3501 does not.

## Consequences

- `internal/mail`, `internal/store`, and `internal/syncmanager` build
  with `CGO_ENABLED=0` and no display server — verified in CI.
- The headless CLI exercises the full engine, which keeps engine bugs
  reproducible without a device or emulator.
- UI state flows exclusively through the two typed channels documented
  in ARCHITECTURE.md; there is no back door.
- The boundary check is a build gate: a violating PR cannot merge.
