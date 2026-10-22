package protocol

import "time"

// Observation retains timing and structural facts only, never generated content.
type Observation struct {
	onFirstOutput func()

	ServiceTier    string
	ServiceTierRaw string
	FirstOutput    time.Time
	FirstText      time.Time
	Terminal       time.Time
	Status         string
	NonText        bool
	Reasoning      bool
}

// output records usable generated content, not stream envelopes or reasoning.
func (o *Observation) generated() {
	if o != nil && o.onFirstOutput != nil {
		notify := o.onFirstOutput
		o.onFirstOutput = nil
		notify()
	}
}

func (o *Observation) output() {
	o.generated()
	if o != nil && o.FirstOutput.IsZero() {
		o.FirstOutput = time.Now()
	}
}

func (o *Observation) text() {
	o.generated()
	if o != nil && o.FirstText.IsZero() {
		o.FirstText = time.Now()
		if o.FirstOutput.IsZero() {
			o.FirstOutput = o.FirstText
		}
	}
}
func (o *Observation) event(m Object, p string) {
	if o == nil {
		return
	}
	switch p {
	case "responses":
		switch str(m["type"]) {
		case "response.output_text.delta":
			if str(m["delta"]) != "" {
				o.text()
			}
		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			o.NonText = true
			if str(m["delta"]) != "" {
				o.output()
			}
		case "response.audio.delta":
			o.NonText = true
			if str(m["delta"]) != "" {
				o.generated()
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			o.Reasoning = true
			if str(m["delta"]) != "" {
				o.generated()
			}
		case "response.output_item.added":
			x := obj(m["item"])
			switch x["type"] {
			case "reasoning":
				o.Reasoning = true
			case "message":
			default:
				o.NonText = true
			}
		case "response.completed", "response.incomplete", "response.failed":
			o.Terminal = time.Now()
			o.Status = str(obj(m["response"])["status"])
			if o.Status == "" {
				switch m["type"] {
				case "response.completed":
					o.Status = "completed"
				case "response.incomplete":
					o.Status = "incomplete"
				default:
					o.Status = "failed"
				}
			}
		}
	case "chat-completions":
		for _, v := range arr(m["choices"]) {
			c := obj(v)
			d := obj(c["delta"])
			if str(d["content"]) != "" {
				o.text()
			}
			if len(arr(d["tool_calls"])) > 0 {
				o.NonText = true
				for _, call := range arr(d["tool_calls"]) {
					if str(obj(obj(call)["function"])["arguments"]) != "" {
						o.output()
					}
				}
			}
			if str(d["reasoning_content"]) != "" {
				o.Reasoning = true
				o.generated()
			}
			if finish := str(c["finish_reason"]); finish != "" {
				o.Terminal = time.Now()
				o.Status = "completed"
				if finish == "length" || finish == "content_filter" {
					o.Status = "incomplete"
				}
			}
		}
	case "anthropic":
		switch str(m["type"]) {
		case "content_block_start":
			b := obj(m["content_block"])
			switch b["type"] {
			case "text":
				if str(b["text"]) != "" {
					o.text()
				}
			case "thinking", "redacted_thinking":
				o.Reasoning = true
				if str(b["thinking"]) != "" || str(b["data"]) != "" {
					o.generated()
				}
			default:
				o.NonText = true
			}
		case "content_block_delta":
			d := obj(m["delta"])
			switch d["type"] {
			case "text_delta":
				if str(d["text"]) != "" {
					o.text()
				}
			case "thinking_delta":
				o.Reasoning = true
				if str(d["thinking"]) != "" {
					o.generated()
				}
			case "input_json_delta":
				o.NonText = true
				if str(d["partial_json"]) != "" {
					o.output()
				}
			}
		case "message_delta":
			o.Status = "completed"
			if obj(m["delta"])["stop_reason"] == "max_tokens" {
				o.Status = "incomplete"
			}
		case "message_stop":
			o.Terminal = time.Now()
		}
	}
}

// Report semantic progress before downstream writes can block, while keeping
// existing delivery timing/observation semantics unchanged. No payload escapes.
func (o *Observation) firstGenerated(m Object, p string) {
	if o == nil || o.onFirstOutput == nil {
		return
	}
	probe := Observation{onFirstOutput: o.onFirstOutput}
	probe.event(m, p)
	o.onFirstOutput = probe.onFirstOutput
}
