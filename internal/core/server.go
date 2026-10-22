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
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/skills"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Version defaults to the source milestone; release builds inject their version with -ldflags -X.
var Version = "0.1.0-dev"

// MaxRequestBytes bounds gateway bodies, including inline image history.
const MaxRequestBytes = 32 << 20

type Server struct {
	apiServiceActive state.APIServiceSettings
	AgentEnvironment func() (managed.Environment, error)
	Skills           *skills.Manager
	Agents           *managed.Manager
	Store            *state.Store
	Log              *slog.Logger
	upstream         *http.Client
	egress           *egress.Manager
	modelCatalog     *modeldev.Client
	keyRateRefresh   *keyRateRefresh
	platformQueries  sync.Map
	endpointMu       sync.Mutex
	endpoints        map[[32]byte]time.Time
	mu               sync.Mutex
	active           map[string]int
	health           map[string]*keyHealth
	keyPolicy        state.KeyCooldownSettings
	probeRuns        map[string]*probeRun
	routing          map[string]*modelSchedule
	affinity         map[string]routeAffinity
	latency          map[string]routeLatency
	cursor           map[string]int
	slots            chan struct{}
	shutdown         chan struct{}
	stopOnce         sync.Once
	restartHandoff   atomic.Bool
	stopping         atomic.Bool
	running          atomic.Bool
	ready            atomic.Bool
	logOnce          sync.Once
	closeOnce        sync.Once
	logs             *logbook.Store
}

func New(store *state.Store, log *slog.Logger) *Server {
	return NewWithModelCatalog(store, log, modeldev.New())
}

