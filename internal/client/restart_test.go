package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestStopForRestartCapabilityAndAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		capable bool
		reply   string
		code    string
		unknown bool
		writes  int32
	}{
		{"old core", false, `{"accepted":true}`, "unsupported", false, 0},
		{"preserved", true, `{"accepted":true,"bindings_preserved":true}`, "", false, 1},
		{"missing preservation acknowledgement", true, `{"accepted":true}`, "invalid_response", true, 1},
		{"refused", true, `{"accepted":false,"bindings_preserved":true}`, "invalid_response", true, 1},
		{"invalid reply", true, `broken`, "invalid_response", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes atomic.Int32
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/control/v1/instance" {
					caps := []string{"graceful_stop"}
					if tc.capable {
						caps = append(caps, "restart_handoff")
					}
					json.NewEncoder(w).Encode(map[string]any{"instance_id": "test", "api_version": 1, "capabilities": caps})
					return
				}
				writes.Add(1)
				var body map[string]bool
				if r.Method != "POST" || r.URL.Path != "/control/v1/shutdown" || r.Header.Get("X-Anchor-Instance") != "test" || r.Header.Get("Authorization") != "Bearer fixture-only" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || !body["for_restart"] {
					t.Error("wrong restart request")
				}
				w.Write([]byte(tc.reply))
			}))
			defer web.Close()
			cl, _, err := Connect(context.Background(), state.Context{Name: "test", Endpoint: web.URL, InstanceID: "test", CredentialRef: "local-owner"}, "fixture-only")
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()
			err = cl.StopForRestart(context.Background())
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var request *RequestError
				if !errors.As(err, &request) || request.Code != tc.code || request.Unknown != tc.unknown {
					t.Fatalf("classification: %v", err)
				}
			}
			if writes.Load() != tc.writes {
				t.Fatal("unsupported mutation or uncertain write replayed")
			}
		})
	}
}
