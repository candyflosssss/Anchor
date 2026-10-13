package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

const help = `Anchor — headless model gateway (development milestone)
Usage: anchor [--data-dir DIR] [--context NAME] COMMAND [--json]
  init [--control-address 127.0.0.1:17861] [--gateway-address 127.0.0.1:17860]
  serve                              Run the local foreground core
  context add NAME --local           Authenticate and bind a local instance
  context use NAME | context list | context inspect NAME | context test NAME
  status                             Query selected context
  stop --approve                     Request graceful shutdown of selected context
  provider list
  provider platform --id ID             Read platform configuration (redacted)
  provider platform-put --id ID --revision N [--platform auto|sub2api|newapi|oneapi]
      [--site-url URL] [--user-id N] [--user-token-stdin | --clear-user-token]
  provider detect-platform --id ID --revision N [--key KEY]
  provider balance-summary --id ID --revision N [--key KEY]
  provider balance|key-quota|effective-rate --id ID --revision N [--key KEY] [--remote-key-id N]
  provider usage-records --id ID --revision N [--key KEY] [--remote-key-id N]
      [--scope account|key] [--page N] [--page-size N]
  provider models --id ID --revision N [--key KEY]  Fetch enabled key directories and group rates
  provider workspace --id ID           Keys, health, mappings and usage
  provider delete|rename|copy --id ID --revision N [--target ID] [--approve] [--copy-keys]
  provider key-put --id ID --revision N --target KEY [--secret-stdin] [--disabled] [--protocol PROTOCOL]
  provider key-model-protocol --id ID --revision N --target KEY --model MODEL [--protocol PROTOCOL|inherit]
  provider key-delete|key-reset --id ID --revision N --target KEY --approve
  provider strategy --id ID --revision N --strategy round-robin|priority
  provider test --id ID --revision N --model MODEL [--target KEY] --approve
  provider model-put|model-delete --id ID --revision N --target ALIAS [--model MODEL] [--model-revision N] [--multiplier N] [--strategy round-robin|priority] [--tools] [--streaming] [--approve]
  provider price-put --id ID --revision N --target ALIAS --input-price USD --output-price USD [--cache-read-price USD] [--cache-write-price USD]
  provider price-import --id ID --revision N --target ALIAS --catalog-provider ID --model MODEL
  logs list|follow|export [--kind requests|system] [--since 1h] [--limit 50]
      [--cursor N] [--until N] [--status RESULT] [--source SOURCE] [--provider ID]
      [--model MODEL] [--key ID] [--http CODE] [--code CODE] [--stage STAGE]
      [--level LEVEL] [--search PREFIX]
  logs detail REQUEST_ID | logs settings
  logs configure [--days 7] [--system-days 7] [--max-requests 50000]
      [--request-mib 256] [--system-mib 32]
  usage                                Persistent per-provider/model totals
  provider add|put --id ID --base-url URL [--base-url URL ...] [--homepage-url URL]
                [--secret-stdin] [--revision N] [--protocol responses|chat-completions|anthropic]
                [--timeout 120] [--max-concurrent 4] [--disabled]
  model list
  model add|put --id ID --candidate PROVIDER:MODEL [--candidate ...]
               [--display-name NAME] [--tools] [--streaming] [--revision N]
               [--codex-catalog FILE --codex-model SOURCE --codex-version X.Y.Z]
  access grant --label LABEL --scope manage|gateway [--ttl 24h] --show-once
  access list | access revoke ID      Local-owner authorization operations
  agent discover                     Read local targets and managed workspace
  agent enroll --target ID --model MODEL --approve [--revision N]
               [--template ALIAS:LOCAL_SOURCE ...] (empty source excludes alias)
  agent add --id ID --adapter codex --codex-home ABS_DIR --model MODEL --approve
            [--model-scope all-compatible|single] [--anchor-executable ABS_PATH]
                                            Register; apply explicitly or on next serve
  agent list | agent inspect ID | agent plan ID
  agent apply ID --revision N --fingerprint SHA256
  agent sync|restore|enable|disable ID --revision N
  agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|single
  agent token ID                     Credential-helper stdout (local, raw secret)
  recover [--id ID]                   Conditional recovery with core stopped
  update config | configure [--channel alpha|beta|rc|stable|dev]
                [--manifest HTTPS_URL | --default-source]
  update check|download [--channel CHANNEL] [--manifest HTTPS_URL]
                [--product desktop --current-version VERSION]
  update apply --approve             Replace local CLI; core must be stopped
  version
Normal output is JSON; logs export is JSONL, logs follow emits repeated JSON pages, agent token is raw secret.
Diagnostics and service logs go to stderr.
Provider IDs accept 1..128 Unicode letters (including Chinese), decimal digits, and ._-; start with a letter or digit.
Secrets may enter through stdin only. No secret-bearing command-line arguments.
init/serve/access/context/recover/update/agent token operate locally. Business commands use the selected context.
Codex base-config/model-catalog management is available; actual Agent connection remains unverified.
New agent add defaults to all-compatible; --model is the default, not the permission boundary.
Catalog metadata must match the exact Codex version and actual upstream capabilities.
Enroll reads the selected local models_cache.json; no executable or network probing.
Restart Codex after configuration changes. Other detected agents remain unsupported.
Remote Codex app-server uses the remote base config; SSH connection management,
other Agent adapters and named profiles are not implemented.
Protocol conversion supports stateless text, function tools and streaming; unsupported content is rejected.
`

