package cli

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestPlatformCLIWorkflowAndCredentials(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, e = s.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	billingEnabled := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			if r.Header.Get("Authorization") != "" {
				t.Error("public probe credential")
			}
			io.WriteString(w, `{"success":true,"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false,"quota_display_type":"USD"}}`)
		case "/api/user/self":
			if r.Header.Get("Authorization") != "Bearer cli-user-fixture" {
				t.Error("incorrect dashboard token")
			}
			io.WriteString(w, `{"success":true,"data":{"quota":345,"used_quota":12}}`)
		case "/v1/dashboard/billing/subscription", "/v1/dashboard/billing/usage":
			if r.Header.Get("Authorization") != "Bearer cli-model-fixture" {
				t.Error("incorrect billing key")
			}
			if !billingEnabled {
				w.WriteHeader(404)
				return
			}
			if strings.HasSuffix(r.URL.Path, "subscription") {
				io.WriteString(w, `{"hard_limit_usd":10.5}`)
			} else {
				io.WriteString(w, `{"total_usage":125}`)
			}
		case "/api/usage/token/":
			if r.Header.Get("Authorization") != "Bearer cli-model-fixture" {
				t.Error("incorrect inference key")
			}
			io.WriteString(w, `{"code":true,"data":{"total_available":25,"total_used":5,"total_granted":30,"unlimited_quota":false}}`)
		case "/api/token/":
			io.WriteString(w, `{"success":true,"data":{"items":[{"id":42,"key":"cli-model-fixture"}],"total":1}}`)
		case "/api/token/42":
			io.WriteString(w, `{"success":true,"data":{"group":"vip"}}`)
		case "/api/pricing":
			io.WriteString(w, `{"success":true,"group_ratio":{"vip":0.80000000000000001}}`)
		case "/api/log/token":
			io.WriteString(w, `{"success":true,"data":[]}`)
		default:
			t.Error("unexpected upstream", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	run := func(input string, wantErr bool, args ...string) string {
		t.Helper()
		var out, diag bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: &diag}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (err != nil) != wantErr {
			t.Fatalf("%v err=%v", args, err)
		}
		if strings.Contains(out.String(), "cli-user-fixture") || strings.Contains(out.String(), "cli-model-fixture") {
			t.Fatal("CLI leaked credential")
		}
		return out.String()
	}
	run("", false, "context", "add", "local", "--local")
	run("", false, "context", "use", "local")
	run("cli-model-fixture\n", false, "provider", "add", "--id", "p", "--base-url", up.URL+"/v1", "--secret-stdin")
	gotAuto := run("", false, "provider", "detect-platform", "--id", "p", "--revision", "1")
	if !strings.Contains(gotAuto, `"platform":"newapi"`) {
		t.Fatal(gotAuto)
	}
	summary := run("", false, "provider", "balance-summary", "--id", "p", "--revision", "1")
	if !strings.Contains(summary, `"scope":"key"`) || !strings.Contains(summary, `"value":25`) {
		t.Fatal(summary)
	}
	billingEnabled = true
	for _, op := range []string{"balance", "balance-summary"} {
		got := run("", false, "provider", op, "--id", "p", "--revision", "1")
		if !strings.Contains(got, `"scope":"billing"`) || !strings.Contains(got, `"value":9.25`) || !strings.Contains(got, `"unit":"USD"`) {
			t.Fatal(got)
		}
	}
	run("", true, "provider", "platform-put", "--id", "p", "--revision", "1", "--platform", "newapi", "--user-token-stdin", "--clear-user-token")
	run("\n", true, "provider", "platform-put", "--id", "p", "--revision", "1", "--platform", "newapi", "--user-token-stdin")
	got := run("cli-user-fixture\n", false, "provider", "platform-put", "--id", "p", "--revision", "1", "--platform", "newapi", "--user-id", "9", "--user-token-stdin")
	if !strings.Contains(got, `"has_user_token":true`) || !strings.Contains(got, `"revision":2`) {
		t.Fatal(got)
	}
	run("", false, "provider", "platform", "--id", "p")
	got = run("", false, "provider", "balance", "--id", "p", "--revision", "2")
	if !strings.Contains(got, `"value":345`) || !strings.Contains(got, `"unit":"quota"`) {
		t.Fatal(got)
	}
	got = run("", false, "provider", "key-quota", "--id", "p", "--revision", "2")
	if !strings.Contains(got, `"value":25`) {
		t.Fatal(got)
	}
	got = run("", false, "provider", "usage-records", "--id", "p", "--revision", "2", "--scope", "key")
	if !strings.Contains(got, `"history":"recent"`) {
		t.Fatal(got)
	}
	got = run("", false, "provider", "effective-rate", "--id", "p", "--revision", "2")
	if !strings.Contains(got, `"multiplier":0.80000000000000001`) {
		t.Fatal(got)
	}
	got = run("", false, "provider", "effective-rate", "--id", "p", "--revision", "2", "--remote-key-id", "42")
	if !strings.Contains(got, `"group":"vip"`) {
		t.Fatal(got)
	}
	run("", true, "provider", "balance", "--id", "p", "--revision", "1")
	run("", true, "provider", "usage-records", "--id", "p", "--revision", "2", "--page", "0")
	run("", false, "provider", "platform-put", "--id", "p", "--revision", "2", "--platform", "newapi", "--clear-user-token")
	got = run("", false, "provider", "balance", "--id", "p", "--revision", "3")
	if !strings.Contains(got, `"scope":"billing"`) || !strings.Contains(got, `"value":9.25`) {
		t.Fatal(got)
	}
	got = run("", false, "provider", "platform-put", "--id", "p", "--revision", "3")
	if !strings.Contains(got, `"platform":"auto"`) {
		t.Fatal(got)
	}
}
