package managed

import (
	"bytes"
	"encoding/json"
	"io"

	"gopkg.in/yaml.v3"
)

// StructuredEditor preserves YAML nodes (including unrelated comments/tags).
// DSH paths start with a Cordis row ID; subsequent components are mapping keys.
// JSON files use the same conditional field editing contract.
type StructuredEditor struct{ DSH bool }

func (e StructuredEditor) parse(raw []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) == 0 {
		kind := yaml.MappingNode
		if e.DSH {
			kind = yaml.SequenceNode
		}
		return &yaml.Node{Kind: kind}, nil
	}
	if !e.DSH && !json.Valid(raw) {
		return nil, ErrUnsafe
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&doc) != nil || len(doc.Content) != 1 {
		return nil, ErrUnsafe
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, ErrUnsafe
	}
	root := doc.Content[0]
	if e.DSH && root.Kind != yaml.SequenceNode || !e.DSH && root.Kind != yaml.MappingNode {
		return nil, ErrUnsafe
	}
	// Reject duplicate keys and aliases: ambiguous writes must never become
	// a destructive normalization. Custom tags on unrelated DSH scalars survive.
	var check func(*yaml.Node) bool
	check = func(n *yaml.Node) bool {
		if n.Kind == yaml.AliasNode {
			return false
		}
		if n.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i]
				if k.Kind != yaml.ScalarNode || seen[k.Value] || k.Value == "<<" {
					return false
				}
				seen[k.Value] = true
			}
		}
		for _, c := range n.Content {
			if !check(c) {
				return false
			}
		}
		return true
	}
	if !check(root) {
		return nil, ErrUnsafe
	}
	if e.DSH {
		seen := map[string]bool{}
		for _, row := range root.Content {
			if row.Kind != yaml.MappingNode {
				return nil, ErrUnsafe
			}
			if id := nodeKey(row, "id"); id != nil {
				if seen[id.Value] {
					return nil, ErrUnsafe
				}
				seen[id.Value] = true
			}
		}
	}
	return root, nil
}
func nodeKey(n *yaml.Node, key string) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func putNode(n *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			if value == nil {
				n.Content = append(n.Content[:i], n.Content[i+2:]...)
			} else {
				n.Content[i+1] = value
			}
			return
		}
	}
	if value != nil {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	}
}
func (e StructuredEditor) field(root *yaml.Node, path []string, create bool) (*yaml.Node, string, error) {
	if len(path) == 0 {
		return nil, "", ErrUnsafe
	}
	n := root
	if e.DSH {
		if len(path) < 2 {
			return nil, "", ErrUnsafe
		}
		n = nil
		for _, row := range root.Content {
			if id := nodeKey(row, "id"); id != nil && id.Value == path[0] {
				n = row
			}
		}
		if n == nil && create {
			n = &yaml.Node{Kind: yaml.MappingNode}
			putNode(n, "id", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]})
			root.Content = append(root.Content, n)
		}
		path = path[1:]
	}
	for _, key := range path[:len(path)-1] {
		if n == nil {
			return nil, path[len(path)-1], nil
		}
		if n.Kind != yaml.MappingNode {
			return nil, "", ErrUnsafe
		}
		next := nodeKey(n, key)
		if next == nil && create {
			next = &yaml.Node{Kind: yaml.MappingNode}
			putNode(n, key, next)
		}
		n = next
	}
	if n != nil && n.Kind != yaml.MappingNode {
		return nil, "", ErrUnsafe
	}
	return n, path[len(path)-1], nil
}
func nodeValue(n *yaml.Node) (Value, error) {
	if n == nil {
		return Value{}, nil
	}
	var v any
	if n.Decode(&v) != nil {
		return Value{}, ErrUnsafe
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return Value{}, ErrUnsafe
	}
	var norm any
	_ = json.Unmarshal(raw, &norm)
	return Value{Exists: true, Data: norm}, nil
}
func (e StructuredEditor) Values(raw []byte, paths [][]string) ([]Value, error) {
	root, err := e.parse(raw)
	if err != nil {
		return nil, err
	}
	out := []Value{}
	for _, path := range paths {
		if e.DSH && len(path) == 1 {
			var row *yaml.Node
			for _, r := range root.Content {
				if id := nodeKey(r, "id"); id != nil && id.Value == path[0] {
					row = r
				}
			}
			v, err := nodeValue(row)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		parent, key, err := e.field(root, path, false)
		if err != nil {
			return nil, err
		}
		var node *yaml.Node
		if parent != nil {
			node = nodeKey(parent, key)
		}
		v, err := nodeValue(node)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (e StructuredEditor) Patch(raw []byte, edits []Edit) ([]byte, error) {
	root, err := e.parse(raw)
	if err != nil {
		return nil, err
	}
	for _, edit := range edits {
		if e.DSH && len(edit.Path) == 1 {
			idx := -1
			var row *yaml.Node
			for i, r := range root.Content {
				if id := nodeKey(r, "id"); id != nil && id.Value == edit.Path[0] {
					idx = i
					row = r
				}
			}
			before, err := nodeValue(row)
			if err != nil {
				return nil, err
			}
			if !equal(before, edit.Before) {
				return nil, ErrChanged
			}
			if edit.After.Exists {
				var n yaml.Node
				if n.Encode(edit.After.Data) != nil {
					return nil, ErrUnsafe
				}
				if id := nodeKey(&n, "id"); id == nil || id.Value != edit.Path[0] {
					return nil, ErrUnsafe
				}
				if idx < 0 {
					root.Content = append(root.Content, &n)
				} else {
					root.Content[idx] = &n
				}
			} else if idx >= 0 {
				root.Content = append(root.Content[:idx], root.Content[idx+1:]...)
			}
			continue
		}
		parent, key, err := e.field(root, edit.Path, false)
		if err != nil {
			return nil, err
		}
		var node *yaml.Node
		if parent != nil {
			node = nodeKey(parent, key)
		}
		before, err := nodeValue(node)
		if err != nil {
			return nil, err
		}
		if !equal(before, edit.Before) {
			return nil, ErrChanged
		}
		if !edit.After.Exists {
			if parent != nil {
				putNode(parent, key, nil)
			}
			continue
		}
		parent, key, err = e.field(root, edit.Path, true)
		if err != nil {
			return nil, err
		}
		var after yaml.Node
		if after.Encode(edit.After.Data) != nil {
			return nil, ErrUnsafe
		}
		putNode(parent, key, &after)
	}
	if !e.DSH {
		var v any
		if root.Decode(&v) != nil {
			return nil, ErrUnsafe
		}
		out, err := json.MarshalIndent(v, "", "  ")
		return append(out, '\n'), err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}
