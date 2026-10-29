package core

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func TestProviderResetAllKeys(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-key")
	f.add(t, "other", "http://127.0.0.1:1/v1", "synthetic-other")
	for i, id := range []string{"second", "disabled"} {
		if err := f.store.MutateProvider("p", int64(i+1), "key-put", id, "synthetic-key", "", id != "disabled", false); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := f.store.Provider("p")
	other, _ := f.store.Provider("other")
	before, _ := f.store.ProviderKeys(p.ID)
	for _, k := range before {
		f.core.finishKey(p, k.ID, 401, "", false)
		f.core.mu.Lock()
		f.core.health[healthID(p, k.ID)].Test = &catalog.Probe{KeyID: k.ID, Failure: "authentication"}
		f.core.mu.Unlock()
	}
	f.core.finishKey(other, "default", 401, "", false)
	cfg, _ := f.store.Settings()
	action := func(in catalog.Action, instance string) (int, catalog.Workspace) {
		t.Helper()
		raw, _ := json.Marshal(in)
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/"+in.ProviderID+"/action", strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", instance)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "synthetic-key") {
			t.Fatal("secret leaked")
		}
		var w catalog.Workspace
		json.Unmarshal(body, &w)
		return resp.StatusCode, w
	}
	in := catalog.Action{ProviderID: p.ID, ExpectedRevision: p.Revision, Action: "key-reset-all", Approved: true}
	for _, tc := range []struct {
		name     string
		edit     func(*catalog.Action)
		instance string
		status   int
	}{
		{"approval", func(a *catalog.Action) { a.Approved = false }, cfg.InstanceID, 400},
		{"stale", func(a *catalog.Action) { a.ExpectedRevision-- }, cfg.InstanceID, 409},
		{"target", func(a *catalog.Action) { a.Target = "default" }, cfg.InstanceID, 400},
		{"identity", func(a *catalog.Action) {}, "wrong-instance", 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := in
			tc.edit(&input)
			if status, _ := action(input, tc.instance); status != tc.status {
				t.Fatalf("status=%d want=%d", status, tc.status)
			}
			w, err := f.core.workspace(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range w.Keys {
				if k.Failures == 0 || k.Test == nil {
					t.Fatalf("rejected reset changed health: %+v", k)
				}
			}
		})
	}
	for i := 0; i < 2; i++ {
		status, w := action(in, cfg.InstanceID)
		if status != 200 || w.Revision != p.Revision || len(w.Keys) != len(before) {
			t.Fatal(status, w)
		}
		for _, k := range w.Keys {
			want := "ready"
			if !k.Enabled {
				want = "disabled"
			}
			if k.State != want || k.Failures != 0 || k.ErrorScore != 0 || k.RetryAt != 0 || k.Test != nil {
				t.Fatalf("Key not reset: %+v", k)
			}
		}
	}
	after, _ := f.store.ProviderKeys(p.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("reset changed persisted Keys")
	}
	w, _ := f.core.workspace(other.ID)
	if w.Keys[0].Failures == 0 || w.Keys[0].RetryAt <= time.Now().Unix() {
		t.Fatal("other provider was reset", w)
	}
	f.add(t, "empty", "http://127.0.0.1:1/v1", "")
	// A provider without saved Keys has nothing to reset.
	in.ProviderID, in.ExpectedRevision = "empty", 1
	if status, w := action(in, cfg.InstanceID); status != 200 || len(w.Keys) != 0 {
		t.Fatal(status, w)
	}
}
