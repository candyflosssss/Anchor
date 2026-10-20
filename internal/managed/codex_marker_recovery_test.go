package managed

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Reproduce a serializer that preserves marker comments but moves unrelated
// tables between the provider and its auth subtable. Both UI update and CLI
// sync must repair this layout, including a missing generated model catalog.
func TestCodexReclaimWithMisplacedMarkers(t *testing.T) {
	for _, action := range []string{"sync", "enroll"} {
		for _, layout := range []string{"interleaved", "duplicate", "missing_end", "crlf"} {
			t.Run(action+"/"+layout, func(t *testing.T) {
				m, env, target := workspaceManager(t)
				b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = m.Start(); err != nil {
					t.Fatal(err)
				}
				before, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				token, err := m.Store.BindingToken(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(b.ConfigRoot, "config.toml")
				raw := readSyncFile(t, path)
				provider := "anchor_" + strings.NewReplacer("-", "_", ".", "_").Replace(b.ID)
				begin := "# anchor:begin " + provider + "\n"
				end := "# anchor:end " + provider + "\n"
				unrelated := []byte("# externally added project and provider, preserve exact bytes\n[projects.\"/user/work\"]\ntrust_level = \"trusted\"\n[model_providers.external]\nbase_url = \"http://127.0.0.1:1234/v1\"\n")
				auth := []byte("[model_providers." + provider + ".auth]")
				if !bytes.Contains(raw, auth) {
					t.Fatal("missing provider auth fixture")
				}
				raw = bytes.Replace(raw, auth, append(append([]byte{}, unrelated...), auth...), 1)
				raw = bytes.Replace(raw, []byte("model = \"public\""), []byte("model = \"external\""), 1)
				switch layout {
				case "duplicate":
					raw = bytes.Replace(raw, []byte(begin), []byte(begin+begin), 1)
				case "missing_end":
					raw = bytes.Replace(raw, []byte(end), nil, 1)
				case "crlf":
					raw = bytes.ReplaceAll(raw, []byte("\n"), []byte("\r\n"))
					unrelated = bytes.ReplaceAll(unrelated, []byte("\n"), []byte("\r\n"))
				}
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				catalog := filepath.Join(b.ConfigRoot, catalogName(b.ID))
				if err = os.Remove(catalog); err != nil {
					t.Fatal(err)
				}
				if _, err = m.SyncMappings(); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(raw, readSyncFile(t, path)) {
					t.Fatal("background sync changed external configuration")
				}
				for i := 0; i < 2; i++ {
					if action == "sync" {
						r, e := m.Refresh(b.ID, b.Revision)
						if e != nil || r.Status != "applied" {
							t.Fatalf("sync %d: %+v %v", i, r, e)
						}
					} else {
						b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true})
						if err != nil {
							t.Fatalf("enroll %d: %v", i, err)
						}
					}
					g, e := m.gateway(b)
					if e != nil {
						t.Fatal(e)
					}
					if _, e = (Codex{}).Verify(b, g); e != nil {
						t.Fatal(e)
					}
					if !bytes.Contains(readSyncFile(t, path), unrelated) {
						t.Fatal("unrelated bytes changed during repair")
					}
				}
				after, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				if after.OperationID != before.OperationID || !bytes.Equal(after.Original, before.Original) {
					t.Fatal("original recovery operation replaced")
				}
				current, err := m.Store.BindingToken(b.ID)
				if err != nil || current != token {
					t.Fatal("credential changed", err)
				}
				if r, e := m.Restore(b.ID, b.Revision); e != nil || r.Status != "restored" {
					t.Fatal(r, e)
				}
				restored := readSyncFile(t, path)
				if !bytes.Contains(restored, unrelated) || !bytes.Contains(restored, []byte("model = \"external\"")) {
					t.Fatal("external recovery values lost")
				}
				if _, e := os.Stat(catalog); !os.IsNotExist(e) {
					t.Fatal("missing catalog recovery baseline lost", e)
				}
			})
		}
	}
}

func TestCodexRestoreWithInterleavedMarkerTables(t *testing.T) {
	_, b, g, e := target(t, "model = \"original\"\n")
	p := planApply(t, b, g, e)
	raw := readSyncFile(t, p.Path)
	extra := []byte("[projects.\"/keep\"]\ntrust_level = \"trusted\"\n")
	needle := []byte("[model_providers.anchor_test.auth]")
	if !bytes.Contains(raw, needle) {
		t.Fatal("missing auth fixture")
	}
	raw = bytes.Replace(raw, needle, append(append([]byte{}, extra...), needle...), 1)
	if err := os.WriteFile(p.Path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := e.Recover(b.ID); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	actual := readSyncFile(t, p.Path)
	if !bytes.Contains(actual, extra) || !bytes.Contains(actual, []byte("model = \"original\"")) {
		t.Fatal(fmt.Sprintf("recovery did not preserve unrelated config: %q", actual))
	}
}
