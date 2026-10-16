package logbook

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
)

// StatsQuery ignores pagination: statistics cover all retained matching requests.
// The half-open time range is bounded to 90 days and at most 600 buckets.
type StatsQuery struct {
	GroupBy    string `json:"group_by,omitempty"`
	Filter     Query  `json:"filter"`
	IntervalMS int64  `json:"interval_ms"`
}
type StatsValues struct {
	Requests           int64    `json:"requests"`
	Completed          int64    `json:"completed"`
	Errors             int64    `json:"errors"`
	InputTokens        *int64   `json:"input_tokens"`
	OutputTokens       *int64   `json:"output_tokens"`
	TotalTokens        *int64   `json:"total_tokens"`
	UsageSamples       int64    `json:"usage_samples"`
	TPS                *float64 `json:"tps"`
	TPSSamples         int64    `json:"tps_samples"`
	ErrorRate          *float64 `json:"error_rate"`
	CacheHitRate       *float64 `json:"cache_hit_rate"`
	CacheSamples       int64    `json:"cache_samples"`
	LatencyMS          *float64 `json:"latency_ms"`
	LatencySamples     int64    `json:"latency_samples"`
	FirstOutputMS      *float64 `json:"first_output_ms"`
	FirstOutputSamples int64    `json:"first_output_samples"`
}
type StatsBucket struct {
	AtMS   int64       `json:"at_ms"`
	Values StatsValues `json:"values"`
}
type StatsGroup struct {
	Value  string      `json:"value"`
	Values StatsValues `json:"values"`
}
type Stats struct {
	GroupBy    string        `json:"group_by,omitempty"`
	Groups     []StatsGroup  `json:"groups,omitempty"`
	GroupCount int           `json:"group_count,omitempty"`
	SinceMS    int64         `json:"since_ms"`
	BeforeMS   int64         `json:"before_ms"`
	IntervalMS int64         `json:"interval_ms"`
	Buckets    []StatsBucket `json:"buckets"`
	Summary    StatsValues   `json:"summary"`
	Health     Health        `json:"health"`
}

// dimensionColumn is a whitelist, not an SQL expression supplied by the client.
func dimensionColumn(name string) string {
	switch name {
	case "agent", "binding", "project", "session":
		return "COALESCE(json_extract(document,'$." + name + "'),'')"
	default:
		return ""
	}
}
func (q *StatsQuery) Validate() error {
	if q.GroupBy != "" && dimensionColumn(q.GroupBy) == "" {
		return ErrInvalid
	}
	if q.Filter.Kind != "" && q.Filter.Kind != "requests" {
		return ErrInvalid
	}
	q.Filter.Kind = "requests"
	// Pagination and sequence snapshots do not constrain full-range statistics.
	q.Filter.Limit, q.Filter.Cursor, q.Filter.Until = 50, 0, 0
	if q.Filter.BeforeMS == 0 {
		q.Filter.BeforeMS = time.Now().UnixMilli()
	}
	if q.Filter.Validate() != nil {
		return ErrInvalid
	}
	span := q.Filter.BeforeMS - q.Filter.SinceMS
	if span <= 0 || span > int64(90*24*time.Hour/time.Millisecond) {
		return ErrInvalid
	}
	if q.IntervalMS == 0 {
		for _, interval := range []int64{10000, 30000, 60000, 300000, 900000, 3600000, 21600000, 86400000} {
			q.IntervalMS = interval
			if (span+interval-1)/interval <= 120 {
				break
			}
		}
	}
	allowed := false
	for _, interval := range []int64{10000, 30000, 60000, 300000, 900000, 3600000, 21600000, 86400000} {
		if q.IntervalMS == interval {
			allowed = true
		}
	}
	if !allowed || (span+q.IntervalMS-1)/q.IntervalMS > 600 {
		return ErrInvalid
	}
	return nil
}

type statsAccumulator struct {
	StatsValues
	input, output, tpsOutput, tpsMS, cacheRead, cacheInput, latency, first int64
}

