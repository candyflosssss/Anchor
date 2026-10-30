// Package protocol converts the explicitly supported stateless text/function subset.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Object = map[string]any

var ErrUnsupported = errors.New("unsupported protocol field or content")

func obj(v any) Object { m, _ := v.(map[string]any); return m }
func arr(v any) []any  { a, _ := v.([]any); return a }
func str(v any) string { s, _ := v.(string); return s }
func num(v any) int64  { n, _ := v.(float64); return int64(n) }
func allowed(m Object, keys string) error {
	if m == nil {
		return ErrUnsupported
	}
	for k, v := range m {
		if v != nil && !strings.Contains(" "+keys+" ", " "+k+" ") {
			return fmt.Errorf("%w: %s", ErrUnsupported, k)
		}
	}
	return nil
}
func text(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	if v == nil {
		return "", nil
	}
	a, ok := v.([]any)
	if !ok {
		return "", ErrUnsupported
	}
	var out strings.Builder
	for _, v := range a {
		m := obj(v)
		if err := allowed(m, "type text annotations"); err != nil {
			return "", err
		}
		switch str(m["type"]) {
		case "text", "input_text", "output_text":
			if annotations, exists := m["annotations"]; exists && annotations != nil {
				a, ok := annotations.([]any)
				if !ok || len(a) > 0 {
					return "", ErrUnsupported
				}
			}
			t, ok := m["text"].(string)
			if !ok {
				return "", ErrUnsupported
			}
			out.WriteString(t)
		default:
			return "", ErrUnsupported
		}
	}
	return out.String(), nil
}
func Decode(raw []byte) (Object, error) {
	var m Object
	err := json.Unmarshal(raw, &m)
	if err != nil || m == nil {
		return nil, errors.New("invalid JSON")
	}
	return m, nil
}
func ConvertRequest(raw []byte, from, to string) ([]byte, error) {
	body, _, err := ConvertRequestMapped(raw, from, to)
	return body, err
}

