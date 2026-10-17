package modelconfig

import (
	"encoding/json"
	"testing"
)

func TestChannelRetriesDefaultsAndValidation(t *testing.T) {
	var legacy *ModelSettings
	if legacy.RetryLimit() != 3 || (&ModelSettings{}).RetryLimit() != 3 {
		t.Fatal("legacy default")
	}
	for _, n := range []int{0, 1, 3, MaxChannelRetries} {
		s := ModelSettings{ChannelRetries: &n}
		if s.Validate() != nil || s.RetryLimit() != n {
			t.Fatal(n)
		}
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var loaded ModelSettings
		if json.Unmarshal(raw, &loaded) != nil || loaded.ChannelRetries == nil || loaded.RetryLimit() != n {
			t.Fatal(string(raw))
		}
	}
	for _, n := range []int{-1, MaxChannelRetries + 1} {
		if (ModelSettings{ChannelRetries: &n}).Validate() == nil {
			t.Fatal("accepted", n)
		}
	}
	for _, raw := range []string{`{"channel_retries":1.5}`, `{"channel_retries":"3"}`, `{"channel_retries":true}`} {
		var s ModelSettings
		if json.Unmarshal([]byte(raw), &s) == nil {
			t.Fatal("accepted", raw)
		}
	}
}
