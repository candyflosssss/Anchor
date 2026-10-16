package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Stream incrementally forwards/converts bounded SSE events. EOF without a terminal
// event is an error and cannot produce a successful completion or replay.
func Stream(w http.ResponseWriter, body io.Reader, from, to, model string) (Usage, error) {
	return StreamObserved(w, body, from, to, model, nil)
}
func StreamObserved(w http.ResponseWriter, body io.Reader, from, to, model string, observation *Observation) (usage Usage, streamErr error) {
	r := Result{ID: fmt.Sprintf("resp_anchor_%d", time.Now().UnixNano()), Model: model}
	emit := &emitter{w: w, p: to, r: &r}
	same := from == to
	terminal := false
	defer func() {
		if streamErr != nil && !terminal {
			usage = Usage{}
		}
		if streamErr != nil && !same {
			// No terminal success and no replay after downstream output has begun.
			if to == "responses" {
				_ = emit.send("error", Object{"code": "upstream_stream_error", "message": "Upstream stream incomplete or unsupported", "param": nil})
			} else if to == "anthropic" {
				_ = emit.send("error", Object{"error": Object{"type": "api_error", "message": "Upstream stream incomplete or unsupported"}})
			} else {
				_ = emit.send("", Object{"error": Object{"type": "upstream_stream_error", "message": "Upstream stream incomplete or unsupported"}})
			}
		}
	}()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	data := ""
	event := ""
	size := 0
	index := map[int]int{}
	chatText := -1
	add := func(up int, item Item) (int, error) {
		if i, ok := index[up]; ok {
			return i, nil
		}
		if len(r.Items) >= 256 {
			return 0, ErrUnsupported
		}
		i := len(r.Items)
		index[up] = i
		r.Items = append(r.Items, item)
		if !same {
			if e := emit.startItem(i); e != nil {
				return i, e
			}
		}
		return i, nil
	}
	process := func() error {
		if data == "" {
			return nil
		}
		payload := data
		data = ""
		if same {
			if e := emit.raw(event, payload); e != nil {
				return e
			}
		}
		if payload == "[DONE]" {
			if from == "chat-completions" {
				terminal = true
			}
			return nil
		}
		m, e := Decode([]byte(payload))
		if e != nil {
			return e
		}
		observation.observeServiceTier(m, from, []byte(payload))
		if m["error"] != nil || m["type"] == "error" || m["type"] == "response.failed" {
			return errors.New("upstream stream failed")
		}

		if same {
			observation.event(m, from)
			switch from {
			case "responses":
				if m["type"] == "response.completed" || m["type"] == "response.incomplete" {
					r.Usage = ReadUsage(obj(m["response"]), from)
					terminal = true
				}
			case "chat-completions":
				if u := ReadUsage(m, from); u.Known {
					r.Usage = u
				}
			case "anthropic":
				if m["type"] == "message_start" {
					r.Usage = ReadUsage(obj(m["message"]), from)
				}
				if m["type"] == "message_delta" {
					if u := obj(m["usage"]); u != nil {
						if full := ReadUsage(Object{"usage": u}, "anthropic"); full.Known {
							r.Usage = full
						} else {
							r.Usage.Output = num(u["output_tokens"])
						}
					}
				}
				if m["type"] == "message_stop" {
					terminal = true
				}
			}
			return nil
		}
		if !same && !emit.started {
			if id := str(m["id"]); id != "" {
				r.ID = id
			}
			if from == "responses" {
				if v := str(obj(m["response"])["id"]); v != "" {
					r.ID = v
				}
			}
			if from == "anthropic" {
				if v := str(obj(m["message"])["id"]); v != "" {
					r.ID = v
				}
			}
			if e = emit.start(); e != nil {
				return e
			}
		}
		delta := func(i int, s string, tool bool) error {
			size += len(s)
			if size > 16<<20 {
				return errors.New("stream exceeds conversion limit")
			}
			if tool {
				r.Items[i].Arguments += s
			} else {
				r.Items[i].Text += s
			}
			if !same {
				return emit.delta(i, s, tool)
			}
			return nil
		}
		switch from {
		case "chat-completions":
			if u := ReadUsage(m, from); u.Known {
				r.Usage = u
			}
			for _, v := range arr(m["choices"]) {
				c := obj(v)
				if num(c["index"]) != 0 {
					return ErrUnsupported
				}
				if finish := str(c["finish_reason"]); finish != "" {
					r.Finish = finish
				}
				d := obj(c["delta"])
				if allowed(d, "role content tool_calls refusal") != nil || c["logprobs"] != nil {
					return ErrUnsupported
				}
				if d["refusal"] != nil && d["refusal"] != "" {
					return ErrUnsupported
				}
				if t := str(d["content"]); t != "" {
					if chatText < 0 {
						chatText, e = add(-1, Item{})
						if e != nil {
							return e
						}
					}
					if e = delta(chatText, t, false); e != nil {
						return e
					}
				}
				for _, v := range arr(d["tool_calls"]) {
					c := obj(v)
					f := obj(c["function"])
					if _, exists := index[int(num(c["index"]))]; !exists && (str(c["id"]) == "" || str(f["name"]) == "") {
						return ErrUnsupported
					}
					if j, exists := index[int(num(c["index"]))]; exists && str(f["name"]) != "" && str(f["name"]) != r.Items[j].Name {
						return ErrUnsupported
					}
					i, e := add(int(num(c["index"])), Item{Tool: true, CallID: str(c["id"]), Name: str(f["name"])})
					if e != nil {
						return e
					}
					if r.Items[i].CallID == "" {
						r.Items[i].CallID = str(c["id"])
					}
					if s := str(f["arguments"]); s != "" {
						if e = delta(i, s, true); e != nil {
							return e
						}
					}
				}
			}
		case "anthropic":
			switch str(m["type"]) {
			case "message_start":
				r.Usage = ReadUsage(obj(m["message"]), from)
			case "content_block_start":
				b := obj(m["content_block"])
				x := Item{}
				switch b["type"] {
				case "text":
				case "tool_use":
					x.Tool = true
					x.CallID = str(b["id"])
					x.Name = str(b["name"])
				default:
					if !same {
						return ErrUnsupported
					}
				}
				i, e := add(int(num(m["index"])), x)
				if e != nil {
					return e
				}
				if t := str(b["text"]); t != "" {
					if e = delta(i, t, false); e != nil {
						return e
					}
				}
			case "content_block_delta":
				i, ok := index[int(num(m["index"]))]
				if !ok {
					return ErrUnsupported
				}
				d := obj(m["delta"])
				switch d["type"] {
				case "text_delta":
					e = delta(i, str(d["text"]), false)
				case "input_json_delta":
					e = delta(i, str(d["partial_json"]), true)
				default:
					if !same {
						return ErrUnsupported
					}
				}
				if e != nil {
					return e
				}
			case "content_block_stop":
				if i, ok := index[int(num(m["index"]))]; ok && r.Items[i].Tool && r.Items[i].Arguments == "" {
					if e = delta(i, "{}", true); e != nil {
						return e
					}
				}
			case "message_delta":
				r.Finish = str(obj(m["delta"])["stop_reason"])
				u := obj(m["usage"])
				if _, ok := u["output_tokens"]; ok {
					if full := ReadUsage(Object{"usage": u}, "anthropic"); full.Known {
						r.Usage = full
					} else {
						r.Usage.Output = num(u["output_tokens"])
					}
				}
			case "message_stop":
				terminal = true
			case "ping":
			default:
				return ErrUnsupported
			}
		case "responses":
			switch str(m["type"]) {
			case "response.output_item.added":
				x := obj(m["item"])
				item := Item{ID: str(x["id"])}
				switch x["type"] {
				case "message":
				case "function_call":
					item.Tool = true
					item.CallID = str(x["call_id"])
					item.Name = str(x["name"])
				default:
					if !same {
						return ErrUnsupported
					}
				}
				_, e = add(int(num(m["output_index"])), item)
				if e != nil {
					return e
				}
			case "response.output_text.delta", "response.function_call_arguments.delta":
				i, ok := index[int(num(m["output_index"]))]
				if !ok {
					return ErrUnsupported
				}
				if e = delta(i, str(m["delta"]), m["type"] == "response.function_call_arguments.delta"); e != nil {
					return e
				}
			case "response.completed", "response.incomplete":
				r.Usage = ReadUsage(obj(m["response"]), from)
				if m["type"] == "response.incomplete" {
					r.Finish = "length"
				}
				terminal = true
			case "response.content_part.added", "response.content_part.done":
				if _, err := text([]any{m["part"]}); err != nil {
					return err
				}
			case "response.created", "response.in_progress", "response.output_item.done", "response.output_text.done", "response.function_call_arguments.done":
				// The preceding deltas carry supported content; these close its envelope.
			default:
				return ErrUnsupported
			}
		}
		observation.event(m, from)
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if e := process(); e != nil {
				return r.Usage, e
			}
			event = ""
			if terminal {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			v := strings.TrimPrefix(line, "data:")
			v = strings.TrimPrefix(v, " ")
			if data != "" {
				data += "\n"
			}
			data += v
			if len(data) > 2<<20 {
				return r.Usage, errors.New("SSE event too large")
			}
		} else if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if e := scanner.Err(); e != nil {
		return r.Usage, e
	}
	if data != "" {
		if e := process(); e != nil {
			return r.Usage, e
		}
	}
	if !terminal {
		return r.Usage, io.ErrUnexpectedEOF
	}
	if !same {
		return r.Usage, emit.done()
	}
	return r.Usage, nil
}

type emitter struct {
	w       http.ResponseWriter
	p       string
	r       *Result
	started bool
	seq     int
}

func (e *emitter) raw(event, data string) error {
	c := http.NewResponseController(e.w)
	_ = c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if event != "" {
		if _, err := fmt.Fprintf(e.w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(e.w, "data: %s\n\n", data); err != nil {
		return err
	}
	err := c.Flush()
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}
func (e *emitter) send(typ string, m Object) error {
	event := ""
	if e.p != "chat-completions" {
		event = typ
		m["type"] = typ
	}
	if e.p == "responses" {
		m["sequence_number"] = e.seq
		e.seq++
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return e.raw(event, string(b))
}
func (e *emitter) chat(delta Object, finish any, usage any) error {
	return e.send("", Object{"id": e.r.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": e.r.Model, "choices": []any{Object{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": usage})
}
func (e *emitter) start() error {
	e.started = true
	if e.p == "responses" {
		m := e.r.JSON(e.p)
		m["output"] = []any{}
		m["status"] = "in_progress"
		return e.send("response.created", Object{"response": m})
	}
	if e.p == "anthropic" {
		m := e.r.JSON(e.p)
		m["content"] = []any{}
		// Provisional start metadata; only the terminal usage is recorded as known.
		if !e.r.Usage.Known {
			m["usage"] = Object{"input_tokens": 0, "output_tokens": 0}
		}
		m["stop_reason"] = nil
		return e.send("message_start", Object{"message": m})
	}
	return e.chat(Object{"role": "assistant", "content": ""}, nil, nil)
}
func (e *emitter) startItem(i int) error {
	x := e.r.Items[i]
	if e.p == "responses" {
		item := e.r.item(i)
		item["status"] = "in_progress"
		if !x.Tool {
			item["content"] = []any{}
		}
		if err := e.send("response.output_item.added", Object{"output_index": i, "item": item}); err != nil {
			return err
		}
		if !x.Tool {
			return e.send("response.content_part.added", Object{"output_index": i, "item_id": item["id"], "content_index": 0, "part": Object{"type": "output_text", "text": "", "annotations": []any{}}})
		}
		return nil
	}
	if e.p == "anthropic" {
		if i > 0 {
			return nil
		}
		b := Object{"type": "text", "text": ""}
		if x.Tool {
			b = Object{"type": "tool_use", "id": x.CallID, "name": x.Name, "input": Object{}}
		}
		return e.send("content_block_start", Object{"index": i, "content_block": b})
	}
	if x.Tool {
		return e.chat(Object{"tool_calls": []any{Object{"index": e.toolIndex(i), "id": x.CallID, "type": "function", "function": Object{"name": x.Name, "arguments": ""}}}}, nil, nil)
	}
	return nil
}
func (e *emitter) toolIndex(i int) int {
	n := 0
	for j := 0; j < i; j++ {
		if e.r.Items[j].Tool {
			n++
		}
	}
	return n
}
func (e *emitter) delta(i int, s string, tool bool) error {
	if e.p == "responses" {
		typ := "response.output_text.delta"
		if tool {
			typ = "response.function_call_arguments.delta"
		}
		m := Object{"output_index": i, "item_id": e.r.item(i)["id"], "delta": s}
		if !tool {
			m["content_index"] = 0
		}
		return e.send(typ, m)
	}
	if e.p == "anthropic" {
		if i > 0 {
			return nil
		}
		d := Object{"type": "text_delta", "text": s}
		if tool {
			d = Object{"type": "input_json_delta", "partial_json": s}
		}
		return e.send("content_block_delta", Object{"index": i, "delta": d})
	}
	if tool {
		return e.chat(Object{"tool_calls": []any{Object{"index": e.toolIndex(i), "function": Object{"arguments": s}}}}, nil, nil)
	}
	return e.chat(Object{"content": s}, nil, nil)
}
func (e *emitter) done() error {
	if e.p == "anthropic" && !e.r.Usage.Known {
		return errors.New("upstream omitted usage required by Messages")
	}
	if !e.started {
		if err := e.start(); err != nil {
			return err
		}
	}
	for i, x := range e.r.Items {
		if x.Tool {
			var args Object
			if x.CallID == "" || x.Name == "" || json.Unmarshal([]byte(x.Arguments), &args) != nil || args == nil {
				return errors.New("incomplete or unsupported tool arguments")
			}
		}
		if e.p == "responses" {
			item := e.r.item(i)
			if x.Tool {
				if err := e.send("response.function_call_arguments.done", Object{"output_index": i, "item_id": item["id"], "arguments": x.Arguments}); err != nil {
					return err
				}
			} else {
				if err := e.send("response.output_text.done", Object{"output_index": i, "item_id": item["id"], "content_index": 0, "text": x.Text}); err != nil {
					return err
				}
				if err := e.send("response.content_part.done", Object{"output_index": i, "item_id": item["id"], "content_index": 0, "part": arr(item["content"])[0]}); err != nil {
					return err
				}
			}
			if err := e.send("response.output_item.done", Object{"output_index": i, "item": item}); err != nil {
				return err
			}
		} else if e.p == "anthropic" {
			if i > 0 {
				b := Object{"type": "text", "text": ""}
				d := Object{"type": "text_delta", "text": x.Text}
				if x.Tool {
					b = Object{"type": "tool_use", "id": x.CallID, "name": x.Name, "input": Object{}}
					d = Object{"type": "input_json_delta", "partial_json": x.Arguments}
				}
				if err := e.send("content_block_start", Object{"index": i, "content_block": b}); err != nil {
					return err
				}
				if err := e.send("content_block_delta", Object{"index": i, "delta": d}); err != nil {
					return err
				}
			}
			if err := e.send("content_block_stop", Object{"index": i}); err != nil {
				return err
			}
		}
	}
	m := e.r.JSON(e.p)
	if e.p == "responses" {
		typ := "response.completed"
		if m["status"] == "incomplete" {
			typ = "response.incomplete"
		}
		return e.send(typ, Object{"response": m})
	}
	if e.p == "anthropic" {
		if err := e.send("message_delta", Object{"delta": Object{"stop_reason": m["stop_reason"], "stop_sequence": nil}, "usage": e.r.Usage.JSON(e.p)}); err != nil {
			return err
		}
		return e.send("message_stop", Object{})
	}
	finish := obj(arr(m["choices"])[0])["finish_reason"]
	if err := e.chat(Object{}, finish, nil); err != nil {
		return err
	}
	if e.r.Usage.Known {
		if err := e.send("", Object{"id": e.r.ID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": e.r.Model, "choices": []any{}, "usage": e.r.Usage.JSON(e.p)}); err != nil {
			return err
		}
	}
	return e.raw("", "[DONE]")
}
