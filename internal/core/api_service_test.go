package core

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestAPIServiceLANBoundary(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		name, host, peer, token, origin string
		lan, app                        bool
		want                            int
	}{
		{name: "LAN authorized", host: "192.168.1.10:17862", peer: "192.168.1.20:45000", token: f.token, lan: true, app: true, want: 200},
		{name: "LAN x-api-key", host: "192.168.1.10:17862", peer: "10.0.0.2:45000", token: f.token, lan: true, app: true, want: 200},
		{name: "LAN disabled", host: "192.168.1.10:17862", peer: "192.168.1.20:45000", token: f.token, app: true, want: 403},
		{name: "managed gateway stays local", host: "192.168.1.10:17860", peer: "192.168.1.20:45000", token: f.token, want: 400},
		{name: "public peer", host: "192.168.1.10:17862", peer: "8.8.8.8:45000", token: f.token, lan: true, app: true, want: 403},
		{name: "public Host", host: "8.8.8.8:17862", peer: "192.168.1.20:45000", token: f.token, lan: true, app: true, want: 403},
		{name: "missing Key", host: "192.168.1.10:17862", peer: "192.168.1.20:45000", lan: true, app: true, want: 401},
		{name: "management Key", host: "192.168.1.10:17862", peer: "192.168.1.20:45000", token: f.owner, lan: true, app: true, want: 401},
		{name: "browser Origin", host: "192.168.1.10:17862", peer: "192.168.1.20:45000", token: f.token, origin: "https://example.invalid", lan: true, app: true, want: 403},
		{name: "private IPv6 Host rejected", host: "[fd00::1]:17862", peer: "192.168.1.20:45000", token: f.token, lan: true, app: true, want: 403},
		{name: "DNS Host", host: "private.example:17862", peer: "192.168.1.20:45000", token: f.token, lan: true, app: true, want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tc.host+"/v1/models", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("Origin", tc.origin)
			if tc.token != "" {
				if tc.name == "LAN x-api-key" {
					r.Header.Set("x-api-key", tc.token)
				} else {
					r.Header.Set("Authorization", "Bearer "+tc.token)
				}
			}
			h := f.core.Gateway()
			if tc.app {
				h = f.core.applicationGateway(tc.lan)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestAPIKeyLogIdentityAndRevocation(t *testing.T) {
	f := setup(t)
	a, one, err := f.store.Grant("phone", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, two, err := f.store.Grant("editor", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{one, two} {
		r := httptest.NewRequest("POST", "http://192.168.1.10:17862/v1/responses", strings.NewReader("{"))
		r.RemoteAddr = "192.168.1.20:12345"
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Anchor-Access-Key", "forged")
		w := httptest.NewRecorder()
		f.core.applicationGateway(true).ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := f.core.logs.List(t.Context(), logbook.Query{AccessKey: &a.ID})
	if err != nil || len(rows.Requests) != 1 || rows.Requests[0].AccessLabel != "phone" {
		t.Fatal(rows, err)
	}
	stats, err := f.core.logs.Statistics(t.Context(), logbook.StatsQuery{GroupBy: "access_key"})
	if err != nil || stats.GroupCount != 2 {
		t.Fatal(stats, err)
	}
	candidates, err := f.core.logs.Suggest(t.Context(), logbook.SuggestionsQuery{Field: "access_key"})
	if err != nil || len(candidates.Items) != 2 {
		t.Fatal(candidates, err)
	}
	encoded, _ := json.Marshal(rows)
	for _, secret := range []string{one, two} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("secret leaked")
		}
	}
	if err := f.store.RevokeGatewayKey(b.ID); err != nil {
		t.Fatal(err)
	}
	res := request(t, "GET", f.gateway.URL+"/v1/models", two, "")
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal(res.StatusCode)
	}
}
func TestAPIListenerLifecycleAndConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(strconv.FormatBool(conflict), func(t *testing.T) {
			addresses := []string{}
			reserved := []net.Listener{}
			var occupied net.Listener
			for range 3 {
				ln, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				reserved = append(reserved, ln)
				addresses = append(addresses, ln.Addr().String())
				t.Cleanup(func() { ln.Close() })
			}
			for i, ln := range reserved {
				if i == 2 && conflict {
					occupied = ln
				} else {
					ln.Close()
				}
			}

			st, err := state.Open(t.TempDir(), true)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if _, err = st.Initialize(addresses[0], addresses[1]); err != nil {
				t.Fatal(err)
			}
			_, port, _ := net.SplitHostPort(addresses[2])
			n, _ := strconv.Atoi(port)
			yes, no := true, false
			if _, err = st.SaveAPIServiceSettings(state.SaveAPIServiceSettings{Enabled: &yes, Port: n, AllowLAN: &no, ExpectedRevision: 1}); err != nil {
				t.Fatal(err)
			}
			_, token, _ := st.Grant("local app", "gateway", 0)
			srv := New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			srv.modelCatalog.Close()
			srv.modelCatalog = modeldev.NewWithTransport(modelCatalogTransport(func(*http.Request) (*http.Response, error) { return nil, modeldev.ErrUnavailable }))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- srv.Run(ctx) }()
			if occupied != nil {
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("occupied API port accepted")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("startup hung")
				}
			} else {
				deadline := time.Now().Add(3 * time.Second)
				for !srv.ready.Load() && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if !srv.ready.Load() {
					t.Fatal("not ready")
				}
				res := request(t, "GET", "http://"+addresses[2]+"/v1/models", token, "")
				res.Body.Close()
				if res.StatusCode != 200 {
					t.Fatal(res.StatusCode)
				}
				v, err := srv.apiServiceWorkspace()
				if err != nil || v.RestartRequired || v.ActiveAddress != addresses[2] {
					t.Fatal(v, err)
				}
				if _, err = st.SaveAPIServiceSettings(state.SaveAPIServiceSettings{Enabled: &no, Port: n, AllowLAN: &no, ExpectedRevision: 2}); err != nil {
					t.Fatal(err)
				}
				v, err = srv.apiServiceWorkspace()
				if err != nil || !v.RestartRequired || v.ActiveAddress != addresses[2] {
					t.Fatal(v, err)
				}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(4 * time.Second):
					t.Fatal("shutdown hung")
				}
			}
			for i, address := range addresses {
				if conflict && i == 2 {
					continue
				}
				ln, err := net.Listen("tcp4", address)
				if err != nil {
					t.Fatal("port leaked", address, err)
				}
				ln.Close()
			}
		})
	}
}
