package logbook

import (
	"context"
	"strings"
	"time"
)

// SuggestionsQuery intentionally has no pagination or active filters: completing
// a value must not hide alternatives because that value is already selected.
// Only indexed, body-free fields that List can search/filter are exposed.
type SuggestionsQuery struct {
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	Prefix   string `json:"prefix"`
	SinceMS  int64  `json:"since_ms"`
	BeforeMS int64  `json:"before_ms"`
	Limit    int    `json:"limit"`
}
type Suggestion struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}
type Suggestions struct {
	Items   []Suggestion `json:"items"`
	HasMore bool         `json:"has_more"`
}

func (q *SuggestionsQuery) Validate() error {
	if q.Kind == "" {
		q.Kind = "requests"
	}
	if q.Kind != "requests" && q.Kind != "system" {
		return ErrInvalid
	}
	if q.Kind == "system" && q.Field != "search" {
		return ErrInvalid
	}
	switch q.Field {
	case "search", "provider", "model", "key", "http", "code", "stage", "agent", "binding", "project", "session":
	default:
		return ErrInvalid
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.SinceMS == 0 {
		q.SinceMS = time.Now().Add(-time.Hour).UnixMilli()
	}
	if q.BeforeMS == 0 {
		q.BeforeMS = time.Now().UnixMilli()
	}
	if q.Limit < 1 || q.Limit > 100 || len(q.Prefix) > 512 || q.SinceMS < 0 || q.BeforeMS < q.SinceMS || q.BeforeMS-q.SinceMS > int64(90*24*time.Hour/time.Millisecond) {
		return ErrInvalid
	}
	return nil
}

func (s *Store) Suggest(ctx context.Context, q SuggestionsQuery) (Suggestions, error) {
	out := Suggestions{Items: []Suggestion{}}
	if q.Validate() != nil {
		return out, ErrInvalid
	}
	columns := map[string]string{"provider": "provider", "model": "model", "key": "key_id", "http": "http", "code": "error_code", "stage": "stage"}
	for _, name := range []string{"agent", "binding", "project", "session"} {
		columns[name] = dimensionColumn(name)
	}
	table, cols := "requests", []string{columns[q.Field]}
	if q.Kind == "system" {
		table, cols = "events", []string{"event"}
	} else if q.Field == "search" {
		cols = []string{"id", "model", "provider", "key_id", "error_code", dimensionColumn("agent"), dimensionColumn("binding"), dimensionColumn("project"), dimensionColumn("session")}
	}
	var selects []string
	var args []any
	for _, col := range cols {
		value := col
		if col == "http" {
			value = "CAST(http AS TEXT)"
		}
		part := "SELECT seq, " + value + " AS value FROM " + table + " WHERE created_at>=? AND created_at<=? AND " + value + "<>''"
		args = append(args, q.SinceMS, q.BeforeMS)
		if q.Prefix != "" {
			// Same case-sensitive prefix range as List, not LIKE wildcards.
			part += " AND " + value + ">=? AND " + value + "<?"
			args = append(args, q.Prefix, q.Prefix+"\U0010ffff")
		}
		selects = append(selects, part)
	}
	// UNION deduplicates a value appearing in several fields of one request.
	query := "SELECT value, COUNT(*) FROM (" + strings.Join(selects, " UNION ") + ") GROUP BY value ORDER BY COUNT(*) DESC, value ASC LIMIT ?"
	args = append(args, q.Limit+1)
	rows, err := s.read.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Suggestion
		if err := rows.Scan(&v.Value, &v.Count); err != nil {
			return out, err
		}
		if len(out.Items) == q.Limit {
			out.HasMore = true
			break
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
