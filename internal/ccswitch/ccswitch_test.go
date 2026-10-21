package ccswitch

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func sourceDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cc-switch.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE providers(id TEXT,app_type TEXT,name TEXT,settings_config TEXT,website_url TEXT,meta TEXT,is_current INTEGER); CREATE TABLE provider_endpoints(id INTEGER,provider_id TEXT,app_type TEXT,url TEXT); CREATE TABLE settings(key TEXT,value TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return path, db
}
func insert(t *testing.T, db *sql.DB, id, app, config, meta string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO providers VALUES(?,?,?,?,'',?,0)`, id, app, "供应商", config, meta); err != nil {
		t.Fatal(err)
	}
}

func TestAllPlatformsAndUniversalReadOnly(t *testing.T) {
	path, db := sourceDB(t)
	fixtures := map[string]string{
		"claude":         `{"env":{"ANTHROPIC_BASE_URL":"https://shared.example/v1","ANTHROPIC_AUTH_TOKEN":"synthetic-common","ANTHROPIC_MODEL":"shared-model"}}`,
		"claude-desktop": `{"env":{"ANTHROPIC_BASE_URL":"https://shared.example/v1","ANTHROPIC_API_KEY":"synthetic-common","ANTHROPIC_MODEL":"shared-model"}}`,
		"codex":          `{"auth":{"OPENAI_API_KEY":"synthetic-common"},"config":"model_provider = 'custom'\nmodel = 'shared-model'\n[model_providers.custom]\nbase_url = 'https://shared.example/v1'\nwire_api = 'responses'"}`,
		"gemini":         `{"env":{"GEMINI_API_KEY":"synthetic-gemini"}}`,
		"agy":            `{"env":{"GEMINI_API_KEY":"synthetic-agy"}}`,
		"grokbuild":      `{"config":"[models]\ndefault = 'main'\n[model.main]\nmodel = 'grok-test'\nbase_url = 'https://grok.example/v1'\napi_key = 'synthetic-grok'\napi_backend = 'responses'"}`,
		"opencode":       `{"npm":"@ai-sdk/openai-compatible","options":{"baseURL":"https://open.example/v1","apiKey":"synthetic-open"},"models":{"org/model":{"name":"Friendly"}}}`,
		"mcode":          `{"api":"anthropic-messages","options":{"baseURL":"https://mcode.example","apiKey":"synthetic-mcode"},"models":{"mcode-test":{}}}`,
		"openclaw":       `{"baseUrl":"https://claw.example/v1","apiKey":"synthetic-claw","api":"openai-completions","models":[{"id":"claw-test"}]}`,
		"hermes":         `{"base_url":"https://hermes.example/v1","api_key":"synthetic-hermes","api_mode":"chat_completions","model":"hermes-test"}`,
		"pi":             `{"apiKey":"synthetic-pi","models":[{"id":"pi-test","baseUrl":"https://pi.example","api":"anthropic-messages"}]}`,
	}
	for app, c := range fixtures {
		insert(t, db, app, app, c, `{}`)
	}
	_, err := db.Exec(`INSERT INTO settings VALUES('universal_providers',?)`, `{"u":{"name":"通用","baseUrl":"https://shared.example/v1","apiKey":"synthetic-common","models":{"claude":{"model":"shared-model"},"codex":{"model":"shared-model"}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	plan, err := Build(context.Background(), path, st)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Report.Failure != "" || len(plan.Providers) != 7 {
		t.Fatalf("%+v", plan.Report)
	}
	platforms := map[string]bool{}
	keys := 0
	for _, p := range plan.Report.Providers {
		for _, src := range p.Sources {
			platforms[src.Platform] = true
		}
		keys += len(p.Keys)
	}
	if len(platforms) != 9 || keys != 8 {
		t.Fatalf("platforms=%v keys=%d", platforms, keys)
	}
	for _, issue := range plan.Report.Issues {
		if issue.Reason != "unsupported_protocol" {
			t.Fatalf("unexpected issue: %+v", issue)
		}
	}
	raw, _ := json.Marshal(plan.Report)
	if strings.Contains(string(raw), "synthetic-") || strings.Contains(string(raw), "apiKey") {
		t.Fatal("credentials exposed")
	}
	second, _ := Build(context.Background(), path, st)
	if second.Report.Fingerprint != plan.Report.Fingerprint {
		t.Fatal("unstable review")
	}
	if err := st.ImportProviders(context.Background(), plan.Providers, plan.Models); err != nil {
		t.Fatal(err)
	}
	for _, p := range plan.Providers {
		saved, _ := st.Provider(p.Provider.ID)
		if !saved.Enabled || !saved.HasSecret || saved.Revision != 1 {
			t.Fatal(saved)
		}
		keys, _ := st.ProviderKeys(p.Provider.ID)
		if len(keys) != len(p.Keys) {
			t.Fatal("lost keys")
		}
	}
	model, err := st.Model("shared-model")
	if err != nil || len(model.Candidates) != 1 || len(model.Candidates[0].KeyIDs) != 2 {
		t.Fatalf("shared mapping: %+v %v", model, err)
	}
	for _, m := range plan.Models {
		for _, c := range m.Model.Candidates {
			if len(c.KeyIDs) == 0 {
				t.Fatal("unrestricted mapping")
			}
		}
	}
	repeat, _ := Build(context.Background(), path, st)
	if len(repeat.Providers) != 0 || repeat.Report.Fingerprint == plan.Report.Fingerprint {
		t.Fatal("repeat would overwrite")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("CC Switch database changed")
	}
}

func TestRejectUnsafeAndUnsupportedConfigurations(t *testing.T) {
	cases := []struct{ app, config, meta, reason string }{
		{"claude", `{"env":{"ANTHROPIC_AUTH_TOKEN":"synthetic"}}`, `{"providerType":"codex_oauth"}`, "managed_auth"},
		{"claude", `{"env":{}}`, `{"apiFormat":"gemini_native"}`, "unsupported_protocol"},
		{"opencode", `{"npm":"@ai-sdk/amazon-bedrock","options":{}}`, `{}`, "unsupported_protocol"},
		{"claude", `{"env":{}}`, `{"isFullUrl":true}`, "request_overrides"},
		{"opencode", `{"options":{"headers":{"x-test":"value"}}}`, `{}`, "request_overrides"},
		{"future", `{}`, `{}`, "unsupported_platform"},
		{"codex", `{"config":"bad [ toml"}`, `{}`, "invalid_config"},
	}
	for _, tc := range cases {
		t.Run(tc.app+tc.reason, func(t *testing.T) {
			path, db := sourceDB(t)
			insert(t, db, "one", tc.app, tc.config, tc.meta)
			st, err := state.Open(t.TempDir(), true)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			p, err := Build(context.Background(), path, st)
			if err != nil || len(p.Providers) != 0 || len(p.Report.Issues) != 1 || p.Report.Issues[0].Reason != tc.reason {
				t.Fatalf("%+v %v", p, err)
			}
		})
	}
	for _, key := range []string{"", "${API_KEY}", "{env:API_KEY}", "{file:secret}", "!get-key", "PROXY_MANAGED", "$KEY", "bad\nkey"} {
		if literal(key) {
			t.Fatalf("reference accepted: %q", key)
		}
	}
}

func TestSnapshotWALAndSourceFailures(t *testing.T) {
	path, db := sourceDB(t)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	insert(t, db, "not-current", "claude", `{"env":{"ANTHROPIC_API_KEY":"synthetic-wal"}}`, `{}`)
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := Build(context.Background(), path, st)
	if err != nil || len(p.Providers) != 1 {
		t.Fatalf("WAL not read: %+v %v", p, err)
	}
	_, err = db.Exec(`UPDATE providers SET settings_config='{"env":{"ANTHROPIC_API_KEY":"synthetic-changed"}}'`)
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := Build(context.Background(), path, st)
	if changed.Report.Fingerprint == p.Report.Fingerprint {
		t.Fatal("changed credential did not invalidate review")
	}
	missing, _ := Build(context.Background(), filepath.Join(t.TempDir(), "missing.db"), st)
	if missing.Report.Failure != "source_missing" {
		t.Fatal(missing)
	}
	bad := filepath.Join(t.TempDir(), "bad.db")
	_ = os.WriteFile(bad, []byte("not a database"), 0600)
	invalid, _ := Build(context.Background(), bad, st)
	if invalid.Report.Failure != "source_invalid" {
		t.Fatal(invalid)
	}
}

func TestImportTransactionRollsBackAndPreservesModels(t *testing.T) {
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := state.Provider{ID: "existing", BaseURL: "https://existing.example/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 30, MaxConcurrent: 1}
	if _, err = st.PutProvider(p, 0, nil); err != nil {
		t.Fatal(err)
	}
	model, err := st.PutModel(state.Model{ID: "shared", DisplayName: "keep", Tools: false, Streaming: true, Strategy: "priority", Candidates: []state.Candidate{{ProviderID: "existing", Model: "old"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	path, db := sourceDB(t)
	insert(t, db, "one", "hermes", `{"base_url":"https://new.example/v1","api_key":"synthetic-new","api_mode":"chat_completions","model":"shared"}`, `{}`)
	plan, _ := Build(context.Background(), path, st)
	if plan.Models[0].Model.Tools || plan.Models[0].Model.DisplayName != "keep" || len(plan.Models[0].Model.Candidates) != 2 {
		t.Fatal("existing metadata lost")
	}
	if _, err = st.PutModel(model, 1); err != nil {
		t.Fatal(err)
	}
	if err = st.ImportProviders(context.Background(), plan.Providers, plan.Models); err != state.ErrConflict {
		t.Fatalf("wanted CAS conflict: %v", err)
	}
	providers, _ := st.Providers()
	if len(providers) != 1 {
		t.Fatal("partial import persisted")
	}
	fresh, _ := Build(context.Background(), path, st)
	if err = st.ImportProviders(context.Background(), fresh.Providers, fresh.Models); err != nil {
		t.Fatal(err)
	}
	if err = st.ImportProviders(context.Background(), fresh.Providers, fresh.Models); err != state.ErrConflict {
		t.Fatal("existing provider overwritten")
	}
}

func TestEndpointsProtocolOverridesAndStableIdentity(t *testing.T) {
	path, db := sourceDB(t)
	insert(t, db, "one", "pi", `{"baseUrl":"https://pi.example/v1","apiKey":"synthetic-pi","api":"openai-completions","models":[{"id":"first"},{"id":"second","api":"anthropic-messages"}]}`, `{}`)
	if _, err := db.Exec(`INSERT INTO provider_endpoints VALUES(1,'one','pi','https://backup.example/v1'),(2,'one','pi','https://backup.example/v1/')`); err != nil {
		t.Fatal(err)
	}
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	plan, err := Build(context.Background(), path, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Providers) != 1 || len(plan.Providers[0].Keys) != 2 || len(plan.Providers[0].Provider.BaseURLs) != 2 || len(plan.Models) != 2 {
		t.Fatalf("%+v", plan.Report)
	}
	if _, err = db.Exec(`UPDATE providers SET name='renamed'`); err != nil {
		t.Fatal(err)
	}
	renamed, _ := Build(context.Background(), path, st)
	if renamed.Providers[0].Provider.ID != plan.Providers[0].Provider.ID {
		t.Fatal("name changed stable provider identity")
	}
	if renamed.Report.Fingerprint == plan.Report.Fingerprint {
		t.Fatal("review failed to notice changed source label")
	}
	for _, model := range plan.Models {
		if len(model.Model.Candidates[0].KeyIDs) != 1 {
			t.Fatal("model protocol expanded to other Keys")
		}
	}
}

func TestMissingModelsAndReferenceKey(t *testing.T) {
	path, db := sourceDB(t)
	insert(t, db, "one", "claude", `{"env":{"ANTHROPIC_API_KEY":"synthetic-direct"}}`, `{}`)
	insert(t, db, "env", "pi", `{"baseUrl":"https://pi.example/v1","api":"openai-completions","apiKey":"PI_API_KEY"}`, `{}`)
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := Build(context.Background(), path, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Providers) != 1 || len(p.Models) != 0 {
		t.Fatal("invented a model or imported an env name")
	}
	reasons := map[string]bool{}
	for _, i := range p.Report.Issues {
		reasons[i.Reason] = true
	}
	if !reasons["models_not_configured"] || !reasons["missing_or_referenced_key"] {
		t.Fatal(p.Report.Issues)
	}
}

func TestSourceConfigLimit(t *testing.T) {
	path, db := sourceDB(t)
	insert(t, db, "huge", "claude", strings.Repeat("x", (1<<20)+1), `{}`)
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := Build(context.Background(), path, st)
	if err != nil || p.Report.Failure != "source_limit" || len(p.Providers) != 0 {
		t.Fatalf("limit: %+v %v", p, err)
	}
}
