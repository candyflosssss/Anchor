package state

import (
	"errors"
	"testing"
	"time"
)

func TestAPIServiceSettingsAndGatewayKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	original, err := s.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.APIServiceSettings()
	if err != nil || v.Enabled || v.AllowLAN || v.Port != 17862 || v.Revision != 1 {
		t.Fatal(v, err)
	}
	yes := true
	in := SaveAPIServiceSettings{Enabled: &yes, Port: 18762, AllowLAN: &yes, ExpectedRevision: 1}
	saved, err := s.SaveAPIServiceSettings(in)
	if err != nil || saved.Address() != "0.0.0.0:18762" || saved.Revision != 2 {
		t.Fatal(saved, err)
	}
	if _, err = s.SaveAPIServiceSettings(in); !errors.Is(err, ErrConflict) {
		t.Fatal("stale", err)
	}
	in.ExpectedRevision = 2
	for _, port := range []int{0, 65536, 17860, 17861} {
		in.Port = port
		if _, err = s.SaveAPIServiceSettings(in); !errors.Is(err, ErrInvalid) {
			t.Fatal(port, err)
		}
	}
	if cfg, _ := s.Settings(); cfg != original {
		t.Fatal("Agent gateway changed", cfg)
	}
	other, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if got, e := other.APIServiceSettings(); e != nil || got != saved {
		t.Fatal(got, e)
	}
	a, token, err := s.Grant("手机助手", "gateway", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got, e := s.AccessForToken(token, "gateway"); e != nil || got.ID != a.ID {
		t.Fatal(got, e)
	}
	manage, mt, err := s.Grant("admin", "manage", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AccessForToken(mt, "gateway"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err = s.RevokeGatewayKey(manage.ID); !errors.Is(err, ErrNotFound) || !s.Authenticate(mt, "manage") {
		t.Fatal("management authorization changed", err)
	}
	keys, err := s.GatewayKeys()
	if err != nil || len(keys) != 1 || keys[0].ID != a.ID {
		t.Fatal(keys, err)
	}
	if err = s.RevokeGatewayKey(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AccessForToken(token, "gateway"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expired, et, err := s.Grant("expired", "gateway", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if s.Authenticate(et, "gateway") {
		t.Fatal("expired accepted", expired.ID)
	}
	for _, input := range []IssueAPIKey{{Label: ""}, {Label: "bad\nlabel"}, {Label: "x", TTLSeconds: -1}, {Label: "x", TTLSeconds: 315360001}} {
		if input.Validate() == nil {
			t.Fatal(input)
		}
	}
}
