package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Version defaults to the source milestone; release builds inject their version with -ldflags -X.
var Version = "0.1.0-dev"

const MaxRequestBytes = 8 << 20

type Server struct {
	Store    *state.Store
	Log      *slog.Logger
	upstream *http.Client
	mu       sync.Mutex
	active   map[string]int
	slots    chan struct{}
}

func New(store *state.Store, log *slog.Logger) *Server {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Explicit provider configuration, not ambient HTTP_PROXY, determines the route.
	tr.Proxy = nil
	tr.DisableCompression = true
	tr.MaxConnsPerHost = 64
	tr.MaxIdleConns = 32
	tr.ResponseHeaderTimeout = 30 * time.Second
	return &Server{Store: store, Log: log, upstream: &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, active: map[string]int{}, slots: make(chan struct{}, 16)}
}
func (s *Server) Close() { s.upstream.CloseIdleConnections() }
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func apiError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, state.ErrConflict):
		fail(w, 409, "conflict", "revision changed or resource already exists")
	case errors.Is(err, state.ErrNotFound):
		fail(w, 404, "not_found", "resource not found")
	default:
		fail(w, 500, "internal_error", "operation failed; no secrets are included in diagnostics")
	}
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		fail(w, 400, "invalid_json", "invalid, unknown or oversized JSON fields")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "invalid_json", "exactly one JSON object is required")
		return false
	}
	return true
}
func (s *Server) guard(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Browsers are not a supported management transport. No wildcard CORS.
		if r.Header.Get("Origin") != "" {
			fail(w, 403, "browser_origin_rejected", "browser requests are not supported")
			return
		}
		host, _, err := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			fail(w, 400, "invalid_host", "Host must be a numeric loopback address and port")
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || !s.Store.Authenticate(strings.TrimPrefix(auth, "Bearer "), scope) {
			fail(w, 401, "unauthorized", "valid bearer credential required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /control/v1/instance", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.Store.Settings()
		if err != nil {
			apiError(w, err)
			return
		}
		username := "unknown"
		if u, err := user.Current(); err == nil {
			username = u.Username
		}
		writeJSON(w, 200, map[string]any{"instance_id": cfg.InstanceID, "version": Version, "api_version": 1, "platform": runtime.GOOS, "user": username, "control_address": cfg.ControlAddress, "gateway_address": cfg.GatewayAddress, "capabilities": []string{"responses_passthrough", "local_context", "provider_model_revision"}})
	})
	mux.HandleFunc("GET /control/v1/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"core": "ready", "gateway": "ready", "upstream_health": "not_probed", "agent_management": "not_implemented"})
	})
	mux.HandleFunc("GET /control/v1/providers", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Store.Providers()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("PUT /control/v1/providers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in struct {
			Provider         state.Provider `json:"provider"`
			ExpectedRevision int64          `json:"expected_revision"`
			Secret           *string        `json:"secret,omitempty"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Provider.ID != r.PathValue("id") {
			fail(w, 400, "invalid_id", "path and object ID must agree")
			return
		}
		if err := in.Provider.Validate(); err != nil {
			fail(w, 400, "invalid_provider", err.Error())
			return
		}
		if in.Secret != nil {
			p := in.Provider
			p.Secret = *in.Secret
			if err := p.Validate(); err != nil {
				fail(w, 400, "invalid_provider", err.Error())
				return
			}
		}
		p, err := s.Store.PutProvider(in.Provider, in.ExpectedRevision, in.Secret)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("GET /control/v1/models", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.Store.Models()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, m)
	})
	mux.HandleFunc("PUT /control/v1/models/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in struct {
			Model            state.Model `json:"model"`
			ExpectedRevision int64       `json:"expected_revision"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Model.ID != r.PathValue("id") {
			fail(w, 400, "invalid_id", "path and object ID must agree")
			return
		}
		if err := in.Model.Validate(); err != nil {
			fail(w, 400, "invalid_model", err.Error())
			return
		}
		for _, c := range in.Model.Candidates {
			if _, err := s.Store.Provider(c.ProviderID); err != nil {
				if errors.Is(err, state.ErrNotFound) {
					fail(w, 400, "invalid_candidate", "candidate provider does not exist")
				} else {
					apiError(w, err)
				}
				return
			}
		}
		m, err := s.Store.PutModel(in.Model, in.ExpectedRevision)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, m)
	})
	return s.guard("manage", mux)
}
func (s *Server) identity(w http.ResponseWriter, r *http.Request) bool {
	cfg, err := s.Store.Settings()
	if err != nil {
		apiError(w, err)
		return false
	}
	if r.Header.Get("X-Anchor-Instance") != cfg.InstanceID {
		fail(w, 409, "instance_mismatch", "write requires the expected instance identity")
		return false
	}
	return true
}
func listenerServer(address string, h http.Handler, ctx context.Context) (*http.Server, net.Listener, error) {
	if err := state.ValidateListen(address); err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, err
	}
	return &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}, ln, nil
}

// Run binds both listeners before publishing ready; no Agent configuration is touched.
func (s *Server) Run(ctx context.Context) error {
	lock, err := platform.AcquireLock(filepath.Join(s.Store.Dir, "instance.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	defer s.Close()
	cfg, err := s.Store.Settings()
	if err != nil {
		return err
	}
	running, cancel := context.WithCancel(context.Background())
	defer cancel()
	control, cl, err := listenerServer(cfg.ControlAddress, s.Control(), running)
	if err != nil {
		return fmt.Errorf("management listen failed: %w", err)
	}
	gateway, gl, err := listenerServer(cfg.GatewayAddress, s.Gateway(), running)
	if err != nil {
		cl.Close()
		return fmt.Errorf("gateway listen failed: %w", err)
	}
	// Route HTTP server errors through the structured, non-secret logger.
	control.ErrorLog = slog.NewLogLogger(s.Log.Handler(), slog.LevelWarn)
	gateway.ErrorLog = control.ErrorLog
	errs := make(chan error, 2)
	go func() { errs <- control.Serve(cl) }()
	go func() { errs <- gateway.Serve(gl) }()
	s.Log.Info("core ready", "instance_id", cfg.InstanceID, "control_address", cl.Addr().String(), "gateway_address", gl.Addr().String())
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errs:
	}
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	// Begin draining both listeners together, then cancel any requests that outlive the deadline.
	shutdowns := make(chan error, 2)
	go func() { shutdowns <- control.Shutdown(deadline) }()
	go func() { shutdowns <- gateway.Shutdown(deadline) }()
	e1, e2 := <-shutdowns, <-shutdowns
	cancel()
	_ = control.Close()
	_ = gateway.Close()
	if e1 != nil || e2 != nil {
		s.Log.Warn("drain deadline exceeded; remaining requests canceled")
	}
	s.Log.Info("core stopped")
	if runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
		return runErr
	}
	return nil
}

// Keep the URL policy colocated with transport setup for testability.
func responseURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/responses"
	return u.String(), nil
}
