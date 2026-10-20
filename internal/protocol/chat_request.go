package protocol

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Conversion errors expose code-owned categories, not user-supplied field names
// or values. allowed() errors may contain secrets in arbitrary JSON keys.
type conversionError struct {
	reason string
	err    error
}

func (e *conversionError) Error() string { return "unsupported protocol conversion: " + e.reason }
func (e *conversionError) Unwrap() error { return e.err }
func conversionFailure(reason string, err error) error {
	var failure *conversionError
	if errors.As(err, &failure) {
		return err
	}
	return &conversionError{reason: reason, err: err}
}

// ConversionReason is safe for error responses and persistent diagnostic logs.
func ConversionReason(err error) string {
	var failure *conversionError
	if errors.As(err, &failure) {
		return failure.reason
	}
	return "request_content"
}

type toolIdentity struct {
	namespace, name string
	custom          bool
}

// ChatMapping follows CPA's request-local declaration index: flatten namespace
// tools and wrap freeform inputs, then restore their identity on the way back.
// This is an independent implementation; no CPA source or dependencies are used.
type ChatMapping struct {
	byChat     map[string]toolIdentity
	byIdentity map[toolIdentity]string
}

func prepareChatRequest(m Object) (*ChatMapping, Object, error) {
	keys := "model input instructions stream temperature top_p tools tool_choice max_output_tokens store background parallel_tool_calls service_tier reasoning text include metadata user safety_identifier prompt_cache_key prompt_cache_retention truncation"
	if err := allowed(m, keys); err != nil {
		return nil, nil, conversionFailure("request_fields", err)
	}
	options := Object{}
	if v := m["reasoning"]; v != nil {
		r := obj(v)
		if err := allowed(r, "effort summary"); err != nil {
			return nil, nil, conversionFailure("reasoning_options", err)
		}
		if effort := r["effort"]; effort != nil {
			s, ok := effort.(string)
			if !ok || !chatReasoningEffort(s) {
				return nil, nil, conversionFailure("reasoning_options", ErrUnsupported)
			}
			options["reasoning_effort"] = s
		}
		if summary := r["summary"]; summary != nil && summary != "auto" && summary != "concise" && summary != "detailed" {
			return nil, nil, conversionFailure("reasoning_options", ErrUnsupported)
		}
		// Chat exposes plaintext reasoning_content, not a summary-style selector.
	}
	delete(m, "reasoning")
	if v := m["text"]; v != nil {
		t := obj(v)
		if err := allowed(t, "format verbosity"); err != nil {
			return nil, nil, conversionFailure("text_format", err)
		}
		if verbosity := t["verbosity"]; verbosity != nil {
			if verbosity != "low" && verbosity != "medium" && verbosity != "high" {
				return nil, nil, conversionFailure("text_format", ErrUnsupported)
			}
			options["verbosity"] = verbosity
		}
		if format := t["format"]; format != nil {
			f := obj(format)
			if err := allowed(f, "type name description schema strict"); err != nil {
				return nil, nil, conversionFailure("text_format", err)
			}
			switch f["type"] {
			case "text", "json_object":
				if err := allowed(f, "type"); err != nil {
					return nil, nil, conversionFailure("text_format", err)
				}
				options["response_format"] = f
			case "json_schema":
				if str(f["name"]) == "" || obj(f["schema"]) == nil {
					return nil, nil, conversionFailure("text_format", ErrUnsupported)
				}
				schema := Object{}
				for k, value := range f {
					if k != "type" {
						schema[k] = value
					}
				}
				options["response_format"] = Object{"type": "json_schema", "json_schema": schema}
			default:
				return nil, nil, conversionFailure("text_format", ErrUnsupported)
			}
		}
	}
	delete(m, "text")
	if v := m["include"]; v != nil {
		list, ok := v.([]any)
		if !ok {
			return nil, nil, conversionFailure("include_options", ErrUnsupported)
		}
		for _, item := range list {
			if item != "reasoning.encrypted_content" {
				return nil, nil, conversionFailure("include_options", ErrUnsupported)
			}
		}
		// Requesting opaque reasoning is optional. Chat cannot generate it; never
		// fabricate ciphertext. Existing opaque history is checked separately.
	}
	delete(m, "include")
	if v := m["truncation"]; v != nil && v != "disabled" {
		return nil, nil, conversionFailure("truncation", ErrUnsupported)
	}
	delete(m, "truncation")
	for _, key := range []string{"metadata", "user", "safety_identifier", "prompt_cache_key", "prompt_cache_retention"} {
		if v, ok := m[key]; ok {
			options[key] = v
			delete(m, key)
		}
	}
	// Responses Lite can carry tool declarations inline with the input history.
	if input, ok := m["input"].([]any); ok {
		kept := make([]any, 0, len(input))
		for _, value := range input {
			item := obj(value)
			if item["type"] != "additional_tools" {
				kept = append(kept, value)
				continue
			}
			children, ok := item["tools"].([]any)
			if !ok || allowed(item, "type tools") != nil || m["tools"] != nil && arr(m["tools"]) == nil {
				return nil, nil, conversionFailure("tools", ErrUnsupported)
			}
			m["tools"] = append(arr(m["tools"]), children...)
		}
		m["input"] = kept
	}
	mapping := &ChatMapping{byChat: map[string]toolIdentity{}, byIdentity: map[toolIdentity]string{}}
	if err := mapping.prepareTools(m); err != nil {
		return nil, nil, conversionFailure("tools", err)
	}
	return mapping, options, nil
}

