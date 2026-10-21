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
	enabled, threshold, seconds, rate, weight := true, 50, 70, 80, 20
	in := state.SaveKeyCooldownSettings{WeightPercent: &weight, Enabled: &enabled, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: &rate, ExpectedRevision: 1}
	if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
		t.Fatal(err)
	}
	status := func() string { return f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}).State }
	f.core.finishKey(p, "default", 500, "", false)
	if status() != "ready" {
		t.Fatal("penalized before threshold")
	}
	f.core.finishKey(p, "default", 500, "", false)
	h := f.core.health[healthID(p, "default")]
	if status() != "cooling" || time.Until(h.Until) < 68*time.Second {
		t.Fatal(status(), h)
	}
	for i := 0; i < 3; i++ {
		if _, ok := f.core.chooseKey(p, nil); ok {
			t.Fatal("cooling Key selected")
		}
	}
	if _, err := f.core.saveKeyCooldownSettings(in); err != state.ErrConflict || status() != "cooling" {
		t.Fatal("conflict changed health", err)
	}
	h.Until = time.Now().Add(-time.Second)
	for i := 0; i < 3; i++ {
		if _, ok := f.core.chooseKey(p, nil); !ok {
			t.Fatal("exclusive recovery probe")
		}
	}
	if status() != "ready" {
		t.Fatal("expiry did not recover")
	}
	for _, code := range []int{401, 403, 429} {
		f.core.finishKey(p, "default", code, "", false)
		if status() != "cooling" {
			t.Fatal(code, status())
		}
		until := h.Until
		f.core.finishKey(p, "default", 200, "", true)
		f.core.finishKey(p, "default", 403, "", false)
		if status() != "cooling" || h.Until != until {
			t.Fatal("in-flight result changed cooldown")
		}
		h.Until = time.Now().Add(-time.Second)
		if status() != "ready" || h.Score != 0 {
			t.Fatal("expiry did not reset score")
		}
	}
	f.core.finishKey(p, "default", 429, "12", false)
	if time.Until(h.Until) < 69*time.Second || time.Until(h.Until) > 70*time.Second {
		t.Fatal("short Retry-After shortened cooldown", h)
	}
	enabled = false
	in.ExpectedRevision = 2
	if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{401, 403, 429, 500, 500, 500} {
		f.core.finishKey(p, "default", code, "600", false)
		if status() != "ready" {
			t.Fatal("disabled policy penalized Key", code)
		}
	}
	// The retained policy remains authoritative even if subsequent DB reads fail.
	f.store.Close()
	f.core.finishKey(p, "default", 401, "", false)
	if status() != "ready" {
		t.Fatal("failed read reenabled automatic penalties")
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
