package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestTransportCausesDoNotExposeWrappedErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"}, {context.Canceled, "cancelled"},
		{&net.DNSError{Err: "private-text", Name: "private-host"}, "dns"},
		{syscall.ECONNRESET, "connection_reset"}, {syscall.ECONNREFUSED, "connection_refused"},
		{io.ErrUnexpectedEOF, "unexpected_eof"}, {errors.New("private-text"), "transport"},
	} {
		err := &url.Error{Op: "Post", URL: "https://private-secret@private-host", Err: fmt.Errorf("private-wrapper: %w", tc.err)}
		if got := transportCause(err); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}

func TestTransportTimeoutClassifiedAndLoggedBeforeAnyReplay(t *testing.T) {
	f := setup(t)
	if f.core.egress == nil {
		t.Fatal("missing transport")
	}
	f.add(t, "p", "https://synthetic.invalid", "synthetic-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	calls := 0
	f.core.upstream.Transport = modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
		return nil, &url.Error{Op: "Post", URL: "https://synthetic-secret@private-host", Err: context.DeadlineExceeded}
	})
	resp := localizedRequest(t, "POST", f.gateway.URL+"/v1/responses", f.token, "zh-CN", `{"model":"public","input":"private-input"}`)
	raw := text(t, resp)
	if resp.StatusCode != 504 || !strings.Contains(raw, "upstream_header_timeout") || !strings.Contains(raw, "响应头") || calls != 1 {
		t.Fatalf("%d %s calls=%d", resp.StatusCode, raw, calls)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || row.Evidence["anchor_error_id"] != "upstream_header_timeout" || row.Evidence["transport_cause"] != "timeout" || row.Evidence["transport_phase"] != "response_headers" || row.Evidence["provider_timeout_seconds"] != "5" || row.AttemptCount != 1 || row.HTTP != nil {
		t.Fatalf("%+v %v", row, err)
	}
}

func TestMappedBindingReportsMissingMappingAndDisabledProviders(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://synthetic.invalid", "synthetic-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	p, _ := f.store.Provider("p")
	p.Enabled = false
	if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	b, err := f.store.AddBinding(state.Binding{ID: "diagnostic-test", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		model, code string
		status      int
	}{{"public", "model_no_enabled_provider", 503}, {"missing", "unknown_model", 404}} {
		resp := localizedRequest(t, "POST", f.gateway.URL+"/v1/responses", token, "zh-CN", `{"model":"`+tc.model+`","input":"private-input"}`)
		if raw := text(t, resp); resp.StatusCode != tc.status || !strings.Contains(raw, tc.code) {
			t.Fatalf("%d %s", resp.StatusCode, raw)
		}
		if err := f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
		if err != nil || row.Model != tc.model || row.AttemptCount != 0 || row.Code != tc.code {
			t.Fatalf("%+v %v", row, err)
		}
	}
}
