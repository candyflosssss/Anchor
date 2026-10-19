package modelconfig

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLegacyModelRetriesAreIgnored(t *testing.T) {
	var s ModelSettings
	if err := json.Unmarshal([]byte(`{"channel_retries":10}`), &s); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(s)
	if err != nil || bytes.Contains(raw, []byte("channel_retries")) {
		t.Fatal(string(raw), err)
	}
}
