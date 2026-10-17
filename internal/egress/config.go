// Package egress owns outbound policy and optional, isolated mihomo processes.
package egress

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrUnavailable = errors.New("configured proxy unavailable; direct fallback is disabled")

const Version = "v1.19.32"

func Normalize(kind, raw, nodeName string) (string, error) {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return "", ErrUnavailable
	}
	if kind != "mihomo" {
		u, e := url.Parse(strings.TrimSpace(raw))
		if e != nil || u.Hostname() == "" || u.Port() == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			return "", ErrUnavailable
		}
		port, e := strconv.Atoi(u.Port())
		if e != nil || port < 1 || port > 65535 {
			return "", ErrUnavailable
		}
		if kind == "http" {
			if u.Scheme != "http" && u.Scheme != "https" {
				return "", ErrUnavailable
			}
		} else if kind == "socks5" {
			if u.Scheme != "socks5" && u.Scheme != "socks5h" {
				return "", ErrUnavailable
			}
		} else {
			return "", ErrUnavailable
		}
		return u.String(), nil
	}
	var doc yaml.Node
	if e := yaml.Unmarshal([]byte(raw), &doc); e != nil || len(doc.Content) != 1 {
		return "", ErrUnavailable
	}
	count := 0
	var check func(*yaml.Node, int) bool
	check = func(n *yaml.Node, depth int) bool {
		count++
		if depth > 12 || count > 2048 || n.Kind == yaml.AliasNode || n.Anchor != "" {
			return false
		}
		for _, c := range n.Content {
			if !check(c, depth+1) {
				return false
			}
		}
		return true
	}
	if !check(&doc, 0) {
		return "", ErrUnavailable
	}
	var data map[string]any
	if e := doc.Decode(&data); e != nil {
		return "", ErrUnavailable
	}
	if nodes, ok := data["proxies"]; ok {
		list, ok := nodes.([]any)
		if !ok || len(list) == 0 {
			return "", ErrUnavailable
		}
		var selected map[string]any
		for _, v := range list {
			m, ok := v.(map[string]any)
			if !ok {
				return "", ErrUnavailable
			}
			if nodeName == "" && len(list) == 1 || m["name"] == nodeName {
				if selected != nil {
					return "", ErrUnavailable
				}
				selected = m
			}
		}
		if selected == nil {
			return "", ErrUnavailable
		}
		data = selected
	}
	allowed := strings.Fields("name type server port username password cipher uuid alterId udp tls skip-cert-verify servername sni network flow client-fingerprint fingerprint alpn ws-opts grpc-opts http-opts h2-opts reality-opts packet-encoding udp-over-tcp udp-over-tcp-version fast-open tfo congestion-controller reduce-rtt request-timeout udp-relay-mode disable-sni obfs obfs-password up down ports hop-interval ip-version")
	keys := map[string]bool{}
	for _, k := range allowed {
		keys[k] = true
	}
	for k := range data {
		if !keys[k] {
			return "", ErrUnavailable
		}
	}
	typ, _ := data["type"].(string)
	switch typ {
	case "http", "socks5", "ss", "vmess", "vless", "trojan", "hysteria2", "tuic":
	default:
		return "", ErrUnavailable
	}
	server, ok := data["server"].(string)
	if !ok || server == "" || strings.ContainsAny(server, "/\r\n\x00") {
		return "", ErrUnavailable
	}
	port, ok := data["port"].(int)
	if !ok || port < 1 || port > 65535 {
		return "", ErrUnavailable
	}
	// Nested transport options may contain URL paths, but never certificate files,
	// provider references, plugins, or executable/configuration paths.
	var safe func(any) bool
	safe = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				lower := strings.ToLower(k)
				if strings.Contains(lower, "file") || strings.Contains(lower, "certificate") || strings.Contains(lower, "private-key") || strings.Contains(lower, "provider") || strings.Contains(lower, "plugin") || strings.Contains(lower, "dialer") {
					return false
				}
				if !safe(v) {
					return false
				}
			}
		case []any:
			for _, v := range x {
				if !safe(v) {
					return false
				}
			}
		}
		return true
	}
	if !safe(data) {
		return "", ErrUnavailable
	}
	data["name"] = "egress"
	b, e := json.Marshal(data)
	return string(b), e
}
