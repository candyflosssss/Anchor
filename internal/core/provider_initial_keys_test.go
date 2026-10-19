package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestProviderInitialKeysHTTPAtomicBoundary(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	raw, err := os.ReadFile("../../desktop/contract/provider-create.json")
	if err != nil {
		t.Fatal(err)
	}
	put := func(body []byte, token, instance string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("PUT", f.control.URL+"/control/v1/providers/multi-key-create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Anchor-Instance", instance)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode, text(t, response)
	}
	if status, _ := put(raw, f.token, cfg.InstanceID); status != 401 {
		t.Fatal(status)
	}
	if status, _ := put(raw, f.owner, "wrong-instance"); status != 409 {
		t.Fatal(status)
	}
	bad := bytes.Replace(raw, []byte(`"id": "备用"`), []byte(`"id": "backup"`), 1)
	if status, body := put(bad, f.owner, cfg.InstanceID); status != 400 || strings.Contains(body, "synthetic-create-") {
		t.Fatal("invalid Key boundary", status)
	}
	rows, _ := f.store.Providers()
	if len(rows) != 0 {
		t.Fatal("invalid batch partially persisted")
	}
	status, body := put(raw, f.owner, cfg.InstanceID)
	if status != 200 || strings.Contains(body, "synthetic-create-") {
		t.Fatal("creation boundary", status)
	}
	keys, _ := f.store.ProviderKeys("multi-key-create")
	if len(keys) != 3 {
		t.Fatal("not all Keys persisted")
	}
	if status, _ = put(raw, f.owner, cfg.InstanceID); status != 409 {
		t.Fatal(status)
	}
	var update map[string]any
	if err = json.Unmarshal(raw, &update); err != nil {
		t.Fatal(err)
	}
	update["expected_revision"] = 1
	bodyRaw, _ := json.Marshal(update)
	if status, _ = put(bodyRaw, f.owner, cfg.InstanceID); status != 400 {
		t.Fatal("initial Keys accepted in update", status)
	}
	p, _ := f.store.Provider("multi-key-create")
	if p.Revision != 1 {
		t.Fatal("failed requests changed revision")
	}
	if strings.Contains(f.logs.String(), "synthetic-create-") {
		t.Fatal("credentials leaked in logs")
	}
}
