package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestModelSettingsBridgeAndCLI(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := core.New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	cfg, err := st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1/v1", Enabled: true, TimeoutSeconds: 30, MaxConcurrent: 1}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.PutModel(state.Model{ID: "mapped", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "upstream"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(Options{Dir: dir})
	defer session.Close()
	session.cfg = &cfg
	b := NewBridge(session)
	out := b.ModelSettings(ModelSettingsQuery{ModelID: "mapped"})
	if out.Problem != nil || out.Data == nil {
		t.Fatal(out)
	}
	n := int64(272000)
	out.Data.Configuration.Limits.Context = &n
	input := SaveModelSettings{ModelID: "mapped", ExpectedRevision: 1, DisplayName: "Mapped", Configuration: out.Data.Configuration, ReferenceAction: "keep"}
	saved := b.SaveModelSettings(input)
	if saved.Problem != nil || saved.Data.Revision != 2 || *saved.Data.Configuration.Limits.Context != n {
		t.Fatal(saved)
	}
	stale := b.SaveModelSettings(input)
	if stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	input.Configuration.DefaultReasoning = &ReasoningDefault{Type: "effort"}
	invalid := b.SaveModelSettings(input)
	if invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	input.Configuration.DefaultReasoning = nil
	input.ExpectedRevision = 2
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		var output bytes.Buffer
		a := cli.App{Out: &output, Err: io.Discard, In: strings.NewReader("")}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (err != nil) != wantErr {
			t.Fatalf("%v: %v", args, err)
		}
		return output.String()
	}
	run(false, "context", "add", "local", "--local")
	run(false, "context", "use", "local")
	if out := run(false, "model", "settings", "--id", "mapped"); !strings.Contains(out, `"context":272000`) {
		t.Fatal(out)
	}
	path := filepath.Join(dir, "settings.json")
	raw, _ := json.Marshal(input)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if out := run(false, "model", "settings", "--id", "mapped", "--file", path); !strings.Contains(out, `"revision":3`) {
		t.Fatal(out)
	}
	run(true, "model", "settings", "--id", "mapped", "--file", path)
	run(true, "model", "settings", "--id", "other", "--file", path)
	if err = os.WriteFile(path, append(raw, []byte(` {}`)...), 0600); err != nil {
		t.Fatal(err)
	}
	run(true, "model", "settings", "--id", "mapped", "--file", path)
}

func TestSharedModelSettingsFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/model-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var out ModelSettingsState
	if json.Unmarshal(raw, &out) != nil || out.ModelID != "mapped" || out.Revision != 2 || out.Configuration.Validate() != nil {
		t.Fatal(string(raw))
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal(raw, &a)
	json.Unmarshal(encoded, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("model settings contract roundtrip drift")
	}
}
