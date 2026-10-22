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
	"strconv"
	"strings"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

const help = `Anchor — headless model gateway (development milestone)
Usage: anchor [--data-dir DIR] [--context NAME] [--ssh-password-stdin] COMMAND [--json]
  init [--control-address 127.0.0.1:17861] [--gateway-address 127.0.0.1:17860]
  serve                              Run the local foreground core
      Inference: one ordinary attempt per channel with safe key/candidate failover; no replay after output or unknown execution.
      Codex owns request/stream retries (default 5); management writes are not retried.
  restart --approve [--timeout 30s]   Restart local core, or explicit --context SSH target; preserve configuration
      Local safety pins: [--expected-instance ID] [--control-port N] (not with --context).
  context add NAME --local           Authenticate and bind a local instance
  context add NAME --ssh TARGET [--ssh-port N] [--identity-file PATH]
      [--remote-anchor anchor] [--remote-control-port 17861] [--trust-new-host]
  context bootstrap [--control-port 17861] --show-once  SSH account bootstrap (local only)
  context use NAME | context list | context inspect NAME | context test NAME
  settings key-cooldown [--enabled=true|false] [--failure-threshold 1..100] [--error-window-seconds 1..3600]
      [--cooldown-seconds 1..3600] [--rate-limit-seconds 1..3600] [--revision N]
  status                             Query selected context
  stop --approve [--for-restart]      Stop core; optional planned-restart handoff
  provider list
  provider import-ccs [--db PATH] [--approve --fingerprint SHA256]
      Preview all CC Switch platforms on the selected instance; apply imports and enables compatible providers atomically.
  provider platform --id ID             Read platform configuration (redacted)
  provider platform-put --id ID --revision N [--platform auto|sub2api|newapi|oneapi]
      [--site-url URL] [--user-id N] [--user-token-stdin | --clear-user-token]
  provider detect-platform --id ID --revision N [--key KEY]
      Manually re-detect and save the platform; normal reads reuse the first cached result.
  provider balance-summary --id ID --revision N [--key KEY]
  provider balance|key-quota|effective-rate --id ID --revision N [--key KEY] [--remote-key-id N]
      NewAPI balance / balance-summary prefer a saved system access token for account balance.
      Without one, read-only token usage can expose a permitted wallet; otherwise billing or Key quota.
      NewAPI raw quota uses site quota_per_unit and display currency; user authorization failures are preserved.
  provider usage-records --id ID --revision N [--key KEY] [--remote-key-id N]
      [--scope account|key] [--page N] [--page-size N]
  provider models --id ID --revision N [--key KEY]  Fetch enabled key directories and group rates
  provider workspace --id ID           Keys, health, mappings and usage
      provider list has_secret includes named Keys, not only the default Key.
  provider delete|rename|copy --id ID --revision N [--target ID] [--approve] [--copy-keys]
      delete removes this provider's mapping sources; other providers keep shared models.
  provider key-put --id ID --revision N --target KEY [--secret-stdin] [--disabled] [--protocol PROTOCOL]
  provider key-model-protocol --id ID --revision N --target KEY --model MODEL [--protocol PROTOCOL|inherit]
  provider key-delete|key-reset --id ID --revision N --target KEY --approve
      key-delete removes only this Key's mapping sources; other Keys and providers keep shared models.
      Removing the last source deletes the model; enabled Binding defaults still block deletion.
  provider strategy --id ID --revision N --strategy round-robin|priority
  provider chat --id ID --revision N --key KEY --model MODEL [--reasoning-effort LEVEL] --approve
      Read a JSON user/assistant message array from stdin; print the final transient reply.
  provider test --id ID --revision N --model MODEL [--target KEY] --approve
      HTML at an unversioned base permits one /v1 fallback; still HTML returns unexpected_html.
  provider model-put|model-delete --id ID --revision N --target ALIAS [--model MODEL] [--model-revision N] [--multiplier N] [--strategy RULE] [--source-key ID | --all-keys] [--speed-variant fast|ultrafast ...] [--service-tier TIER] [--tools] [--streaming] [--approve]
  provider price-put --id ID --revision N --target ALIAS --input-price USD --output-price USD [--cache-read-price USD] [--cache-write-price USD]
  catalog providers [--refresh]
  catalog models --provider ID [--search TEXT --limit 50 --refresh]
  catalog model --provider ID --model MODEL [--refresh]
  catalog estimate --provider ID --model MODEL [--input-tokens N --output-tokens N]
                   [--cache-read-tokens N --cache-write-tokens N --reasoning-tokens N --refresh]
  provider price-import --id ID --revision N --target ALIAS --catalog-provider ID --model MODEL
  logs suggestions [--field search|provider|model|key|http|code|stage|agent|binding|project|session] [--prefix value] [--since 1h] [--limit 20] [--kind requests|system]
  logs stats [--since 1h | --from RFC3339 --to RFC3339] [--interval 1m] [--group-by agent|binding|project|session] [--include-errors] [--errors-only] [request filters]
  logs list|follow|export [--kind requests|system] [--since 1h | --from RFC3339 --to RFC3339] [--errors-only] [--limit 50]
      [--cursor N] [--until N] [--status RESULT] [--source SOURCE] [--provider ID]
      [--model MODEL] [--key ID] [--http CODE] [--code CODE] [--stage STAGE]
      [--level LEVEL] [--search PREFIX]
  Attribution filters: [--agent TYPE] [--binding ID] [--project ID] [--session ID]; explicit empty selects unknown.
  Authenticated Codex requests can use native thread IDs and local project metadata; explicit tags take precedence.
  Historical logs are not backfilled; session titles and conversation content are not collected.
  Conversion failures retain route IDs and safe conversion_target/conversion_reason evidence in detail/export.
  logs detail REQUEST_ID | logs settings
  logs configure [--days 7] [--system-days 7] [--max-requests 50000]
      [--request-mib 256] [--system-mib 32]
  usage                                Persistent per-provider/model totals
  provider add|put --id ID --base-url URL [--base-url URL ...] [--homepage-url URL]
                [--secret-stdin] [--revision N] [--protocol responses|chat-completions|anthropic]
                [--timeout 120] [--max-concurrent 4] [--cost-multiplier N] [--disabled]
  model list
  model settings --id ID [--file FILE]
  model routing --id ID --revision N --strategy RULE --candidate PROVIDER:MODEL...
  RULE: priority|round-robin|cache-priority|lowest-price|lowest-latency
  model add|put --id ID --candidate PROVIDER:MODEL [--candidate ...] [--strategy RULE] [--candidate-key INDEX:KEY] [--service-tier priority|ultrafast]
               [--display-name NAME] [--tools] [--streaming] [--revision N]
               [--codex-catalog FILE --codex-model SOURCE --codex-version X.Y.Z]
  access grant --label LABEL --scope manage|gateway [--ttl 24h] --show-once
  access list | access revoke ID      Local-owner authorization operations
  agent discover                     Read targets, mapped model availability and bindings
  agent enroll --target ID --model MODEL --approve [--revision N] [--model-scope all-mapped|all-compatible] [--rebuild]
               [--template ALIAS:LOCAL_SOURCE ...] (empty source excludes alias)
  agent add --id ID --adapter codex|claude|claude-desktop|dsh|dsh-desktop --config-root ABS_DIR --model MODEL --approve
            [--model-scope all-compatible|all-mapped|single] [--anchor-executable ABS_PATH]
                                            Register; apply explicitly or on next serve
  agent codex-settings [--retries 0..10 --revision N]  Unified retries; restart Codex after saving
  agent list | agent inspect ID | agent plan ID
  agent apply ID --revision N --fingerprint SHA256
  agent sync|restore|enable|disable ID --revision N
  agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|all-mapped|single
  proxy list                        List proxies, bindings and built-in mihomo status
  proxy put ID --name NAME --kind http|socks5|mihomo [--secret-stdin] [--node NAME] --revision N --approve
  proxy delete ID --revision N --approve
  proxy bind PROVIDER [--key KEY] --mode direct|system|inherit|proxy [--proxy ID] --revision N --approve
      direct ignores application proxies (not OS/router interception); system requires an OS/environment proxy, never falls back direct.
  skill scan | list                  Read-only Skill inventory and immutable library
  skill inspect | diff [ID]           Inspect snapshots or --source ID; see CLI docs
  skill register-project PATH        Add an explicitly approved project root
  skill import SOURCE                Copy source; requires --hash, --revision, --approve
  skill create NAME | save ID         Save a snapshot with --content-stdin
  skill link ID | update MOUNT        Explicitly mount a fixed --version
  skill unlink MOUNT | delete ID      Remove owned link / unreferenced library entry
  agent token ID                     Credential-helper stdout (local, raw secret)
  recover [--id ID]                   Conditional recovery with core stopped
  update config | configure [--channel alpha|beta|rc|stable|dev]
                [--manifest HTTPS_URL | --default-source]
  update check|download [--channel CHANNEL] [--manifest HTTPS_URL]
                [--product desktop --current-version VERSION]
  update apply --approve             Replace local CLI; core must be stopped
  update upgrade --approve           Prepare newer CLI; briefly restart and verify local core
  update desktop --approve           Linux/Windows desktop runtime update; auto-reopen running GUI
      [--channel CHANNEL] [--manifest HTTPS_URL] [--expected-instance ID] [--control-port N]
      Safety pins: [--expected-version VERSION --expected-sha256 HASH]
  --context NAME update check        Check the enrolled SSH machine's own update source
  --context NAME update upgrade --approve  Update only that SSH machine; verify version/identity
  version
Normal output is JSON; logs export is JSONL, logs follow emits repeated JSON pages, agent token is raw secret.
Logs include optional reference_cost: usd is original; adjustment stores supplier/group multipliers, final usd and token parts.
Reference prices refresh at core startup and hourly; mapping changes check the cache. New estimates require prices under 3h old.
Key group rates refresh at startup/hourly and after configuration changes; rates older than 3h stay unknown for new costs/routing.
Attempts retain reference_usd and cost_adjustment snapshots. Missing prices are omitted; unknown usage/rates stay null.
Logs retain optional known_usage {input, output, attempts} subtotals across retries; unknown root totals stay null.
Logs include safe raw service-tier evidence; fast_triggered is true/false/unknown from upstream reports, not measured speed.
Logs mark encrypted-context cleanup, removed reasoning count and recovery outcome; native Responses may retry once before output after a definite encrypted-content rejection.
Native Responses remove null, non-string or empty encrypted reasoning fields and their orphan IDs before dispatch; summaries and nonempty ciphertext are preserved.
Stream inspection shares the upstream deadline and logs a safe inspection outcome, including when no cleanup occurs.
Diagnostics and service logs go to stderr.
Agent failures distinguish revision conflicts, external configuration edits, unsafe configuration, unavailable default models and invalid catalogs; unclassified failures remain agent_operation_failed.
Model deletion protects enabled Binding defaults; disabled Bindings do not block deletion.
Agent enable requires an existing default model; use agent set-models to replace a removed default.
Provider, Key, public model and Context names accept 1..128 Unicode letters (including Chinese), decimal digits, and ._-; start with a letter or digit.
Agent binding names use the same characters, with a 64 UTF-8 byte limit.
Secrets may enter through stdin only. No secret-bearing command-line arguments.
init/serve/access/context/recover/agent token operate locally. update is local except explicit --context check/upgrade. restart is local unless --context explicitly selects SSH. Business commands use the selected context.
Codex, Claude Code CLI (claude), Claude Desktop (claude-desktop) and independent DSH Web (dsh) / Desktop (dsh-desktop) profile management are available; runtime connection remains unverified.
Claude CLI modelPicker lists mapped aliases on Claude Code >=2.1.242; older clients may show a partial list.
Claude Desktop requires an applied local configLibrary profile; it can affect Chat/Cowork/Code. Organization/bootstrap profiles are not managed. Fully quit and relaunch Desktop.
DSH Web/Desktop sync mapped display names, vision/input, context and supported off/minimal/low/medium/high/xhigh/max efforts.
DSH defaults use the binding default model; native none/default/null and toggle/budget reasoning are not relabelled.
DSH maxTokens exports explicit default output tokens only, not the output capacity alone.
Codex CLI enrollment defaults to all-compatible; Claude/DSH and desktop enrollment use all-mapped.
All-mapped reads model mappings with enabled providers, without Codex metadata/tools/streaming filters.
Mapping/provider changes refresh applied Agent model lists without overwriting external changes.
agent sync explicitly reclaims approved fields and catalogs; external values become recovery baselines.
agent enroll updates the selected default and latest mappings, reclaiming approved fields when the gateway is ready.
Codex enroll --rebuild requires --approve --model-scope all-mapped and no --template: back up the entire config, then replace it with a clean configuration; other settings remain only in the backup.
Unset model context stays within the first models.dev input-price band when known; explicit limits are retained.
Long-context pricing is selected by actual request input (including cache), not the configured window.
provider price-import rejects context-tiered prices; use model-catalog model/estimate or model settings references.
Codex all-mapped uses generic Anchor coding instructions only when no operator template exists.
Codex catalogs project supported reasoning efforts only; upstream max/default/null remain stored, not relabelled.
All-compatible requires exact-version Codex metadata and the selected local models_cache.json for enrollment.
--codex-home remains a Codex-only alias for --config-root. No executable or upstream probing.
Restart Codex/Claude Code after changes; DSH defaults apply to new sessions.
Remote Codex app-server uses the remote base config. SSH contexts share the typed management API;
Gemini, OpenCode and other Agent profiles remain unsupported.
Protocol conversion supports stateless text, function tools and streaming; unsupported content is rejected.
`

