package egress

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func Available() bool {
	if len(License) == 0 || len(Notice) == 0 || runtime.GOARCH != "amd64" {
		return false
	}
	_, e := bundle.ReadFile(bundlePath)
	return e == nil
}

// The archive is pinned, not a user-provided executable path. Validate both the
// embedded archive and existing extraction before executing it.
func executable(root string) (string, error) {
	if !Available() {
		return "", ErrUnavailable
	}
	archive, e := bundle.ReadFile(bundlePath)
	if e != nil {
		return "", ErrUnavailable
	}
	h := sha256.Sum256(archive)
	if hex.EncodeToString(h[:]) != bundleHash {
		return "", ErrUnavailable
	}
	var reader io.ReadCloser
	if runtime.GOOS == "windows" {
		z, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if e != nil {
			return "", ErrUnavailable
		}
		for _, f := range z.File {
			if filepath.Ext(f.Name) == ".exe" {
				if reader != nil {
					reader.Close()
					return "", ErrUnavailable
				}
				reader, e = f.Open()
				if e != nil {
					return "", ErrUnavailable
				}
			}
		}
	} else {
		reader, e = gzip.NewReader(bytes.NewReader(archive))
		if e != nil {
			return "", ErrUnavailable
		}
	}
	if reader == nil {
		return "", ErrUnavailable
	}
	defer reader.Close()
	data, e := io.ReadAll(io.LimitReader(reader, (100<<20)+1))
	if e != nil || len(data) > 100<<20 {
		return "", ErrUnavailable
	}
	dir := filepath.Join(root, "mihomo", Version)
	if e = platform.SecureDir(dir); e != nil {
		return "", ErrUnavailable
	}
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	want := sha256.Sum256(data)
	if old, e := os.ReadFile(path); e == nil {
		if sha256.Sum256(old) == want {
			return path, nil
		}
		return "", ErrUnavailable
	}
	f, e := os.CreateTemp(dir, ".install-")
	if e != nil {
		return "", ErrUnavailable
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(data); e == nil {
		e = f.Chmod(0700)
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return "", ErrUnavailable
	}
	if e = os.Rename(temp, path); e != nil {
		return "", ErrUnavailable
	}
	return path, nil
}

type engine struct {
	cmd  *exec.Cmd
	done chan struct{}
	dir  string
	port int
}

func (e *engine) close() {
	if e == nil {
		return
	}
	_ = e.cmd.Process.Kill()
	<-e.done
	_ = os.RemoveAll(e.dir)
}
func startEngine(ctx context.Context, root, secret string, listenPort int) (*engine, string, error) {
	binary, e := executable(root)
	if e != nil {
		return nil, "", ErrUnavailable
	}
	parent := filepath.Join(root, "mihomo", "runs")
	if e = platform.SecureDir(parent); e != nil {
		return nil, "", ErrUnavailable
	}
	dir, e := os.MkdirTemp(parent, "node-")
	if e != nil {
		return nil, "", ErrUnavailable
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		return nil, "", ErrUnavailable
	}
	password := hex.EncodeToString(token)
	ln, e := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(listenPort)))
	if e != nil {
		return nil, "", ErrUnavailable
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	var node map[string]any
	if e = json.Unmarshal([]byte(secret), &node); e != nil {
		return nil, "", ErrUnavailable
	}
	config := map[string]any{"mixed-port": port, "allow-lan": false, "bind-address": "127.0.0.1", "skip-auth-prefixes": []string{}, "authentication": []string{"anchor:" + password}, "mode": "rule", "log-level": "silent", "ipv6": true, "find-process-mode": "off", "proxies": []any{node}, "rules": []string{"MATCH,egress"}}
	data, e := json.Marshal(config)
	if e != nil {
		return nil, "", ErrUnavailable
	}
	path := filepath.Join(dir, "config.json")
	if e = os.WriteFile(path, data, 0600); e != nil {
		return nil, "", ErrUnavailable
	}
	cmd := exec.Command(binary, "-d", dir, "-f", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	configureProcess(cmd)
	if e = cmd.Start(); e != nil {
		return nil, "", ErrUnavailable
	}
	release, err := trackProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, "", ErrUnavailable
	}
	run := &engine{cmd: cmd, done: make(chan struct{}), dir: dir, port: port}
	go func() { _ = cmd.Wait(); release(); close(run.done) }()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			run.close()
			return nil, "", ErrUnavailable
		case <-deadline.C:
			run.close()
			return nil, "", ErrUnavailable
		case <-run.done:
			run.close()
			return nil, "", ErrUnavailable
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
			if err == nil {
				conn.Close()
				keep = true
				return run, fmt.Sprintf("http://anchor:%s@127.0.0.1:%d", password, port), nil
			}
		}
	}
}
