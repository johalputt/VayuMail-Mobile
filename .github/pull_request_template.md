<!--
Thank you for contributing. Read GOVERNANCE-CONSTITUTION.md and
CONTRIBUTING.md first. The `constitution` CI job and the checks below are
required — a red pipeline blocks merge.
-->

## What and why

<!-- What does this change do, and what problem does it solve? -->

## Constitution & architecture

- [ ] I read the relevant rules in `GOVERNANCE-CONSTITUTION.md`.
- [ ] No new dependency, or the dependency is Apache-2.0/MIT/BSD and added
      to `docs/ADR-0006` in this PR.
- [ ] `internal/{mail,store,syncmanager}` still import no Gio.
- [ ] No file exceeds 400 lines; no credential touches disk; nothing
      blocks the Gio frame goroutine.
- [ ] Architecture/concurrency/schema/permission/dependency changes have a
      new or updated ADR.

## Testing

- [ ] `make lint` (gofmt + vet + boundary + constitution) passes.
- [ ] `make race` passes; new logic has table-driven tests.
- [ ] `COMPLIANCE-TRACKER.md` updated if this ships or changes a
      stub/partial/pending item.

## Notes for reviewers

<!-- Anything non-obvious: trade-offs, follow-ups, screenshots. -->
