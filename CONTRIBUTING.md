# Contributing to VayuMail-Mobile

Thank you for contributing. This project is governed by a constitution;
read [GOVERNANCE-CONSTITUTION.md](GOVERNANCE-CONSTITUTION.md) before writing
any code. All ten rules are binding — pull requests that violate a rule are
not merged until the violation is resolved.

## Before you start

1. Read `GOVERNANCE-CONSTITUTION.md` — all 10 rules, no exceptions.
2. Read `docs/ARCHITECTURE.md` — do not deviate from the goroutine topology
   or channel types without writing a new ADR.
3. Read `COMPLIANCE-TRACKER.md` — know what is complete vs stubbed.
4. Read the ADR most relevant to the files you are editing.

## The ADR process

Any change that affects architecture, the concurrency model, the database
schema, the permission set, or the dependency list requires an Architecture
Decision Record **before** code is written.

- ADRs live in `docs/` and are numbered sequentially:
  `ADR-000N-short-title.md`.
- An ADR has four sections: **Decision**, **Context**, **Status**,
  **Consequences**.
- ADRs are never deleted. A superseded ADR gets a `Status: superseded by
  ADR-000M` line; the replacement links back.
- Dependency changes additionally update
  `docs/ADR-0006-dependency-license-audit.md` in the same commit.

## Code style

- Idiomatic Go. `gofmt`-clean, `go vet`-clean, `staticcheck`-clean.
- Every exported type, function, and method has a complete doc comment.
- Errors are never discarded with `_`. Wrap with
  `fmt.Errorf("context: %w", err)` or document the reason for discarding.
- Logging: `log/slog` only, structured key-value pairs. No `fmt.Println`
  in production paths.
- No global mutable state; dependencies are injected explicitly.
- No file exceeds 400 lines (Rule 10). Split before you reach it.
- No `sync.Mutex` held across an I/O call. Prefer channels where the
  pattern fits naturally.
- Incomplete work is marked `// STUB: <reason>` and tracked in
  `COMPLIANCE-TRACKER.md` — in the same commit.

## Tests

- Table-driven for all non-trivial logic.
- Fixtures live in `test/fixtures/` — no live servers, no network calls
  in CI.
- Deterministic: must pass `go test -race -count=3 ./...` every time.
- `goleak.VerifyNone(t)` in every syncmanager test.
- Coverage target: 80%+ on `internal/mail/` and `internal/store/`.

## Local checks (run before every push)

```sh
make lint   # gofmt + go vet + package boundary check
make race   # go test -race -count=1 ./...
```

CI runs the same pipeline plus staticcheck; a red pipeline blocks merge.

## Commit and PR hygiene

- Small, focused commits with descriptive messages.
- A PR that adds a dependency must include the ADR-0006 row and the SPDX
  license confirmation in its description.
- A PR that touches `platform/` manifests must link the permission ADR
  that authorizes it.
