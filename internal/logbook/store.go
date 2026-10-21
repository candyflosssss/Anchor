package logbook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
	_ "modernc.org/sqlite"
)

const QueueSize = 2048
const BatchSize = 128
const MaxRequestSize = 32 << 10
const MaxEventSize = 4 << 10

type command struct {
	raw      []byte
	request  *Request
	event    *Event
	settings *Settings
	ack      chan error
}
type Store struct {
	write     *sql.DB
	read      *sql.DB
	queue     chan command
	done      chan struct{}
	mu        sync.RWMutex
	closed    bool
	settings  atomic.Pointer[Settings]
	dropped   atomic.Int64
	failures  atomic.Int64
	errorCode atomic.Pointer[string]
	counts    atomic.Pointer[Health]
}

func Open(dir string) (*Store, error) {
	path := filepath.Join(dir, "logs.db")
	if err := platform.SecureDir(dir); err != nil {
		return nil, err
	}
	if err := platform.SecureFile(path); err != nil {
		return nil, err
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p, RawQuery: "_txlock=immediate"}).String()
	w, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	fail := func(e error) (*Store, error) { w.Close(); return nil, e }
	if _, err = w.Exec(`PRAGMA busy_timeout=1000; PRAGMA auto_vacuum=INCREMENTAL; PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA wal_autocheckpoint=256; PRAGMA journal_size_limit=4194304; PRAGMA foreign_keys=ON; PRAGMA cache_size=-8192; PRAGMA max_page_count=262144;`); err != nil {
		return fail(err)
	}
	var version int
	if err = w.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(ErrUnavailable)
	}
	if _, err = w.Exec(schema); err != nil {
		return fail(err)
	}
	settings := DefaultSettings()
	var raw string
	if err = w.QueryRow(`SELECT value FROM meta WHERE key='settings'`).Scan(&raw); err == nil {
		if json.Unmarshal([]byte(raw), &settings) != nil || settings.Validate() != nil {
			return fail(ErrUnavailable)
		}
	} else if err != sql.ErrNoRows {
		return fail(err)
	}
	// A durable start record without an end record cannot be declared successful.
	for {
		result, e := w.Exec(`UPDATE requests SET status='interrupted',error_code='core_interrupted',stage='cleanup',document=json_set(document,'$.status','interrupted','$.code','core_interrupted','$.stage','cleanup','$.delivery','unknown') WHERE seq IN (SELECT seq FROM requests WHERE status='pending' LIMIT 128)`)
		if e != nil {
			return fail(e)
		}
		n, e := result.RowsAffected()
		if e != nil {
			return fail(e)
		}
		if n == 0 {
			break
		}
	}
	rd, err := sql.Open("sqlite", dsn+"&_pragma=busy_timeout(1000)&_pragma=query_only(1)")
	if err != nil {
		return fail(err)
	}
	rd.SetMaxOpenConns(2)
	rd.SetMaxIdleConns(2)
	s := &Store{write: w, read: rd, queue: make(chan command, QueueSize), done: make(chan struct{})}
	s.settings.Store(&settings)
	if err = s.refreshCounts(); err != nil {
		rd.Close()
		return fail(err)
	}
	var dropped, failures int64
	if err = w.QueryRow(`SELECT dropped,failures FROM counters WHERE id=1`).Scan(&dropped, &failures); err != nil {
		rd.Close()
		return fail(err)
	}
	s.dropped.Store(dropped)
	s.failures.Store(failures)
	go s.run()
	return s, nil
}
func (s *Store) submit(c command) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false
	}
	select {
	case s.queue <- c:
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

// Serialize at submission: callers may mutate their trace immediately after returning.
// The queue contains only immutable, size-bounded documents.
func (s *Store) Request(r Request) bool {
	r.fillKnownUsage()
	if len(r.Attempts) > 64 {
		r.Attempts = r.Attempts[:64]
		r.AttemptsTruncated = true
	}
	if r.Attempts == nil {
		r.Attempts = []Attempt{}
	}
	raw, err := json.Marshal(r)
	// Rich reference breakdowns may exceed the per-record budget on many retries.
	// Preserve root totals and explicitly truncate attempt detail, never drop the
	// entire request merely because its captured pricing snapshots grew.
	for err == nil && len(raw) > MaxRequestSize && len(r.Attempts) > 0 && r.ReferenceCost != nil && r.ReferenceCost.Adjustment != nil {
		r.Attempts = r.Attempts[:len(r.Attempts)-1]
		r.AttemptsTruncated = true
		raw, err = json.Marshal(r)
	}
	if err != nil || len(raw) > MaxRequestSize {
		s.dropped.Add(1)
		return false
	}
	if r.HTTP != nil {
		v := *r.HTTP
		r.HTTP = &v
	}
	r.Attempts = nil
	r.Evidence = nil
	return s.submit(command{request: &r, raw: raw})
}
func (s *Store) Event(e Event) bool {
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > MaxEventSize {
		s.dropped.Add(1)
		return false
	}
	e.Fields = nil
	return s.submit(command{event: &e, raw: raw})
}
func (s *Store) Settings() Settings { return *s.settings.Load() }
func (s *Store) Health() Health {
	h := *s.counts.Load()
	h.Dropped = s.dropped.Load()
	h.WriteFailures = s.failures.Load()
	h.QueueDepth = len(s.queue)
	if e := s.errorCode.Load(); e != nil {
		h.Error = *e
	}
	return h
}
func (s *Store) Flush(ctx context.Context) error { return s.wait(ctx, command{}) }
func (s *Store) Configure(ctx context.Context, in Settings) error {
	if err := in.Validate(); err != nil {
		return err
	}
	return s.wait(ctx, command{settings: &in})
}
func (s *Store) wait(ctx context.Context, c command) error {
	c.ack = make(chan error, 1)
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrUnavailable
	}
	select {
	case s.queue <- c:
		s.mu.RUnlock()
	case <-ctx.Done():
		s.mu.RUnlock()
		return ctx.Err()
	}
	select {
	case err := <-c.ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Store) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
	}
	s.mu.Unlock()
	<-s.done
	return nil
}
func (s *Store) refreshCounts() error {
	var h Health
	if err := s.write.QueryRow(`SELECT request_count,request_bytes,event_count,event_bytes FROM counters WHERE id=1`).Scan(&h.RequestCount, &h.RequestBytes, &h.EventCount, &h.EventBytes); err != nil {
		return err
	}
	s.counts.Store(&h)
	return nil
}
func (s *Store) run() {
	defer close(s.done)
	defer s.write.Close()
	defer s.read.Close()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	cleanup := time.NewTicker(time.Second)
	defer cleanup.Stop()
	maintenance := time.NewTicker(time.Minute)
	defer maintenance.Stop()
	batch := make([]command, 0, BatchSize)
	var batchErr error
	flush := func() {
		if len(batch) > 0 {
			batchErr = s.writeBatch(batch)
			batch = batch[:0]
		}
	}
	for {
		select {
		case c, ok := <-s.queue:
			if !ok {
				flush()
				s.persistHealth()
				return
			}
			if c.ack != nil {
				flush()
				err := batchErr
				if c.settings != nil {
					err = s.configure(*c.settings)
				}
				c.ack <- err
				continue
			}
			batch = append(batch, c)
			if len(batch) == BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-cleanup.C:
			flush()
			s.clean()
		case <-maintenance.C:
			flush()
			s.persistHealth()
			_, _ = s.write.Exec(`PRAGMA incremental_vacuum(64); PRAGMA wal_checkpoint(TRUNCATE);`)
		}
	}
}
func (s *Store) writeBatch(batch []command) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, err := s.write.BeginTx(ctx, nil)
	if err == nil {
		for _, c := range batch {
			if c.request != nil {
				r := c.request
				raw := c.raw
				_, err = tx.ExecContext(ctx, `INSERT INTO requests(id,created_at,status,source,model,provider,key_id,http,error_code,stage,size,document) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,source=excluded.source,model=excluded.model,provider=excluded.provider,key_id=excluded.key_id,http=excluded.http,error_code=excluded.error_code,stage=excluded.stage,size=excluded.size,document=excluded.document`, r.ID, r.StartedAtMS, r.Status, r.Source, r.Model, r.Provider, r.Key, r.HTTP, r.Code, r.Stage, len(raw)+1024, string(raw))
			} else if c.event != nil {
				e := c.event
				raw := c.raw
				_, err = tx.ExecContext(ctx, `INSERT INTO events(id,created_at,level,source,event,request_id,size,document) VALUES(?,?,?,?,?,?,?,?)`, e.ID, e.AtMS, e.Level, e.Source, e.Event, e.RequestID, len(raw)+512, string(raw))
			}
			if err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		s.failures.Add(1)
		s.dropped.Add(int64(len(batch)))
		code := "write_failed"
		s.errorCode.Store(&code)
		return ErrUnavailable
	}
	s.errorCode.Store(nil)
	s.clean()
	_ = s.refreshCounts()
	return nil
}
func (s *Store) configure(in Settings) error {
	raw, _ := json.Marshal(in)
	if _, err := s.write.Exec(`INSERT INTO meta(key,value) VALUES('settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
		return ErrUnavailable
	}
	s.settings.Store(&in)
	err := s.clean()
	s.persistHealth()
	return err
}
func (s *Store) persistHealth() {
	_, _ = s.write.Exec(`UPDATE counters SET dropped=?,failures=? WHERE id=1`, s.dropped.Load(), s.failures.Load())
}
func (s *Store) clean() (err error) {
	defer func() {
		if err != nil {
			s.failures.Add(1)
			code := "retention_failed"
			s.errorCode.Store(&code)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cfg := s.Settings()
	now := time.Now()
	cutoff := now.Add(-time.Duration(cfg.Days) * 24 * time.Hour).UnixMilli()
	systemCutoff := now.Add(-time.Duration(cfg.SystemDays) * 24 * time.Hour).UnixMilli()
	// Two bounded batches per flush. Retention never walks all history in one transaction.
	for range 2 {
		_, err := s.write.ExecContext(ctx, `DELETE FROM requests WHERE seq IN (SELECT seq FROM requests WHERE created_at<? ORDER BY created_at LIMIT 128)`, cutoff)
		if err != nil {
			return err
		}
		_, err = s.write.ExecContext(ctx, `DELETE FROM events WHERE seq IN (SELECT seq FROM events WHERE created_at<? ORDER BY created_at LIMIT 128)`, systemCutoff)
		if err != nil {
			return err
		}
		if err = s.refreshCounts(); err != nil {
			return err
		}
		h := *s.counts.Load()
		if h.RequestCount > int64(cfg.MaxRequests) || h.RequestBytes > cfg.RequestBytes {
			n := min(int64(128), max(int64(1), h.RequestCount-int64(cfg.MaxRequests)))
			if h.RequestBytes > cfg.RequestBytes {
				n = 128
			}
			if _, err = s.write.ExecContext(ctx, `DELETE FROM requests WHERE seq IN (SELECT seq FROM requests ORDER BY seq LIMIT ?)`, n); err != nil {
				return err
			}
		}
		if h.EventBytes > cfg.SystemBytes || h.EventCount > 10000 {
			if _, err = s.write.ExecContext(ctx, `DELETE FROM events WHERE seq IN (SELECT seq FROM events ORDER BY seq LIMIT 128)`); err != nil {
				return err
			}
		}
	}
	return s.refreshCounts()
}
func (s *Store) Detail(ctx context.Context, id string) (Request, error) {
	if id == "" || len(id) > 128 {
		return Request{}, ErrInvalid
	}
	var raw string
	err := s.read.QueryRowContext(ctx, `SELECT document FROM requests WHERE id=?`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return Request{}, ErrNotFound
	}
	if err != nil {
		return Request{}, ErrUnavailable
	}
	var r Request
	if json.Unmarshal([]byte(raw), &r) != nil {
		return r, ErrUnavailable
	}
	r.fillKnownUsage()
	return r, nil
}
func (s *Store) List(ctx context.Context, q Query) (Page, error) {
	out := Page{Requests: []Request{}, Events: []Event{}, Health: s.Health()}
	if err := q.Validate(); err != nil {
		return out, err
	}
	table := "requests"
	if q.Kind == "system" {
		table = "events"
	}
	if err := s.read.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM `+table).Scan(&out.Latest); err != nil {
		return out, ErrUnavailable
	}
	where, args := logWhere(q, table)
	args = append(args, q.Limit+1)
	rows, err := s.read.QueryContext(ctx, fmt.Sprintf("SELECT seq,document FROM %s WHERE %s ORDER BY seq DESC LIMIT ?", table, strings.Join(where, " AND ")), args...)
	if err != nil {
		return out, ErrUnavailable
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var seq int64
		var raw string
		if rows.Scan(&seq, &raw) != nil {
			return out, ErrUnavailable
		}
		count++
		if count > q.Limit {
			out.HasMore = true
			break
		}
		out.NextCursor = seq
		if table == "requests" {
			var r Request
			if json.Unmarshal([]byte(raw), &r) != nil {
				return out, ErrUnavailable
			}
			r.fillKnownUsage()
			out.Requests = append(out.Requests, r)
		} else {
			var e Event
			if json.Unmarshal([]byte(raw), &e) != nil {
				return out, ErrUnavailable
			}
			out.Events = append(out.Events, e)
		}
	}
	if err = rows.Err(); err != nil {
		return out, ErrUnavailable
	}
	if !out.HasMore {
		out.NextCursor = 0
	}
	return out, nil
}