func chatReasoningEffort(effort string) bool {
	switch effort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "default":
		return true
	}
	return false
}

func (c *ChatMapping) alias(id toolIdentity) (string, error) {
	if name, exists := c.byIdentity[id]; exists {
		return name, nil
	}
	if id.name == "" {
		return "", ErrUnsupported
	}
	name := id.name
	if id.namespace != "" {
		name = id.namespace + "__" + id.name
	}
	valid := len(name) <= 64
	for _, ch := range name {
		valid = valid && (ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-')
	}
	if !valid {
		name = fmt.Sprintf("anchor_tool_%x", sha256.Sum256([]byte(fmt.Sprintf("%q/%q/%t", id.namespace, id.name, id.custom))))[:60]
	}
	// Never silently deduplicate two distinct identities that flatten alike.
	if prior, exists := c.byChat[name]; exists && prior != id {
		name = fmt.Sprintf("anchor_tool_%x", sha256.Sum256([]byte(fmt.Sprintf("%q/%q/%t", id.namespace, id.name, id.custom))))[:60]
	}
	if prior, exists := c.byChat[name]; exists && prior != id {
		return "", ErrUnsupported
	}
	c.byIdentity[id], c.byChat[name] = name, id
	return name, nil
}

// A namespace-less history/choice may name a declared local tool, but only
// when that identity is unambiguous. Never guess between equal local names.
func (c *ChatMapping) resolve(id toolIdentity, history bool) (string, error) {
	if name, ok := c.byIdentity[id]; ok {
		return name, nil
	}
	if id.namespace == "" {
		match := ""
		for candidate, name := range c.byIdentity {
			if candidate.name == id.name && candidate.custom == id.custom {
				if match != "" && match != name {
					return "", ErrUnsupported
				}
				match = name
			}
		}
		if match != "" {
			return match, nil
		}
		if candidate, ok := c.byChat[id.name]; ok && candidate.custom == id.custom {
			return id.name, nil
		}
	}
	if !history {
		return "", ErrUnsupported
	}
	return c.alias(id)
}

func (c *ChatMapping) prepareTools(m Object) error {
	tools := []any{}
	declared := map[toolIdentity]string{}
	var add func(Object, string) error
	add = func(t Object, namespace string) error {
		if t["type"] == "namespace" {
			if namespace != "" || allowed(t, "type name description tools") != nil || str(t["name"]) == "" {
				return ErrUnsupported
			}
			children, ok := t["tools"].([]any)
			if !ok || len(children) == 0 {
				return ErrUnsupported
			}
			for _, child := range children {
				if err := add(obj(child), str(t["name"])); err != nil {
					return err
				}
			}
			return nil
		}
		custom := t["type"] == "custom"
		if t["type"] != "function" && !custom {
			return ErrUnsupported
		}
		keys := "type name description parameters strict"
		if custom {
			keys = "type name description format"
		}
		if err := allowed(t, keys); err != nil {
			return err
		}
		if custom && t["format"] != nil {
			format := obj(t["format"])
			if format["type"] == "grammar" {
				if allowed(format, "type syntax definition") != nil || (format["syntax"] != "lark" && format["syntax"] != "regex") || str(format["definition"]) == "" {
					return conversionFailure("custom_tool_grammar", ErrUnsupported)
				}
				// Chat has no native freeform grammar constraint. Preserve it as
				// an instruction, not as a fabricated structured-output guarantee.
				t["description"] = str(t["description"]) + "\nThe input must follow this " + str(format["syntax"]) + " grammar:\n" + str(format["definition"])
			} else if allowed(format, "type") != nil || format["type"] != "text" {
				return conversionFailure("custom_tool_grammar", ErrUnsupported)
			}
		}
		id := toolIdentity{namespace: namespace, name: str(t["name"]), custom: custom}
		definition, _ := json.Marshal(t)
		if prior, exists := declared[id]; exists {
			if prior != string(definition) {
				return ErrUnsupported
			}
			return nil
		}
		declared[id] = string(definition)
		name, err := c.alias(id)
		if err != nil {
			return err
		}
		flat := Object{"type": "function", "name": name}
		for _, key := range []string{"description", "parameters", "strict"} {
			if v, ok := t[key]; ok {
				flat[key] = v
			}
		}
		if custom {
			flat["parameters"] = Object{"type": "object", "properties": Object{"input": Object{"type": "string"}}, "required": []any{"input"}, "additionalProperties": false}
		}
		tools = append(tools, flat)
		return nil
	}
	if v := m["tools"]; v != nil {
		list, ok := v.([]any)
		if !ok {
			return ErrUnsupported
		}
		for _, tool := range list {
			if err := add(obj(tool), ""); err != nil {
				return err
			}
		}
		m["tools"] = tools
	}
	if choice := obj(m["tool_choice"]); choice != nil {
		if allowed(choice, "type name namespace") != nil || (choice["type"] != "function" && choice["type"] != "custom") {
			return ErrUnsupported
		}
		id, err := chatToolIdentity(choice, choice["type"] == "custom")
		if err != nil {
			return err
		}
		name, err := c.resolve(id, false)
		if err != nil {
			return err
		}
		m["tool_choice"] = Object{"type": "function", "name": name}
	}
	for _, value := range arr(m["input"]) {
		item := obj(value)
		switch item["type"] {
		case "function_call", "custom_tool_call":
			custom := item["type"] == "custom_tool_call"
			keys := "type id call_id name namespace arguments status"
			if custom {
				keys = "type id call_id name namespace input status"
			}
			if allowed(item, keys) != nil {
				return ErrUnsupported
			}
			id, err := chatToolIdentity(item, custom)
			if err != nil {
				return err
			}
			name, err := c.resolve(id, true)
			if err != nil {
				return err
			}
			item["name"], item["type"] = name, "function_call"
			delete(item, "namespace")
			if custom {
				input, ok := item["input"].(string)
				if !ok {
					return ErrUnsupported
				}
				args, _ := json.Marshal(Object{"input": input})
				item["arguments"] = string(args)
				delete(item, "input")
			}
		case "custom_tool_call_output":
			item["type"] = "function_call_output"
		}
	}
	return nil
}

func chatToolIdentity(item Object, custom bool) (toolIdentity, error) {
	id := toolIdentity{name: str(item["name"]), custom: custom}
	if id.name == "" {
		return id, ErrUnsupported
	}
	if v := item["namespace"]; v != nil {
		namespace, ok := v.(string)
		if !ok {
			return id, ErrUnsupported
		}
		id.namespace = namespace
	}
	return id, nil
}

func chatHistoryReasoning(item Object) (string, error) {
	if allowed(item, "type id status summary content encrypted_content") != nil {
		return "", conversionFailure("reasoning_history", ErrUnsupported)
	}
	var out strings.Builder
	for _, key := range []string{"summary", "content"} {
		if item[key] == nil {
			continue
		}
		parts, ok := item[key].([]any)
		if !ok {
			return "", conversionFailure("reasoning_history", ErrUnsupported)
		}
		for _, part := range parts {
			p := obj(part)
			if allowed(p, "type text") != nil || (p["type"] != "summary_text" && p["type"] != "reasoning_text") {
				return "", conversionFailure("reasoning_history", ErrUnsupported)
			}
			t, ok := p["text"].(string)
			if !ok {
				return "", conversionFailure("reasoning_history", ErrUnsupported)
			}
			out.WriteString(t)
		}
	}
	if v := item["encrypted_content"]; v != nil && v != "" {
		return "", conversionFailure("encrypted_reasoning_history", ErrUnsupported)
	}
	return out.String(), nil
}

func (c *ChatMapping) restore(item *Item) {
	if c == nil || !item.Tool {
		return
	}
	if id, exists := c.byChat[item.Name]; exists {
		item.Name, item.Namespace, item.Custom = id.name, id.namespace, id.custom
	}
}

func customToolInput(arguments string) (string, error) {
	var args Object
	if json.Unmarshal([]byte(arguments), &args) != nil || len(args) != 1 {
		return "", ErrUnsupported
	}
	input, ok := args["input"].(string)
	if !ok {
		return "", ErrUnsupported
	}
	return input, nil
}

// RestoreResult is also used for non-streaming responses. Custom wrappers are
// validated before downstream headers are committed, never returned as text.
func (c *ChatMapping) RestoreResult(r *Result) error {
	for i := range r.Items {
		c.restore(&r.Items[i])
		if r.Items[i].Custom {
			if _, err := customToolInput(r.Items[i].Arguments); err != nil {
				return err
			}
		}
	}
	return nil
}

func chatIngressOptions(m, out Object) error {
	if v := m["reasoning_effort"]; v != nil {
		out["reasoning"] = Object{"effort": v}
	}
	text := Object{}
	if v := m["verbosity"]; v != nil {
		text["verbosity"] = v
	}
	if v := m["response_format"]; v != nil {
		f := obj(v)
		if allowed(f, "type json_schema") != nil {
			return conversionFailure("text_format", ErrUnsupported)
		}
		switch f["type"] {
		case "text", "json_object":
			if allowed(f, "type") != nil {
				return conversionFailure("text_format", ErrUnsupported)
			}
			text["format"] = f
		case "json_schema":
			s := obj(f["json_schema"])
			if allowed(s, "name description schema strict") != nil || str(s["name"]) == "" || obj(s["schema"]) == nil {
				return conversionFailure("text_format", ErrUnsupported)
			}
			format := Object{"type": "json_schema"}
			for key, value := range s {
				format[key] = value
			}
			text["format"] = format
		default:
			return conversionFailure("text_format", ErrUnsupported)
		}
	}
	if len(text) > 0 {
		out["text"] = text
	}
	for _, key := range []string{"metadata", "user", "safety_identifier", "prompt_cache_key", "prompt_cache_retention"} {
		if v, ok := m[key]; ok {
			out[key] = v
		}
	}
	return nil
}
