package store

import (
	"context"
	"fmt"
	"strings"
)

// SearchResult is one FTS5 hit with its BM25 relevance rank. Lower rank
// means more relevant (SQLite bm25() returns negative scores).
type SearchResult struct {
	Message Message
	Rank    float64
}

// Search runs a full-text query over sender, subject, and snippet fields
// of every cached message, best matches first. The query is sanitized into
// FTS5 prefix terms so raw user input can never produce a syntax error.
//
// Search covers fields indexed at ingest; body search requires the body to
// have been fetched first (ADR-0002).
func (db *DB) Search(ctx context.Context, accountID int64, query string, limit int) ([]SearchResult, error) {
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := db.sql.QueryContext(ctx, `
		SELECT `+prefixedMessageCols+`, bm25(messages_fts) AS rank
		FROM messages_fts
		JOIN messages m ON m.id = messages_fts.rowid
		WHERE messages_fts MATCH ?
			AND m.account_id = ? AND m.is_deleted = 0
		ORDER BY rank LIMIT ?`,
		ftsQuery, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: search %q: %w", query, err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var m Message
		var date int64
		var rank float64
		err := rows.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID,
			&m.ThreadID, &m.MessageID, &m.InReplyTo, &m.FromAddr,
			&m.FromName, &m.ToAddrs, &m.CcAddrs, &m.Subject, &m.Snippet,
			&m.BodyText, &m.BodyHTML, &m.HasAttachments, &m.PGPStatus,
			&m.IsRead, &m.IsFlagged, &m.IsDeleted, &date, &m.SizeBytes,
			&m.Flags, &rank)
		if err != nil {
			return nil, fmt.Errorf("store: scan search result: %w", err)
		}
		m.Date = unixUTC(date)
		out = append(out, SearchResult{Message: m, Rank: rank})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}
	return out, nil
}

// prefixedMessageCols mirrors messageCols with an explicit table alias so
// the FTS join is unambiguous.
const prefixedMessageCols = `m.id, m.account_id, m.folder_id, m.uid,
	COALESCE(m.thread_id,''), COALESCE(m.message_id,''),
	COALESCE(m.in_reply_to,''), m.from_addr, COALESCE(m.from_name,''),
	m.to_addrs, COALESCE(m.cc_addrs,''), COALESCE(m.subject,''),
	COALESCE(m.snippet,''), COALESCE(m.body_text,''),
	COALESCE(m.body_html,''), m.has_attachments,
	COALESCE(m.pgp_status,''), m.is_read, m.is_flagged, m.is_deleted,
	m.date, COALESCE(m.size_bytes,0), COALESCE(m.flags,'')`

// buildFTSQuery turns free-form user input into a safe FTS5 expression:
// each whitespace-separated term becomes a quoted prefix match, terms are
// implicitly ANDed. Double quotes inside terms are stripped so user input
// cannot break out of the quoting.
func buildFTSQuery(input string) string {
	fields := strings.Fields(input)
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		if f == "" {
			continue
		}
		terms = append(terms, `"`+f+`"*`)
	}
	return strings.Join(terms, " ")
}
