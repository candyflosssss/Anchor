package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) agents(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("agent requires discover/enroll/add/list/inspect/plan/apply/sync/restore/enable/disable/set-models/codex-settings")
	}
	switch args[0] {
	case "codex-settings":
		f := a.flags("agent codex-settings")
		retries := f.Int("retries", -1, "unified Codex request/stream retries, 0..10 (default 5)")
		revision := f.Int64("revision", 0, "expected settings revision (required when saving)")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		writing := false
		f.Visit(func(flag *flag.Flag) {
			if flag.Name == "retries" {
				writing = true
			}
		})
		if !writing {
			if *revision != 0 {
				return errors.New("--revision requires --retries")
			}
			v, err := c.CodexSettings(ctx)
			if err != nil {
				return err
			}
			return a.output(v)
		}
		in := state.SaveCodexSettings{Retries: retries, ExpectedRevision: *revision}
		if err := in.Validate(); err != nil {
			return errors.New("--retries must be 0..10 and --revision is required")
		}
		v, err := c.SaveCodexSettings(ctx, in)
		if err != nil {
			return err
		}
		if err := a.output(v); err != nil {
			return err
		}
		if len(v.NeedsSync) > 0 {
			return errors.New("settings saved; some Codex targets need agent sync; restart Codex after syncing")
		}
		return nil

	case "discover":
		if len(args) != 1 {
			return errors.New("agent discover takes no arguments")
		}
		v, err := c.AgentWorkspace(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	case "enroll":
		f := a.flags("agent enroll")
		target := f.String("target", "", "discovered local target ID (required)")
		scope := f.String("model-scope", "", "all-mapped or all-compatible; Codex defaults to all-compatible, other adapters to all-mapped")
		model := f.String("model", "", "default public model (required)")
		revision := f.Int64("revision", 0, "existing binding revision; 0 for new target")
		approve := f.Bool("approve", false, "approve target-local catalog and configuration management")
		rebuild := f.Bool("rebuild", false, "back up and replace the entire Codex config with a clean all-mapped configuration")
		var templates repeated
		f.Var(&templates, "template", "public-model:local-catalog-model; empty source excludes alias; repeatable")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		choices := map[string]string{}
		for _, item := range templates {
			alias, source, ok := strings.Cut(item, ":")
			if !ok || !state.ValidModelID(alias) {
				return errors.New("--template requires public-model:local-catalog-model")
			}
			if _, duplicate := choices[alias]; duplicate {
				return errors.New("duplicate template choice")
			}
			choices[alias] = source
		}
		if *target == "" || *model == "" || !*approve || *revision < 0 {
			return errors.New("--target, --model and --approve are required; revision must be nonnegative")
		}
		if *rebuild && (*scope != "all-mapped" || len(choices) != 0) {
			return errors.New("--rebuild requires --model-scope all-mapped and no --template")
		}
		v, err := c.EnrollAgent(ctx, managed.EnrollAgent{Rebuild: *rebuild, ModelScope: *scope, TargetID: *target, Model: *model, ExpectedRevision: *revision, ModelTemplates: choices, Approved: *approve})
		if err != nil {
			return err
		}
		return a.output(v)

	case "list":
		if len(args) != 1 {
			return errors.New("agent list takes no arguments")
		}
		v, e := c.Agents(ctx)
		if e != nil {
			return e
		}
		return a.output(v)
	case "add":
		f := a.flags("agent add")
		id := f.String("id", "", "binding ID: Chinese supported, at most 64 UTF-8 bytes")
		adapter := f.String("adapter", "codex", "adapter: codex, claude, dsh (Web) or dsh-desktop")
		root := f.String("config-root", "", "approved absolute configuration root; DSH home for dsh/web or dsh-desktop/desktop; profile must exist")
		codexRoot := f.String("codex-home", "", "alias for --config-root with --adapter codex")
		model := f.String("model", "", "default public model (tools and streaming required)")
		scope := f.String("model-scope", "", "model permission scope: all-compatible, all-mapped or single; legacy bindings stay single")
		exe := f.String("anchor-executable", "", "stable absolute Anchor binary path")
		approved := f.Bool("approve", false, "approve long-term configuration management; applies on next serve or agent apply")
		if e := parse(f, args[1:]); e != nil {
			return e
		}
		if *codexRoot != "" {
			if *adapter != "codex" || *root != "" {
				return errors.New("--codex-home is a Codex-only alias; use one root flag")
			}
			*root = *codexRoot
		}
		if *scope == "" {
			*scope = "all-compatible"
			if *adapter != "codex" {
				*scope = "all-mapped"
			}
		}
		if *scope != "single" && *scope != "all-compatible" && *scope != "all-mapped" {
			return errors.New("--model-scope must be all-compatible, all-mapped or single")
		}
		if !*approved {
			return errors.New("--approve is required; review the configuration root first")
		}
		if *exe == "" {
			v, e := os.Executable()
			if e != nil {
				return e
			}
			*exe = v
		}
		b := state.Binding{ID: *id, AdapterID: *adapter, AdapterVersion: "1", ConfigRoot: *root, Model: *model, ModelScope: *scope, Executable: *exe, Enabled: true}
		// The core resolves catalog version from the selected default model and
		// performs full Binding/target/policy validation before approval is stored.
		v, e := c.AddAgent(ctx, b)
		if e != nil {
			return e
		}
		return a.output(v)
	case "set-models":
		if len(args) < 2 {
			return errors.New("agent set-models requires binding ID")
		}
		f := a.flags("agent set-models")
		revision := f.Int64("revision", 0, "expected binding revision (required)")
		model := f.String("model", "", "new default public model (required)")
		scope := f.String("model-scope", "", "single, all-mapped or all-compatible (required; explicit authorization)")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *revision < 1 || *model == "" || (*scope != "single" && *scope != "all-compatible" && *scope != "all-mapped") {
			return errors.New("--revision, --model and explicit --model-scope required")
		}
		v, err := c.SetAgentModels(ctx, args[1], *revision, *model, *scope)
		if err != nil {
			return err
		}
		return a.output(v)
	case "inspect", "plan":
		if len(args) != 2 {
			return errors.New("agent query requires one binding ID")
		}
		v, e := c.AgentQuery(ctx, args[1], args[0])
		if e != nil {
			return e
		}
		return a.output(v)
	case "apply", "sync", "restore", "enable", "disable":
		if len(args) < 2 {
			return errors.New("agent action requires binding ID")
		}
		f := a.flags("agent " + args[0])
		revision := f.Int64("revision", 0, "expected binding revision (required)")
		fp := f.String("fingerprint", "", "reviewed plan fingerprint (required for apply)")
		if e := parse(f, args[2:]); e != nil {
			return e
		}
		if *revision < 1 {
			return errors.New("--revision is required")
		}
		if args[0] == "apply" && *fp == "" {
			return errors.New("review agent plan and pass --fingerprint")
		}
		v, e := c.AgentAction(ctx, args[1], args[0], *revision, *fp)
		if e != nil {
			return e
		}
		if e = a.output(v); e != nil {
			return e
		}
		if v["status"] == "conflict" {
			return errors.New("conditional restore has conflicts; third-party changes preserved")
		}
		if m, ok := v["management"].(map[string]any); ok && m["status"] == "conflict" {
			return errors.New("binding disabled; restore conflicts preserved")
		}
		return nil
	default:
		return errors.New("unknown agent command")
	}
}
func (a App) recover(s *state.Store, args []string) error {
	f := a.flags("recover")
	id := f.String("id", "", "optional binding ID; otherwise recover all registered targets")
	if e := parse(f, args); e != nil {
		return e
	}
	if *id != "" && !state.ValidID(*id) {
		return errors.New("invalid binding ID")
	}
	// The OS lock prevents standalone recovery racing a running core.
	lock, e := platform.AcquireLock(filepath.Join(s.Dir, "instance.lock"))
	if e != nil {
		return errors.New("stop Anchor before running independent recovery")
	}
	defer lock.Close()
	bs, e := s.Bindings()
	if e != nil {
		return e
	}
	results := []managed.Result{}
	failed := false
	found := *id == ""
	engine := managed.NewEngine(s)
	for _, b := range bs {
		if *id != "" && *id != b.ID {
			continue
		}
		found = true
		r, e := engine.Recover(b.ID)
		if e != nil {
			r = managed.Result{BindingID: b.ID, Status: "recovery_pending"}
			failed = true
		}
		if r.Status == "conflict" {
			failed = true
		}
		results = append(results, r)
	}
	if !found {
		return state.ErrNotFound
	}
	if e = a.output(results); e != nil {
		return e
	}
	if failed {
		return fmt.Errorf("recovery incomplete; conflicts were preserved")
	}
	return nil
}
