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
			start := strings.Index(out, begin)
			if start > 0 && out[start-1] != '\n' {
				return nil, ErrUnsafe
			}
			finish := strings.Index(out, end)
			if finish > 0 && out[finish-1] != '\n' {
				return nil, ErrUnsafe
			}
			if edit.Before.Exists {
				if start < 0 || finish < start || strings.Count(out, begin) != 1 || strings.Count(out, end) != 1 {
					return nil, ErrUnsafe
				}
				finish += len(end)
				if strings.HasPrefix(out[finish:], "\r\n") {
					finish += 2
				} else if strings.HasPrefix(out[finish:], "\n") {
					finish++
				}
				out = out[:start] + out[finish:]
			} else if start >= 0 || finish >= 0 {
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
				out += begin + nl + strings.ReplaceAll(enc.String(), "\n", nl) + end + nl
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
