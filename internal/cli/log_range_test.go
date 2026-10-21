package cli

import (
	"flag"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
)

func TestLogFixedRange(t *testing.T) {
	const from = "2026-10-21T08:00:00+08:00"
	const to = "2026-10-21T08:00:01.001+08:00"
	for _, stats := range []bool{false, true} {
		f := flag.NewFlagSet("range", flag.ContinueOnError)
		q := logbook.Query{}
		if err := logRange(f, &q, time.Hour, from, to, stats); err != nil {
			t.Fatal(err)
		}
		want := int64(1000)
		if stats {
			want++
		}
		if q.BeforeMS-q.SinceMS != want {
			t.Fatalf("%+v", q)
		}
	}
	for _, pair := range [][2]string{{from, ""}, {"", to}, {to, from}, {from, from}, {"bad", to}, {from, "2027-10-21T08:00:00+08:00"}} {
		if logRange(flag.NewFlagSet("range", flag.ContinueOnError), &logbook.Query{}, time.Hour, pair[0], pair[1], false) == nil {
			t.Fatal(pair)
		}
	}
	f := flag.NewFlagSet("range", flag.ContinueOnError)
	f.Duration("since", time.Hour, "")
	_ = f.Parse([]string{"--since", "1h"})
	if logRange(f, &logbook.Query{}, time.Hour, from, to, false) == nil {
		t.Fatal("conflicting since accepted")
	}
}

func TestLogFixedRangeSubMillisecondBounds(t *testing.T) {
	const from = "2026-10-21T08:00:00.000000001Z"
	const to = "2026-10-21T08:00:00.001000001Z"
	base, _ := time.Parse(time.RFC3339, "2026-10-21T08:00:00Z")
	for _, stats := range []bool{false, true} {
		q := logbook.Query{}
		if err := logRange(flag.NewFlagSet("range", flag.ContinueOnError), &q, time.Hour, from, to, stats); err != nil {
			t.Fatal(err)
		}
		wantEnd := base.UnixMilli() + 1
		if stats {
			wantEnd++
		}
		if q.SinceMS != base.UnixMilli()+1 || q.BeforeMS != wantEnd {
			t.Fatalf("sub-millisecond range includes the wrong timestamps: %+v", q)
		}
	}
	if logRange(flag.NewFlagSet("range", flag.ContinueOnError), &logbook.Query{}, time.Hour, from, "2026-10-21T08:00:00.000999999Z", true) == nil {
		t.Fatal("range containing no millisecond timestamps accepted")
	}
}
