package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

// RuntimeResult is returned only after the replacement core is authenticated and ready.
type RuntimeResult struct {
	Ready      bool   `json:"ready"`
	Version    string `json:"version"`
	InstanceID string `json:"instance_id"`
	ProcessID  int    `json:"process_id"`
}

type Runtime struct {
	Version  string   `json:"version"`
	Artifact Artifact `json:"artifact"`
	Path     string   `json:"path"`
}

func runtimePath(dir, product, version string, a Artifact) string {
	name := "anchor"
	if product == "desktop" {
		name = "anchor-desktop"
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "updates", "runtime", product+"-"+version+"-"+a.SHA256, name)
}

// PrepareRuntime installs beside the running executable, never overwriting it.
// Caller holds the update lock through preparation and activation.
func (s *Service) PrepareRuntime(ctx context.Context, dir string, approved InstallApproval) (Runtime, error) {
	var out Runtime
	d, err := s.readApprovedDownload(dir, approved)
	if err != nil {
		return out, err
	}
	if (s.Product == "desktop" && d.Check.Artifact.Kind != "runtime") || (s.Product == "cli" && d.Check.Artifact.Kind != "binary") {
		return out, ErrUnavailable
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return out, err
	}
	out = Runtime{Version: d.Check.Version, Artifact: *d.Check.Artifact}
	out.Path = runtimePath(dir, s.Product, out.Version, out.Artifact)
	root := filepath.Dir(out.Path)
	if err := platform.SecureDir(root); err != nil {
		return out, err
	}
	if verify(out.Path, out.Artifact) != nil {
		f, err := os.CreateTemp(root, ".runtime-*")
		if err != nil {
			return out, err
		}
		defer os.Remove(f.Name())
		defer f.Close()
		source, err := os.Open(d.Path)
		if err != nil {
			return out, err
		}
		defer source.Close()
		if _, err = io.Copy(f, io.LimitReader(source, out.Artifact.Bytes+1)); err != nil {
			return out, err
		}
		if err = platform.SecureFile(f.Name()); err != nil {
			return out, err
		}
		if err = f.Chmod(0700); err != nil {
			return out, err
		}
		if err = f.Sync(); err != nil {
			return out, err
		}
		if err = f.Close(); err != nil {
			return out, err
		}
		if err = verify(f.Name(), out.Artifact); err != nil {
			return out, err
		}
		if err = platform.ReplaceManagedFile(f.Name(), out.Path); err != nil {
			return out, err
		}
	}
	// Check the actual executable and dynamic loader before stopping a service.
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probe, out.Path, "--runtime-version")
	hideRuntimeWindow(cmd)
	var stdout limitedOutput
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return out, fmt.Errorf("runtime preflight failed: %w", err)
	}
	var version struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(stdout.data, &version) != nil || version.Version != out.Version {
		return out, ErrIntegrity
	}
	return out, nil
}

type limitedOutput struct{ data []byte }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4096 {
		return 0, ErrInvalid
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (s *Service) ActivateRuntime(dir string, r Runtime) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if r.Path != runtimePath(abs, s.Product, r.Version, r.Artifact) {
		return ErrInvalid
	}
	if err := verify(r.Path, r.Artifact); err != nil {
		return err
	}
	return writeJSON(filepath.Join(abs, "updates", s.Product+"-active.json"), r)
}

// ActiveRuntime returns only a strictly newer, intact overlay. Installed launchers
// keep working after application updates without requiring admin privileges.
func ActiveRuntime(dir, product, version string) (Runtime, error) {
	var r Runtime
	abs, err := filepath.Abs(dir)
	if err != nil {
		return r, err
	}
	b, err := readFile(filepath.Join(abs, "updates", product+"-active.json"), maxMetadata)
	if errors.Is(err, os.ErrNotExist) {
		return Runtime{}, nil
	}
	if err != nil {
		return r, err
	}
	if json.Unmarshal(b, &r) != nil || validateArtifact(r.Artifact, product) != nil || r.Artifact.Runtime != nil || r.Artifact.Target != runtime.GOOS+"-"+runtime.GOARCH {
		return Runtime{}, ErrInvalid
	}
	if (product == "desktop" && r.Artifact.Kind != "runtime") || (product == "cli" && r.Artifact.Kind != "binary") {
		return Runtime{}, ErrInvalid
	}
	cmp, err := compareVersions(r.Version, version)
	if err != nil {
		return Runtime{}, err
	}
	if cmp <= 0 {
		return Runtime{}, nil
	}
	if r.Path != runtimePath(abs, product, r.Version, r.Artifact) {
		return Runtime{}, ErrInvalid
	}
	return r, verify(r.Path, r.Artifact)
}

// ForwardRuntime preserves stdio, arguments and the child's exit status.
func ForwardRuntime(ctx context.Context, dir, product, version string, args []string) (bool, int, error) {
	r, err := ActiveRuntime(dir, product, version)
	if err != nil || r.Path == "" {
		return false, 0, err
	}
	if runtime.GOOS == "linux" {
		return true, 0, execRuntime(r.Path, args)
	}
	cmd := exec.Command(r.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return true, exit.ExitCode(), nil
	}
	return true, 0, err
}

// LaunchRuntime never inherits a console, pipes or the old window lifetime.
func LaunchRuntime(executable string, args ...string) (int, <-chan error, error) {
	cmd := exec.Command(executable, args...)
	detachRuntime(cmd)
	sink, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0, nil, err
	}
	defer sink.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = sink, sink, sink
	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait(); close(exited) }()
	return cmd.Process.Pid, exited, nil
}
