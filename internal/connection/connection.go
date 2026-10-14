// Package connection owns management transports shared by CLI and desktop.
package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

type Error struct{ Code string }

func (e *Error) Error() string  { return "connection failed (" + e.Code + ")" }
func failure(code string) error { return &Error{Code: code} }

type Bootstrap struct {
	Settings state.Settings `json:"settings"`
	Access   state.Access   `json:"access"`
	Token    string         `json:"token"`
}

// Live owns only its tunnel. Closing never stops the target core.
type Live struct {
	Profile  state.Context
	Endpoint string
	token    string
	mu       sync.Mutex
	streams  map[*sshStream]struct{}
	closed   bool
	lastCode string
	native   *ssh.Client
}

func (l *Live) Close() {
	l.mu.Lock()
	l.closed = true
	streams := make([]*sshStream, 0, len(l.streams))
	for stream := range l.streams {
		streams = append(streams, stream)
	}
	l.mu.Unlock()
	if l.native != nil {
		_ = l.native.Close()
	}
	for _, stream := range streams {
		stream.Close()
	}
}
func (l *Live) Connect(ctx context.Context) (*client.Client, client.Instance, error) {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed {
		return nil, client.Instance{}, failure("tunnel_failed")
	}
	c := state.Context{Name: l.Profile.Name, Endpoint: l.Endpoint, InstanceID: l.Profile.InstanceID, CredentialRef: "local-owner"}
	if l.Profile.Transport != "ssh" {
		return client.Connect(ctx, c, l.token)
	}
	cl, info, err := client.ConnectWithDialer(ctx, c, l.token, l.dial)
	if err != nil {
		var request *client.RequestError
		if errors.As(err, &request) && request.Code == "offline" {
			l.mu.Lock()
			code := l.lastCode
			if code == "" {
				code = "tunnel_failed"
			}
			for stream := range l.streams {
				var ssh *Error
				if errors.As(sshError(stream.stderr.String(), code), &ssh) {
					code = ssh.Code
				}
			}
			l.mu.Unlock()
			return nil, info, failure(code)
		}
	}
	return cl, info, err
}

// Each HTTP connection uses OpenSSH's stdio forward. No management listener or
// temporary port is exposed; inherited forwards remain cleared. net.Pipe gives
// HTTP real deadline/cancellation semantics while process pipes carry SSH data.
type sshStream struct {
	net.Conn
	process *exec.Cmd
	exited  chan struct{}
	stderr  *boundedBuffer
	once    sync.Once
	owner   *Live
}

func (s *sshStream) Close() error {
	var err error
	s.once.Do(func() {
		err = s.Conn.Close()
		_ = s.process.Process.Kill()
		<-s.exited
		s.owner.mu.Lock()
		delete(s.owner.streams, s)
		var ssh *Error
		if errors.As(sshError(s.stderr.String(), "tunnel_failed"), &ssh) {
			s.owner.lastCode = ssh.Code
		}
		s.owner.mu.Unlock()
	})
	return err
}
func (l *Live) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	if l.native != nil {
		conn, err := l.native.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Profile.RemoteControlPort)))
		if err != nil {
			return nil, failure("tunnel_failed")
		}
		return conn, nil
	}
	path, err := sshBinary()
	if err != nil {
		return nil, err
	}
	args := append(sshArgs(l.Profile), "-W", net.JoinHostPort("127.0.0.1", strconv.Itoa(l.Profile.RemoteControlPort)), "--", l.Profile.SSHTarget)
	cmd := exec.Command(path, args...)
	hideWindow(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, failure("tunnel_failed")
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, failure("tunnel_failed")
	}
	stderr := &boundedBuffer{limit: 16384}
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, failure("tunnel_failed")
	}
	front, back := net.Pipe()
	stream := &sshStream{Conn: front, process: cmd, exited: make(chan struct{}), stderr: stderr, owner: l}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		front.Close()
		back.Close()
		return nil, failure("tunnel_failed")
	}
	if l.streams == nil {
		l.streams = make(map[*sshStream]struct{})
	}
	l.streams[stream] = struct{}{}
	l.mu.Unlock()
	go func() { _, _ = io.Copy(input, back); input.Close() }()
	go func() { _, _ = io.Copy(back, output); back.Close(); _ = cmd.Wait(); close(stream.exited) }()
	// Dial cancellation must not cancel successful connections when DialContext returns.
	if ctx.Err() != nil {
		stream.Close()
		return nil, ctx.Err()
	}
	return stream, nil
}

