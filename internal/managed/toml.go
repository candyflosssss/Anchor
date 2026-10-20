// Package managed owns configuration planning and recovery. It has no upstream
// HTTP client and never executes an Agent or a user-provided script.
package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/candyflosssss/Anchor/internal/naming"
)

var ErrUnsafe = errors.New("unsupported or unsafe configuration; no destructive reserialization was attempted")
var ErrChanged = errors.New("configuration changed; inspect and recover before retrying")

type Value struct {
	Exists bool `json:"exists"`
	Data   any  `json:"data,omitempty"`
}
type Edit struct {
	Path   []string `json:"path"`
	Before Value    `json:"before"`
	After  Value    `json:"after"`
	// PlanningBefore keeps the pre-enrollment shape when explicit takeover
	// changes Before to an external recovery value (e.g. a created DSH row).
	PlanningBefore *Value `json:"planning_before,omitempty"`
}

// Editor is deliberately narrow. Future JSON/YAML adapters can implement the
// same field-level contract without acquiring independent filesystem privileges.
type Editor interface {
	Values([]byte, [][]string) ([]Value, error)
	Patch([]byte, []Edit) ([]byte, error)
}
type TOMLEditor struct{}

func document(b []byte) (map[string]any, error) {
	m := map[string]any{}
	if _, e := toml.Decode(string(b), &m); e != nil {
		return nil, ErrUnsafe
	}
	return m, nil
}
func normalize(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}
func at(m map[string]any, path []string) Value {
	var v any = m
	for _, p := range path {
		obj, ok := v.(map[string]any)
		if !ok {
			return Value{}
		}
		v, ok = obj[p]
		if !ok {
			return Value{}
		}
	}
	return Value{true, normalize(v)}
}
func equal(a, b Value) bool {
	return a.Exists == b.Exists && reflect.DeepEqual(normalize(a.Data), normalize(b.Data))
}
func (TOMLEditor) Values(b []byte, paths [][]string) ([]Value, error) {
	m, e := document(b)
	if e != nil {
		return nil, e
	}
	out := []Value{}
	for _, p := range paths {
		out = append(out, at(m, p))
	}
	return out, nil
}

var simpleKey = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func scalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		b, _ := json.Marshal(x)
		return string(b), nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	default:
		return "", ErrUnsafe
	}
}

// Recovery JSON uses floating point numbers. Emit integral TOML numbers as
// integers so Codex's unsigned timeout/retry fields remain valid.
func tomlNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if math.Trunc(x) == x && math.Abs(x) < (1<<53) {
			return int64(x)
		}
		return x
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			out[k] = tomlNumbers(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = tomlNumbers(v)
		}
		return out
	default:
		return v
	}
}

