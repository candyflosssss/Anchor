package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) workspace(id string) (catalog.Workspace, error) {
	out := catalog.Workspace{Prices: []catalog.Price{}, Estimates: []catalog.Estimate{}, ProviderID: id, Keys: []catalog.KeyStatus{}, Models: []catalog.ModelMapping{}, Usage: []catalog.UsageRow{}}
	p, err := s.Store.Provider(id)
	if err != nil {
		return out, err
	}
	out.Revision = p.Revision
	out.Strategy = s.Store.KeyStrategy(id)
	keys, err := s.Store.ProviderKeys(id)
	if err != nil {
		return out, err
	}
	for _, k := range keys {
		out.Keys = append(out.Keys, s.keyStatus(p, k))
	}
	models, err := s.Store.Models()
	if err != nil {
		return out, err
	}
	for _, m := range models {
		for index, c := range m.Candidates {
			if c.ProviderID == id {
				out.Models = append(out.Models, catalog.ModelMapping{ServiceTier: m.ServiceTier, KeyIDs: append([]string{}, c.KeyIDs...), CandidateIndex: index, ID: m.ID, DisplayName: m.DisplayName, Upstream: c.Model, Tools: m.Tools, Streaming: m.Streaming, Revision: m.Revision, CandidateCount: len(m.Candidates), Multiplier: c.EffectiveMultiplier(), Strategy: m.RoutingStrategy()})
			}
		}
	}
	usage, err := s.Store.Usage(id)
	if err != nil {
		return out, err
	}
	for _, u := range usage {
		out.Usage = append(out.Usage, catalog.UsageRow(u))
	}
	out.Prices, err = s.Store.Prices(id)
	if err != nil {
		return out, err
	}
	for _, u := range out.Usage {
		estimate := catalog.Estimate{Model: u.Model}
		for _, price := range out.Prices {
			if price.Model != u.Model || u.Known == 0 {
				continue
			}
			if (u.CacheRead > 0 && price.CacheReadUSD == nil) || (u.CacheWrite > 0 && price.CacheWriteUSD == nil) {
				continue
			}
			cost := float64(max(0, u.InputTokens-u.CacheRead-u.CacheWrite))*price.InputUSD + float64(u.OutputTokens)*price.OutputUSD
			if price.CacheReadUSD != nil {
				cost += float64(u.CacheRead) * *price.CacheReadUSD
			}
			if price.CacheWriteUSD != nil {
				cost += float64(u.CacheWrite) * *price.CacheWriteUSD
			}
			cost /= 1e6
			estimate.USD = &cost
		}
		out.Estimates = append(out.Estimates, estimate)
	}
	return out, nil
}
func (s *Server) providerFeatureRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/providers/{id}/workspace", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.workspace(r.PathValue("id"))
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /control/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Store.Usage("")
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/providers/{id}/action", s.providerAction)
}
func (s *Server) providerAction(w http.ResponseWriter, r *http.Request) {
	if !s.identity(w, r) {
		return
	}
	var in catalog.Action
	if !decode(w, r, &in) {
		return
	}
	if in.ProviderID != r.PathValue("id") || !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 {
		apiError(w, state.ErrInvalid)
		return
	}
	p, err := s.Store.Provider(in.ProviderID)
	if err != nil {
		apiError(w, err)
		return
	}
	if p.Revision != in.ExpectedRevision {
		apiError(w, state.ErrConflict)
		return
	}
	target := p.ID
	var probe *catalog.Probe
	var imported *catalog.Price
	switch in.Action {
	case "delete", "rename", "key-delete", "model-delete":
		if !in.Approved {
			apiError(w, state.ErrInvalid)
			return
		}
	}
	switch in.Action {
	case "price-put":
		if in.Price == nil {
			err = state.ErrInvalid
			break
		}
		in.Price.Model = in.Target
		err = s.Store.PutPrice(p.ID, p.Revision, *in.Price)
	case "price-import":
		imported, err = s.importPrice(r.Context(), in.Target, in.CatalogProvider, in.Model)
		if errors.Is(err, errTieredReferencePrice) {
			fail(w, apierror.TieredPrice)
			return
		}
	case "delete", "copy", "rename", "key-put", "key-delete", "strategy":
		err = s.Store.MutateProvider(p.ID, p.Revision, in.Action, in.Target, in.Secret, in.Strategy, in.Enabled, in.CopyKeys, in.Protocol)
		if in.Action == "rename" || in.Action == "copy" {
			target = in.Target
		}
		if err == nil && in.Action == "delete" {
			s.syncAgentMappings(w)
			writeJSON(w, 200, catalog.Workspace{Prices: []catalog.Price{}, Estimates: []catalog.Estimate{}, ProviderID: p.ID, Deleted: true, Keys: []catalog.KeyStatus{}, Models: []catalog.ModelMapping{}, Usage: []catalog.UsageRow{}})
			return
		}
	case "key-model-protocol":
		err = s.Store.PutKeyModelProtocol(p.ID, p.Revision, in.Target, in.Model, in.Protocol)
	case "key-reset":
		if !in.Approved {
			err = state.ErrInvalid
			break
		}
		keys, e := s.Store.ProviderKeys(p.ID)
		err = e
		found := false
		for _, k := range keys {
			if k.ID == in.Target {
				found = true
			}
		}
		if !found {
			err = state.ErrNotFound
			break
		}
		s.mu.Lock()
		delete(s.health, healthID(p, in.Target))
		s.mu.Unlock()
	case "test":
		if !in.Approved || in.Model == "" || len(in.Model) > 256 || strings.ContainsAny(in.Model, "\r\n") {
			err = state.ErrInvalid
			break
		}
		probe, err = s.probe(r.Context(), p, in.Target, in.Model)
	case "model-put":
		if !state.ValidModelID(in.Target) || in.Model == "" || !state.ValidModelStrategy(in.Strategy) {
			err = state.ErrInvalid
			break
		}
		expected := in.ModelRevision
		m, e := s.Store.Model(in.Target)
		if errors.Is(e, state.ErrNotFound) && expected == 0 {
			m = state.Model{ID: in.Target, DisplayName: in.Target, Tools: in.Tools, Streaming: in.Streaming, Strategy: "round-robin"}
		} else if e != nil {
			err = e
			break
		}
		index := -1
		for i, c := range m.Candidates {
			if c.ProviderID == p.ID {
				// A provider-scoped editor cannot safely edit multiple legacy entries.
				if index >= 0 {
					err = state.ErrInvalid
					break
				}
				index = i
			}
		}
		if err != nil {
			break
		}
		if expected == 0 {
			if index >= 0 {
				err = state.ErrConflict
				break
			}
			// Attach to the latest snapshot, preserving all existing candidates and metadata.
			expected = m.Revision
			m.Candidates = append(m.Candidates, state.Candidate{ProviderID: p.ID, Model: in.Model, Multiplier: in.Multiplier, KeyIDs: in.KeyIDs})
		} else {
			if m.Revision != expected {
				err = state.ErrConflict
				break
			}
			if index < 0 {
				err = state.ErrInvalid
				break
			}
			m.Candidates[index].Model = in.Model
			if in.KeyIDs != nil {
				m.Candidates[index].KeyIDs = in.KeyIDs
			}
			if in.Multiplier != 0 {
				m.Candidates[index].Multiplier = in.Multiplier
			}
			m.Tools, m.Streaming = in.Tools, in.Streaming
			if m.Settings != nil {
				m.Settings.Configuration.Capabilities.ToolCall = &m.Tools
				m.Settings.Configuration.Streaming = &m.Streaming
				if !m.Tools {
					m.Settings.Configuration.ParallelToolCalls = nil
				}
			}
		}
		// Joining an existing public model preserves its shared strategy/capabilities.
		// Editing an existing mapping can explicitly change the shared strategy.
		if in.Strategy != "" && (m.Revision == 0 || in.ModelRevision > 0) {
			m.Strategy = in.Strategy
		}

		if in.ServiceTier != nil {
			if !state.ValidServiceTier(*in.ServiceTier) {
				err = state.ErrInvalid
				break
			}
			// Joining a shared alias cannot silently change its speed policy.
			if m.Revision > 0 && in.ModelRevision == 0 && m.ServiceTier != *in.ServiceTier {
				err = state.ErrConflict
				break
			}
			m.ServiceTier = *in.ServiceTier
		}
		if m.Validate() != nil {
			err = state.ErrInvalid
			break
		}
		if m.Revision == 0 {
			s.initializeModelCatalog(r.Context(), &m)
		}
		_, err = s.Store.PutProviderModelVariants(m, expected, p.ID, p.Revision, in.SpeedVariants)
	case "model-delete":
		m, e := s.Store.Model(in.Target)
		if e != nil {
			err = e
			break
		}
		found := false
		for _, c := range m.Candidates {
			if c.ProviderID == p.ID {
				found = true
			}
		}
		if !found {
			err = state.ErrInvalid
			break
		}
		err = s.Store.DeleteProviderModel(in.Target, in.ModelRevision, p.ID, p.Revision)
	default:
		err = state.ErrInvalid
	}
	if err != nil {
		apiError(w, err)
		return
	}
	s.syncAgentMappings(w)
	out, err := s.workspace(target)
	if err != nil {
		apiError(w, err)
		return
	}
	out.Probe = probe
	out.ImportedPrice = imported
	writeJSON(w, 200, out)
}
func (s *Server) probe(ctx context.Context, p state.Provider, keyID, model string) (*catalog.Probe, error) {
	return s.probeObserved(ctx, p, keyID, model, nil)
}