// Buffers are bounded and stderr is never returned to callers or the UI.
type boundedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if left := b.limit - len(b.data); left > 0 {
		if len(p) > left {
			p = p[:left]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func (b *boundedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
func sshError(stderr, fallback string) error {
	text := strings.ToLower(stderr)
	switch {
	case strings.Contains(text, "host key verification failed"), strings.Contains(text, "remote host identification has changed"), strings.Contains(text, "offending"), strings.Contains(text, "no matching host key"):
		return failure("ssh_host_key")
	case strings.Contains(text, "permission denied"), strings.Contains(text, "authentication failed"), strings.Contains(text, "no supported authentication"):
		return failure("ssh_auth")
	case strings.Contains(text, "connection refused"), strings.Contains(text, "timed out"), strings.Contains(text, "could not resolve"), strings.Contains(text, "no route to host"):
		return failure("ssh_unreachable")
	default:
		return failure(fallback)
	}
}
func sshArgs(c state.Context) []string {
	policy := "yes"
	if c.TrustNewHost {
		policy = "accept-new"
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=" + policy, "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ExitOnForwardFailure=yes", "-o", "PermitLocalCommand=no", "-o", "ClearAllForwardings=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "RemoteCommand=none"}
	if c.SSHPort > 0 {
		args = append(args, "-p", strconv.Itoa(c.SSHPort))
	}
	if c.SSHIdentityFile != "" {
		args = append(args, "-i", c.SSHIdentityFile)
	}
	return args
}
func sshBinary() (string, error) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return "", failure("ssh_missing")
	}
	return path, nil
}

// Enroll invokes a single fixed command on the authenticated remote account.
// User input cannot supply shell syntax. SSH config/agent supplies authentication.
func Enroll(ctx context.Context, c state.Context) (state.Context, string, error) {
	c.Transport = "ssh"
	c.CredentialRef = "ssh-manage"
	c.InstanceID = "pending"
	c.AccessID = "pending"
	c.Endpoint = ""
	if c.RemoteExecutable == "" {
		c.RemoteExecutable = "anchor"
	}
	if c.RemoteControlPort == 0 {
		c.RemoteControlPort = 17861
	}
	if err := c.Validate(); err != nil {
		return state.Context{}, "", failure("invalid")
	}
	path, err := sshBinary()
	if err != nil {
		return state.Context{}, "", err
	}
	command := c.RemoteExecutable + " context bootstrap --control-port " + strconv.Itoa(c.RemoteControlPort) + " --show-once"
	args := append(sshArgs(c), "--", c.SSHTarget, command)
	cmd := exec.CommandContext(ctx, path, args...)
	hideWindow(cmd)
	stdout := &boundedBuffer{limit: 65537}
	stderr := &boundedBuffer{limit: 16384}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return state.Context{}, "", sshError(stderr.String(), "remote_anchor")
	}
	raw := stdout.String()
	if len(raw) > 65536 {
		return state.Context{}, "", failure("remote_anchor")
	}
	var out Bootstrap
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF || !state.ValidOpaqueID(out.Settings.InstanceID) || !state.ValidOpaqueID(out.Access.ID) || out.Access.Scope != "manage" || out.Access.Revoked || out.Token == "" || len(out.Token) > 8192 || out.Settings.ControlAddress != net.JoinHostPort("127.0.0.1", strconv.Itoa(c.RemoteControlPort)) {
		return state.Context{}, "", failure("remote_anchor")
	}
	c.InstanceID = out.Settings.InstanceID
	c.AccessID = out.Access.ID
	return c, out.Token, nil
}
func Open(ctx context.Context, s *state.Store, c state.Context) (*Live, error) {
	if c.SSHAuth == "password" {
		return nil, failure("ssh_auth")
	}
	if err := c.Validate(); err != nil {
		return nil, failure("invalid")
	}
	if c.Transport == "" || c.Transport == "local" {
		token, err := s.OwnerToken()
		if err != nil {
			return nil, err
		}
		return &Live{Profile: c, Endpoint: c.Endpoint, token: token}, nil
	}
	token, err := s.ContextCredential(c.Name)
	if err != nil {
		return nil, failure("unauthorized")
	}
	live, err := Tunnel(ctx, c, token)
	var request *client.RequestError
	if !errors.As(err, &request) || request.Status != 401 {
		return live, err
	}
	// Re-authenticate through the SSH account when a saved management grant has expired.
	// A replacement identity must never silently replace the enrolled instance.
	renewed, fresh, err := Enroll(ctx, c)
	if err != nil {
		return nil, err
	}
	if renewed.InstanceID != c.InstanceID {
		return nil, client.ErrIdentity
	}
	live, err = Tunnel(ctx, renewed, fresh)
	if err != nil {
		return nil, err
	}
	if err = s.PutContextWithCredential(renewed, fresh); err != nil {
		live.Close()
		return nil, err
	}
	return live, nil
}

func Tunnel(ctx context.Context, c state.Context, token string) (*Live, error) {
	if c.Validate() != nil || c.Transport != "ssh" || token == "" {
		return nil, failure("invalid")
	}
	if _, err := sshBinary(); err != nil {
		return nil, err
	}
	live := &Live{Profile: c, Endpoint: "http://127.0.0.1:" + strconv.Itoa(c.RemoteControlPort), token: token}
	cl, _, err := live.Connect(ctx)
	if err != nil {
		live.Close()
		return nil, err
	}
	cl.Close()
	return live, nil
}