type App struct {
	In          io.Reader
	Out, Err    io.Writer
	sshPassword string
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
	passwordInput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--ssh-password-stdin" {
			passwordInput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if passwordInput {
		if a.In == nil {
			return errors.New("SSH password requires stdin")
		}
		for _, arg := range args {
			if arg == "--secret-stdin" {
				return errors.New("SSH and provider secrets cannot share stdin")
			}
			if arg == "--content-stdin" {
				return errors.New("SSH password and Skill content cannot share stdin")
			}
		}
		raw, err := io.ReadAll(io.LimitReader(a.In, 8193))
		if err != nil || len(raw) > 8192 {
			return errors.New("invalid SSH password input")
		}
		a.sshPassword = strings.TrimRight(string(raw), "\r\n")
		if a.sshPassword == "" {
			return errors.New("empty SSH password input")
		}
	}
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
			return a.remoteUpdate(ctx, dir, target, args[1:])
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
	case "restart":
		return a.restart(ctx, dir, target, args[1:])
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
	if args[0] == "context" && len(args) > 1 && args[1] == "bootstrap" {
		if target != "" {
			return errors.New("context bootstrap is local-only; omit --context")
		}
		return a.bootstrap(ctx, dir, args[2:])
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
	live, err := connection.OpenWithPassword(ctx, store, c, a.sshPassword)
	if err != nil {
		return err
	}
	defer live.Close()
	cl, _, err := live.Connect(ctx)
	if err != nil {
		return err
	}
	defer cl.Close()
	switch args[0] {
	case "stop":
		f := a.flags("stop")
		approved := f.Bool("approve", false, "confirm graceful shutdown of the selected instance")
		forRestart := f.Bool("for-restart", false, "preserve managed configuration for a planned restart; does not start a replacement")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*approved {
			return errors.New("stop requires --approve; closes listeners (restores managed configuration unless --for-restart)")
		}
		if *forRestart {
			err = cl.StopForRestart(ctx)
		} else {
			err = cl.Stop(ctx)
		}
		if err != nil {
			return err
		}
		result := map[string]any{"accepted": true, "instance_id": c.InstanceID, "shutdown_complete": false}
		if *forRestart {
			result["bindings_preserved"] = true
			result["restart_required"] = true
		}
		return a.output(result)
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
	case "settings":
		return a.keyCooldownSettings(ctx, cl, args[1:])
	case "proxy":
		return a.proxies(ctx, cl, args[1:])
	case "skill":
		return a.skills(ctx, cl, args[1:])
	case "usage":
		if len(args) != 1 {
			return errors.New("usage takes no arguments")
		}
		v, err := cl.Usage(ctx)
		if err != nil {
			return err
		}
		return a.output(v)
	case "catalog":
		return a.modelCatalog(ctx, cl, args[1:])
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
		ssh := f.String("ssh", "", "SSH target or alias")
		sshPort := f.Int("ssh-port", 0, "SSH port; 0 uses SSH config")
		identity := f.String("identity-file", "", "SSH identity path")
		executable := f.String("remote-anchor", "anchor", "remote Anchor executable; default discovers PATH, ~/.local/bin and ~/bin")
		controlPort := f.Int("remote-control-port", 17861, "remote loopback control port")
		trust := f.Bool("trust-new-host", false, "accept first-seen host keys; changed keys still fail")
		if err := parse(f, args[2:]); err != nil {
			return err
		}
		if *local == (*ssh != "") {
			return errors.New("choose exactly one of --local or --ssh")
		}
		if *ssh != "" {
			if _, err := s.Context(args[1]); !errors.Is(err, state.ErrNotFound) {
				return errors.New("context already exists or cannot be read")
			}
			live, token, err := connection.EnrollConnection(ctx, state.Context{Name: args[1], SSHTarget: *ssh, SSHPort: *sshPort, SSHIdentityFile: *identity, RemoteExecutable: *executable, RemoteControlPort: *controlPort, TrustNewHost: *trust}, a.sshPassword)
			if err != nil {
				return err
			}
			profile := live.Profile
			live.Close()
			if err := s.PutContextWithCredential(profile, token); err != nil {
				return err
			}
			return a.output(profile)
		}
		if *sshPort != 0 || *identity != "" || *executable != "anchor" || *controlPort != 17861 || *trust {
			return errors.New("SSH options require --ssh")
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
		live, err := connection.OpenWithPassword(ctx, s, c, a.sshPassword)
		if err != nil {
			return err
		}
		defer live.Close()
		cl, i, err := live.Connect(ctx)
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
	case "import-ccs":
		return a.importCCSwitch(ctx, c, args[1:])
	case "chat":
		return a.providerChat(ctx, c, args[1:])
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
	protocol := f.String("protocol", "", "request protocol for anonymous requests and the default Key; omitted keeps existing protocols (new: responses)")
	timeout := f.Int("timeout", 120, "total request timeout in seconds")
	max := f.Int("max-concurrent", 4, "provider concurrency")
	cost := f.Float64("cost-multiplier", 0, "provider recharge cost factor 0.000001..1000; omitted preserves existing or defaults to 1")
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
	p := state.Provider{CostMultiplier: *cost, ID: *id, HomepageURL: *homepage, BaseURLs: append([]string(nil), bases...), Protocol: *protocol, Enabled: !*disabled, TimeoutSeconds: *timeout, MaxConcurrent: *max}
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
		return errors.New("model requires list, add, put, settings or routing")
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

	if args[0] == "settings" {
		f := a.flags("model settings")
		id := f.String("id", "", "public model ID (Chinese supported)")
		file := f.String("file", "", "SaveModelSettings JSON file (max 1 MiB); file omitted previews price-aware models.dev defaults and context_policy (no write)")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *file == "" {
			out, err := c.ModelSettings(ctx, catalog.ModelSettingsQuery{ModelID: *id})
			if err != nil {
				return err
			}
			return a.output(out)
		}
		fh, err := os.Open(*file)
		if err != nil {
			return errors.New("cannot read model settings file")
		}
		defer fh.Close()
		raw, err := io.ReadAll(io.LimitReader(fh, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			return errors.New("model settings file exceeds 1 MiB")
		}
		var in catalog.SaveModelSettings
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			return errors.New("invalid model settings JSON")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return errors.New("model settings file must contain one JSON object")
		}
		if in.ModelID != *id {
			return errors.New("--id and model_id must agree")
		}
		out, err := c.SaveModelSettings(ctx, in)
		if err != nil {
			return err
		}
		return a.output(out)
	}
	if args[0] == "routing" {
		f := a.flags("model routing")
		id := f.String("id", "", "public model ID (Chinese supported)")
		revision := f.Int64("revision", 0, "expected model revision")
		strategy := f.String("strategy", "", "priority, round-robin, cache-priority, lowest-price or lowest-latency")
		var order repeated
		f.Var(&order, "candidate", "complete ordered provider:model list")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		in := catalog.ModelRouting{ModelID: *id, ExpectedRevision: *revision, Strategy: *strategy}
		for _, raw := range order {
			provider, model, ok := strings.Cut(raw, ":")
			if !ok {
				return errors.New("candidate must be provider:model")
			}
			in.Order = append(in.Order, catalog.RouteCandidate{ProviderID: provider, Model: model})
		}
		out, err := c.ConfigureModelRouting(ctx, in)
		if err != nil {
			return err
		}
		return a.output(out)
	}
	if args[0] != "add" && args[0] != "put" {
		return errors.New("unknown model command")
	}
	f := a.flags("model " + args[0])
	serviceTier := f.String("service-tier", "", "fixed upstream service tier: priority (Fast), ultrafast; empty forwards caller tier")
	id := f.String("id", "", "public model ID (Chinese supported)")
	strategy := f.String("strategy", "priority", "model routing strategy")
	display := f.String("display-name", "", "display name")
	tools := f.Bool("tools", false, "advertise function tools")
	stream := f.Bool("streaming", true, "advertise SSE (disable with --streaming=false)")
	rev := f.Int64("revision", 0, "expected current revision")
	catalog := f.String("codex-catalog", "", "local version-matched Codex catalog JSON (max 1 MiB)")
	source := f.String("codex-model", "", "source catalog model; verify upstream capabilities match")
	version := f.String("codex-version", "", "exact source/target Codex version X.Y.Z")
	var candidates, candidateKeys repeated
	f.Var(&candidateKeys, "candidate-key", "1-based candidate index:key ID; repeat for allowed source Keys")
	f.Var(&candidates, "candidate", "ordered provider:model mapping; creation imports unambiguous exact models.dev defaults and reference prices")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "add" && *rev != 0 {
		return errors.New("add requires revision 0")
	}
	if args[0] == "put" && *rev < 1 {
		return errors.New("put requires the current --revision")
	}
	m := state.Model{ServiceTier: *serviceTier, ID: *id, Strategy: *strategy, DisplayName: *display, Tools: *tools, Streaming: *stream}
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
	for _, raw := range candidateKeys {
		index, key, ok := strings.Cut(raw, ":")
		n, err := strconv.Atoi(index)
		if !ok || err != nil || n < 1 || n > len(m.Candidates) {
			return errors.New("candidate-key must be INDEX:KEY (1-based candidate index)")
		}
		m.Candidates[n-1].KeyIDs = append(m.Candidates[n-1].KeyIDs, key)
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
	if len(args) == 1 && args[0] == "--runtime-version" {
		fmt.Printf("{\"version\":%q}\n", core.Version)
		return 0
	}
	if dir, _, _, err := globals(args); err == nil {
		forwarded, code, err := updater.ForwardRuntime(ctx, dir, "cli", core.Version, args)
		if err != nil {
			fmt.Fprintln(os.Stderr, "anchor: runtime activation failed:", err)
			return 1
		}
		if forwarded {
			return code
		}
	}

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
	target := f.String("target", "", "new provider ID, key ID, or model alias (Chinese supported)")
	approve := f.Bool("approve", false, "confirm destructive operation or inference cost")
	copyKeys := f.Bool("copy-keys", false, "explicitly copy credentials")
	disabled := f.Bool("disabled", false, "disable key")
	stdin := f.Bool("secret-stdin", false, "read secret from stdin; omission retains existing key")
	strategy := f.String("strategy", "round-robin", "Key or model routing strategy")
	var speedVariants repeated
	f.Var(&speedVariants, "speed-variant", "atomically create an independent fast or ultrafast model sibling; repeat; model-put only")
	serviceTier := f.String("service-tier", "", "fixed tier: priority, ultrafast, or empty to forward caller tier; model-put only; omit preserves")
	var sourceKeys repeated
	f.Var(&sourceKeys, "source-key", "allowed source Key ID for model-put; repeat; omit to preserve")
	allKeys := f.Bool("all-keys", false, "clear source Key restriction on model-put")
	model := f.String("model", "", "upstream model")
	mr := f.Int64("model-revision", 0, "current model revision; zero creates or attaches; new aliases import exact models.dev defaults when unambiguous")
	multiplier := f.Float64("multiplier", 0, "reserved legacy model multiplier 0.01..1000; not used for routing")
	tools := f.Bool("tools", false, "advertise function tools")
	stream := f.Bool("streaming", true, "advertise streaming (disable with --streaming=false)")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if !state.ValidProviderID(*id) || *revision < 1 {
		return errors.New("saved --id and current --revision are required")
	}
	in := catalog.Action{Protocol: *protocol, ProviderID: *id, ExpectedRevision: *revision, Action: args[0], Target: *target, Approved: *approve, CopyKeys: *copyKeys, Enabled: !*disabled, Strategy: *strategy, Model: *model, ModelRevision: *mr, Multiplier: *multiplier, Tools: *tools, Streaming: *stream}
	if args[0] != "model-put" {
		explicitTier := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "service-tier" {
				explicitTier = true
			}
		})
		if len(speedVariants) > 0 || explicitTier {
			return errors.New("speed options require provider model-put")
		}
	}
	if args[0] == "model-put" {
		in.SpeedVariants = speedVariants
		f.Visit(func(v *flag.Flag) {
			if v.Name == "service-tier" {
				in.ServiceTier = serviceTier
			}
		})
		if *allKeys && len(sourceKeys) > 0 {
			return errors.New("--all-keys conflicts with --source-key")
		}
		if *allKeys {
			in.KeyIDs = []string{}
		} else if len(sourceKeys) > 0 {
			in.KeyIDs = append([]string{}, sourceKeys...)
		}
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
