package managed

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
	"gopkg.in/yaml.v3"
)

const independentDSHInserts = `# plugin insertions stay in order
- insert:
    - id: docs-mcp
      name: example-docs
      config:
        expression: !!js '() => 1'
        id: agent-default-model # ordinary plugin data, not an entry
- id: tools-group
  insert:
    - id: extra-tools
      group: true
      config:
        - id: extra-tool
          name: example-tool
- insert:
    - id: named-tools
      name: cordis:group
      config:
        - id: nested-tools
          name: '@deepseek-ai/cordis-plugin-group'
          group: false
          config:
            - id: named-tool
              name: example-tool
              config: {expression: !!js '() => 2'}
- insert: []
- id: docs-mcp
  disabled: false
`

// Compare the YAML nodes as well as values: expressions, comments, and order
// belong to the user even when Anchor reserializes the enclosing document.
func dshInsertNodes(t *testing.T, raw []byte) []*yaml.Node {
	t.Helper()
	root, err := (StructuredEditor{DSH: true}).parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []*yaml.Node
	var clearPositions func(*yaml.Node)
	clearPositions = func(n *yaml.Node) {
		n.Line, n.Column = 0, 0
		for _, c := range n.Content {
			clearPositions(c)
		}
	}
	for _, row := range root.Content {
		if nodeKey(row, "insert") != nil {
			clearPositions(row)
			nodes = append(nodes, row)
		}
	}
	return nodes
}

