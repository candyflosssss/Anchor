package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

type Usage struct {
	Known                                bool
	Input, Output, CacheRead, CacheWrite int64
	CacheReadKnown, ReasoningKnown       bool
	Reasoning                            int64
}

func ReadUsage(m Object, p string) Usage {
	u := obj(m["usage"])
	if u == nil {
		return Usage{}
	}
	input, output := "input_tokens", "output_tokens"
	if p == "chat-completions" {
		input, output = "prompt_tokens", "completion_tokens"
	}
	validToken := func(v any) bool { n, ok := v.(float64); return ok && n >= 0 && n <= 1e12 && math.Trunc(n) == n }
	hasIn, hasOut := validToken(u[input]), validToken(u[output])
	if !hasIn || !hasOut {
		return Usage{}
	}
	r := Usage{Known: hasIn && hasOut, Input: num(u[input]), Output: num(u[output])}
	if p == "anthropic" {
		r.CacheRead = num(u["cache_read_input_tokens"])
		r.CacheWrite = num(u["cache_creation_input_tokens"])
		r.Input += r.CacheRead + r.CacheWrite
	} else if p == "chat-completions" {
		r.CacheRead = num(obj(u["prompt_tokens_details"])["cached_tokens"])
	} else {
		r.CacheRead = num(obj(u["input_tokens_details"])["cached_tokens"])
	}
	details := obj(u["output_tokens_details"])
	if p == "chat-completions" {
		details = obj(u["completion_tokens_details"])
	}
	if validToken(details["reasoning_tokens"]) {
		r.ReasoningKnown = true
		r.Reasoning = num(details["reasoning_tokens"])
	}
	if r.Reasoning > r.Output {
		return Usage{}
	}
	switch p {
	case "anthropic":
		r.CacheReadKnown = validToken(u["cache_read_input_tokens"])
	case "chat-completions":
		r.CacheReadKnown = validToken(obj(u["prompt_tokens_details"])["cached_tokens"])
	default:
		r.CacheReadKnown = validToken(obj(u["input_tokens_details"])["cached_tokens"])
	}
	if r.CacheRead < 0 || r.CacheWrite < 0 || r.CacheRead+r.CacheWrite > r.Input {
		return Usage{}
	}
	return r
}
func (u Usage) JSON(p string) any {
	if !u.Known {
		return nil
	}
	if p == "chat-completions" {
		return Object{"prompt_tokens": u.Input, "completion_tokens": u.Output, "total_tokens": u.Input + u.Output, "prompt_tokens_details": Object{"cached_tokens": u.CacheRead}}
	}
	if p == "anthropic" {
		return Object{"input_tokens": max(0, u.Input-u.CacheRead-u.CacheWrite), "output_tokens": u.Output, "cache_read_input_tokens": u.CacheRead, "cache_creation_input_tokens": u.CacheWrite}
	}
	return Object{"input_tokens": u.Input, "output_tokens": u.Output, "total_tokens": u.Input + u.Output, "input_tokens_details": Object{"cached_tokens": u.CacheRead}}
}

type Item struct {
	ID, CallID, Name, Text, Arguments string
	Tool                              bool
}
type Result struct {
	ID, Model, Finish string
	Items             []Item
	Usage             Usage
}

// ValidateProbeResponse checks a minimal inference result without requiring that
// its reasoning items can be represented in another protocol. A token-limited
// response may contain reasoning only; it still proves the upstream executed.
func ValidateProbeResponse(raw []byte, p string) (Usage, error) {
	r, err := parseResponse(raw, p, true)
	return r.Usage, err
}

func ParseResponse(raw []byte, p string) (Result, error) {
	return parseResponse(raw, p, false)
}

