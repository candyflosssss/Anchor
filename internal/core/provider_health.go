package core

import (
	"fmt"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type keyHealth struct {
	Failures int
	Invalid  bool
	Until    time.Time
	Probing  bool
	Test     *catalog.Probe
}

// Remove obsolete revisions while preserving newer snapshots used by in-flight requests.
func (s *Server) pruneHealthLocked(p state.Provider) {
	prefix := p.ID + "/"
	for id := range s.health {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		rest := strings.TrimPrefix(id, prefix)
		rev, _, _ := strings.Cut(rest, "/")
		n, err := strconv.ParseInt(rev, 10, 64)
		if err == nil && n < p.Revision {
			delete(s.health, id)
		}
	}
}
func healthID(p state.Provider, k string) string { return fmt.Sprintf("%s/%d/%s", p.ID, p.Revision, k) }
func (s *Server) chooseKey(p state.Provider, tried map[string]bool) (state.ProviderKey, bool) {
	return s.chooseRouteKey(p, tried, nil, "")
}
func (s *Server) chooseRouteKey(p state.Provider, tried map[string]bool, allowed []string, preferred string) (state.ProviderKey, bool) {
	keys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		return state.ProviderKey{}, false
	}
	if len(keys) == 0 {
		keys = []state.ProviderKey{{ID: "anonymous", Enabled: true, Protocol: p.Protocol}}
	}
	strategy := s.Store.KeyStrategy(p.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneHealthLocked(p)
	start := 0
	if strategy != "priority" {
		start = s.cursor[p.ID] % len(keys)
	}
	if preferred != "" {
		for i, k := range keys {
			if k.ID == preferred && k.Enabled && !tried[k.ID] {
				start = i
				break
			}
		}
	}
	for i := 0; i < len(keys); i++ {
		k := keys[(start+i)%len(keys)]
		if !k.Enabled || tried[k.ID] || !candidateAllowsKey(allowed, k.ID) {
			continue
		}
		h := s.health[healthID(p, k.ID)]
		if h != nil {
			if h.Invalid || h.Until.After(time.Now()) || h.Probing {
				continue
			}
			if !h.Until.IsZero() {
				h.Probing = true
			}
		}
		s.cursor[p.ID] = (start + i + 1) % len(keys)
		return k, true
	}
	return state.ProviderKey{}, false
}
func retryDelay(v string) time.Duration {
	d := 30 * time.Second
	if n, e := strconv.Atoi(v); e == nil && n >= 0 {
		d = time.Duration(n) * time.Second
	} else if t, e := http.ParseTime(v); e == nil {
		d = time.Until(t)
	}
	if d < time.Second {
		d = time.Second
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
func (s *Server) finishKey(p state.Provider, k string, status int, after string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneHealthLocked(p)
	id := healthID(p, k)
	h := s.health[id]
	if h == nil {
		h = &keyHealth{}
		s.health[id] = h
	}
	h.Probing = false
	if success {
		h.Failures = 0
		h.Invalid = false
		h.Until = time.Time{}
		return
	}
	h.Failures++
	if status == 401 || status == 403 {
		h.Invalid = true
	}
	if status == 429 {
		h.Until = time.Now().Add(retryDelay(after))
	} else if h.Failures >= 3 {
		h.Until = time.Now().Add(30 * time.Second)
	}
}
func (s *Server) keyStatus(p state.Provider, k state.ProviderKey) catalog.KeyStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := catalog.KeyStatus{Rate: k.Rate, EffectiveMultiplier: p.EffectiveKeyMultiplier(k), ID: k.ID, Enabled: k.Enabled, State: "ready", Protocol: k.Protocol, ModelProtocols: k.ModelProtocols}
	if !k.Enabled {
		out.State = "disabled"
	}
	if h := s.health[healthID(p, k.ID)]; h != nil {
		out.Failures = h.Failures
		out.Test = h.Test
		if h.Invalid {
			out.State = "invalid"
		} else if h.Probing {
			out.State = "half-open"
		} else if !h.Until.IsZero() {
			out.RetryAt = h.Until.Unix()
			if h.Until.After(time.Now()) {
				out.State = "cooling"
			} else {
				out.State = "half-open"
			}
		}
	}
	if !k.Enabled {
		out.State = "disabled"
	}
	return out
}

// pickURL rotates across a provider's request base URLs for load balancing.
// Discovery and connectivity tests keep using the first URL for determinism.
func (s *Server) pickURL(p state.Provider) string {
	urls := p.URLs()
	if len(urls) == 0 {
		return p.BaseURL
	}
	if len(urls) == 1 {
		return urls[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := "url\x00" + p.ID
	i := s.cursor[key] % len(urls)
	s.cursor[key] = (i + 1) % len(urls)
	return urls[i]
}