// Patch keeps all unrelated bytes and verifies the entire semantic document.
// Ambiguous/dotted/quoted/multiline root keys are rejected, not normalized.
// Unicode provider table names are emitted by the TOML encoder as quoted keys.
func (TOMLEditor) Patch(b []byte, edits []Edit) ([]byte, error) {
	original, e := document(b)
	if e != nil {
		return nil, e
	}
	expected, e := document(b)
	if e != nil {
		return nil, e
	}
	out := string(b)
	nl := "\n"
	if bytes.Contains(b, []byte("\r\n")) {
		nl = "\r\n"
	}
	for _, edit := range edits {
		if len(edit.Path) == 0 || !equal(at(original, edit.Path), edit.Before) {
			return nil, withDiagnostic(ErrChanged, "patch_configuration", "managed_field_mismatch", diagnosticField(edit.Path))
		}
		for i, p := range edit.Path {
			unicodeProvider := len(edit.Path) == 2 && edit.Path[0] == "model_providers" && i == 1 && naming.ValidID(p)
			if !simpleKey.MatchString(p) && !unicodeProvider {
				return nil, ErrUnsafe
			}
		}
		var obj = expected
		for _, p := range edit.Path[:len(edit.Path)-1] {
			v, ok := obj[p]
			if !ok {
				v = map[string]any{}
				obj[p] = v
			}
			obj, ok = v.(map[string]any)
			if !ok {
				return nil, ErrUnsafe
			}
		}
		key := edit.Path[len(edit.Path)-1]
		if edit.After.Exists {
			obj[key] = edit.After.Data
		} else {
			delete(obj, key)
		}
		if len(edit.Path) == 1 {
			lines := strings.SplitAfter(out, "\n")
			found := -1
			rootEnd := len(lines)
			re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*(.*?)(\r?\n)?$`)
			for i, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "[") || strings.HasPrefix(line, "# anchor:begin ") {
					rootEnd = i
					break
				}
				if re.MatchString(line) {
					if found != -1 {
						return nil, ErrUnsafe
					}
					found = i
				}
			}
			if edit.Before.Exists && found < 0 {
				return nil, ErrUnsafe
			}
			replacement := ""
			if edit.After.Exists {
				v, e := scalar(edit.After.Data)
				if e != nil {
					return nil, e
				}
				replacement = key + " = " + v + nl
			}
			if found >= 0 {
				// Only a one-line string/bool assignment may be rewritten.
				match := re.FindStringSubmatch(lines[found])
				raw := strings.TrimSpace(match[1])
				if strings.HasPrefix(raw, `"""`) || strings.HasPrefix(raw, `'''`) {
					return nil, ErrUnsafe
				}
				lines[found] = replacement
				out = strings.Join(lines, "")
			} else if replacement != "" {
				lines = append(lines[:rootEnd], append([]string{replacement}, lines[rootEnd:]...)...)
				out = strings.Join(lines, "")
			}
		} else if len(edit.Path) == 2 && edit.Path[0] == "model_providers" {
			begin := "# anchor:begin " + key
			end := "# anchor:end " + key
			if edit.Before.Exists {
				// Editors can preserve our comments while moving unrelated tables
				// inside them. Always locate the actual provider sections; marker
				// boundaries never establish ownership of intervening bytes.
				out, e = removeProviderSections(out, key)
				if e != nil {
					return nil, e
				}
			} else if strings.Contains(out, begin) || strings.Contains(out, end) {
				return nil, ErrUnsafe
			}
			if edit.After.Exists {
				table, ok := edit.After.Data.(map[string]any)
				if !ok {
					return nil, ErrUnsafe
				}
				var enc bytes.Buffer
				if e := toml.NewEncoder(&enc).Encode(map[string]any{"model_providers": map[string]any{key: tomlNumbers(table)}}); e != nil {
					return nil, ErrUnsafe
				}
				if out != "" && !strings.HasSuffix(out, "\n") {
					out += nl
				}
				// Do not redeclare the parent table: third-party serializers often
				// emit an explicit [model_providers] even when it is empty.
				section := strings.TrimPrefix(enc.String(), "[model_providers]\n")
				out += begin + nl + strings.ReplaceAll(section, "\n", nl) + end + nl
			}
		} else {
			return nil, ErrUnsafe
		}
	}
	// Omit empty parent tables on both sides after a table removal.
	clean := func(m map[string]any) {
		if v, ok := m["model_providers"].(map[string]any); ok && len(v) == 0 {
			delete(m, "model_providers")
		}
	}
	actual, e := document([]byte(out))
	if e != nil {
		return nil, e
	}
	clean(actual)
	clean(expected)
	if !reflect.DeepEqual(normalize(actual), normalize(expected)) {
		return nil, ErrUnsafe
	}
	return []byte(out), nil
}

// removeProviderSections handles ordinary, quoted and nested provider tables
// even when marker comments are missing, duplicated or enclose other tables.
// It never reserializes the
// document. The caller still verifies the entire resulting semantic document,
// so ambiguous inline/dotted/multiline forms fail closed without a file write.
func removeProviderSections(raw, provider string) (string, error) {
	const probe = "__anchor_section_probe__"
	var probePath func(map[string]any) []string
	probePath = func(m map[string]any) []string {
		if m[probe] == true {
			return []string{}
		}
		for key, value := range m {
			if child, ok := value.(map[string]any); ok {
				if path := probePath(child); path != nil {
					return append([]string{key}, path...)
				}
			}
		}
		return nil
	}
	lines := strings.SplitAfter(raw, "\n")
	var out strings.Builder
	removing, found := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			if doc, err := document([]byte(trimmed + "\n" + probe + " = true\n")); err == nil {
				path := probePath(doc)
				removing = len(path) >= 2 && path[0] == "model_providers" && path[1] == provider
				found = found || removing
			}
		}
		if trimmed == "# anchor:begin "+provider || trimmed == "# anchor:end "+provider {
			continue
		}
		if !removing || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
		}
	}
	if !found {
		return "", ErrUnsafe
	}
	return out.String(), nil
}
