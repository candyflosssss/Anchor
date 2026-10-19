package core

import (
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
	"testing"
	"time"
)

func TestKeyCooldownConfiguredPolicyAndLiveDisable(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-key")
	p, _ := f.store.Provider("p")
	enabled, threshold, seconds, rate := true, 2, 70, 80
	in := state.SaveKeyCooldownSettings{Enabled: &enabled, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: &rate, ExpectedRevision: 1}
	if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
		t.Fatal(err)
	}
	f.core.finishKey(p, "default", 500, "", false)
	if _, ok := f.core.chooseKey(p, nil); !ok {
		t.Fatal("cooled before threshold")
	}
	f.core.finishKey(p, "default", 500, "", false)
	status := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
	if status.State != "cooling" || status.RetryAt < time.Now().Add(68*time.Second).Unix() {
		t.Fatal(status)
	}
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("cooling key chosen")
	}
	f.core.health[healthID(p, "default")].Until = time.Now().Add(-time.Second)
	if _, ok := f.core.chooseKey(p, nil); !ok {
		t.Fatal("no half-open probe")
	}
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("multiple half-open probes")
	}
	f.core.finishKey(p, "default", 200, "", true)
	if status = f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}); status.State != "ready" || status.Failures != 0 {
		t.Fatal(status)
	}
	f.core.finishKey(p, "default", 429, "", false)
	status = f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
	if status.RetryAt < time.Now().Add(78*time.Second).Unix() {
		t.Fatal(status)
	}
	// A rejected stale write cannot clear cooling state.
	if _, err := f.core.saveKeyCooldownSettings(in); err != state.ErrConflict {
		t.Fatal(err)
	}
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("conflict cleared cooldown")
	}
	enabled = false
	in.ExpectedRevision = 2
	if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.core.chooseKey(p, nil); !ok {
		t.Fatal("disable did not clear cooling")
	}
	for _, code := range []int{429, 500, 500, 500} {
		f.core.finishKey(p, "default", code, "600", false)
		if _, ok := f.core.chooseKey(p, nil); !ok {
			t.Fatal("disabled policy cooled key", code)
		}
	}
	f.core.finishKey(p, "default", 401, "", false)
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("authentication failure bypassed")
	}
	enabled = true
	in.ExpectedRevision = 3
	if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("policy write cleared authentication protection")
	}
	// Existing explicit reset remains the recovery entry point.
	f.core.finishKey(p, "default", 200, "", true)
	f.core.finishKey(p, "default", 429, "12", false)
	if status = f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}); status.RetryAt > time.Now().Add(13*time.Second).Unix() {
		t.Fatal("Retry-After ignored", status)
	}
}
func TestKeyCooldownRetryAfterBounds(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"", 80 * time.Second}, {"invalid", 80 * time.Second}, {"0", time.Second}, {"999999999999", time.Hour}, {"120", 120 * time.Second}, {time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), time.Second}} {
		if got := configuredRetryDelay(tc.value, 80); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	if got := configuredRetryDelay(future, 80); got < 118*time.Second || got > 120*time.Second {
		t.Fatal(got)
	}
}

func TestKeyCooldownSettingsAuthorization(t *testing.T) {
	f := setup(t)
	for _, method := range []string{"GET", "POST"} {
		r := request(t, method, f.control.URL+"/control/v1/settings/key-cooldown", f.token, `{}`)
		if r.StatusCode != 401 {
			t.Fatal(method, r.StatusCode)
		}
	}
	req, _ := http.NewRequest("GET", f.control.URL+"/control/v1/settings/key-cooldown", nil)
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("Origin", "http://localhost:14201")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal(response.StatusCode)
	}
}
