package ccswitch

import (
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

func obj(m map[string]any, k string) map[string]any { v, _ := m[k].(map[string]any); return v }
func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func protocol(v string) string {
	switch v {
	case "responses", "openai_responses", "openai-responses", "codex_responses":
		return "responses"
	case "chat", "chat-completions", "chat_completions", "openai_chat", "openai-completions":
		return "chat-completions"
	case "anthropic", "anthropic_messages", "anthropic-messages":
		return "anthropic"
	}
	return ""
}

type endpoint struct {
	URL, Secret, Protocol string
	Models                []string
}

var environmentName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Explicit wire formats win. Platform defaults are used only where the source
// client defines them, never from a provider or model's marketing name.
func parseRecord(r record) ([]endpoint, string) {
	c, m := r.Config, r.Meta
	if c == nil {
		return nil, "invalid_config"
	}
	if str(m, "providerType") != "" || c["oauth"] != nil || obj(c, "auth")["tokens"] != nil {
		return nil, "managed_auth"
	}
	if m["isFullUrl"] == true || len(obj(obj(m, "localProxyRequestOverrides"), "headers")) > 0 || obj(m, "localProxyRequestOverrides")["body"] != nil || len(obj(c, "headers")) > 0 || len(obj(obj(c, "options"), "headers")) > 0 {
		return nil, "request_overrides"
	}
	var e endpoint
	add := func(v string) {
		if v != "" {
			e.Models = append(e.Models, v)
		}
	}
	switch r.App {
	case "claude", "claude-desktop":
		env := obj(c, "env")
		e.URL = str(env, "ANTHROPIC_BASE_URL")
		if e.URL == "" {
			e.URL = str(c, "base_url", "baseURL", "apiEndpoint")
		}
		if e.URL == "" {
			e.URL = "https://api.anthropic.com"
		}
		e.Secret = str(env, "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY")
		if e.Secret == "" {
			e.Secret = str(c, "apiKey", "api_key")
		}
		format := str(m, "apiFormat")
		if format == "" {
			format = str(c, "api_format")
		}
		if format == "" {
			format = "anthropic"
			if c["openrouter_compat_mode"] == true || c["openrouter_compat_mode"] == "true" || c["openrouter_compat_mode"] == float64(1) {
				format = "openai_chat"
			}
		}
		e.Protocol = protocol(format)
		for _, k := range []string{"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_FABLE_MODEL", "ANTHROPIC_SMALL_FAST_MODEL"} {
			add(str(env, k))
		}
	case "codex":
		var doc map[string]any
		if _, err := toml.Decode(str(c, "config"), &doc); err != nil {
			return nil, "invalid_config"
		}
		p := doc
		if name := str(doc, "model_provider"); name != "" {
			p = obj(obj(doc, "model_providers"), name)
			if p == nil {
				return nil, "invalid_config"
			}
		}
		if len(obj(p, "http_headers")) > 0 || len(obj(p, "env_http_headers")) > 0 {
			return nil, "request_overrides"
		}
		e.URL = str(p, "base_url")
		if e.URL == "" && str(doc, "model_provider") == "" {
			e.URL = "https://api.openai.com/v1"
		}
		e.Secret = str(obj(c, "auth"), "OPENAI_API_KEY")
		if e.Secret == "" {
			e.Secret = str(p, "experimental_bearer_token")
		}
		format := str(m, "apiFormat")
		if format == "" {
			format = str(c, "apiFormat")
		}
		if format == "" {
			format = str(p, "wire_api")
		}
		if format == "" {
			format = "responses"
		}
		e.Protocol = protocol(format)
		add(str(doc, "model"))
	case "gemini", "agy":
		return nil, "unsupported_protocol"
	case "grokbuild":
		var doc map[string]any
		if _, err := toml.Decode(str(c, "config"), &doc); err != nil {
			return nil, "invalid_config"
		}
		p := obj(obj(doc, "model"), str(obj(doc, "models"), "default"))
		e.URL = str(p, "base_url")
		e.Secret = str(p, "api_key")
		e.Protocol = protocol(str(p, "api_backend"))
		add(str(p, "model"))
	case "opencode", "mcode":
		o := obj(c, "options")
		e.URL = str(o, "baseURL")
		e.Secret = str(o, "apiKey")
		if r.App == "mcode" {
			e.Protocol = protocol(str(c, "api"))
		} else {
			switch str(c, "npm") {
			case "@ai-sdk/openai":
				e.Protocol = "responses"
			case "@ai-sdk/openai-compatible":
				e.Protocol = "chat-completions"
			case "@ai-sdk/anthropic":
				e.Protocol = "anthropic"
			}
		}
		models := obj(c, "models")
		ids := make([]string, 0, len(models))
		for id := range models {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			v, _ := models[id].(map[string]any)
			if str(v, "api") != "" && protocol(str(v, "api")) != e.Protocol {
				return nil, "mixed_model_protocol"
			}
			add(id)
		}
	case "hermes":
		e.URL = str(c, "base_url", "baseUrl")
		e.Secret = str(c, "api_key", "apiKey")
		e.Protocol = protocol(str(c, "api_mode", "apiMode"))
		add(str(c, "model"))
	case "openclaw", "pi":
		e.URL = str(c, "baseUrl")
		e.Secret = str(c, "apiKey")
		// Pi's resolver treats a plain uppercase variable name as an env
		// reference. Never save that name as if it were an API credential.
		if r.App == "pi" && environmentName.MatchString(e.Secret) {
			return nil, "missing_or_referenced_key"
		}
		e.Protocol = protocol(str(c, "api"))
		models, _ := c["models"].([]any)
		result := []endpoint{}
		for _, v := range models {
			model, _ := v.(map[string]any)
			one := e
			if u := str(model, "baseUrl"); u != "" {
				one.URL = u
			}
			if p := str(model, "api"); p != "" {
				one.Protocol = protocol(p)
			}
			if len(obj(model, "headers")) > 0 {
				return nil, "request_overrides"
			}
			if id := str(model, "id"); id != "" {
				one.Models = []string{id}
				result = append(result, one)
			}
		}
		if len(result) > 0 {
			return result, ""
		}
	default:
		return nil, "unsupported_platform"
	}
	return []endpoint{e}, ""
}

// Universal records are actual provider definitions, including apps that have
// not yet been materialized into the platform providers table.
func expandUniversal(r record) []record {
	out := []record{}
	for _, app := range []string{"claude", "codex", "gemini"} {
		v := r.Config
		models := obj(obj(v, "models"), app)
		c := map[string]any{}
		switch app {
		case "claude":
			env := map[string]any{"ANTHROPIC_BASE_URL": str(v, "baseUrl"), "ANTHROPIC_AUTH_TOKEN": str(v, "apiKey"), "ANTHROPIC_MODEL": str(models, "model")}
			for _, pair := range [][2]string{{"haikuModel", "ANTHROPIC_DEFAULT_HAIKU_MODEL"}, {"sonnetModel", "ANTHROPIC_DEFAULT_SONNET_MODEL"}, {"opusModel", "ANTHROPIC_DEFAULT_OPUS_MODEL"}} {
				env[pair[1]] = str(models, pair[0])
			}
			c["env"] = env
		case "codex":
			// Build native TOML through the encoder, avoiding interpolation of keys.
			base := str(v, "baseUrl")
			if !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://"), "/") {
				base += "/v1"
			}
			var b strings.Builder
			_ = toml.NewEncoder(&b).Encode(map[string]any{"model": str(models, "model"), "base_url": base, "wire_api": "responses"})
			c["config"] = b.String()
			c["auth"] = map[string]any{"OPENAI_API_KEY": str(v, "apiKey")}
		}
		out = append(out, record{ID: r.ID, App: app, Name: r.Name, Website: r.Website, Config: c, Meta: r.Meta})
	}
	return out
}
