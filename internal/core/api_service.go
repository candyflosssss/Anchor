package core

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type accessKey struct{}
type applicationListenerKey struct{}
type APIServiceWorkspace = state.APIServiceWorkspace

func (s *Server) apiServiceWorkspace() (APIServiceWorkspace, error) {
	out := APIServiceWorkspace{BaseURLs: []string{}}
	var err error
	out.Settings, err = s.Store.APIServiceSettings()
	if err != nil {
		return out, err
	}
	out.Keys, err = s.Store.GatewayKeys()
	if err != nil {
		return out, err
	}
	s.mu.Lock()
	active := s.apiServiceActive
	s.mu.Unlock()
	out.RestartRequired = active.Enabled != out.Settings.Enabled || (out.Settings.Enabled && (active.Port != out.Settings.Port || active.AllowLAN != out.Settings.AllowLAN))
	if active.Enabled {
		out.ActiveAddress = active.Address()
		out.BaseURLs = append(out.BaseURLs, "http://127.0.0.1:"+strconv.Itoa(active.Port)+"/v1")
		if active.AllowLAN {
			addresses, _ := net.InterfaceAddrs()
			seen := map[string]bool{}
			for _, address := range addresses {
				ip, _, err := net.ParseCIDR(address.String())
				if err == nil && ip.To4() != nil && ip.IsPrivate() && !seen[ip.String()] {
					seen[ip.String()] = true
					out.BaseURLs = append(out.BaseURLs, "http://"+net.JoinHostPort(ip.String(), strconv.Itoa(active.Port))+"/v1")
				}
			}
		}
	}
	return out, nil
}
func (s *Server) apiServiceRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /control/v1/api-service", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		v, err := s.apiServiceWorkspace()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
	m.HandleFunc("POST /control/v1/api-service/settings", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in state.SaveAPIServiceSettings
		if !decode(w, r, &in) {
			return
		}
		if _, err := s.Store.SaveAPIServiceSettings(in); err != nil {
			apiError(w, err)
			return
		}
		v, err := s.apiServiceWorkspace()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
	m.HandleFunc("POST /control/v1/api-service/keys", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in state.IssueAPIKey
		if !decode(w, r, &in) {
			return
		}
		if err := in.Validate(); err != nil {
			apiError(w, err)
			return
		}
		a, token, err := s.Store.Grant(in.Label, "gateway", time.Duration(in.TTLSeconds)*time.Second)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, state.IssuedAPIKey{Access: a, Token: token})
	})
	m.HandleFunc("DELETE /control/v1/api-service/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		if err := s.Store.RevokeGatewayKey(r.PathValue("id")); err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, struct {
			Revoked bool `json:"revoked"`
		}{true})
	})
}

// Only this additional listener allows private IPv4 LAN clients. The original
// model and management listeners retain their loopback Host policy.
func (s *Server) applicationGateway(allowLAN bool) http.Handler {
	next := s.Gateway()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), applicationListenerKey{}, allowLAN)))
	})
}
func allowedAPIHost(r *http.Request, scope string) bool {
	host, port, err := net.SplitHostPort(r.Host)
	ip := net.ParseIP(host)
	number, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || portErr != nil || number < 1 || number > 65535 {
		return false
	}
	lan, application := r.Context().Value(applicationListenerKey{}).(bool)
	if !application || !lan || scope != "gateway" {
		return ip.IsLoopback()
	}
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	remote := net.ParseIP(peer)
	return err == nil && remote != nil && (remote.IsLoopback() || (remote.To4() != nil && remote.IsPrivate())) && (ip.IsLoopback() || (ip.To4() != nil && ip.IsPrivate()))
}
