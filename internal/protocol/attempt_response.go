package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/candyflosssss/Anchor/internal/apierror"
)

const MaxResponseBytes = 16 << 20

type Response struct {
	Body        []byte
	Usage       Usage
	Observation Observation
	Diagnostics Diagnostics
}

// TranslateResponse owns bounded decoding, semantic validation and client encoding.
// It does not read HTTP headers, write a client response or persist observations.
func (a *Attempt) TranslateResponse(body io.Reader) (Response, error) {
	out := Response{Diagnostics: a.Diagnostics()}
	out.Diagnostics.Stage = StageResponse
	fail := func(category string, err error) (Response, error) {
		out.Diagnostics.Category = category
		out.Diagnostics.Compatibility = Unsupported
		return out, newTranslationError(a.client, a.upstream, StageResponse, category, err)
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxResponseBytes+1))
	if err != nil || len(data) > MaxResponseBytes {
		return fail("incomplete_body", io.ErrUnexpectedEOF)
	}
	m, err := Decode(data)
	if err != nil {
		category := "unsupported_content"
		if a.client == a.upstream {
			category = "invalid_json"
		}
		return fail(category, err)
	}
	out.Observation.ServiceTier = ReadServiceTier(m, a.upstream)
	out.Observation.ServiceTierRaw = ReadRawServiceTier(data, a.upstream)
	out.Usage = ReadUsage(m, a.upstream)
	status, err := generationStatus(m, a.upstream)
	out.Observation.Status = status
	if err != nil {
		return fail("upstream_error", err)
	}
	if a.client == a.upstream {
		out.Body = data
		return out, nil
	}
	if status == "incomplete" {
		// Filter/refusal finishes cannot be relabeled as a successful text reply.
		if a.upstream == "chat-completions" {
			for _, v := range arr(m["choices"]) {
				if obj(v)["finish_reason"] == "content_filter" {
					return fail("unsupported_finish", ErrUnsupported)
				}
			}
		}
		if a.upstream == "anthropic" && m["stop_reason"] == "refusal" {
			return fail("unsupported_finish", ErrUnsupported)
		}
	}
	result, err := ParseResponse(data, a.upstream)
	if err != nil {
		return fail(ResponseValidationReason(err), err)
	}
	out.Usage = result.Usage
	if a.client == "responses" && a.upstream == "chat-completions" {
		err = a.mapping.RestoreResult(&result)
	}
	if err != nil || result.CheckTarget(a.client) != nil {
		return fail("unsupported_content", ErrUnsupported)
	}
	if a.client == "anthropic" && !out.Usage.Known {
		return fail("missing_usage", ErrUnsupported)
	}
	result.Model = a.public
	out.Body, err = json.Marshal(result.JSON(a.client))
	out.Body = append(out.Body, '\n')
	if err != nil {
		return fail("unsupported_content", err)
	}
	return out, nil
}

// Existing API error contracts remain gateway-owned HTTP delivery decisions.
func ResponseErrorID(err error) apierror.ID {
	switch ErrorDiagnostics(err).Category {
	case "incomplete_body":
		return apierror.UpstreamIncomplete
	case "invalid_json":
		return apierror.UpstreamInvalidJSON
	case "upstream_error":
		d := ErrorDiagnostics(err)
		if d.Client != d.Upstream {
			return apierror.UpstreamUnsupportedContent
		}
		return apierror.UpstreamFailed
	case "missing_usage":
		return apierror.UpstreamMissingUsage
	default:
		return apierror.UpstreamUnsupportedContent
	}
}

// generationStatus distinguishes explicit nonterminal/failure from HTTP success.
// Missing status in legacy native replies remains unknown, never fabricated complete.
func generationStatus(m Object, p string) (string, error) {
	if m["error"] != nil {
		return "failed", ErrUnsupported
	}
	var finish string
	switch p {
	case "responses":
		switch m["status"] {
		case "completed":
			return "completed", nil
		case "incomplete":
			return "incomplete", nil
		case nil, "":
			return "", nil
		default:
			return "failed", ErrUnsupported
		}
	case "chat-completions":
		for _, v := range arr(m["choices"]) {
			f := str(obj(v)["finish_reason"])
			if f == "length" || f == "content_filter" {
				return "incomplete", nil
			}
			if f != "stop" && f != "tool_calls" && f != "function_call" {
				return "", ErrUnsupported
			}
			finish = f
		}
	case "anthropic":
		finish = str(m["stop_reason"])
		if finish == "max_tokens" || finish == "refusal" {
			return "incomplete", nil
		}
		if finish != "end_turn" && finish != "tool_use" && finish != "stop_sequence" {
			return "", ErrUnsupported
		}
	}
	if finish != "" {
		return "completed", nil
	}
	return "", ErrUnsupported
}

type StreamOptions struct {
	Language apierror.Language
	// OnFirstOutput reports actual generated content, including reasoning/tool data,
	// once. Envelopes, role-only deltas and heartbeats do not satisfy it.
	OnFirstOutput func()
	// FailureID optionally identifies a local network deadline error.
	FailureID func() apierror.ID
	// ErrorWriter permits a final local failure frame after the upstream context
	// is cancelled. Its owner supplies independent delivery limits.
	ErrorWriter io.Writer
}

// TranslateStream performs synchronous incremental reads/writes. The owner must
// cancel/close a blocking network reader on cancellation; no goroutine is spawned.
func (a *Attempt) TranslateStream(ctx context.Context, w io.Writer, body io.Reader, options StreamOptions) (Response, error) {
	out := Response{Diagnostics: a.Diagnostics()}
	out.Diagnostics.Stage = StageStream
	out.Observation.onFirstOutput = options.OnFirstOutput
	usage, err := translateStream(&contextWriter{ctx: ctx, w: w}, &contextReader{ctx: ctx, r: body}, a.upstream, a.client, a.public, &out.Observation, a.mapping, options)
	out.Observation.onFirstOutput = nil
	out.Usage = usage
	if err != nil {
		category := "invalid_stream"
		if ctx.Err() != nil {
			err = ctx.Err()
			category = "cancelled"
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			category = "cancelled"
		} else if errors.Is(err, io.ErrUnexpectedEOF) {
			category = "missing_terminal"
		}
		out.Diagnostics.Category = category
		out.Diagnostics.Compatibility = Unsupported
		return out, newTranslationError(a.client, a.upstream, StageStream, category, err)
	}
	return out, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}
func (w *contextWriter) Flush() error { return flushStream(w.w) }