// logWhere keeps list and full-range statistics on identical filter semantics.
func logWhere(q Query, table string) ([]string, []any) {
	where := []string{"created_at>=?"}
	args := []any{q.SinceMS}
	add := func(clause string, v any) { where = append(where, clause); args = append(args, v) }
	if q.BeforeMS > 0 {
		add("created_at<=?", q.BeforeMS)
	}
	if q.Cursor > 0 {
		add("seq<?", q.Cursor)
	}
	if q.Until > 0 {
		add("seq<=?", q.Until)
	}
	for _, v := range []struct{ col, val string }{{"source", q.Source}} {
		if v.val != "" {
			add(v.col+"=?", v.val)
		}
	}
	if table == "requests" {
		if q.ErrorsOnly {
			where = append(where, "status IN ('failed','interrupted')")
		}
		for _, v := range []struct{ col, val string }{{"status", q.Status}, {"provider", q.Provider}, {"model", q.Model}, {"key_id", q.Key}, {"error_code", q.Code}, {"stage", q.Stage}} {
			if v.val != "" {
				add(v.col+"=?", v.val)
			}
		}
		for _, dimension := range []struct {
			name  string
			value *string
		}{{"agent", q.Agent}, {"binding", q.Binding}, {"project", q.Project}, {"session", q.Session}} {
			if dimension.value != nil {
				add(dimensionColumn(dimension.name)+"=?", *dimension.value)
			}
		}
		if q.HTTP != nil {
			add("http=?", *q.HTTP)
		}
		if q.Search != "" {
			// UNION uses indexed prefix ranges rather than a body/JSON search or leading wildcard.
			var parts []string
			for _, col := range []string{"id", "model", "provider", "key_id", "error_code", dimensionColumn("agent"), dimensionColumn("binding"), dimensionColumn("project"), dimensionColumn("session")} {
				parts = append(parts, "SELECT seq FROM requests WHERE "+col+">=? AND "+col+"<?")
				args = append(args, q.Search, q.Search+"\U0010ffff")
			}
			where = append(where, "seq IN ("+strings.Join(parts, " UNION ")+")")
		}
	} else {
		if q.Level != "" {
			add("level=?", q.Level)
		}
		if q.Search != "" {
			add("event>=?", q.Search)
			add("event<?", q.Search+"\U0010ffff")
		}
	}
	return where, args
}
