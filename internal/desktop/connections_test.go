package desktop

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
	"golang.org/x/crypto/ssh"
)

// A real OpenSSH client connects to an isolated SSH server and core. This fixture
// never reads the user's SSH config/keys/known_hosts or opens a public listener.
func isolatedSSH(t *testing.T) (SSHConnectionInput, *state.Store) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("isolated OpenSSH launcher is Linux-only")
	}
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH unavailable")
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen unavailable")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "identity")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("key fixture: %v %s", err, out)
	}
	authorizedBytes, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	authorized, _, _, _, err := ssh.ParseAuthorizedKey(authorizedBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) == "fixture-password" {
			return nil, nil
		}
		return nil, fmt.Errorf("fixture authentication")
	}, PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(key.Marshal(), authorized.Marshal()) {
			return nil, nil
		}
		return nil, fmt.Errorf("fixture authentication")
	}}
	config.AddHostKey(host)
	store, err := state.Open(filepath.Join(dir, "remote-state"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	server := core.New(store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	t.Cleanup(func() { server.Close() })
	web := httptest.NewServer(server.Control())
	t.Cleanup(web.Close)
	cfg, err := store.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, _ := net.SplitHostPort(cfg.ControlAddress)
	controlPort, _ := strconv.Atoi(portText)
	// Reproduce SSH's noninteractive PATH: Anchor exists only in the remote
	// account's user-local directory, including spaces in HOME.
	remoteHome := filepath.Join(dir, "remote home")
	remoteBin := filepath.Join(remoteHome, ".local", "bin")
	if err := os.MkdirAll(remoteBin, 0700); err != nil {
		t.Fatal(err)
	}
	remoteScript := "#!/bin/sh\nif [ \"$*\" = 'context bootstrap --help' ]; then exit 0; fi\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(remoteBin, "anchor"), []byte(remoteScript), 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sockets []net.Conn
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range sockets {
			c.Close()
		}
	})
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			sockets = append(sockets, raw)
			mu.Unlock()
			go func() {
				defer raw.Close()
				session, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer session.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					switch channel.ChannelType() {
					case "session":
						ch, reqs, err := channel.Accept()
						if err != nil {
							continue
						}
						go func() {
							defer ch.Close()
							for request := range reqs {
								if request.Type != "exec" {
									request.Reply(false, nil)
									continue
								}
								var command struct{ Command string }
								_ = ssh.Unmarshal(request.Payload, &command)
								probe := exec.Command("/bin/sh", "-c", command.Command)
								probe.Env = []string{"HOME=" + remoteHome, "PATH=/usr/bin:/bin"}
								args, probeErr := probe.Output()
								if probeErr != nil || string(args) != fmt.Sprintf("context\nbootstrap\n--control-port\n%d\n--show-once\n", controlPort) {
									request.Reply(false, nil)
									return
								}
								request.Reply(true, nil)
								access, token, err := store.Grant("fixture", "manage", 0)
								if err != nil {
									return
								}
								_ = json.NewEncoder(ch).Encode(connection.Bootstrap{Settings: cfg, Access: access, Token: token})
								_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							}
						}()
					case "direct-tcpip":
						var forward struct {
							Host       string
							Port       uint32
							Origin     string
							OriginPort uint32
						}
						_ = ssh.Unmarshal(channel.ExtraData(), &forward)
						if forward.Host != "127.0.0.1" || int(forward.Port) != controlPort {
							channel.Reject(ssh.Prohibited, "fixture target")
							continue
						}
						target, err := net.Dial("tcp", cfg.ControlAddress)
						if err != nil {
							channel.Reject(ssh.ConnectionFailed, "fixture offline")
							continue
						}
						ch, reqs, err := channel.Accept()
						if err != nil {
							target.Close()
							continue
						}
						go ssh.DiscardRequests(reqs)
						go func() {
							defer ch.Close()
							defer target.Close()
							go func() { _, _ = io.Copy(target, ch); target.Close() }()
							_, _ = io.Copy(ch, target)
						}()
					default:
						channel.Reject(ssh.UnknownChannelType, "fixture channel")
					}
				}
			}()
		}
	}()
	// Options precede Anchor's args and isolate all key/config persistence.
	wrapper := filepath.Join(dir, "ssh")
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
	script := "#!/bin/sh\nexec " + quote(realSSH) + " -F /dev/null -o UserKnownHostsFile=" + quote(filepath.Join(dir, "known_hosts")) + " -o GlobalKnownHostsFile=/dev/null -o IdentitiesOnly=yes -i " + quote(key) + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return SSHConnectionInput{Name: "fixture-server", Target: "fixture@127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, RemoteExecutable: "anchor", RemoteControlPort: controlPort, TrustNewHost: true}, store
}