// ConvertRequestMapped retains request-local tool identities for the return
// path. A mapping must never be shared between requests or candidate attempts.
func ConvertRequestMapped(raw []byte, from, to string) ([]byte, *ChatMapping, error) {
	if from == to {
		return raw, nil, nil
	}
	m, err := Decode(raw)
	if err != nil {
		return nil, nil, err
	}
	if from != "responses" {
		m, err = toResponses(m, from)
		if err != nil {
			return nil, nil, err
		}
	}
	var mapping *ChatMapping
	var options Object
	if to == "chat-completions" || to == "anthropic" {
		if to == "anthropic" {
			mapping, options, err = prepareAnthropicRequest(m)
		} else {
			mapping, options, err = prepareChatRequest(m)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	if to != "responses" {
		m, err = fromResponses(m, to)
		if err != nil {
			return nil, nil, conversionFailure("request_content", err)
		}
	}
	for k, v := range options {
		m[k] = v
	}
	body, err := json.Marshal(m)
	return body, mapping, err
}
func toResponses(m Object, from string) (Object, error) {
	keys := "model messages stream temperature top_p tools tool_choice max_tokens"
	if from == "anthropic" {
		keys += " system metadata"
	} else {
		keys += " max_completion_tokens stream_options parallel_tool_calls service_tier reasoning_effort response_format metadata user safety_identifier prompt_cache_key prompt_cache_retention verbosity"
	}
	if err := allowed(m, keys); err != nil {
		return nil, err
	}
	out := Object{"model": m["model"], "store": false}
	if from == "anthropic" {
		if err := prepareAnthropicIngress(m, out); err != nil {
			return nil, err
		}
	}
	if from == "chat-completions" {
		if err := chatIngressOptions(m, out); err != nil {
			return nil, err
		}
		if tier, ok := m["service_tier"]; ok {
			out["service_tier"] = tier
		}
	}
	for _, k := range []string{"stream", "temperature", "top_p", "parallel_tool_calls"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if m["max_tokens"] != nil && m["max_completion_tokens"] != nil {
		return nil, ErrUnsupported
	}
	if m["stream_options"] != nil && obj(m["stream_options"]) == nil {
		return nil, ErrUnsupported
	}
	if opts := obj(m["stream_options"]); opts != nil {
		if err := allowed(opts, "include_usage"); err != nil {
			return nil, err
		}
	}
	if v, ok := m["max_tokens"]; ok {
		out["max_output_tokens"] = v
	}
	if v, ok := m["max_completion_tokens"]; ok {
		out["max_output_tokens"] = v
	}
	if v := m["system"]; v != nil {
		t, e := text(v)
		if e != nil {
			return nil, e
		}
		out["instructions"] = t
	}
	input := []any{}
	messages, ok := m["messages"].([]any)
	if !ok || len(messages) == 0 {
		return nil, ErrUnsupported
	}
	for _, v := range messages {
		msg := obj(v)
		messageKeys := "role content"
		if from == "chat-completions" {
			messageKeys += " tool_calls tool_call_id"
			messageKeys += " reasoning_content"
		}
		if err := allowed(msg, messageKeys); err != nil {
			return nil, err
		}
		role := str(msg["role"])
		if from == "anthropic" && role != "user" && role != "assistant" {
			return nil, ErrUnsupported
		}
		switch role {
		case "user", "assistant", "system", "developer", "tool":
		default:
			return nil, ErrUnsupported
		}
		if reasoning := msg["reasoning_content"]; reasoning != nil {
			t, ok := reasoning.(string)
			if !ok || role != "assistant" {
				return nil, conversionFailure("reasoning_history", ErrUnsupported)
			}
			if t != "" {
				input = append(input, Object{"type": "reasoning", "summary": []any{Object{"type": "summary_text", "text": t}}})
			}
		}
		if role == "tool" {
			t, e := text(msg["content"])
			if e != nil {
				return nil, e
			}
			input = append(input, Object{"type": "function_call_output", "call_id": msg["tool_call_id"], "output": t})
			continue
		}
		if from == "anthropic" {
			if blocks, ok := msg["content"].([]any); ok {
				for _, v := range blocks {
					b := obj(v)
					switch str(b["type"]) {
					case "text":
						if err := allowed(b, "type text"); err != nil {
							return nil, err
						}
						if _, ok := b["text"].(string); !ok {
							return nil, ErrUnsupported
						}
						input = append(input, Object{"role": role, "content": b["text"]})
					case "tool_use":
						if err := allowed(b, "type id name input"); err != nil {
							return nil, err
						}
						if role != "assistant" || str(b["id"]) == "" || str(b["name"]) == "" || obj(b["input"]) == nil {
							return nil, ErrUnsupported
						}
						args, _ := json.Marshal(b["input"])
						input = append(input, Object{"type": "function_call", "call_id": b["id"], "name": b["name"], "arguments": string(args)})
					case "tool_result":
						if err := allowed(b, "type tool_use_id content is_error"); err != nil {
							return nil, err
						}
						if role != "user" || str(b["tool_use_id"]) == "" {
							return nil, ErrUnsupported
						}
						t, e := text(b["content"])
						if e != nil {
							return nil, e
						}
						if flag := b["is_error"]; flag != nil {
							failed, ok := flag.(bool)
							if !ok {
								return nil, ErrUnsupported
							}
							if failed {
								t = "[Tool execution failed]\n" + t
							}
						}
						input = append(input, Object{"type": "function_call_output", "call_id": b["tool_use_id"], "output": t})
					default:
						return nil, ErrUnsupported
					}
				}
				continue
			}
		}
		t, e := text(msg["content"])
		if e != nil {
			return nil, e
		}
		if t != "" {
			input = append(input, Object{"role": role, "content": t})
		}
		if msg["tool_calls"] != nil {
			if _, ok := msg["tool_calls"].([]any); !ok {
				return nil, ErrUnsupported
			}
		}
		for _, v := range arr(msg["tool_calls"]) {
			call := obj(v)
			if err := allowed(call, "id type function"); err != nil {
				return nil, err
			}
			if call["type"] != "function" {
				return nil, ErrUnsupported
			}
			f := obj(call["function"])
			if err := allowed(f, "name arguments"); err != nil {
				return nil, err
			}
			input = append(input, Object{"type": "function_call", "call_id": call["id"], "name": f["name"], "arguments": f["arguments"]})
		}
	}
	out["input"] = input
	if ts, exists := m["tools"]; exists {
		tools := []any{}
		list, ok := ts.([]any)
		if !ok && ts != nil {
			return nil, ErrUnsupported
		}
		for _, v := range list {
			t := obj(v)
			if from == "anthropic" {
				if err := allowed(t, "type name description input_schema"); err != nil {
					return nil, err
				}
				if t["type"] != nil && t["type"] != "custom" || str(t["name"]) == "" || obj(t["input_schema"]) == nil {
					return nil, ErrUnsupported
				}
				tools = append(tools, Object{"type": "function", "name": t["name"], "description": t["description"], "parameters": t["input_schema"]})
			} else {
				if t["type"] != "function" {
					return nil, ErrUnsupported
				}
				f := obj(t["function"])
				if err := allowed(t, "type function"); err != nil {
					return nil, err
				}
				if err := allowed(f, "name description parameters strict"); err != nil {
					return nil, err
				}
				c := Object{"type": "function"}
				for k, v := range f {
					c[k] = v
				}
				tools = append(tools, c)
			}
		}
		out["tools"] = tools
	}
	if v := m["tool_choice"]; v != nil {
		if from == "anthropic" {
			c := obj(v)
			if err := allowed(c, "type name disable_parallel_tool_use"); err != nil {
				return nil, err
			}
			if flag := c["disable_parallel_tool_use"]; flag != nil {
				disabled, ok := flag.(bool)
				if !ok || c["type"] == "none" {
					return nil, ErrUnsupported
				}
				out["parallel_tool_calls"] = !disabled
			}
			if c["type"] == "tool" {
				if str(c["name"]) == "" {
					return nil, ErrUnsupported
				}
			} else if c["name"] != nil {
				return nil, ErrUnsupported
			}
			switch c["type"] {
			case "auto":
				out["tool_choice"] = "auto"
			case "any":
				out["tool_choice"] = "required"
			case "none":
				out["tool_choice"] = "none"
			case "tool":
				out["tool_choice"] = Object{"type": "function", "name": c["name"]}
			default:
				return nil, ErrUnsupported
			}
		} else if c := obj(v); c != nil {
			if allowed(c, "type function") != nil || allowed(obj(c["function"]), "name") != nil {
				return nil, ErrUnsupported
			}
			if c["type"] != "function" {
				return nil, ErrUnsupported
			}
			out["tool_choice"] = Object{"type": "function", "name": obj(c["function"])["name"]}
		} else {
			out["tool_choice"] = v
		}
	}
	return out, nil
}
func fromResponses(m Object, to string) (Object, error) {
	keys := "model input instructions stream temperature top_p tools tool_choice max_output_tokens store background parallel_tool_calls"
	if to == "chat-completions" {
		keys += " service_tier"
	}
	if err := allowed(m, keys); err != nil {
		return nil, err
	}
	if m["store"] == true || m["background"] == true {
		return nil, ErrUnsupported
	}
	out := Object{"model": m["model"]}
	if to == "chat-completions" {
		if tier, ok := m["service_tier"]; ok {
			out["service_tier"] = tier
		}
	}
	for _, k := range []string{"stream", "temperature", "top_p"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if to == "anthropic" {
		out["max_tokens"] = 4096
		if v, ok := m["max_output_tokens"]; ok {
			out["max_tokens"] = v
		}
		if v, ok := m["instructions"]; ok {
			out["system"] = v
		}
	} else {
		if v, ok := m["max_output_tokens"]; ok {
			out["max_completion_tokens"] = v
		}
		if v, ok := m["parallel_tool_calls"]; ok {
			out["parallel_tool_calls"] = v
		}
		if m["stream"] == true {
			out["stream_options"] = Object{"include_usage": true}
		}
	}
	messages := []any{}
	if to != "anthropic" && m["instructions"] != nil {
		messages = append(messages, Object{"role": "system", "content": m["instructions"]})
	}
	input, ok := m["input"].([]any)
	if t, yes := m["input"].(string); yes {
		input = []any{Object{"role": "user", "content": t}}
		ok = true
	}
	if !ok {
		return nil, ErrUnsupported
	}
	for _, v := range input {
		item := obj(v)
		switch str(item["type"]) {
		case "", "message":
			keys := "type role content id status"
			if to == "chat-completions" {
				keys += " reasoning_content"
			}
			if err := allowed(item, keys); err != nil {
				return nil, err
			}
			role := str(item["role"])
			if role != "user" && role != "assistant" && role != "system" && role != "developer" {
				return nil, ErrUnsupported
			}
			t, e := text(item["content"])
			if e != nil {
				return nil, e
			}
			if to == "anthropic" && (role == "system" || role == "developer") {
				if len(messages) > 0 {
					return nil, ErrUnsupported
				}
				if prior := str(out["system"]); prior != "" {
					t = prior + "\n" + t
				}
				out["system"] = t
			} else {
				message := Object{"role": role, "content": t}
				if item["reasoning_content"] != nil {
					if _, ok := item["reasoning_content"].(string); !ok || role != "assistant" {
						return nil, ErrUnsupported
					}
					message["reasoning_content"] = item["reasoning_content"]
				}
				messages = append(messages, message)
			}
		case "reasoning":
			t, err := chatHistoryReasoning(item)
			if err != nil {
				return nil, err
			}
			if t != "" {
				if to != "chat-completions" {
					return nil, conversionFailure("reasoning_history", ErrUnsupported)
				}
				messages = append(messages, Object{"role": "assistant", "content": "", "reasoning_content": t})
			}
		case "function_call":
			if str(item["call_id"]) == "" || str(item["name"]) == "" || !json.Valid([]byte(str(item["arguments"]))) {
				return nil, ErrUnsupported
			}
			if err := allowed(item, "type id call_id name arguments status"); err != nil {
				return nil, err
			}
			if to == "anthropic" {
				var args Object
				if json.Unmarshal([]byte(str(item["arguments"])), &args) != nil || args == nil {
					return nil, ErrUnsupported
				}
				messages = append(messages, Object{"role": "assistant", "content": []any{Object{"type": "tool_use", "id": item["call_id"], "name": item["name"], "input": args}}})
			} else {
				messages = append(messages, Object{"role": "assistant", "content": nil, "tool_calls": []any{Object{"id": item["call_id"], "type": "function", "function": Object{"name": item["name"], "arguments": item["arguments"]}}}})
			}
		case "function_call_output":
			if str(item["call_id"]) == "" {
				return nil, ErrUnsupported
			}
			if err := allowed(item, "type id call_id output status"); err != nil {
				return nil, err
			}
			t, e := text(item["output"])
			if e != nil {
				return nil, e
			}
			if to == "anthropic" {
				messages = append(messages, Object{"role": "user", "content": []any{Object{"type": "tool_result", "tool_use_id": item["call_id"], "content": t}}})
			} else {
				messages = append(messages, Object{"role": "tool", "tool_call_id": item["call_id"], "content": t})
			}
		default:
			return nil, ErrUnsupported
		}
	}
	// Merge adjacent Anthropic roles so parallel tool results form a single turn.
	if to == "anthropic" {
		merged := []any{}
		for _, v := range messages {
			m := obj(v)
			if t, ok := m["content"].(string); ok {
				m["content"] = []any{Object{"type": "text", "text": t}}
			}
			if len(merged) > 0 && obj(merged[len(merged)-1])["role"] == m["role"] {
				prev := obj(merged[len(merged)-1])
				prev["content"] = append(arr(prev["content"]), arr(m["content"])...)
			} else {
				merged = append(merged, m)
			}
		}
		messages = merged
	}
	// Chat requires parallel tool calls in one assistant turn, before their results.
	if to == "chat-completions" {
		merged := []any{}
		for _, v := range messages {
			m := obj(v)
			if len(merged) > 0 && m["role"] == "assistant" && obj(merged[len(merged)-1])["role"] == "assistant" {
				prev := obj(merged[len(merged)-1])
				prev["content"] = str(prev["content"]) + str(m["content"])
				if reasoning, ok := m["reasoning_content"]; ok {
					prev["reasoning_content"] = str(prev["reasoning_content"]) + str(reasoning)
				}
				if calls := arr(m["tool_calls"]); len(calls) > 0 {
					prev["tool_calls"] = append(arr(prev["tool_calls"]), calls...)
				}
			} else {
				merged = append(merged, m)
			}
		}
		messages = merged
	}
	out["messages"] = messages
	if ts, exists := m["tools"]; exists {
		tools := []any{}
		list, ok := ts.([]any)
		if !ok && ts != nil {
			return nil, ErrUnsupported
		}
		for _, v := range list {
			t := obj(v)
			if t["type"] != "function" {
				return nil, ErrUnsupported
			}
			if err := allowed(t, "type name description parameters strict"); err != nil {
				return nil, err
			}
			if to == "anthropic" {
				if t["strict"] == true {
					return nil, ErrUnsupported
				}
				tools = append(tools, Object{"name": t["name"], "description": t["description"], "input_schema": t["parameters"]})
			} else {
				f := Object{}
				for k, v := range t {
					if k != "type" {
						f[k] = v
					}
				}
				tools = append(tools, Object{"type": "function", "function": f})
			}
		}
		out["tools"] = tools
	}
	if v := m["tool_choice"]; v != nil {
		if c := obj(v); c != nil {
			if allowed(c, "type name") != nil {
				return nil, ErrUnsupported
			}
			if c["type"] != "function" {
				return nil, ErrUnsupported
			}
			if to == "anthropic" {
				out["tool_choice"] = Object{"type": "tool", "name": c["name"]}
			} else {
				out["tool_choice"] = Object{"type": "function", "function": Object{"name": c["name"]}}
			}
		} else if to == "anthropic" {
			typ := str(v)
			if typ == "required" {
				typ = "any"
			}
			if typ != "auto" && typ != "any" && typ != "none" {
				return nil, ErrUnsupported
			}
			out["tool_choice"] = Object{"type": typ}
		} else {
			out["tool_choice"] = v
		}
	}
	if to == "anthropic" {
		if flag, exists := m["parallel_tool_calls"]; exists && flag != nil {
			parallel, ok := flag.(bool)
			if !ok {
				return nil, ErrUnsupported
			}
			if len(arr(out["tools"])) == 0 {
				return out, nil
			}
			choice := obj(out["tool_choice"])
			if choice == nil {
				choice = Object{"type": "auto"}
				out["tool_choice"] = choice
			}
			if choice["type"] != "none" {
				choice["disable_parallel_tool_use"] = !parallel
			}
		}
	}
	return out, nil
}
func Endpoint(protocol string) string {
	switch protocol {
	case "anthropic":
		return "/messages"
	case "chat-completions":
		return "/chat/completions"
	default:
		return "/responses"
	}
}