// NewWithModelCatalog shares the supplied read-only catalog client across
// metadata queries and price imports. Server owns and closes the client.
func NewWithModelCatalog(store *state.Store, log *slog.Logger, catalogClient *modeldev.Client) *Server {
	if catalogClient == nil {
		catalogClient = modeldev.New()
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Explicit provider configuration, not ambient HTTP_PROXY, determines the route.
	tr.Proxy = nil
	tr.DisableCompression = true
	tr.MaxConnsPerHost = 64
	tr.MaxIdleConns = 32
	// The per-provider request context owns the entire deadline, including
	// waiting for response headers. A separate 30s cap truncated slow inference.
	tr.ResponseHeaderTimeout = 0
	outbound := egress.New(store, tr)
	policy, err := store.KeyCooldownSettings()
	if err != nil {
		// A broken read must not silently enable the default policy.
		policy = state.KeyCooldownSettings{}
	}
	return &Server{keyPolicy: policy, egress: outbound, Skills: skills.New(store.Dir), modelCatalog: catalogClient, keyRateRefresh: newKeyRateRefresh(), Agents: managed.NewManager(store), Store: store, Log: log, upstream: &http.Client{Transport: outbound, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, routing: map[string]*modelSchedule{}, affinity: map[string]routeAffinity{}, latency: map[string]routeLatency{}, active: map[string]int{}, health: map[string]*keyHealth{}, cursor: map[string]int{}, slots: make(chan struct{}, 64), shutdown: make(chan struct{})}
}
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.stopping.Store(true)
		s.keyRateRefresh.close()
		s.mu.Lock()
		runs := make([]*probeRun, 0, len(s.probeRuns))
		for _, run := range s.probeRuns {
			run.cancel()
			run.expiry.Stop()
			runs = append(runs, run)
		}
		s.mu.Unlock()
		for _, run := range runs {
			<-run.done
		}
		s.upstream.CloseIdleConnections()
		s.egress.Close()
		s.modelCatalog.Close()
		if s.logs != nil {
			_ = s.logs.Close()
		}
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, id apierror.ID, args ...any) {
	language := apierror.LanguageOf(w)
	status, detail := apierror.Render(id, language, args...)
	if recorder, ok := w.(interface{ logFailure(string) }); ok {
		recorder.logFailure(detail.Code)
	}
	if recorder, ok := w.(interface{ logErrorID(apierror.ID) }); ok {
		recorder.logErrorID(id)
	}
	w.Header().Set("Content-Language", string(detail.Language))
	w.Header().Add("Vary", "Accept-Language")
	writeJSON(w, status, struct {
		Error apierror.Detail `json:"error"`
	}{detail})
}
func apiError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, state.ErrReferenced):
		fail(w, apierror.Referenced)
	case errors.Is(err, state.ErrInvalid):
		fail(w, apierror.Invalid)
	case errors.Is(err, state.ErrConflict):
		fail(w, apierror.Conflict)
	case errors.Is(err, state.ErrNotFound):
		fail(w, apierror.NotFound)
	default:
		fail(w, apierror.Internal)
	}
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		fail(w, apierror.InvalidJSONFields)
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(w, apierror.SingleJSONObject)
		return false
	}
	return true
}
func (s *Server) guard(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = apierror.WithLanguage(w, apierror.Negotiate(r.Header.Get("Accept-Language")))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Browsers are not a supported management transport. No wildcard CORS.
		if r.Header.Get("Origin") != "" {
			fail(w, apierror.BrowserOriginRejected)
			return
		}
		if !allowedAPIHost(r, scope) {
			if _, application := r.Context().Value(applicationListenerKey{}).(bool); application {
				fail(w, apierror.APIClientRejected)
			} else {
				fail(w, apierror.InvalidHost)
			}
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" && scope == "gateway" && r.Header.Get("x-api-key") != "" {
			auth = "Bearer " + r.Header.Get("x-api-key")
		}
		valid := false
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if scope == "gateway" {
				if a, err := s.Store.AccessForToken(token, scope); err == nil {
					valid = true
					r = r.WithContext(context.WithValue(r.Context(), accessKey{}, a))
				}
			} else {
				valid = s.Store.Authenticate(token, scope)
			}
		}
		if !valid && scope == "gateway" && strings.HasPrefix(auth, "Bearer ") {
			if b, err := s.Store.BindingForToken(strings.TrimPrefix(auth, "Bearer ")); err == nil {
				valid = true
				r = r.WithContext(context.WithValue(r.Context(), bindingKey{}, b))
			}
		}
		if !valid {
			fail(w, apierror.Unauthorized)
			return
		}
		if s.running.Load() && !s.ready.Load() && !s.stopping.Load() && (scope == "gateway" || (r.Method != http.MethodGet && r.URL.Path != "/control/v1/shutdown")) {
			fail(w, apierror.Starting)
			return
		}
		if s.stopping.Load() && (scope == "gateway" || r.Method != http.MethodGet) {
			fail(w, apierror.Stopping)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) Control() http.Handler {
	s.initLogs()
	mux := http.NewServeMux()
	s.apiServiceRoutes(mux)
	s.ccSwitchRoutes(mux)
	s.logRoutes(mux)
	s.modelCatalogRoutes(mux)
	s.skillRoutes(mux)
	s.proxyRoutes(mux)
	mux.HandleFunc("POST /control/v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in *struct {
			ForRestart json.RawMessage `json:"for_restart"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in == nil {
			fail(w, apierror.ShutdownObjectRequired)
			return
		}
		forRestart := false
		switch strings.TrimSpace(string(in.ForRestart)) {
		case "", "false":
		case "true":
			forRestart = true
		default:
			fail(w, apierror.RestartBooleanRequired)
			return
		}
		if !s.running.Load() {
			fail(w, apierror.NotRunning)
			return
		}
		s.stopOnce.Do(func() {
			s.restartHandoff.Store(forRestart)
			s.stopping.Store(true)
			close(s.shutdown)
		})
		// Requests that crossed the stopping guard cannot change the first
		// accepted shutdown intent (including signal-driven ordinary stops).
		if s.restartHandoff.Load() != forRestart {
			fail(w, apierror.ShutdownModeConflict)
			return
		}
		result := map[string]bool{"accepted": true}
		if forRestart {
			result["bindings_preserved"] = s.restartHandoff.Load()
		}
		writeJSON(w, 200, result)
	})
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
		writeJSON(w, 200, map[string]any{"instance_id": cfg.InstanceID, "version": Version, "api_version": 1, "process_id": os.Getpid(), "platform": runtime.GOOS, "user": username, "control_address": cfg.ControlAddress, "gateway_address": cfg.GatewayAddress, "capabilities": []string{"graceful_stop", "restart_handoff", "responses_passthrough", "local_context", "provider_model_revision", "codex_configuration_management", "conditional_recovery", "binding_gateway_auth", "skill_library_v1", "proxy_v1", "api_service_v1"}})
	})
	mux.HandleFunc("GET /control/v1/status", func(w http.ResponseWriter, r *http.Request) {
		agents, err := s.Agents.Status()
		if err != nil {
			apiError(w, err)
			return
		}
		health := "ready"
		for _, a := range agents {
			if a.Status != "applied" && a.Status != "restored" && a.Status != "unmanaged" {
				health = "partial"
			}
		}
		phase := "ready"
		if s.running.Load() && !s.ready.Load() {
			phase = "starting"
		}
		if s.stopping.Load() {
			phase = "stopping"
		}
		writeJSON(w, 200, map[string]any{"core": phase, "gateway": phase, "upstream_health": "per_key", "agent_management": health, "bindings": agents, "agent_connection_verification": "unknown"})
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
			Provider         state.Provider             `json:"provider"`
			ExpectedRevision int64                      `json:"expected_revision"`
			Secret           *string                    `json:"secret,omitempty"`
			InitialKeys      []state.InitialProviderKey `json:"initial_keys,omitempty"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Provider.ID != r.PathValue("id") {
			fail(w, apierror.InvalidID)
			return
		}
		if err := in.Provider.Validate(); err != nil {
			fail(w, apierror.InvalidProvider)
			return
		}
		if in.Secret != nil {
			p := in.Provider
			p.Secret = *in.Secret
			if err := p.Validate(); err != nil {
				fail(w, apierror.InvalidProvider)
				return
			}
		}
		p, err := s.Store.PutProviderWithKeys(in.Provider, in.ExpectedRevision, in.Secret, in.InitialKeys)
		if err != nil {
			apiError(w, err)
			return
		}
		if in.ExpectedRevision == 0 {
			// Identify an auto platform once when the supplier is first created.
			// The result (including a classified failure) is cached without changing
			// the provider revision or returning credentials.
			_, _ = s.QueryProvider(r.Context(), providerapi.Query{ProviderID: p.ID, ExpectedRevision: p.Revision, Operation: "detect-platform"})
		}
		s.syncAgentMappings(w)
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /control/v1/providers/{id}/discover-models", s.discoverProviderModels)
	mux.HandleFunc("POST /control/v1/providers/{id}/probe", s.providerProbe)
	mux.HandleFunc("POST /control/v1/providers/{id}/chat", s.providerChat)
	mux.HandleFunc("POST /control/v1/models/{id}/routing", s.configureModelRouting)
	s.keyCooldownRoutes(mux)
	mux.HandleFunc("GET /control/v1/models/{id}/settings", s.getModelSettings)
	mux.HandleFunc("POST /control/v1/models/{id}/settings", s.saveModelSettings)
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
			fail(w, apierror.InvalidID)
			return
		}
		// Scoped settings writes own the metadata and server-captured reference.
		if in.ExpectedRevision > 0 {
			current, err := s.Store.Model(in.Model.ID)
			if err != nil {
				apiError(w, err)
				return
			}
			in.Model.Settings = current.Settings
			if in.Model.Settings != nil {
				in.Model.Settings.Configuration.Capabilities.ToolCall = &in.Model.Tools
				in.Model.Settings.Configuration.Streaming = &in.Model.Streaming
				if !in.Model.Tools {
					in.Model.Settings.Configuration.ParallelToolCalls = nil
				}
			}
		} else if in.Model.Settings != nil {
			apiError(w, state.ErrInvalid)
			return
		}
		if err := in.Model.Validate(); err != nil {
			fail(w, apierror.InvalidModelConfiguration)
			return
		}
		for _, c := range in.Model.Candidates {
			if _, err := s.Store.Provider(c.ProviderID); err != nil {
				if errors.Is(err, state.ErrNotFound) {
					fail(w, apierror.InvalidCandidate)
				} else {
					apiError(w, err)
				}
				return
			}
		}
		if in.ExpectedRevision == 0 {
			s.initializeModelCatalog(r.Context(), &in.Model)
		}
		m, err := s.Store.PutModel(in.Model, in.ExpectedRevision)
		if err != nil {
			apiError(w, err)
			return
		}
		s.syncAgentMappings(w)
		writeJSON(w, 200, m)
	})
	s.providerFeatureRoutes(mux)
	s.providerPlatformRoutes(mux)
	s.agentRoutes(mux)
	return s.guard("manage", mux)
}
func (s *Server) identity(w http.ResponseWriter, r *http.Request) bool {
	cfg, err := s.Store.Settings()
	if err != nil {
		apiError(w, err)
		return false
	}
	if r.Header.Get("X-Anchor-Instance") != cfg.InstanceID {
		fail(w, apierror.InstanceMismatch)
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

// Run binds both listeners before any approved configuration is applied.
func (s *Server) Run(ctx context.Context) error {
	s.running.Store(true)
	defer s.running.Store(false)
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
	servers := []*http.Server{control, gateway}
	listeners := []net.Listener{cl, gl}
	apiSettings, err := s.Store.APIServiceSettings()
	if err != nil {
		cl.Close()
		gl.Close()
		return err
	}
	if apiSettings.Enabled {
		ln, e := net.Listen("tcp4", apiSettings.Address())
		if e != nil {
			cl.Close()
			gl.Close()
			return fmt.Errorf("application API listen failed: %w", e)
		}
		apiServer := &http.Server{Handler: s.applicationGateway(apiSettings.AllowLAN), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return running }, ErrorLog: control.ErrorLog}
		servers = append(servers, apiServer)
		listeners = append(listeners, ln)
	}
	s.mu.Lock()
	s.apiServiceActive = apiSettings
	s.mu.Unlock()
	errs := make(chan error, len(servers))
	for i, server := range servers {
		go func() { errs <- server.Serve(listeners[i]) }()
	}
	s.modelCatalog.StartRefresh()
	stopSkillCollection := s.startSkillCollection()
	defer stopSkillCollection()
	s.keyRateRefresh.start(s)
	managedResults, managedErr := s.Agents.Start()
	if managedErr != nil {
		s.Log.Warn("agent startup reconciliation unavailable")
	}
	for _, result := range managedResults {
		s.Log.Info("binding startup", "binding_id", result.BindingID, "status", result.Status)
	}
	s.ready.Store(true)
	s.Log.Info("core ready", "instance_id", cfg.InstanceID, "control_address", cl.Addr().String(), "gateway_address", gl.Addr().String())
	var runErr error
	select {
	case <-ctx.Done():
	case <-s.shutdown:
	case runErr = <-errs:
	}
	// Signals/listener errors select ordinary recovery unless an authenticated
	// handoff already won. Freeze intent before deciding whether to restore.
	s.stopOnce.Do(func() {
		s.stopping.Store(true)
		close(s.shutdown)
	})
	s.stopping.Store(true)
	s.ready.Store(false)
	if s.restartHandoff.Load() {
		// A planned restart must not expose pre-Anchor routes to clients that
		// load configuration while the old core is draining. The replacement
		// verifies the existing journal; it never captures Anchor as the baseline.
		s.Agents.PreserveForRestart()
		s.Log.Info("bindings preserved for restart")
	} else {
		restored, restoreErr := s.Agents.Stop()
		if restoreErr != nil {
			s.Log.Warn("agent recovery incomplete; run anchor recover")
		}
		for _, result := range restored {
			s.Log.Info("binding shutdown", "binding_id", result.BindingID, "status", result.Status)
		}
	}
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	// Begin draining all listeners together, then cancel any requests that outlive the deadline.
	shutdowns := make(chan error, len(servers))
	for _, server := range servers {
		go func() { shutdowns <- server.Shutdown(deadline) }()
	}
	drainFailed := false
	for range servers {
		if <-shutdowns != nil {
			drainFailed = true
		}
	}
	cancel()
	for _, server := range servers {
		_ = server.Close()
	}
	s.mu.Lock()
	s.apiServiceActive = state.APIServiceSettings{}
	s.mu.Unlock()
	if drainFailed {
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
