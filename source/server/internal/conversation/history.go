package conversation

import (
	"context"
	"fmt"
	"time"
)

// HistoryReader is a read-only seam. conversationID MUST come from the trusted
// execution context, never from model arguments. It does not grant access to
// parents, siblings, children or other conversations in the same project.
type HistoryReader interface {
	ReadHistory(context.Context, string, HistoryQuery) ([]HistoryEntry, error)
}
type HistoryQuery struct {
	Query  string `json:"query,omitempty"`
	TurnID string `json:"turn_id,omitempty"`
	After  int64  `json:"after,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type HistoryEntry struct {
	Cursor    int64  `json:"cursor"`
	TurnID    string `json:"turn_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func (q HistoryQuery) Validate() error {
	if len(q.Query) > 512 || len(q.TurnID) > 256 || q.After < 0 || q.Limit < 1 || q.Limit > 50 {
		return fmt.Errorf("invalid history query bounds")
	}
	if (q.Query == "") == (q.TurnID == "") {
		return fmt.Errorf("provide exactly one of query or turn_id")
	}
	return nil
}
func (s *sqliteStore) ReadHistory(ctx context.Context, conversationID string, q HistoryQuery) ([]HistoryEntry, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("history requires a conversation")
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Text only: never expose system prompts, tool payloads, reasoning blocks,
	// attachments or content_json. instr is literal, not SQL/LIKE/FTS syntax.
	rows, err := s.db.QueryContext(ctx, `SELECT rowid,id,role,substr(CAST(content AS BLOB),1,2048),length(CAST(content AS BLOB))>2048 FROM turns WHERE conversation_id=? AND role IN ('user','assistant') AND rowid>? AND ((?<>'' AND instr(content,?)>0) OR (?<>'' AND id=?)) ORDER BY rowid LIMIT ?`, conversationID, q.After, q.Query, q.Query, q.TurnID, q.TurnID, q.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.Cursor, &e.TurnID, &e.Role, &e.Content, &e.Truncated); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// HistoryReaderFunc adapts an authority-checking host callback.
type HistoryReaderFunc func(context.Context, string, HistoryQuery) ([]HistoryEntry, error)

func (f HistoryReaderFunc) ReadHistory(ctx context.Context, id string, q HistoryQuery) ([]HistoryEntry, error) {
	return f(ctx, id, q)
}