func ratio(n, d int64, scale float64) *float64 {
	if d <= 0 {
		return nil
	}
	v := float64(n) / float64(d) * scale
	return &v
}
func (a *statsAccumulator) values() StatsValues {
	v := a.StatsValues
	if v.UsageSamples > 0 {
		in, out, total := a.input, a.output, a.input+a.output
		v.InputTokens = &in
		v.OutputTokens = &out
		v.TotalTokens = &total
	}
	v.TPS = ratio(a.tpsOutput, a.tpsMS, 1000)
	v.ErrorRate = ratio(v.Errors, v.Completed, 100)
	v.CacheHitRate = ratio(a.cacheRead, a.cacheInput, 100)
	v.LatencyMS = ratio(a.latency, v.LatencySamples, 1)
	v.FirstOutputMS = ratio(a.first, v.FirstOutputSamples, 1)
	return v
}
func (a *statsAccumulator) add(status string, input, output, elapsed, cache, first sql.NullInt64, attempts int64) {
	a.Requests++
	if status == "succeeded" || status == "failed" || status == "interrupted" {
		a.Completed++
	}
	if status == "failed" || status == "interrupted" {
		a.Errors++
	}
	if input.Valid && output.Valid && input.Int64 >= 0 && output.Int64 >= 0 {
		a.UsageSamples++
		a.input += input.Int64
		a.output += output.Int64
	}
	if status == "succeeded" && output.Valid && output.Int64 >= 0 && elapsed.Valid && elapsed.Int64 > 0 {
		a.TPSSamples++
		a.tpsOutput += output.Int64
		a.tpsMS += elapsed.Int64
	}
	if attempts == 1 && input.Valid && input.Int64 > 0 && cache.Valid && cache.Int64 >= 0 && cache.Int64 <= input.Int64 {
		a.CacheSamples++
		a.cacheRead += cache.Int64
		a.cacheInput += input.Int64
	}
	if status != "pending" && elapsed.Valid && elapsed.Int64 >= 0 {
		a.LatencySamples++
		a.latency += elapsed.Int64
	}
	if first.Valid && first.Int64 >= 0 {
		a.FirstOutputSamples++
		a.first += first.Int64
	}
}
func (s *Store) Statistics(ctx context.Context, q StatsQuery) (Stats, error) {
	out := Stats{Buckets: []StatsBucket{}, Health: s.Health()}
	if q.Validate() != nil {
		return out, ErrInvalid
	}
	out.SinceMS, out.BeforeMS, out.IntervalMS = q.Filter.SinceMS, q.Filter.BeforeMS, q.IntervalMS
	count := int((out.BeforeMS - out.SinceMS + out.IntervalMS - 1) / out.IntervalMS)
	buckets := make([]statsAccumulator, count)
	var total statsAccumulator
	groups := map[string]*statsAccumulator{}
	groupColumn := "''"
	if q.GroupBy != "" {
		groupColumn = dimensionColumn(q.GroupBy)
	}
	out.GroupBy = q.GroupBy
	where, args := logWhere(q.Filter, "requests")
	// Stats uses [since,before), whereas the historical list includes before.
	where = append(where, "created_at<?")
	args = append(args, out.BeforeMS)
	// Extract only numeric observations, never load attempts/evidence/body documents into memory.
	rows, err := s.read.QueryContext(ctx, `SELECT created_at,status,
 json_extract(document,'$.input'),json_extract(document,'$.output'),json_extract(document,'$.elapsed_ms'),
 json_extract(document,'$.metrics.cache_read_tokens'),
 COALESCE(json_extract(document,'$.metrics.time_to_first_output_ms'),json_extract(document,'$.metrics.time_to_first_text_ms')),
 COALESCE(json_extract(document,'$.attempt_count'),json_array_length(document,'$.attempts'),0), `+groupColumn+`
 FROM requests WHERE `+strings.Join(where, " AND "), args...)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var at, attempts int64
		var status, group string
		var input, output, elapsed, cache, first sql.NullInt64
		if rows.Scan(&at, &status, &input, &output, &elapsed, &cache, &first, &attempts, &group) != nil {
			return out, ErrUnavailable
		}
		i := (at - out.SinceMS) / out.IntervalMS
		buckets[i].add(status, input, output, elapsed, cache, first, attempts)
		total.add(status, input, output, elapsed, cache, first, attempts)
		if q.GroupBy != "" {
			if groups[group] == nil {
				groups[group] = &statsAccumulator{}
			}
			groups[group].add(status, input, output, elapsed, cache, first, attempts)
		}
	}
	if rows.Err() != nil {
		return out, ErrUnavailable
	}
	for i := range buckets {
		out.Buckets = append(out.Buckets, StatsBucket{AtMS: out.SinceMS + int64(i)*out.IntervalMS, Values: buckets[i].values()})
	}
	// Bound the response and DOM, not the statistics: total includes every match.
	for value, accumulator := range groups {
		out.Groups = append(out.Groups, StatsGroup{Value: value, Values: accumulator.values()})
	}
	out.GroupCount = len(out.Groups)
	sort.Slice(out.Groups, func(i, j int) bool {
		if out.Groups[i].Values.Requests != out.Groups[j].Values.Requests {
			return out.Groups[i].Values.Requests > out.Groups[j].Values.Requests
		}
		return out.Groups[i].Value < out.Groups[j].Value
	})
	if len(out.Groups) > 100 {
		out.Groups = out.Groups[:100]
	}
	out.Summary = total.values()
	return out, nil
}
