package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/state"
)

func localizedRequest(t *testing.T, method, url, token, language, body string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept-Language", language)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestAPIErrorsNegotiateBeforeAuthAndRemainRequestScoped(t *testing.T) {
	f := setup(t)
	for _, endpoint := range []string{f.gateway.URL + "/v1/models", f.control.URL + "/control/v1/status"} {
		for _, language := range []string{"zh-CN", "en", "zh;q=0.8,en;q=0.1", "fr", ""} {
			resp := localizedRequest(t, "GET", endpoint, "", language, "")
			var body struct {
				Error apierror.Detail `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			lang := apierror.Negotiate(language)
			_, want := apierror.Render(apierror.Unauthorized, lang)
			want.Language = ""
			if resp.StatusCode != 401 || body.Error != want || resp.Header.Get("Content-Language") != string(lang) || resp.Header.Get("Vary") != "Accept-Language" {
				t.Fatalf("%s: status=%d headers=%v error=%+v", language, resp.StatusCode, resp.Header, body.Error)
			}
		}
	}
}

func TestLocalizedBindingRejectionKeepsLogCodeAndStopsBeforeUpstream(t *testing.T) {
	f := setup(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.store.AddBinding(state.Binding{ID: "language-test", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "allowed", ModelScope: "single", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "zh-CN"} {
		resp := localizedRequest(t, "POST", f.gateway.URL+"/v1/responses", token, lang, `{"model":"denied","input":"private-input"}`)
		var body struct {
			Error apierror.Detail `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_, want := apierror.Render(apierror.BindingModelDenied, apierror.Language(lang))
		want.Language = ""
		if resp.StatusCode != 403 || body.Error != want || resp.Header.Get("Content-Language") != lang {
			t.Fatalf("%s: status=%d error=%+v", lang, resp.StatusCode, body.Error)
		}
		if err := f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
		if err != nil || row.Code != "binding_model_denied" || row.AttemptCount != 0 || row.Status != "failed" {
			t.Fatalf("language changed logging/dispatch: %+v %v", row, err)
		}
	}
}

func TestLanguagePreferenceDoesNotRewriteUpstreamSSE(t *testing.T) {
	f := setup(t)
	const upstreamError = "event: error\ndata: {\"type\":\"error\",\"code\":\"remote_error\",\"message\":\"Upstream original message\"}\n\n"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, upstreamError)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "fixture")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	resp := localizedRequest(t, "POST", f.gateway.URL+"/v1/responses", f.token, "zh-CN", `{"model":"public","input":"fixture","stream":true}`)
	if raw := text(t, resp); resp.StatusCode != 200 || raw != upstreamError || resp.Header.Get("Content-Language") != "" {
		t.Fatalf("upstream error was rewritten: %d %q headers=%v", resp.StatusCode, raw, resp.Header)
	}
}
