package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestInitIdempotentAndLocalOnly(t *testing.T) {
	dir := t.TempDir()
	var out, diagnostics bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
	if e := a.Run(context.Background(), []string{"init", "--data-dir", dir}); e != nil {
		t.Fatal(e)
	}
	first := out.String()
	if strings.Contains(first, "token") {
		t.Fatal("init leaked credential")
	}
	out.Reset()
	if e := a.Run(context.Background(), []string{"--data-dir", dir, "init"}); e != nil {
		t.Fatal(e)
	}
	if first != out.String() {
		t.Fatal("init rotated state")
	}
	if e := a.Run(context.Background(), []string{"--data-dir", dir, "--context", "remote", "serve"}); e == nil {
		t.Fatal("context accepted for local-only serve")
	}
}
func TestCLIManagementWorkflow(t *testing.T) {
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
	host := strings.TrimPrefix(web.URL, "http://")
	if _, e = s.Initialize(host, "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	run := func(input string, args ...string) string {
		t.Helper()
		var out, errout bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: &errout}
		args = append([]string{"--data-dir", dir}, args...)
		if e := a.Run(context.Background(), args); e != nil {
			t.Fatal(args, e, errout.String())
		}
		if errout.Len() != 0 {
			t.Fatal("unexpected diagnostics", errout.String())
		}
		var v any
		if e := json.Unmarshal(out.Bytes(), &v); e != nil {
			t.Fatal("stdout is not JSON", out.String())
		}
		return out.String()
	}
	run("", "context", "add", "local", "--local")
	run("", "context", "use", "local")
	if status := run("", "status", "--json"); !strings.Contains(status, `"core":"ready"`) {
		t.Fatal(status)
	}
	out := run("provider-secret\n", "provider", "add", "--id", "p", "--base-url", "https://example.invalid/v1", "--secret-stdin")
	if strings.Contains(out, "provider-secret") {
		t.Fatal("provider write leaked secret")
	}
	if out := run("", "logs", "list"); !strings.Contains(out, `"requests":[]`) {
		t.Fatal(out)
	}
	if out := run("", "logs", "settings"); !strings.Contains(out, `"max_requests":50000`) {
		t.Fatal(out)
	}
	if out := run("", "logs", "configure", "--days", "14"); !strings.Contains(out, `"days":14`) {
		t.Fatal(out)
	}
	run("", "model", "add", "--id", "public", "--candidate", "p:actual", "--tools", "--streaming")
	// Chinese IDs must survive CLI parsing, URL encoding, API checks and SQLite.
	for _, id := range []string{"摸鱼AI", strings.Repeat("鱼", 128)} {
		out := run("chinese-provider-secret", "provider", "add", "--id", id, "--base-url", "https://example.invalid/v1", "--secret-stdin")
		var created state.Provider
		if err := json.Unmarshal([]byte(out), &created); err != nil || created.ID != id || created.Revision != 1 || !created.HasSecret || strings.Contains(out, "chinese-provider-secret") {
			t.Fatal("Chinese provider add failed or leaked secret", err)
		}
		out = run("", "provider", "put", "--id", id, "--base-url", "https://updated.invalid/v1", "--revision", "1")
		var updated state.Provider
		if err := json.Unmarshal([]byte(out), &updated); err != nil || updated.ID != id || updated.Revision != 2 || !updated.HasSecret {
			t.Fatal("Chinese provider update failed", err)
		}
		stored, err := s.Provider(id)
		if err != nil || stored.Secret != "chinese-provider-secret" {
			t.Fatal("Chinese provider update lost secret", err)
		}
	}
	out = run("", "provider", "add", "--id", "multi", "--homepage-url", "https://www.example.invalid", "--base-url", "https://a.example.invalid/v1", "--base-url", "https://b.example.invalid/v1", "--protocol", "anthropic")
	var multi state.Provider
	if err := json.Unmarshal([]byte(out), &multi); err != nil || multi.HomepageURL != "https://www.example.invalid" || !reflect.DeepEqual(multi.BaseURLs, []string{"https://a.example.invalid/v1", "https://b.example.invalid/v1"}) || multi.BaseURL != multi.BaseURLs[0] || multi.HasSecret {
		t.Fatal("multi-URL provider add failed", err, out)
	}
	if stored, err := s.Provider("multi"); err != nil || stored.Protocol != "anthropic" {
		t.Fatal("provider protocol seed was not stored", err)
	}
	out = run("", "provider", "put", "--id", "multi", "--base-url", "https://c.example.invalid/v1", "--revision", "1")
	if stored, err := s.Provider("multi"); err != nil || stored.Protocol != "anthropic" || !reflect.DeepEqual(stored.URLs(), []string{"https://c.example.invalid/v1"}) || stored.HomepageURL != "" {
		t.Fatal("put without --protocol must keep the stored protocol", err, out)
	}
	shared := run("", "provider", "model-put", "--id", "p", "--revision", "1", "--target", "shared", "--model", "up-p", "--multiplier", "0.5", "--strategy", "round-robin")
	if !strings.Contains(shared, `"multiplier":0.5`) {
		t.Fatal(shared)
	}
	run("", "provider", "model-put", "--id", "multi", "--revision", "2", "--target", "shared", "--model", "up-multi", "--multiplier", "2")
	if m, err := s.Model("shared"); err != nil || len(m.Candidates) != 2 || m.Strategy != "round-robin" {
		t.Fatal(m, err)
	}
	run("", "provider", "model-delete", "--id", "p", "--revision", "1", "--target", "shared", "--model-revision", "2", "--approve")
	if m, err := s.Model("shared"); err != nil || len(m.Candidates) != 1 || m.Candidates[0].ProviderID != "multi" {
		t.Fatal(m, err)
	}
	run("", "model", "routing", "--id", "shared", "--revision", "3", "--strategy", "lowest-latency", "--candidate", "multi:up-multi")
	if m, err := s.Model("shared"); err != nil || m.Strategy != "lowest-latency" || m.Revision != 4 {
		t.Fatal(m, err)
	}
	run("", "model", "add", "--id", "key-scoped", "--candidate", "p:actual", "--candidate-key", "1:default", "--strategy", "cache-priority")
	if m, err := s.Model("key-scoped"); err != nil || len(m.Candidates[0].KeyIDs) != 1 || m.Strategy != "cache-priority" {
		t.Fatal(m, err)
	}
	run("", "model", "add", "--id", "chinese-provider-model", "--candidate", "摸鱼AI:actual", "--tools", "--streaming")
	run("", "model", "put", "--id", "chinese-provider-model", "--candidate", "摸鱼AI:updated-model", "--revision", "1", "--tools", "--streaming")
	listed := run("", "provider", "list")
	if !strings.Contains(listed, "摸鱼AI") || strings.Contains(listed, "chinese-provider-secret") {
		t.Fatal("Chinese provider list failed or leaked secret")
	}

	run("", "provider", "list")
	run("", "model", "list")
	run("", "context", "test", "local")
	grant := run("", "access", "grant", "--label", "diagnostic", "--scope", "gateway", "--show-once")
	var data struct {
		Access state.Access `json:"access"`
		Token  string       `json:"token"`
	}
	if e = json.Unmarshal([]byte(grant), &data); e != nil || data.Token == "" {
		t.Fatal("grant missing token", e)
	}
	if listed := run("", "access", "list"); strings.Contains(listed, data.Token) {
		t.Fatal("access list leaked token")
	}
	run("", "access", "revoke", data.Access.ID)
	if s.Authenticate(data.Token, "gateway") {
		t.Fatal("CLI revoke did not work")
	}
}
func TestCLIRequiresExplicitSecretDisclosure(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if e = a.Run(context.Background(), []string{"--data-dir", dir, "access", "grant", "--label", "test", "--scope", "gateway"}); e == nil {
		t.Fatal("grant disclosed without explicit flag")
	}
	if out.Len() != 0 {
		t.Fatal("failed grant printed token")
	}
}
func TestCLIUnknownContextDoesNotFallback(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	a := App{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}
	if e = a.Run(context.Background(), []string{"--data-dir", dir, "--context", "unknown", "status"}); e == nil {
		t.Fatal("unknown context fell back")
	}
}
func TestCoreRunAndCancelClosesListeners(t *testing.T) {
	// Reserve ephemeral ports only long enough to choose the test addresses.
	free := func() string {
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		address := ln.Addr().String()
		ln.Close()
		return address
	}
	s, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	control, gateway := free(), free()
	if _, e = s.Initialize(control, gateway); e != nil {
		t.Fatal(e)
	}
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = srv.Run(ctx); e != nil {
		t.Fatal(e)
	}
	for _, address := range []string{control, gateway} {
		ln, e := net.Listen("tcp", address)
		if e != nil {
			t.Fatal("listener leaked", address, e)
		}
		ln.Close()
	}
}

