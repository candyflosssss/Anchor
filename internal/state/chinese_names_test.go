package state

import (
	"errors"
	"testing"
)

func TestChineseNamesPersistenceAndReferences(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg, err := s.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PutContext(Context{Name: "本机连接", Endpoint: "http://127.0.0.1:17861", InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}); err != nil {
		t.Fatal(err)
	}
	if err = s.UseContext("本机连接"); err != nil {
		t.Fatal(err)
	}
	p, err := s.PutProvider(provider("中文供应商"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "主力密钥", "synthetic-test-secret", "", true, false, "responses"); err != nil {
		t.Fatal(err)
	}
	p, err = s.Provider(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.PutKeyModelProtocol(p.ID, p.Revision, "主力密钥", "upstream/model", "anthropic"); err != nil {
		t.Fatal(err)
	}
	m, err := s.PutModel(Model{ID: "日常模型", DisplayName: "日常对话", Candidates: []Candidate{{ProviderID: p.ID, Model: "upstream/model", KeyIDs: []string{"主力密钥"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.Provider(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-delete", "主力密钥", "", "", false, false); !errors.Is(err, ErrReferenced) {
		t.Fatal("referenced Chinese Key was not protected", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	c, err := reopened.Context("本机连接")
	if err != nil || c.Name != "本机连接" {
		t.Fatal(c, err)
	}
	keys, err := reopened.ProviderKeys(p.ID)
	if err != nil || len(keys) != 1 || keys[0].ID != "主力密钥" || keys[0].Secret != "synthetic-test-secret" || keys[0].EffectiveProtocol("upstream/model") != "anthropic" {
		t.Fatal("Chinese Key did not persist", err)
	}
	stored, err := reopened.Model(m.ID)
	if err != nil || stored.ID != m.ID || stored.Candidates[0].KeyIDs[0] != "主力密钥" {
		t.Fatal(stored, err)
	}
	// Remove the model first; deletion must still find the exact Chinese Key.
	if err = reopened.DeleteModel(m.ID, m.Revision); err != nil {
		t.Fatal(err)
	}
	if err = reopened.MutateProvider(p.ID, p.Revision, "key-delete", "主力密钥", "", "", false, false); err != nil {
		t.Fatal(err)
	}
}
