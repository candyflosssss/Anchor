package cli

import (
	"context"
	"errors"
	"flag"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"time"
)

func (a App) logs(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("logs requires list, stats, suggestions, detail, follow, export, settings or configure")
	}
	switch args[0] {
	case "suggestions":
		f := a.flags("logs suggestions")
		q := logbook.SuggestionsQuery{}
		f.StringVar(&q.Kind, "kind", "requests", "requests|system (system supports only search)")
		f.StringVar(&q.Field, "field", "search", "search|provider|model|key|http|code|stage|agent|binding|project|session")
		f.StringVar(&q.Prefix, "prefix", "", "case-sensitive value prefix")
		f.IntVar(&q.Limit, "limit", 20, "maximum candidates (1..100)")
		since := f.Duration("since", time.Hour, "lookback duration (up to 2160h)")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *since <= 0 || *since > 90*24*time.Hour {
			return errors.New("since must be in (0,90d]")
		}
		now := time.Now()
		q.SinceMS, q.BeforeMS = now.Add(-*since).UnixMilli(), now.UnixMilli()
		if q.Validate() != nil {
			return logbook.ErrInvalid
		}
		v, err := c.LogSuggestions(ctx, q)
		if err != nil {
			return err
		}
		return a.output(v)
	case "detail":
		if len(args) != 2 {
			return errors.New("logs detail requires request ID")
		}
		v, err := c.LogDetail(ctx, args[1])
		if err != nil {
			return err
		}
		return a.output(v)
	case "settings":
		if len(args) != 1 {
			return errors.New("logs settings takes no arguments")
		}
		v, err := c.LogSettings(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	case "configure":
		v, err := c.LogSettings(ctx)
		if err != nil {
			return err
		}
		f := a.flags("logs configure")
		f.IntVar(&v.Days, "days", v.Days, "request retention days (1..90)")
		f.IntVar(&v.SystemDays, "system-days", v.SystemDays, "system retention days (1..90)")
		f.IntVar(&v.MaxRequests, "max-requests", v.MaxRequests, "maximum retained requests (100..200000)")
		req := f.Int64("request-mib", v.RequestBytes>>20, "request storage budget (1..512 MiB)")
		sys := f.Int64("system-mib", v.SystemBytes>>20, "system storage budget (1..64 MiB)")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		v.RequestBytes = *req << 20
		v.SystemBytes = *sys << 20
		v, err = c.ConfigureLogs(ctx, v)
		if err != nil {
			return err
		}
		return a.output(v)
	case "list", "follow", "export", "stats":
		f := a.flags("logs " + args[0])
		q := logbook.Query{}
		var interval *time.Duration
		var groupBy string
		if args[0] == "stats" {
			f.StringVar(&groupBy, "group-by", "", "agent|binding|project|session (top 100 groups)")
			interval = f.Duration("interval", 0, "bucket interval: 10s|30s|1m|5m|15m|1h|6h|24h (default auto, max 600 buckets)")
		}
		var agent, binding, project, session string
		f.StringVar(&agent, "agent", "", "exact authenticated Agent type; empty selects unattributed")
		f.StringVar(&binding, "binding", "", "exact enrolled binding ID; empty selects unattributed")
		f.StringVar(&project, "project", "", "exact client-declared project ID; empty selects unattributed")
		f.StringVar(&session, "session", "", "exact client-declared session ID; empty selects unattributed")
		f.StringVar(&q.Kind, "kind", "requests", "requests|system")
		f.IntVar(&q.Limit, "limit", 50, "page size (1..200)")
		f.Int64Var(&q.Cursor, "cursor", 0, "older-page cursor")
		f.Int64Var(&q.Until, "until", 0, "snapshot sequence upper bound")
		since := f.Duration("since", time.Hour, "lookback duration (for example 24h)")
		f.StringVar(&q.Status, "status", "", "request result")
		f.StringVar(&q.Source, "source", "", "proxy|probe or event source")
		f.StringVar(&q.Provider, "provider", "", "provider ID")
		f.StringVar(&q.Model, "model", "", "public model")
		f.StringVar(&q.Key, "key", "", "key ID")
		f.StringVar(&q.Code, "code", "", "error code")
		f.StringVar(&q.Stage, "stage", "", "failure stage")
		f.StringVar(&q.Level, "level", "", "event severity")
		f.StringVar(&q.Search, "search", "", "case-sensitive identifier prefix")
		httpStatus := f.Int("http", 0, "actual upstream HTTP status")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *since <= 0 || *since > 90*24*time.Hour {
			return errors.New("since must be in (0,90d]")
		}
		now := time.Now()
		q.SinceMS = now.Add(-*since).UnixMilli()
		if args[0] == "stats" {
			q.BeforeMS = now.UnixMilli()
		}
		f.Visit(func(flag *flag.Flag) {
			switch flag.Name {
			case "agent":
				q.Agent = &agent
			case "binding":
				q.Binding = &binding
			case "project":
				q.Project = &project
			case "session":
				q.Session = &session
			}
		})
		if *httpStatus != 0 {
			q.HTTP = httpStatus
		}
		if q.Validate() != nil {
			return logbook.ErrInvalid
		}
		if args[0] == "stats" {
			if *interval < 0 || *interval%time.Millisecond != 0 {
				return logbook.ErrInvalid
			}
			query := logbook.StatsQuery{Filter: q, IntervalMS: interval.Milliseconds(), GroupBy: groupBy}
			if query.Validate() != nil {
				return logbook.ErrInvalid
			}
			v, err := c.LogStats(ctx, query)
			if err != nil {
				return err
			}
			return a.output(v)
		}
		if args[0] == "export" {
			return c.ExportLogs(ctx, q, a.Out)
		}
		for {
			v, err := c.Logs(ctx, q)
			if err != nil {
				return err
			}
			if err = a.output(v); err != nil {
				return err
			}
			if args[0] != "follow" {
				return nil
			}
			// Re-query the newest bounded page: pending records can be updated in place.
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	default:
		return errors.New("unknown logs command")
	}
}
