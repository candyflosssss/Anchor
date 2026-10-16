package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
)

// Read text-only conversation history from stdin, never command-line arguments.
func (a App) providerChat(ctx context.Context, c *client.Client, args []string) error {
	f := a.flags("provider chat")
	id := f.String("id", "", "saved enabled provider ID")
	revision := f.Int64("revision", 0, "current provider revision")
	key := f.String("key", "", "exact enabled saved Key ID, or anonymous")
	model := f.String("model", "", "upstream model ID")
	approve := f.Bool("approve", false, "confirm upstream inference cost")
	if err := parse(f, args); err != nil {
		return err
	}
	in := catalog.ChatRequest{ProviderID: *id, ExpectedRevision: *revision, KeyID: *key, Model: *model, Approved: *approve, Action: "start", Messages: []catalog.ChatMessage{{Role: "user", Content: "validation"}}}
	if in.Validate() != nil {
		return errors.New("provider chat requires --id, --revision, --key, --model and --approve")
	}
	if a.In == nil {
		return errors.New("provider chat requires a JSON message array on stdin")
	}
	// Bound encoded input too, including malicious whitespace and escaped strings.
	raw, err := io.ReadAll(io.LimitReader(a.In, (128<<10)+1))
	if err != nil || len(raw) > 128<<10 {
		return errors.New("chat stdin exceeds 128 KiB or cannot be read")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in.Messages) != nil || decoder.Decode(new(any)) != io.EOF || in.Validate() != nil {
		return errors.New("chat stdin must be 1..80 alternating user/assistant text messages ending with user, at most 64 KiB of text")
	}
	next, err := c.ProviderChat(ctx, in)
	if err != nil {
		return err
	}
	in.RunID = next.RunID
	in.Action = "read"
	in.Messages = nil
	defer func() {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		in.Action = "cancel"
		_, _ = c.ProviderChat(cancelCtx, in)
	}()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for next.Status == "running" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		next, err = c.ProviderChat(ctx, in)
		if err != nil {
			return err
		}
	}
	if err := a.output(next); err != nil {
		return err
	}
	if next.Status != "completed" || next.Result == nil || next.Result.Failure != "" {
		return errors.New("chat did not complete successfully; inspect result.failure")
	}
	return nil
}