type App struct {
	In       io.Reader
	Out, Err io.Writer
}

func (a App) output(v any) error { return json.NewEncoder(a.Out).Encode(v) }
func (a App) flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(a.Err)
	return f
}
func parse(f *flag.FlagSet, args []string) error {
	if err := f.Parse(args); err != nil {
		return err
	}
	if len(f.Args()) != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}
func globals(args []string) (dir, target string, rest []string, err error) {
	dir, err = state.DefaultDir()
	if err != nil {
		return
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			continue
		case arg == "--data-dir" || arg == "--context":
			if i+1 >= len(args) {
				err = errors.New("global option requires a value")
				return
			}
			i++
			if arg == "--data-dir" {
				dir = args[i]
			} else {
				target = args[i]
			}
		case strings.HasPrefix(arg, "--data-dir="):
			dir = strings.TrimPrefix(arg, "--data-dir=")
		case strings.HasPrefix(arg, "--context="):
			target = strings.TrimPrefix(arg, "--context=")
		default:
			rest = append(rest, arg)
		}
	}
	if dir == "" {
		err = errors.New("data directory cannot be empty")
	}
	return
}
func (a App) Run(ctx context.Context, args []string) error {
	dir, target, args, err := globals(args)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err = io.WriteString(a.Out, help)
		return err
	}
	if args[0] == "version" {
		return a.output(map[string]string{"version": core.Version})
	}
	switch args[0] {
	case "update":
		if target != "" {
			return errors.New("update is local-only; omit --context")
		}
		return a.update(ctx, dir, args[1:])
	case "init":
		if target != "" {
			return errors.New("init is local-only; do not specify --context")
		}
		f := a.flags("init")
		control := f.String("control-address", "127.0.0.1:17861", "management address")
		gateway := f.String("gateway-address", "127.0.0.1:17860", "model address")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if err := state.ValidateListen(*control); err != nil {
			return err
		}
		if err := state.ValidateListen(*gateway); err != nil {
			return err
		}
		if err := platform.SecureDir(dir); err != nil {
			return err
		}
		lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
		if err != nil {
			return err
		}
		defer lock.Close()
		store, err := state.Open(dir, true)
		if err != nil {
			return err
		}
		defer store.Close()
		cfg, err := store.Initialize(*control, *gateway)
		if err != nil {
			return err
		}
		return a.output(map[string]any{"initialized": true, "settings": cfg, "credential_protection": "current-user permissions/ACL; not encryption"})
	case "serve":
		if target != "" {
			return errors.New("serve is local-only; do not specify --context")
		}
		if len(args) != 1 {
			return errors.New("serve has no command-specific arguments")
		}
		store, err := state.Open(dir, false)
		if err != nil {
			return err
		}
		defer store.Close()
		return core.New(store, slog.New(slog.NewJSONHandler(a.Err, nil))).Run(ctx)
	}
	store, err := state.Open(dir, false)
	if err != nil {
		return err
	}
	defer store.Close()
	switch args[0] {
	case "recover":
		if target != "" {
			return errors.New("recover is local-only; omit --context")
		}
		return a.recover(store, args[1:])
	case "agent":
		if len(args) > 1 && args[1] == "token" {
			if target != "" || len(args) != 3 || !state.ValidID(args[2]) {
				return errors.New("agent token requires a local binding ID")
			}
			token, err := store.BindingToken(args[2])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(a.Out, token)
			return err
		}
	case "access":
		if target != "" {
			return errors.New("access is local-only; do not specify --context")
		}
		return a.access(store, args[1:])
	case "context":
		if target != "" {
			return errors.New("context editing is local-only; omit --context")
		}
		return a.contexts(ctx, store, args[1:])
	}
	c, err := store.Context(target)
	if err != nil {
		return err
	}
	token, err := store.OwnerToken()
	if err != nil {
		return err
	}
	cl, _, err := client.Connect(ctx, c, token)
	if err != nil {
		return err
	}
	defer cl.Close()
	switch args[0] {
	case "stop":
		f := a.flags("stop")
		approved := f.Bool("approve", false, "confirm graceful shutdown of the selected instance")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*approved {
			return errors.New("stop requires --approve; closes listeners and restores managed configuration")
		}
		if err := cl.Stop(ctx); err != nil {
			return err
		}
		return a.output(map[string]any{"accepted": true, "instance_id": c.InstanceID, "shutdown_complete": false})
	case "status":
		if len(args) != 1 {
			return errors.New("status has no command-specific arguments")
		}
		v, err := cl.Status(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	case "agent":
		return a.agents(ctx, cl, args[1:])
	case "usage":
		if len(args) != 1 {
			return errors.New("usage takes no arguments")
		}
		v, err := cl.Usage(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	case "logs":
		return a.logs(ctx, cl, args[1:])
	case "provider":
		return a.providers(ctx, cl, args[1:])
	case "model":
		return a.models(ctx, cl, args[1:])
	default:
		return errors.New("unknown command; run anchor help")
	}
}
func (a App) secret() (string, error) {
	b, err := io.ReadAll(io.LimitReader(a.In, 8193))
	if err != nil || len(b) > 8192 {
		return "", errors.New("secret stdin read failed or exceeded 8192 bytes")
	}
	return strings.TrimSpace(string(b)), nil
}
func (a App) access(s *state.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("access requires grant, list or revoke")
	}
	switch args[0] {
	case "grant":
		f := a.flags("access grant")
		label := f.String("label", "", "client label")
		scope := f.String("scope", "", "manage or gateway")
		ttl := f.Duration("ttl", 0, "expiry duration; 0 is nonexpiring")
		show := f.Bool("show-once", false, "explicitly disclose new token")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*show {
			return errors.New("explicit --show-once is required; output contains a secret")
		}
		grant, token, err := s.Grant(*label, *scope, *ttl)
		if err != nil {
			return err
		}
		return a.output(map[string]any{"access": grant, "token": token})
	case "list":
		if len(args) != 1 {
			return errors.New("access list takes no arguments")
		}
		grants, err := s.AccessList()
		if err != nil {
			return err
		}
		return a.output(grants)
	case "revoke":
		if len(args) != 2 {
			return errors.New("access revoke requires an access ID")
		}
		if err := s.Revoke(args[1]); err != nil {
			return err
		}
		return a.output(map[string]bool{"revoked": true})
	default:
		return errors.New("unknown access command")
	}
}
func (a App) contexts(ctx context.Context, s *state.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("context requires add, use, list, inspect or test")
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			return errors.New("context add requires a name")
		}
		f := a.flags("context add")
		local := f.Bool("local", false, "local core")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if !*local {
			return errors.New("only --local contexts are implemented in this milestone")
		}
		cfg, err := s.Settings()
		if err != nil {
			return err
		}
		c := state.Context{Name: args[1], Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}
		token, err := s.OwnerToken()
		if err != nil {
			return err
		}
		cl, _, err := client.Connect(ctx, c, token)
		if err != nil {
			return err
		}
		cl.Close()
		if err := s.PutContext(c); err != nil {
			return err
		}
		return a.output(c)
	case "use":
		if len(args) != 2 {
			return errors.New("context use requires a name")
		}
		if err := s.UseContext(args[1]); err != nil {
			return err
		}
		return a.output(map[string]string{"current_context": args[1]})
	case "list":
		if len(args) != 1 {
			return errors.New("context list takes no arguments")
		}
		v, err := s.Contexts()
		if err != nil {
			return err
		}
		return a.output(v)
	case "inspect", "test":
		if len(args) != 2 {
			return errors.New("context inspect/test requires a name")
		}
		c, err := s.Context(args[1])
		if err != nil {
			return err
		}
		if args[0] == "inspect" {
			return a.output(c)
		}
		token, err := s.OwnerToken()
		if err != nil {
			return err
		}
		cl, i, err := client.Connect(ctx, c, token)
		if err != nil {
			return err
		}
		defer cl.Close()
		return a.output(i)
	default:
		return errors.New("unknown context command")
	}
}
func (a App) providers(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("provider requires list, models, add or put")
	}
	switch args[0] {
	case "platform", "platform-put", "detect-platform", "balance-summary", "balance", "key-quota", "usage-records", "effective-rate":
		return a.providerPlatform(ctx, c, args)
	}
	if args[0] == "list" {
		if len(args) != 1 {
			return errors.New("provider list takes no arguments")
		}
		v, err := c.Providers(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	}
	if args[0] == "models" {
		f := a.flags("provider models")
		key := f.String("key", "", "enabled saved key ID; omission queries all enabled keys and merges models with group rates")
		id := f.String("id", "", "saved provider ID")
		revision := f.Int64("revision", 0, "expected current provider revision (required)")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !state.ValidProviderID(*id) || *revision < 1 {
			return errors.New("provider models requires a valid --id and current --revision")
		}
		result, err := c.DiscoverProviderModels(ctx, catalog.DiscoverProviderModels{ProviderID: *id, ExpectedRevision: *revision, KeyID: *key})
		if err != nil {
			return err
		}
		return a.output(result)
	}
	if args[0] == "workspace" {
		f := a.flags("provider workspace")
		id := f.String("id", "", "provider ID")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		v, err := c.ProviderWorkspace(ctx, *id)
		if err != nil {
			return err
		}
		return a.output(v)
	}
	if args[0] != "add" && args[0] != "put" {
		return a.providerFeature(ctx, c, args)
	}
	f := a.flags("provider " + args[0])
	id := f.String("id", "", "provider ID: 1..128 Unicode letters (including Chinese), digits, or ._-; start with a letter or digit")
	var bases repeated
	f.Var(&bases, "base-url", "request API base including optional /v1, not /responses; repeat for round-robin load balancing (max 16)")
	homepage := f.String("homepage-url", "", "optional provider homepage, display only")
	protocol := f.String("protocol", "", "default protocol for a newly created default Key; empty keeps the stored value or uses responses")
	timeout := f.Int("timeout", 120, "total request timeout in seconds")
	max := f.Int("max-concurrent", 4, "provider concurrency")
	disabled := f.Bool("disabled", false, "disable provider")
	rev := f.Int64("revision", 0, "expected current revision")
	stdin := f.Bool("secret-stdin", false, "read API key from stdin")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "add" && *rev != 0 {
		return errors.New("add requires revision 0")
	}
	if args[0] == "put" && *rev < 1 {
		return errors.New("put requires the current --revision")
	}
	p := state.Provider{ID: *id, HomepageURL: *homepage, BaseURLs: append([]string(nil), bases...), Protocol: *protocol, Enabled: !*disabled, TimeoutSeconds: *timeout, MaxConcurrent: *max}
	if err := p.Validate(); err != nil {
		return err
	}
	var secret *string
	if *stdin {
		v, err := a.secret()
		if err != nil {
			return err
		}
		secret = &v
	}
	out, err := c.PutProvider(ctx, p, *rev, secret)
	if err != nil {
		return err
	}
	return a.output(out)
}

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }
func (a App) models(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("model requires list, add or put")
	}
	if args[0] == "list" {
		if len(args) != 1 {
			return errors.New("model list takes no arguments")
		}
		v, err := c.Models(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	}
	if args[0] != "add" && args[0] != "put" {
		return errors.New("unknown model command")
	}
	f := a.flags("model " + args[0])
	id := f.String("id", "", "public model ID")
	display := f.String("display-name", "", "display name")
	tools := f.Bool("tools", false, "advertise function tools")
	stream := f.Bool("streaming", false, "advertise SSE")
	rev := f.Int64("revision", 0, "expected current revision")
	catalog := f.String("codex-catalog", "", "local version-matched Codex catalog JSON (max 1 MiB)")
	source := f.String("codex-model", "", "source catalog model; verify upstream capabilities match")
	version := f.String("codex-version", "", "exact source/target Codex version X.Y.Z")
	var candidates repeated
	f.Var(&candidates, "candidate", "ordered provider:model mapping")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "add" && *rev != 0 {
		return errors.New("add requires revision 0")
	}
	if args[0] == "put" && *rev < 1 {
		return errors.New("put requires the current --revision")
	}
	m := state.Model{ID: *id, DisplayName: *display, Tools: *tools, Streaming: *stream}
	if *catalog != "" || *source != "" || *version != "" {
		if *catalog == "" || *source == "" || *version == "" {
			return errors.New("--codex-catalog, --codex-model and --codex-version must be supplied together")
		}
		file, err := os.Open(*catalog)
		if err != nil {
			return errors.New("cannot read Codex catalog")
		}
		raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if err != nil {
			return errors.New("cannot read Codex catalog")
		}
		m.Codex, err = state.ImportCodexMetadata(raw, *source, *version)
		if err != nil {
			return err
		}
	}
	for _, v := range candidates {
		provider, model, ok := strings.Cut(v, ":")
		if !ok {
			return errors.New("candidate must be provider:model")
		}
		m.Candidates = append(m.Candidates, state.Candidate{ProviderID: provider, Model: model})
	}
	if err := m.Validate(); err != nil {
		return err
	}
	out, err := c.PutModel(ctx, m, *rev)
	if err != nil {
		return err
	}
	return a.output(out)
}

