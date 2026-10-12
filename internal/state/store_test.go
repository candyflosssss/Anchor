package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	return s
}
func provider(id string) Provider {
	return Provider{ID: id, BaseURL: "https://example.invalid/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 2}
}
func TestInitializeIdentityAndCredentialScopes(t *testing.T) {
	s := newStore(t)
	first, e := s.Settings()
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Initialize("127.0.0.1:19001", "127.0.0.1:19002")
	if e != nil || again != first {
		t.Fatalf("identity changed: %v", e)
	}
	owner, e := s.OwnerToken()
	if e != nil {
		t.Fatal(e)
	}
	if !s.Authenticate(owner, "manage") || s.Authenticate(owner, "gateway") {
		t.Fatal("owner scope isolation failed")
	}
	access, token, e := s.Grant("test", "gateway", time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	if !s.Authenticate(token, "gateway") || s.Authenticate(token, "manage") {
		t.Fatal("gateway scope isolation failed")
	}
	if e = s.Revoke(access.ID); e != nil {
		t.Fatal(e)
	}
	if s.Authenticate(token, "gateway") {
		t.Fatal("revoked token remains authorized")
	}
	if s.Authenticate("garbage", "manage") {
		t.Fatal("invalid credential accepted")
	}
	b, _ := json.Marshal(access)
	if strings.Contains(string(b), token) {
		t.Fatal("access metadata leaked secret")
	}
	var hash string
	if e = s.db.QueryRow("SELECT hash FROM access WHERE id=?", access.ID).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	if hash == token {
		t.Fatal("token persisted in plaintext")
	}
}
func TestExpiredGrant(t *testing.T) {
	s := newStore(t)
	a, token, e := s.Grant("expired", "manage", time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`UPDATE access SET expires_at=? WHERE id=?`, time.Now().Unix()-1, a.ID); e != nil {
		t.Fatal(e)
	}
	if s.Authenticate(token, "manage") {
		t.Fatal("expired grant accepted")
	}
}
func TestProviderRevisionsAndSecretPreservation(t *testing.T) {
	s := newStore(t)
	secret := "never-serialize-this-secret"
	p := provider("one")
	saved, e := s.PutProvider(p, 0, &secret)
	if e != nil || saved.Revision != 1 {
		t.Fatal(saved, e)
	}
	b, _ := json.Marshal(saved)
	if strings.Contains(string(b), secret) || saved.Secret != "" || !saved.HasSecret {
		t.Fatal("provider secret leaked")
	}
	if _, e = s.PutProvider(p, 0, nil); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate create should conflict: %v", e)
	}
	p.Enabled = false
	if _, e = s.PutProvider(p, 1, nil); e != nil {
		t.Fatal(e)
	}
	got, e := s.Provider("one")
	if e != nil || got.Secret != secret || got.Revision != 2 || got.Enabled {
		t.Fatal(got, e)
	}
	if _, e = s.PutProvider(p, 1, nil); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", e)
	}
	empty := ""
	if _, e = s.PutProvider(p, 2, &empty); e != nil {
		t.Fatal(e)
	}
	got, e = s.Provider("one")
	if e != nil || got.HasSecret {
		t.Fatal(got, e)
	}
}
func TestModelValidationAndCandidateOrder(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"a", "b"} {
		if _, e := s.PutProvider(provider(id), 0, nil); e != nil {
			t.Fatal(e)
		}
	}
	m := Model{ID: "public", Tools: true, Streaming: true, Candidates: []Candidate{{ProviderID: "b", Model: "model-b"}, {ProviderID: "a", Model: "model-a"}}}
	if _, e := s.PutModel(m, 0); e != nil {
		t.Fatal(e)
	}
	got, e := s.Model("public")
	if e != nil || got.Candidates[0].ProviderID != "b" || got.Revision != 1 {
		t.Fatal(got, e)
	}
	if _, e = s.PutModel(m, 0); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale model create accepted: %v", e)
	}
	m.ID = "missing"
	m.Candidates = []Candidate{{ProviderID: "absent", Model: "model"}}
	if _, e = s.PutModel(m, 0); e == nil {
		t.Fatal("missing provider accepted")
	}
}
func TestContextsDoNotContainSecrets(t *testing.T) {
	s := newStore(t)
	cfg, _ := s.Settings()
	c := Context{Name: "local", Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}
	if _, e := s.Context(""); e == nil {
		t.Fatal("missing default should not fallback")
	}
	if e := s.PutContext(c); e != nil {
		t.Fatal(e)
	}
	if e := s.UseContext("local"); e != nil {
		t.Fatal(e)
	}
	got, e := s.Context("")
	if e != nil || got != c {
		t.Fatal(got, e)
	}
	token, _ := s.OwnerToken()
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), token) {
		t.Fatal("context secret leak")
	}
	if _, e := s.Context("unknown"); !errors.Is(e, ErrNotFound) {
		t.Fatal("unknown context did not fail")
	}
}
func TestProtectedFilesAndLock(t *testing.T) {
	s := newStore(t)
	if runtime.GOOS == "linux" {
		for _, p := range []string{s.Dir, filepath.Join(s.Dir, "state.db"), filepath.Join(s.Dir, "owner.token")} {
			info, e := os.Stat(p)
			if e != nil {
				t.Fatal(e)
			}
			if info.Mode().Perm()&0077 != 0 {
				t.Fatalf("wide permissions: %s %v", p, info.Mode())
			}
		}
	}
	p := filepath.Join(s.Dir, "instance.lock")
	lock, e := platform.AcquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := platform.AcquireLock(p); e == nil {
		second.Close()
		t.Fatal("duplicate lock acquired")
	}
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	next, e := platform.AcquireLock(p)
	if e != nil {
		t.Fatal(e)
	}
	next.Close()
}
func TestSymlinkStorageRejected(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("symlink creation requires optional Windows privileges")
	}
	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(link, true); e == nil {
		t.Fatal("symlink directory accepted")
	}
	real := filepath.Join(root, "real")
	if e := os.Mkdir(real, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(target, "outside")
	if e := os.WriteFile(file, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(file, filepath.Join(real, "state.db")); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(real, true); e == nil {
		t.Fatal("symlink file accepted")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "keep" {
		t.Fatal("external file modified")
	}
}
func TestSchemaVersionGuard(t *testing.T) {
	s := newStore(t)
	dir := s.Dir
	if _, e := s.db.Exec(`PRAGMA user_version=5`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	if future, e := Open(dir, false); e == nil {
		future.Close()
		t.Fatal("future schema accepted")
	}
}
func TestInputPolicies(t *testing.T) {
	for _, v := range []string{"0.0.0.0:17861", "localhost:17861", "example.org:17861", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:not-a-port", "127.0.0.1:65536"} {
		if ValidateListen(v) == nil {
			t.Fatalf("unsafe listener accepted: %s", v)
		}
	}
	for _, v := range []string{"http://example.org/v1", "https://user:secret@example.org/v1", "https://example.org/v1?key=secret", "https://example.org/#fragment"} {
		if ValidateURL(v) == nil {
			t.Fatalf("unsafe provider URL accepted: %s", v)
		}
	}
	for _, v := range []string{"https://example.org/v1", "http://127.0.0.1:8000/v1", "http://[::1]:8000/v1"} {
		if e := ValidateURL(v); e != nil {
			t.Fatal(v, e)
		}
	}
	p := provider("p")
	p.Protocol = "chat"
	if p.Validate() == nil {
		t.Fatal("unsupported protocol accepted")
	}
	p = provider("p")
	p.Secret = "secret\r\nheader"
	if p.Validate() == nil {
		t.Fatal("header injection accepted")
	}
}

func TestConcurrentWritersHaveOneRevisionWinner(t *testing.T) {
	s := newStore(t)
	other, e := Open(s.Dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if _, e = s.PutProvider(provider("p"), 0, nil); e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, store := range []*Store{s, other} {
		go func(st *Store) { <-start; _, e := st.PutProvider(provider("p"), 1, nil); results <- e }(store)
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict))) {
		t.Fatal("expected one winner and one conflict", first, second)
	}
}
func TestDatabasePathIsURIEscaped(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Windows does not permit ? in file names")
	}
	dir := filepath.Join(t.TempDir(), "测试 # ?mode=memory")
	s, e := Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	first, _ := s.Settings()
	s.Close()
	s, e = Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	second, e := s.Settings()
	if e != nil || first != second {
		t.Fatal("path was interpreted as URI options", e)
	}
}