func TestDSHIndependentInsertsLifecycle(t *testing.T) {
	for _, adapter := range []string{"dsh", "dsh-desktop"} {
		for _, existing := range []bool{false, true} {
			t.Run(adapter+map[bool]string{false: "/created", true: "/existing"}[existing], func(t *testing.T) {
				m, env, _ := workspaceManager(t)
				root := filepath.Join(env.Home, ".dsh")
				a := AdapterFor(adapter).(StructuredAgent)
				path := a.path(state.Binding{ConfigRoot: root})
				original := []byte(independentDSHInserts)
				if existing {
					original = append(original, []byte("- id: llm-pi-ai\n  config: {providers: {user: {keep: true}}}\n- id: agent-default-model\n  config: {provider: user, model: old-model, reasoningEffort: medium}\n")...)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				wantInserts := dshInsertNodes(t, original)
				targets, err := Discover(env)
				if err != nil {
					t.Fatal(err)
				}
				var targetID string
				for _, target := range targets {
					if target.Agent == adapter && target.ConfigRoot == root && target.Status == "ready" {
						targetID = target.ID
					}
				}
				if targetID == "" {
					t.Fatal("profile not configurable", targets)
				}
				if !bytes.Equal(original, readSyncFile(t, path)) {
					t.Fatal("discovery wrote profile")
				}
				if _, err := m.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { m.Stop() })
				b, err := m.Enroll(env, EnrollAgent{TargetID: targetID, Model: "public", ModelScope: "all-mapped", Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				assertInserts := func() {
					t.Helper()
					if !reflect.DeepEqual(wantInserts, dshInsertNodes(t, readSyncFile(t, path))) {
						t.Fatal("user insertions changed")
					}
				}
				assertInserts()
				if _, err := m.Store.PutModel(state.Model{ID: "second", Candidates: []state.Candidate{{ProviderID: "p", Model: "upstream-second"}}}, 0); err != nil {
					t.Fatal(err)
				}
				b, err = m.Enroll(env, EnrollAgent{TargetID: targetID, Model: "second", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				assertDSHValue(t, readSyncFile(t, path), []string{"agent-default-model", "config", "model"}, "second")
				assertInserts()
				g, err := m.gateway(b)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.Verify(b, g); err != nil {
					t.Fatal(err)
				}
				before, journal := readSyncFile(t, path), readSyncFile(t, m.Engine.journalPath(b.ID))
				if _, err := m.Refresh(b.ID, b.Revision); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, readSyncFile(t, path)) || !bytes.Equal(journal, readSyncFile(t, m.Engine.journalPath(b.ID))) {
					t.Fatal("no-op sync changed profile or journal")
				}
				// A conflicting insertion must not publish config or journal changes.
				bad := append(append([]byte(nil), before...), []byte("- insert: [{id: hidden-model, name: 'cordis:group', config: [{id: agent-default-model, name: example-shadow}]}]\n")...)
				if err := os.WriteFile(path, bad, 0600); err != nil {
					t.Fatal(err)
				}
				operations := []func() error{
					func() error { _, err := a.Inspect(b); return err },
					func() error { _, err := a.Verify(b, g); return err },
					func() error { _, err := m.Engine.Reconcile(b, g); return err },
					func() error { _, err := a.Plan(b, g); return err },
					func() error { _, err := m.Engine.Recover(b.ID); return err },
				}
				for _, operation := range operations {
					if err := operation(); !errors.Is(err, ErrUnsafe) {
						t.Fatal("unsafe live insertion accepted", err)
					}
					if !bytes.Equal(bad, readSyncFile(t, path)) || !bytes.Equal(journal, readSyncFile(t, m.Engine.journalPath(b.ID))) {
						t.Fatal("unsafe operation changed config or journal")
					}
				}
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
				// A plugin insertion added while managed must survive conditional restore.
				changed := append(readSyncFile(t, path), []byte("- insert:\n    - id: user-added\n      name: example-added\n")...)
				if err := os.WriteFile(path, changed, 0600); err != nil {
					t.Fatal(err)
				}
				wantInserts = dshInsertNodes(t, changed)
				if _, r, err := m.SetEnabled(b.ID, b.Revision, false); err != nil || r.Status != "restored" {
					t.Fatal(r, err)
				}
				assertInserts()
				restored := readSyncFile(t, path)
				if bytes.Contains(restored, []byte("anchor-"+b.ID)) {
					t.Fatal("managed provider retained")
				}
				if existing {
					assertDSHValue(t, restored, []string{"agent-default-model", "config", "model"}, "old-model")
				} else {
					assertDSHValue(t, restored, []string{"agent-default-model"}, nil)
					assertDSHValue(t, restored, []string{"llm-pi-ai"}, nil)
				}
			})
		}
	}
}

func TestDSHNamedGroupInserts(t *testing.T) {
	for _, name := range []string{"cordis:group", "@deepseek-ai/cordis-plugin-group"} {
		for _, flag := range []struct{ label, field string }{{"absent", ""}, {"false", ", group: false"}, {"true", ", group: true"}} {
			t.Run(name+"/group="+flag.label, func(t *testing.T) {
				group := func(config string) string {
					return fmt.Sprintf("- insert: [{id: tools, name: %q%s, config: %s}]\n", name, flag.field, config)
				}
				editor := StructuredEditor{DSH: true}
				safe := []byte(group("[{id: extra-tool, name: example-tool, config: {id: agent-default-model}}]"))
				out, err := editor.Patch(safe, nil)
				if err != nil {
					t.Fatal("independent named group rejected", err)
				}
				if !reflect.DeepEqual(dshInsertNodes(t, safe), dshInsertNodes(t, out)) {
					t.Fatal("independent named group changed")
				}
				cases := map[string]string{
					"default collision":  group("[{id: agent-default-model, name: example-shadow}]"),
					"provider collision": group("[{id: llm-pi-ai, name: example-shadow}]"),
					"nested collision":   group("[{id: nested, name: 'cordis:group', config: [{id: agent-default-model, name: example-shadow}]}]"),
					"duplicate child":    group("[{id: duplicate, name: example-tool}]") + "- insert: [{id: duplicate, name: example-tool}]\n",
					"non-list children":  group("{id: agent-default-model, name: example-shadow}"),
					"later children":     group("[]") + "- id: tools\n  config: [{id: agent-default-model, name: example-shadow}]\n",
				}
				for label, raw := range cases {
					t.Run(label, func(t *testing.T) {
						if _, err := editor.Values([]byte(raw), nil); !errors.Is(err, ErrUnsafe) {
							t.Fatal("unsafe named group read accepted", err)
						}
						if _, err := editor.Patch([]byte(raw), nil); !errors.Is(err, ErrUnsafe) {
							t.Fatal("unsafe named group write accepted", err)
						}
					})
				}
			})
		}
	}
}

func TestDSHUnsafeInsertsRejected(t *testing.T) {
	cases := map[string]string{
		"duplicate inserted IDs":          "- insert: [{id: duplicate, name: example}]\n- insert: [{id: duplicate, name: example}]\n",
		"duplicate nested ID":             "- insert: [{id: duplicate, name: example}, {id: tools, name: example-group, group: true, config: [{id: duplicate, name: example}]}]\n",
		"empty target ID":                 "- id: ''\n  insert: []\n",
		"later group conversion":          "- insert: [{id: tools, name: example-group, group: false, config: [{id: agent-default-model, name: example-shadow}]}]\n- id: tools\n  group: true\n",
		"later group children":            "- insert: [{id: tools, name: example-group, group: true, config: []}]\n- id: tools\n  config: [{id: llm-pi-ai, name: example-shadow}]\n",
		"managed target":                  "- id: llm-pi-ai\n  insert: []\n",
		"inserted provider":               "- insert: [{id: llm-pi-ai, name: example-shadow}]\n",
		"inserted default":                "- insert: [{id: agent-default-model, name: example-shadow}]\n",
		"uppercase group shadows default": "- insert: [{id: tools, name: example-group, group: TRUE, config: [{id: agent-default-model, name: example-shadow}]}]\n",
		"group shadows default":           "- insert: [{id: tools, name: example-group, group: true, config: [{id: nested, name: example-group, group: true, config: [{id: agent-default-model, name: example-shadow}]}]}]\n",
		"mixed override":                  "- id: tools\n  insert: []\n  config: {}\n",
		"missing name":                    "- insert: [{id: unrelated}]\n",
		"expression name":                 "- insert: [{id: unrelated, name: !!js 'dynamic'}]\n",
		"false insert":                    "- insert: false\n",
		"tagged list":                     "- insert: !custom []\n",
		"null insert":                     "- insert: null\n",
		"non-list insert":                 "- insert: {id: unrelated}\n",
		"non-entry insert":                "- insert: [unrelated]\n",
		"expression id":                   "- insert: [{id: !!js 'dynamic', name: example}]\n",
		"dynamic group":                   "- insert: [{id: tools, name: example-group, group: !!js 'true', config: []}]\n",
		"malformed group":                 "- insert: [{id: tools, name: example-group, group: true, config: {id: agent-default-model, name: example-shadow}}]\n",
		"nested patch":                    "- insert: [{insert: [{id: unrelated}]}]\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			editor := StructuredEditor{DSH: true}
			if _, err := editor.Values([]byte(raw), nil); !errors.Is(err, ErrUnsafe) {
				t.Fatal("unsafe read accepted", err)
			}
			if _, err := editor.Patch([]byte(raw), nil); !errors.Is(err, ErrUnsafe) {
				t.Fatal("unsafe recovery write accepted", err)
			}
		})
	}
}