// Main separates machine-readable output from diagnostic text and handles Ctrl+C via ctx.
func Main(ctx context.Context, args []string) int {
	a := App{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	if err := a.Run(ctx, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "anchor:", err)
		return 1
	}
	return 0
}

func (a App) providerFeature(ctx context.Context, c *client.Client, args []string) error {
	switch args[0] {
	case "delete", "rename", "copy", "key-put", "key-delete", "key-reset", "key-model-protocol", "strategy", "test", "model-put", "model-delete", "price-put", "price-import":
	default:
		return errors.New("unknown provider command")
	}
	f := a.flags("provider " + args[0])
	protocol := f.String("protocol", "", "key default protocol, or model override (inherit clears it)")
	inputPrice := f.Float64("input-price", -1, "USD per million input tokens")
	outputPrice := f.Float64("output-price", -1, "USD per million output tokens")
	readPrice := f.Float64("cache-read-price", -1, "USD per million cached input tokens; omitted means unknown")
	writePrice := f.Float64("cache-write-price", -1, "USD per million cache creation tokens; omitted means unknown")
	catalogProvider := f.String("catalog-provider", "", "models.dev provider ID")
	id := f.String("id", "", "saved provider ID")
	revision := f.Int64("revision", 0, "current provider revision")
	target := f.String("target", "", "new provider ID, key ID, or model alias")
	approve := f.Bool("approve", false, "confirm destructive operation or inference cost")
	copyKeys := f.Bool("copy-keys", false, "explicitly copy credentials")
	disabled := f.Bool("disabled", false, "disable key")
	stdin := f.Bool("secret-stdin", false, "read secret from stdin; omission retains existing key")
	strategy := f.String("strategy", "round-robin", "round-robin or priority")
	model := f.String("model", "", "upstream model")
	mr := f.Int64("model-revision", 0, "current model revision; zero creates or attaches a new provider")
	multiplier := f.Float64("multiplier", 0, "relative cost multiplier 0.01..1000; omitted preserves existing or defaults to 1")
	tools := f.Bool("tools", false, "advertise function tools")
	stream := f.Bool("streaming", false, "advertise streaming")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if !state.ValidProviderID(*id) || *revision < 1 {
		return errors.New("saved --id and current --revision are required")
	}
	in := catalog.Action{Protocol: *protocol, ProviderID: *id, ExpectedRevision: *revision, Action: args[0], Target: *target, Approved: *approve, CopyKeys: *copyKeys, Enabled: !*disabled, Strategy: *strategy, Model: *model, ModelRevision: *mr, Multiplier: *multiplier, Tools: *tools, Streaming: *stream}
	if args[0] == "model-put" {
		explicitStrategy := false
		f.Visit(func(flag *flag.Flag) {
			if flag.Name == "strategy" {
				explicitStrategy = true
			}
		})
		if !explicitStrategy {
			in.Strategy = ""
		}
	}
	if *stdin {
		v, err := a.secret()
		if err != nil {
			return err
		}
		in.Secret = v
	}
	in.CatalogProvider = *catalogProvider
	if args[0] == "price-put" {
		in.Price = &catalog.Price{InputUSD: *inputPrice, OutputUSD: *outputPrice, Source: "custom"}
		if *readPrice >= 0 {
			in.Price.CacheReadUSD = readPrice
		}
		if *writePrice >= 0 {
			in.Price.CacheWriteUSD = writePrice
		}
	}
	out, err := c.ProviderAction(ctx, in)
	if err != nil {
		return err
	}
	return a.output(out)
}
