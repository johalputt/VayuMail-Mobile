# ADR-0002 — SQLite FTS5 for full-text search

## Status

Accepted — v0.1.0.

## Decision

Full-text search runs on SQLite's FTS5 extension over an external-content
virtual table (`messages_fts`) kept in sync by three triggers on
`messages`. There is no external search index and no search service.
The driver is `modernc.org/sqlite` (pure Go, FTS5 compiled in).

## Context

- Rule 1 requires pure Go; `modernc.org/sqlite` ships FTS5 without cgo.
- One binary, one database file: search must work fully offline and add
  zero operational surface.
- Search latency must feel instant on a phone; FTS5 with BM25 ranking
  over sender/subject/snippet is well within budget for six-figure
  message counts.

## Consequences

- Search covers the fields indexed at ingest: `from_addr`, `from_name`,
  `subject`, `snippet`. Body search requires the body to have been
  fetched first; messages over the inline-fetch cap (512 KiB) are
  envelope-only until opened.
- User input is sanitized into quoted prefix terms
  (`internal/store/search.go`), so raw input can never produce an FTS5
  syntax error or escape the query.
- The FTS index adds modest database size (a fraction of message text);
  accepted for offline capability.
- Ranking is BM25 as provided by FTS5; no learned ranking, by design
  (the app learns nothing about the user).