func (s *Server) probeObserved(ctx context.Context, p state.Provider, keyID, model string, notify func(string, string)) (*catalog.Probe, error) {
	return s.inferenceObserved(ctx, p, keyID, model, "", nil, nil, notify)
}

func (s *Server) inferenceObserved(ctx context.Context, p state.Provider, keyID, model, reasoningEffort string, messages []catalog.ChatMessage, selectedKey *state.ProviderKey, notify func(string, string)) (*catalog.Probe, error) {
	key := state.ProviderKey{ID: "anonymous", Enabled: true, Protocol: p.Protocol}
	if selectedKey != nil {
		key = *selectedKey
	} else {
		keys, err := s.Store.ProviderKeys(p.ID)
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			found := false
			for _, k := range keys {
				if k.ID == keyID || (keyID == "" && k.Enabled) {
					key = k
					found = true
					break
				}
			}
			if !found {
				return nil, state.ErrInvalid
			}
		} else if keyID != "" && keyID != "anonymous" {
			return nil, state.ErrInvalid
		}
	}
	p.Protocol = key.EffectiveProtocol(model)
	out := &catalog.Probe{Revision: p.Revision, KeyID: key.ID, Model: model, At: time.Now().Unix()}
	start := time.Now()
	id, err := state.Token()
	if err != nil {
		return nil, err
	}
	callerCtx := ctx
	stream := notify != nil
	row := logbook.Request{ID: id, StartedAtMS: start.UnixMilli(), Status: "pending", Source: "probe", ReasoningObserved: true, Stream: &stream, UpstreamModel: model, Model: model, Protocol: p.Protocol, Provider: p.ID, Key: key.ID, Revision: p.Revision, Delivery: "none", Stage: "route", Attempts: []logbook.Attempt{}, Evidence: map[string]string{"sent_service_tier": "not_sent", "fast_triggered": "unknown"}}
	row.ReferenceCost = s.cachedLogReference(model)
	costFactors := logCostFactors(p, key)
	if reasoningEffort != "" {
		row.ReasoningEffort = &reasoningEffort
	}
	if s.logs != nil {
		s.logs.Request(row)
	}
	var actualHTTP *int
	stage := "route"
	defer func() {
		ms := time.Since(start).Milliseconds()
		row.ElapsedMS = &ms
		row.HTTP = actualHTTP
		row.Stage = stage
		row.Code = out.Failure
		row.Status = "failed"
		if out.Failure == "" && actualHTTP != nil {
			row.Status = "succeeded"
			row.Delivery = "complete"
			row.Stage = "respond"
		}
		if callerCtx.Err() != nil {
			row.Status = "cancelled"
			if row.Code == "" {
				row.Code = "client_cancelled"
			}
		}
		if row.Code == "" && row.Status != "succeeded" {
			row.Code = "probe_interrupted"
		}
		if s.logs != nil {
			s.logs.Request(row)
		}
	}()

	attempted := false
	after := ""
	u := protocol.Usage{}
	defer func() {
		out.ElapsedMS = time.Since(start).Milliseconds()
		if attempted {
			row.AttemptCount = 1
			a := logbook.Attempt{CostAdjustment: costFactors, Number: 1, Provider: p.ID, Key: key.ID, Model: model, Protocol: p.Protocol, HTTP: actualHTTP, ElapsedMS: out.ElapsedMS, Outcome: "failed", Code: out.Failure, Stage: stage}
			if out.Failure == "" {
				a.Outcome = "succeeded"
			}
			if u.Known {
				a.Input = &u.Input
				a.Output = &u.Output
				row.Input = &u.Input
				row.Output = &u.Output
			}
			t := requestTrace{row: row}
			t.priceAttempt(&a, u)
			if row.ReferenceCost != nil {
				row.ReferenceCost.USD = a.ReferenceUSD // preserve single-probe decimal representation
			}
			row.Attempts = []logbook.Attempt{a}
			if u.CacheReadKnown {
				row.Metrics.CacheReadTokens = &u.CacheRead
			}
			if u.ReasoningKnown {
				row.Metrics.ReasoningTokens = &u.Reasoning
			}
			s.finishKey(p, key.ID, out.HTTPStatus, after, out.Failure == "")
			s.mu.Lock()
			if h := s.health[healthID(p, key.ID)]; h != nil {
				h.Test = out
			}
			s.mu.Unlock()
			v := state.Usage{ProviderID: p.ID, Model: "test:" + model, Requests: 1, ElapsedMS: out.ElapsedMS, InputTokens: u.Input, OutputTokens: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
			if u.Known {
				v.Known = 1
			}
			if out.Failure != "" {
				v.Failures = 1
			}
			if s.Store.RecordUsage(v) != nil {
				s.Log.Error("test usage persistence failed")
			}
		}
	}()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		out.Failure = "busy"
		return out, nil
	}
	if !s.take(p.ID, p.MaxConcurrent) {
		out.Failure = "busy"
		return out, nil
	}
	defer s.release(p.ID)
	input := any("Reply OK.")
	timeout := 10 * time.Second
	maxTokens, maxText, maxBody := 16, 16384, int64(1<<20)
	if messages != nil {
		input = messages
		timeout = 120 * time.Second
		maxTokens = 4096
		maxText = catalog.ChatMaxOutputBytes
		maxBody = 8 << 20
	}
	raw, _ := json.Marshal(map[string]any{"model": model, "input": input, "max_output_tokens": maxTokens, "store": false, "stream": stream})
	body, err := protocol.ConvertRequest(raw, "responses", p.Protocol)
	if err != nil {
		return nil, err
	}
	// Apply the selected native option only after text conversion. The general
	// cross-protocol converter deliberately refuses lossy reasoning conversion.
	if reasoningEffort != "" {
		body, err = chatReasoningBody(body, p.Protocol, reasoningEffort)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, min(time.Duration(p.TimeoutSeconds)*time.Second, timeout))
	defer cancel()
	req, err := http.NewRequestWithContext(egress.WithKeyRoute(ctx, p.ID, key.ID, key.Secret != ""), "POST", strings.TrimRight(p.BaseURL, "/")+protocol.Endpoint(p.Protocol), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	authorizeUpstream(req, p, key.Secret)
	recordSentServiceTier(row.Evidence, body, key.Secret)
	attempted = true
	stage = "upstream_connect"
	resp, err := s.inferenceResponse(req, p.BaseURL, p.Protocol, row.Evidence)
	if errors.Is(err, egress.ErrUnavailable) {
		out.Failure = "proxy_unavailable"
		return out, nil
	}
	if err != nil {
		out.Failure = "network"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.Failure = "timeout"
		}
		return out, nil
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	v := resp.StatusCode
	actualHTTP = &v
	stage = "upstream_response"
	after = resp.Header.Get("Retry-After")
	switch resp.StatusCode {
	case 200:
	case 401, 403:
		out.Failure = "authentication"
	case 429:
		out.Failure = "rate_limited"
	default:
		out.Failure = "http_error"
	}
	if out.Failure != "" {
		return out, nil
	}
	if row.Evidence["response_kind"] == "html" {
		stage = "response_validate"
		out.Failure = "unexpected_html"
		row.Evidence["validation_reason"] = "html_instead_of_api"
		return out, nil
	}
	if notify != nil {
		notify("receiving", "")
		if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			output := &probeOutput{header: http.Header{}, secret: key.Secret, limit: maxText, strict: messages != nil, notify: notify}
			observation := &protocol.Observation{}
			u, err = protocol.StreamObserved(output, io.LimitReader(resp.Body, maxBody+1), p.Protocol, "responses", model, observation)
			recordReturnedServiceTier(row.Evidence, observation.ServiceTier, observation.ServiceTierRaw, key.Secret)
			stage = "response_validate"
			if err != nil || observation.Status == "failed" || observation.Status == "incomplete" {
				out.Failure = "invalid_response"
			}
			if ctx.Err() != nil {
				out.Failure = "timeout"
			}
			if messages != nil && strings.TrimSpace(output.text) == "" {
				out.Failure = "invalid_response"
			}
			value := publicProbeText(output.text, key.Secret, false)
			if messages != nil && len(value) > maxText {
				out.Failure = "invalid_response"
			}
			if out.Failure == "" {
				notify("receiving", value)
			}
			return out, nil
		}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		out.Failure = "network"
		return out, nil
	}
	if int64(len(data)) > maxBody {
		out.Failure = "invalid_response"
		return out, nil
	}
	stage = "response_validate"
	row.Evidence["body_bytes"] = strconv.Itoa(len(data))
	row.Evidence["json_valid"] = strconv.FormatBool(json.Valid(data))
	var response protocol.Object
	if json.Unmarshal(data, &response) == nil {
		recordReturnedServiceTier(row.Evidence, protocol.ReadServiceTier(response, p.Protocol), protocol.ReadRawServiceTier(data, p.Protocol, key.Secret))
	}
	usage, err := protocol.ValidateProbeResponse(data, p.Protocol)
	if err != nil {
		out.Failure = "invalid_response"
		row.Evidence["validation_reason"] = protocol.ResponseValidationReason(err)
	} else {
		u = usage
		row.Evidence["validation"] = "passed"
		if notify != nil {
			parsed, e := protocol.ParseProbeResponse(data, p.Protocol)
			if e == nil {
				var text strings.Builder
				for _, item := range parsed.Items {
					if !item.Tool {
						text.WriteString(item.Text)
					}
				}
				value := text.String()
				if len(value) > maxText {
					if messages != nil {
						out.Failure = "invalid_response"
						return out, nil
					}
					value = value[:maxText]
				}
				if messages != nil && strings.TrimSpace(value) == "" {
					out.Failure = "invalid_response"
					return out, nil
				}
				if key.Secret != "" {
					value = strings.ReplaceAll(value, key.Secret, "[redacted]")
				}
				if messages != nil && len(value) > maxText {
					out.Failure = "invalid_response"
					return out, nil
				}
				notify("receiving", value)
			}
		}
	}
	return out, nil
}

func chatReasoningBody(body []byte, protocolName, effort string) ([]byte, error) {
	var request map[string]any
	if !catalog.ValidChatReasoningEffort(effort) || json.Unmarshal(body, &request) != nil {
		return nil, state.ErrInvalid
	}
	switch protocolName {
	case "responses":
		request["reasoning"] = map[string]string{"effort": effort}
	case "chat-completions":
		request["reasoning_effort"] = effort
	case "anthropic":
		if effort == "none" || effort == "minimal" {
			return nil, state.ErrInvalid
		}
		// Effort does not imply enabling thinking or inventing a token budget.
		request["output_config"] = map[string]string{"effort": effort}
	default:
		return nil, state.ErrInvalid
	}
	return json.Marshal(request)
}