func TestVersionWithoutInitializedInstance(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"--data-dir", dir, "version"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(out.Bytes(), &value); err != nil || value["version"] != core.Version || len(value) != 1 {
		t.Fatal("version output must match the linked build version", out.String(), err)
	}
}

func TestHelpDocumentsChineseNames(t *testing.T) {
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{"including Chinese", "Key, public model and Context names", "64 UTF-8 byte limit"} {
		if !strings.Contains(out.String(), hint) {
			t.Fatal("name help missing", hint)
		}
	}
}

func TestProviderModelsCLI(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := core.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer server.Close()
	control := httptest.NewServer(server.Control())
	defer control.Close()
	if _, err = st.Initialize(strings.TrimPrefix(control.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var rootHTML atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/v1/sub2api/billing" {
			if r.Header.Get("Authorization") != "Bearer synthetic-selected" && r.Header.Get("Authorization") != "Bearer synthetic-second" {
				t.Error("incorrect billing probe auth")
			}
			w.WriteHeader(404)
			return
		}
		if (r.URL.Path != "/models" && !(rootHTML.Load() && r.URL.Path == "/v1/models")) || r.Method != "GET" || (calls.Load() > 1 && r.Header.Get("Authorization") != "Bearer synthetic-selected" && r.Header.Get("Authorization") != "Bearer synthetic-second") {
			t.Error("unexpected request")
		}
		if rootHTML.Load() && r.URL.Path == "/models" {
			io.WriteString(w, `<!doctype html><html>site frontend</html>`)
			return
		}
		io.WriteString(w, `{"data":[{"id":"cli-model"}]}`)
	}))
	defer upstream.Close()
	run := func(args ...string) (string, error) {
		var out, diagnostics bytes.Buffer
		err := (App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}).Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		return out.String(), err
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}, {"provider", "add", "--id", "中文目录", "--base-url", upstream.URL, "--disabled"}} {
		if _, err = run(args...); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := run("provider", "models", "--id", "中文目录", "--revision", "1")
	if err != nil {
		t.Fatal(err)
	}
	var result catalog.ProviderModels
	if json.Unmarshal([]byte(raw), &result) != nil || result.HTTPStatus != 200 || result.Failure != "" || len(result.Models) != 1 || result.Models[0] != "cli-model" {
		t.Fatal(raw)
	}
	for _, args := range [][]string{{"provider", "models", "--id", "中文目录"}, {"provider", "models", "--id", "中文目录", "--revision", "2"}, {"provider", "models", "--id", "中文目录", "--revision", "1", "--secret-stdin"}} {
		if _, err = run(args...); err == nil {
			t.Fatal("accepted invalid query", args)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected upstream request count", calls.Load())
	}
	saved, _ := st.Provider("中文目录")
	if saved.Revision != 1 || saved.Enabled {
		t.Fatal("query changed config")
	}
	if err = st.MutateProvider(saved.ID, 1, "key-put", "selected", "synthetic-selected", "", true, false, "chat-completions"); err != nil {
		t.Fatal(err)
	}
	raw, err = run("provider", "models", "--id", saved.ID, "--revision", "2", "--key", "selected")
	if err != nil || json.Unmarshal([]byte(raw), &result) != nil || result.KeyID != "selected" || result.Failure != "" {
		t.Fatal(raw, err)
	}
	if _, err = run("provider", "models", "--id", saved.ID, "--revision", "2", "--key", "missing"); err == nil || calls.Load() != 3 {
		t.Fatal("missing key was queried", err)
	}
	rootHTML.Store(true)
	raw, err = run("provider", "models", "--id", saved.ID, "--revision", "2", "--key", "selected")
	if err != nil || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != "" || result.KeyID != "selected" || len(result.Models) != 1 || result.Models[0] != "cli-model" || calls.Load() != 6 {
		t.Fatal("CLI did not resolve root HTML through /v1/models", raw, err)
	}
	if err = st.MutateProvider(saved.ID, 2, "key-put", "second", "synthetic-second", "", false, false, "responses"); err != nil {
		t.Fatal(err)
	}
	raw, err = run("provider", "models", "--id", saved.ID, "--revision", "3")
	if err != nil || json.Unmarshal([]byte(raw), &result) != nil || result.KeyID != "" || result.Failure != "" || len(result.Models) != 1 || len(result.Entries) != 1 || len(result.Entries[0].KeyIDs) != 1 || result.Entries[0].KeyIDs[0] != "selected" || len(result.Keys) != 1 || !result.Keys[0].Enabled || calls.Load() != 9 {
		t.Fatal("CLI did not skip disabled keys", raw, err)
	}
	for _, keyArg := range [][]string{{"--key", "second"}, {}} {
		if len(keyArg) == 0 {
			if err = st.MutateProvider(saved.ID, 3, "key-put", "selected", "", "", false, false); err != nil {
				t.Fatal(err)
			}
		}
		revision := "3"
		if len(keyArg) == 0 {
			revision = "4"
		}
		raw, err = run(append([]string{"provider", "models", "--id", saved.ID, "--revision", revision}, keyArg...)...)
		if err != nil || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != "no_enabled_keys" || len(result.Models) != 0 || len(result.Keys) != 0 || calls.Load() != 9 {
			t.Fatal("CLI queried disabled keys or fell back to anonymous", raw, err)
		}
	}

}

func TestKeyProtocolCLI(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := core.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer server.Close()
	control := httptest.NewServer(server.Control())
	defer control.Close()
	if _, err = st.Initialize(strings.TrimPrefix(control.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(input string, args ...string) (string, error) {
		var out, diagnostics bytes.Buffer
		err := (App{In: strings.NewReader(input), Out: &out, Err: &diagnostics}).Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		return out.String(), err
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}, {"provider", "add", "--id", "p", "--base-url", "http://127.0.0.1:1"}} {
		if _, err = run("", args...); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := run("synthetic-selected", "provider", "key-put", "--id", "p", "--revision", "1", "--target", "主力密钥", "--secret-stdin", "--protocol", "anthropic")
	var w catalog.Workspace
	if err != nil || json.Unmarshal([]byte(raw), &w) != nil || len(w.Keys) != 1 || w.Keys[0].Protocol != "anthropic" || strings.Contains(raw, "synthetic-selected") {
		t.Fatal(raw, err)
	}
	raw, err = run("", "provider", "key-model-protocol", "--id", "p", "--revision", "2", "--target", "主力密钥", "--model", "actual/model", "--protocol", "chat-completions")
	if err != nil || json.Unmarshal([]byte(raw), &w) != nil || len(w.Keys[0].ModelProtocols) != 1 || w.Keys[0].ModelProtocols[0].Model != "actual/model" {
		t.Fatal(raw, err)
	}
	for _, args := range [][]string{
		{"provider", "key-model-protocol", "--id", "p", "--revision", "2", "--target", "主力密钥", "--model", "actual/model", "--protocol", "inherit"},
		{"provider", "key-model-protocol", "--id", "p", "--revision", "3", "--target", "主力密钥", "--model", "actual/model", "--protocol", "bad"},
		{"provider", "key-model-protocol", "--id", "p", "--revision", "3", "--target", "missing", "--model", "actual/model"},
	} {
		if _, err = run("", args...); err == nil {
			t.Fatal("invalid write succeeded", args)
		}
	}
	raw, err = run("", "provider", "key-model-protocol", "--id", "p", "--revision", "3", "--target", "主力密钥", "--model", "actual/model", "--protocol", "inherit")
	if err != nil || json.Unmarshal([]byte(raw), &w) != nil || len(w.Keys[0].ModelProtocols) != 0 || w.Keys[0].Protocol != "anthropic" {
		t.Fatal(raw, err)
	}
	var help bytes.Buffer
	if err = (App{Out: &help, Err: io.Discard}).Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"key-model-protocol", "[--key KEY]", "[--protocol PROTOCOL]"} {
		if !strings.Contains(help.String(), text) {
			t.Fatal("help missing", text)
		}
	}
}