func parseResponse(raw []byte, p string, probe bool) (Result, error) {
	m, e := Decode(raw)
	if e != nil {
		return Result{}, e
	}
	r := Result{ID: str(m["id"]), Model: str(m["model"]), Finish: "stop", Usage: ReadUsage(m, p)}
	switch p {
	case "responses":
		if m["status"] == "failed" || m["error"] != nil {
			return r, errors.New("upstream failed")
		}
		if m["status"] == "incomplete" {
			r.Finish = "length"
		}
		for _, v := range arr(m["output"]) {
			x := obj(v)
			switch x["type"] {
			case "reasoning":
				if !probe {
					return r, ErrUnsupported
				}
			case "message":
				t, e := text(x["content"])
				if e != nil {
					return r, e
				}
				r.Items = append(r.Items, Item{ID: str(x["id"]), Text: t})
			case "function_call":
				r.Items = append(r.Items, Item{ID: str(x["id"]), CallID: str(x["call_id"]), Name: str(x["name"]), Arguments: str(x["arguments"]), Tool: true})
			default:
				return r, ErrUnsupported
			}
		}
	case "chat-completions":
		choices := arr(m["choices"])
		if len(choices) != 1 {
			return r, ErrUnsupported
		}
		c := obj(choices[0])
		r.Finish = str(c["finish_reason"])
		msg := obj(c["message"])
		if msg["refusal"] != nil && msg["refusal"] != "" {
			return r, ErrUnsupported
		}
		t, e := text(msg["content"])
		if e != nil {
			return r, e
		}
		if t != "" {
			r.Items = append(r.Items, Item{Text: t})
		}
		for _, v := range arr(msg["tool_calls"]) {
			x := obj(v)
			if x["type"] != "function" {
				return r, ErrUnsupported
			}
			f := obj(x["function"])
			r.Items = append(r.Items, Item{CallID: str(x["id"]), Name: str(f["name"]), Arguments: str(f["arguments"]), Tool: true})
		}
	case "anthropic":
		r.Finish = str(m["stop_reason"])
		for _, v := range arr(m["content"]) {
			x := obj(v)
			switch x["type"] {
			case "text":
				r.Items = append(r.Items, Item{Text: str(x["text"])})
			case "tool_use":
				args, _ := json.Marshal(x["input"])
				r.Items = append(r.Items, Item{CallID: str(x["id"]), Name: str(x["name"]), Arguments: string(args), Tool: true})
			default:
				return r, ErrUnsupported
			}
		}
	}
	for _, item := range r.Items {
		if item.Tool {
			var args Object
			if item.CallID == "" || item.Name == "" || json.Unmarshal([]byte(item.Arguments), &args) != nil || args == nil {
				return r, ErrUnsupported
			}
		}
	}
	if r.ID == "" {
		return r, errors.New("missing response id")
	}
	return r, nil
}
func (r *Result) item(i int) Object {
	x := r.Items[i]
	id := x.ID
	if id == "" {
		id = fmt.Sprintf("item_%s_%d", r.ID, i)
	}
	if x.Tool {
		return Object{"type": "function_call", "id": id, "call_id": x.CallID, "name": x.Name, "arguments": x.Arguments, "status": "completed"}
	}
	return Object{"type": "message", "id": id, "role": "assistant", "status": "completed", "content": []any{Object{"type": "output_text", "text": x.Text, "annotations": []any{}}}}
}
func (r *Result) JSON(p string) Object {
	if p == "responses" {
		items := []any{}
		for i := range r.Items {
			items = append(items, r.item(i))
		}
		status := "completed"
		var incomplete any
		if r.Finish == "length" || r.Finish == "max_tokens" {
			status = "incomplete"
			incomplete = Object{"reason": "max_output_tokens"}
		}
		return Object{"id": r.ID, "object": "response", "created_at": time.Now().Unix(), "status": status, "model": r.Model, "output": items, "usage": r.Usage.JSON(p), "error": nil, "incomplete_details": incomplete}
	}
	if p == "anthropic" {
		content := []any{}
		reason := "end_turn"
		if r.Finish == "length" || r.Finish == "max_tokens" {
			reason = "max_tokens"
		}
		for _, x := range r.Items {
			if x.Tool {
				var args Object
				_ = json.Unmarshal([]byte(x.Arguments), &args)
				content = append(content, Object{"type": "tool_use", "id": x.CallID, "name": x.Name, "input": args})
				if reason != "max_tokens" {
					reason = "tool_use"
				}
			} else {
				content = append(content, Object{"type": "text", "text": x.Text})
			}
		}
		return Object{"id": r.ID, "type": "message", "role": "assistant", "model": r.Model, "content": content, "stop_reason": reason, "stop_sequence": nil, "usage": r.Usage.JSON(p)}
	}
	msg := Object{"role": "assistant", "content": ""}
	calls := []any{}
	t := ""
	finish := "stop"
	if r.Finish == "length" || r.Finish == "max_tokens" {
		finish = "length"
	}
	for _, x := range r.Items {
		if x.Tool {
			calls = append(calls, Object{"id": x.CallID, "type": "function", "function": Object{"name": x.Name, "arguments": x.Arguments}})
		} else {
			t += x.Text
		}
	}
	msg["content"] = t
	if len(calls) > 0 {
		msg["tool_calls"] = calls
		if finish != "length" {
			finish = "tool_calls"
		}
	}
	return Object{"id": r.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": r.Model, "choices": []any{Object{"index": 0, "message": msg, "finish_reason": finish}}, "usage": r.Usage.JSON(p)}
}