func TestSSHDesktopEnrollmentSelectionAndIsolation(t *testing.T) {
	input, remote := isolatedSSH(t)
	local, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	_, err = local.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(Options{Dir: local.Dir})
	defer s.Close()
	b := NewBridge(s)
	// First contact is rejected until the UI explicitly permits first-host trust.
	untrusted := input
	untrusted.TrustNewHost = false
	if r := b.ConnectSSH(untrusted); r.Problem == nil || r.Problem.Code != "ssh_host_key" {
		t.Fatalf("first contact: %+v", r.Problem)
	}
	result := b.ConnectSSH(input)
	if result.Problem != nil || result.Data == nil || result.Data.Transport != "ssh" || result.Data.Source != "remote" {
		t.Fatalf("enroll: %+v", result)
	}
	remoteCfg, _ := remote.Settings()
	if result.Data.InstanceID != remoteCfg.InstanceID {
		t.Fatal("wrong instance")
	}
	profiles := b.Connections()
	if profiles.Problem != nil || len(profiles.Data) != 2 || !profiles.Data[1].Active {
		t.Fatalf("profiles: %+v", profiles)
	}
	if r := b.Status(); r.Problem != nil || r.Data.InstanceID != remoteCfg.InstanceID {
		t.Fatalf("status: %+v", r)
	}
	token, _ := local.ContextCredential(input.Name)
	encoded, _ := json.Marshal(profiles)
	if token == "" || bytes.Contains(encoded, []byte(token)) {
		t.Fatal("credential persistence/disclosure")
	}
	before := s.remote
	if r := b.UseConnection(SelectConnection{Name: "absent"}); r.Problem == nil || s.remote != before {
		t.Fatal("failed switch replaced current session")
	}
	if r := b.ConnectSSH(input); r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatal("duplicate accepted")
	}
	profile, _ := local.Context(input.Name)
	if err := remote.Revoke(profile.AccessID); err != nil {
		t.Fatal(err)
	}
	if r := b.UseConnection(SelectConnection{Name: input.Name}); r.Problem != nil {
		t.Fatalf("reauthorize: %+v", r.Problem)
	}
	freshToken, _ := local.ContextCredential(input.Name)
	if freshToken == token {
		t.Fatal("expired/revoked grant was not refreshed")
	}
	s.Close()
	// Desktop preference survives without selecting the CLI's context.
	fresh := NewSession(Options{Dir: local.Dir})
	defer fresh.Close()
	if r := NewBridge(fresh).Start(); r.Problem != nil || r.Data.ProfileName != input.Name {
		t.Fatalf("reopen: %+v", r)
	}
	if _, err := local.Context(""); err == nil {
		t.Fatal("desktop changed CLI preference")
	}
	fresh.Close()
	live, err := connection.Open(t.Context(), remote, state.Context{Name: "local", Endpoint: "http://" + remoteCfg.ControlAddress, InstanceID: remoteCfg.InstanceID, CredentialRef: "local-owner"})
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	cl, _, err := live.Connect(t.Context())
	if err != nil {
		t.Fatal("closing UI stopped remote core", err)
	}
	cl.Close()
}

func TestConnectionSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/connections.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract string                   `json:"contract"`
		Input    SSHConnectionInput       `json:"input"`
		Profiles ConnectionProfilesResult `json:"profiles"`
		Session  SessionResult            `json:"session"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Contract != Contract || fixture.Input.TrustNewHost || fixture.Session.Data.Transport != "ssh" || len(fixture.Profiles.Data) != 2 {
		t.Fatal("connection fixture drift")
	}
}

func TestPasswordDesktopConnectionPinsHostAndForgetsSecret(t *testing.T) {
	input, remote := isolatedSSH(t)
	input.Name = "password-server"
	input.Password = "wrong-password"
	local, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	_, _ = local.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	s := NewSession(Options{Dir: local.Dir})
	defer s.Close()
	b := NewBridge(s)
	if r := b.ConnectSSH(input); r.Problem == nil || r.Problem.Code != "ssh_auth" {
		t.Fatalf("wrong password: %+v", r.Problem)
	}
	input.Password = "fixture-password"
	r := b.ConnectSSH(input)
	if r.Problem != nil {
		t.Fatalf("password enroll: %+v", r.Problem)
	}
	profile, _ := local.Context(input.Name)
	if profile.SSHAuth != "password" || profile.SSHHostKey == "" {
		t.Fatal("host pin not saved")
	}
	raw, _ := json.Marshal(profile)
	if bytes.Contains(raw, []byte(input.Password)) {
		t.Fatal("password persisted")
	}
	if r := b.UseConnection(SelectConnection{Name: input.Name}); r.Problem != nil {
		t.Fatalf("active password session reuse: %+v", r.Problem)
	}
	s.Close()
	fresh := NewSession(Options{Dir: local.Dir})
	defer fresh.Close()
	fb := NewBridge(fresh)
	if r := fb.Start(); r.Problem == nil || r.Problem.Code != "ssh_auth" {
		t.Fatalf("password survived restart: %+v", r.Problem)
	}
	if r := fb.UseConnection(SelectConnection{Name: input.Name, Password: input.Password}); r.Problem != nil {
		t.Fatalf("password reconnect: %+v", r.Problem)
	}
	profile, _ = local.Context(input.Name)
	if err := remote.Revoke(profile.AccessID); err != nil {
		t.Fatal(err)
	}
	if r := fb.UseConnection(SelectConnection{Name: input.Name, Password: input.Password}); r.Problem != nil {
		t.Fatalf("password reauthorize: %+v", r.Problem)
	}
	fresh.Close()
	profile.SSHHostKey = "changed-host-key"
	if err := local.PutContextWithCredential(profile, "fixture-token"); err != nil {
		t.Fatal(err)
	}
	last := NewSession(Options{Dir: local.Dir})
	defer last.Close()
	if r := NewBridge(last).UseConnection(SelectConnection{Name: input.Name, Password: input.Password}); r.Problem == nil || r.Problem.Code != "ssh_host_key" {
		t.Fatalf("changed host: %+v", r.Problem)
	}
}
