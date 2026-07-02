package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SearchResult is one FTS5 hit with its BM25 relevance rank. Lower rank
// means more relevant (SQLite bm25() returns negative scores).
type SearchResult struct {
	Message Message
	Rank    float64
}

// Search runs a full-text query over sender, subject, snippet, and body
// of every cached message, best matches first. Free text is sanitized
// into FTS5 prefix terms; the operators below refine the match:
//
//	from:alice          sender address/name contains the term
//	subject:report      subject contains the term
//	has:attachment      only messages with attachments
//	is:unread           only unread messages
//	before:2026-01-31   received before the date
//	after:2026-01-01    received after the date
//
// Body search covers bodies that have been fetched (ADR-0002).
func (db *DB) Search(ctx context.Context, accountID int64, query string, limit int) ([]SearchResult, error) {
	parsed := parseQuery(query)
	if parsed.fts == "" && !parsed.hasFilters() {
		return nil, nil
	}

	where := []string{"m.account_id = ?", "m.is_deleted = 0"}
	args := []any{accountID}
	ftsJoin := ""
	rankExpr := "0"
	orderBy := "m.date DESC"
	if parsed.fts != "" {
		ftsJoin = "JOIN messages_fts ON messages_fts.rowid = m.id"
		where = append(where, "messages_fts MATCH ?")
		args = append(args, parsed.fts)
		rankExpr = "bm25(messages_fts)"
		orderBy = "rank"
	}
	if parsed.hasAttachment {
		where = append(where, "m.has_attachments = 1")
	}
	if parsed.unread {
		where = append(where, "m.is_read = 0")
	}
	if !parsed.before.IsZero() {
		where = append(where, "m.date < ?")
		args = append(args, parsed.before.Unix())
	}
	if !parsed.after.IsZero() {
		where = append(where, "m.date > ?")
		args = append(args, parsed.after.Unix())
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT %s, %s AS rank FROM messages m %s
		WHERE %s ORDER BY %s LIMIT ?`,
		prefixedMessageCols, rankExpr, ftsJoin,
		strings.Join(where, " AND "), orderBy)

	rows, err := db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: search %q: %w", query, err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		m, rank, err := scanPrefixedMessageWithRank(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan search result: %w", err)
		}
		out = append(out, SearchResult{Message: m, Rank: rank})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}
	return out, nil
}

// prefixedMessageCols mirrors messageCols with an explicit table alias so
// joined queries are unambiguous. Keep in sync with messageCols.
const prefixedMessageCols = `m.id, m.account_id, m.folder_id, m.uid,
	COALESCE(m.thread_id,''), COALESCE(m.message_id,''),
	COALESCE(m.in_reply_to,''), m.from_addr, COALESCE(m.from_name,''),
	m.to_addrs, COALESCE(m.cc_addrs,''), COALESCE(m.subject,''),
	COALESCE(m.snippet,''), COALESCE(m.body_text,''),
	COALESCE(m.body_html,''), m.has_attachments,
	COALESCE(m.pgp_status,''), m.is_read, m.is_flagged, m.is_deleted,
	m.date, COALESCE(m.size_bytes,0), COALESCE(m.flags,''),
	m.has_trackers, m.is_list, COALESCE(m.list_unsubscribe,''),
	m.snooze_until, COALESCE(m.attachments,'')`

func scanPrefixedMessage(row interface{ Scan(...any) error }) (Message, error) {
	return scanMessage(row)
}

func scanPrefixedMessageWithRank(row interface{ Scan(...any) error }) (Message, float64, error) {
	var m Message
	var date, snooze int64
	var rank float64
	err := row.Scan(&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.ThreadID,
		&m.MessageID, &m.InReplyTo, &m.FromAddr, &m.FromName, &m.ToAddrs,
		&m.CcAddrs, &m.Subject, &m.Snippet, &m.BodyText, &m.BodyHTML,
		&m.HasAttachments, &m.PGPStatus, &m.IsRead, &m.IsFlagged,
		&m.IsDeleted, &date, &m.SizeBytes, &m.Flags, &m.HasTrackers,
		&m.IsList, &m.ListUnsubscribe, &snooze, &m.Attachments, &rank)
	if err != nil {
		return Message{}, 0, err
	}
	m.Date = unixUTC(date)
	if snooze > 0 {
		m.SnoozeUntil = unixUTC(snooze)
	}
	return m, rank, nil
}

// parsedQuery is the structured form of a search input.
type parsedQuery struct {
	fts           string
	hasAttachment bool
	unread        bool
	before, after time.Time
}

func (p parsedQuery) hasFilters() bool {
	return p.hasAttachment || p.unread || !p.before.IsZero() || !p.after.IsZero()
}

// parseQuery splits operators from free text and builds a safe FTS5
// expression: free terms become quoted prefix matches over all columns;
// from:/subject: terms become column-scoped matches.
func parseQuery(input string) parsedQuery {
	var p parsedQuery
	var ftsTerms []string
	quote := func(s string) string {
		return `"` + strings.ReplaceAll(s, `"`, "") + `"*`
	}
	for _, field := range strings.Fields(input) {
		lower := strings.ToLower(field)
		switch {
		case strings.HasPrefix(lower, "from:"):
			if v := field[len("from:"):]; v != "" {
				ftsTerms = append(ftsTerms,
					"({from_addr from_name} : "+quote(v)+")")
			}
		case strings.HasPrefix(lower, "subject:"):
			if v := field[len("subject:"):]; v != "" {
				ftsTerms = append(ftsTerms, "(subject : "+quote(v)+")")
			}
		case lower == "has:attachment":
			p.hasAttachment = true
		case lower == "is:unread":
			p.unread = true
		case strings.HasPrefix(lower, "before:"):
			if t, err := time.Parse("2006-01-02", field[len("before:"):]); err == nil {
				p.before = t
			}
		case strings.HasPrefix(lower, "after:"):
			if t, err := time.Parse("2006-01-02", field[len("after:"):]); err == nil {
				p.after = t
			}
		default:
			cleaned := strings.ReplaceAll(field, `"`, "")
			if cleaned != "" {
				ftsTerms = append(ftsTerms, quote(cleaned))
			}
		}
	}
	p.fts = strings.Join(ftsTerms, " ")
	return p
}
