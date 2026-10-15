package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStopRequiresApprovalAndUsesSelectedIdentity(t *testing.T) {
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg, err := st.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	var stops atomic.Int32
	var capable atomic.Bool
	var restart atomic.Bool
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/control/v1/instance" {
			caps := []string{"graceful_stop"}
			if capable.Load() {
				caps = append(caps, "restart_handoff")
			}
			json.NewEncoder(w).Encode(map[string]any{"instance_id": cfg.InstanceID, "api_version": 1, "capabilities": caps})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/control/v1/shutdown" || r.Header.Get("X-Anchor-Instance") != cfg.InstanceID {
			t.Error("wrong shutdown target")
		}
		var body map[string]bool
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid shutdown body")
		}
		restart.Store(body["for_restart"])
		stops.Add(1)
		json.NewEncoder(w).Encode(map[string]bool{"accepted": true, "bindings_preserved": restart.Load()})
	}))
	defer web.Close()
	if err := st.PutContext(state.Context{Name: "selected", Endpoint: web.URL, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := App{In: strings.NewReader(""), Out: &out, Err: &out}
	prefix := []string{"--data-dir", st.Dir, "--context", "selected", "stop"}
	for _, args := range [][]string{nil, {"--approve=false"}, {"--for-restart"}, {"--approve", "unexpected"}, {"--force"}} {
		if err := app.Run(context.Background(), append(append([]string{}, prefix...), args...)); err == nil {
			t.Fatal("invalid stop accepted")
		}
	}
	if stops.Load() != 0 {
		t.Fatal("unconfirmed request sent")
	}
	out.Reset()
	if err := app.Run(context.Background(), append(prefix, "--approve")); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Accepted   bool   `json:"accepted"`
		InstanceID string `json:"instance_id"`
		Complete   bool   `json:"shutdown_complete"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || !result.Accepted || result.InstanceID != cfg.InstanceID || result.Complete || stops.Load() != 1 {
		t.Fatalf("invalid stop output: %s", out.String())
	}
	if restart.Load() {
		t.Fatal("normal stop selected preservation")
	}
	if err := app.Run(context.Background(), append(append([]string{}, prefix...), "--approve", "--for-restart")); err == nil || stops.Load() != 1 {
		t.Fatal("old core was stopped for restart")
	}
	capable.Store(true)
	out.Reset()
	if err := app.Run(context.Background(), append(append([]string{}, prefix...), "--approve", "--for-restart")); err != nil {
		t.Fatal(err)
	}
	var handoff map[string]any
	if json.Unmarshal(out.Bytes(), &handoff) != nil || handoff["bindings_preserved"] != true || handoff["restart_required"] != true || handoff["shutdown_complete"] != false || stops.Load() != 2 || !restart.Load() {
		t.Fatal("restart output contract", out.String())
	}
	if !strings.Contains(help, "stop --approve") || !strings.Contains(help, "--for-restart") {
		t.Fatal("help drift")
	}
}
