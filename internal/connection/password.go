package connection

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
	"golang.org/x/crypto/ssh"
)

// Password auth is an in-process SSH transport. Secrets are transient and never
// become profile fields. The server key is pinned in the public profile.
func passwordSSH(ctx context.Context, c state.Context, password string) (*ssh.Client, string, error) {
	if password == "" || len(password) > 8192 {
		return nil, "", failure("ssh_auth")
	}
	login, host := "", c.SSHTarget
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		login, host = host[:i], host[i+1:]
	}
	if login == "" {
		u, err := user.Current()
		if err != nil {
			return nil, "", failure("invalid")
		}
		login = u.Username
	}
	host = strings.Trim(host, "[]")
	port := c.SSHPort
	if port == 0 {
		port = 22
	}
	raw, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, "", failure("ssh_unreachable")
	}
	deadline := time.Now().Add(15 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = raw.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	defer stop()
	pin := ""
	hostFailure := false
	config := &ssh.ClientConfig{User: login, Auth: []ssh.AuthMethod{ssh.Password(password), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
		if len(questions) != 1 {
			return nil, failure("ssh_auth")
		}
		return []string{password}, nil
	})}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		pin = base64.StdEncoding.EncodeToString(key.Marshal())
		if c.SSHHostKey != "" && c.SSHHostKey == pin {
			return nil
		}
		if c.SSHHostKey == "" && c.TrustNewHost {
			return nil
		}
		hostFailure = true
		return failure("ssh_host_key")
	}}
	conn, channels, requests, err := ssh.NewClientConn(raw, net.JoinHostPort(host, strconv.Itoa(port)), config)
	if err != nil {
		raw.Close()
		if hostFailure {
			return nil, "", failure("ssh_host_key")
		}
		return nil, "", failure("ssh_auth")
	}
	if ctx.Err() != nil {
		conn.Close()
		return nil, "", failure("ssh_unreachable")
	}
	_ = raw.SetDeadline(time.Time{})
	return ssh.NewClient(conn, channels, requests), pin, nil
}
func passwordBootstrap(ctx context.Context, sshClient *ssh.Client, c state.Context) (state.Context, string, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return state.Context{}, "", failure("tunnel_failed")
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	stdout := &boundedBuffer{limit: 65537}
	stderr := &boundedBuffer{limit: 16384}
	session.Stdout = stdout
	session.Stderr = stderr
	command := bootstrapCommand(c)
	if err = session.Run(command); err != nil {
		return state.Context{}, "", remoteBootstrapError(stderr.String(), "remote_anchor")
	}
	var output Bootstrap
	decoder := json.NewDecoder(bytes.NewBufferString(stdout.String()))
	decoder.DisallowUnknownFields()
	if len(stdout.String()) > 65536 || decoder.Decode(&output) != nil || decoder.Decode(new(any)) != io.EOF || !state.ValidOpaqueID(output.Settings.InstanceID) || !state.ValidOpaqueID(output.Access.ID) || output.Access.Scope != "manage" || output.Access.Revoked || output.Token == "" || len(output.Token) > 8192 || output.Settings.ControlAddress != net.JoinHostPort("127.0.0.1", strconv.Itoa(c.RemoteControlPort)) {
		return state.Context{}, "", failure("remote_anchor")
	}
	c.InstanceID = output.Settings.InstanceID
	c.AccessID = output.Access.ID
	return c, output.Token, nil
}
func EnrollConnection(ctx context.Context, c state.Context, password string) (*Live, string, error) {
	if password == "" {
		profile, token, err := Enroll(ctx, c)
		if err != nil {
			return nil, "", err
		}
		live, err := Tunnel(ctx, profile, token)
		return live, token, err
	}
	c.Transport = "ssh"
	c.CredentialRef = "ssh-manage"
	c.InstanceID = "pending"
	c.AccessID = "pending"
	c.Endpoint = ""
	c.SSHAuth = "password"
	if c.RemoteExecutable == "" {
		c.RemoteExecutable = "anchor"
	}
	if c.RemoteControlPort == 0 {
		c.RemoteControlPort = 17861
	}
	if c.Validate() != nil {
		return nil, "", failure("invalid")
	}
	native, pin, err := passwordSSH(ctx, c, password)
	if err != nil {
		return nil, "", err
	}
	c.SSHHostKey = pin
	profile, token, err := passwordBootstrap(ctx, native, c)
	if err != nil {
		native.Close()
		return nil, "", err
	}
	live := &Live{Profile: profile, Endpoint: "http://127.0.0.1:" + strconv.Itoa(c.RemoteControlPort), token: token, native: native}
	cl, _, err := live.Connect(ctx)
	if err != nil {
		live.Close()
		return nil, "", err
	}
	cl.Close()
	return live, token, nil
}
func OpenWithPassword(ctx context.Context, store *state.Store, c state.Context, password string) (*Live, error) {
	if c.SSHAuth != "password" {
		return Open(ctx, store, c)
	}
	if c.Validate() != nil {
		return nil, failure("invalid")
	}
	native, _, err := passwordSSH(ctx, c, password)
	if err != nil {
		return nil, err
	}
	token, err := store.ContextCredential(c.Name)
	if err != nil {
		native.Close()
		return nil, failure("unauthorized")
	}
	live := &Live{Profile: c, Endpoint: "http://127.0.0.1:" + strconv.Itoa(c.RemoteControlPort), token: token, native: native}
	cl, _, err := live.Connect(ctx)
	var request *client.RequestError
	if errors.As(err, &request) && request.Status == 401 {
		renewed, fresh, e := passwordBootstrap(ctx, native, c)
		if e != nil {
			live.Close()
			return nil, e
		}
		if renewed.InstanceID != c.InstanceID {
			live.Close()
			return nil, client.ErrIdentity
		}
		live.Profile = renewed
		live.token = fresh
		cl, _, err = live.Connect(ctx)
		if err == nil {
			err = store.PutContextWithCredential(renewed, fresh)
		}
	}
	if cl != nil {
		cl.Close()
	}
	if err != nil {
		live.Close()
		return nil, err
	}
	return live, nil
}
