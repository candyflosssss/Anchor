package updater

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

// ApplyBinary must be called only after explicit approval. It replaces the
// local CLI, not the selected context. No automatic service restart or migration.
func (s *Service) ApplyBinary(dir, executable string) (string, error) {
	if s.Product != "cli" {
		return "", ErrInvalid
	}
	updateLock, err := Lock(dir)
	if err != nil {
		return "", err
	}
	defer updateLock.Close()
	coreLock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		return "", fmt.Errorf("%w: stop the local core first", ErrBusy)
	}
	defer coreLock.Close()
	d, err := s.ReadDownload(dir)
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	dataDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if relative, err := filepath.Rel(filepath.Join(dataDir, "updates", "runtime"), executable); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: runtime-managed CLI requires update upgrade --approve", ErrInvalid)
	}
	root := filepath.Dir(executable)
	if err := platform.CheckManagedPath(root, executable); err != nil {
		return "", err
	}
	backup := executable + ".previous"
	if err := platform.CheckManagedPath(root, backup); err != nil {
		return "", err
	}
	// Copy onto the executable filesystem, then rename; never truncate a live
	// binary. User-unwritable installs fail before moving the existing executable.
	st, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(root, ".anchor-new-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	source, err := os.Open(d.Path)
	if err != nil {
		return "", err
	}
	defer source.Close()
	if _, err := io.Copy(f, source); err != nil {
		return "", err
	}
	if err := f.Chmod(st.Mode().Perm()); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := verify(f.Name(), *d.Check.Artifact); err != nil {
		return "", err
	}
	if err := platform.ReplaceManagedFile(executable, backup); err != nil {
		return "", fmt.Errorf("binary backup failed (permissions or running process): %w", err)
	}
	if err := platform.ReplaceManagedFile(f.Name(), executable); err != nil {
		if restore := platform.ReplaceManagedFile(backup, executable); restore != nil {
			return "", fmt.Errorf("install failed; restore %s manually: %w", backup, restore)
		}
		return "", fmt.Errorf("install failed; previous binary restored: %w", err)
	}
	return backup, nil
}

// StartInstaller delegates to the user's native package application. Linux
// privilege prompts belong to that application; Anchor never invokes sudo.
func StartInstaller(ctx context.Context, d Download) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.Check.Product != "desktop" || d.Check.Artifact == nil {
		return ErrInvalid
	}
	if err := validateArtifact(*d.Check.Artifact, "desktop"); err != nil {
		return err
	}
	if d.Check.Artifact.Target != runtime.GOOS+"-"+runtime.GOARCH {
		return ErrInvalid
	}
	if err := verify(d.Path, *d.Check.Artifact); err != nil {
		return err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(d.Path)
	} else {
		cmd = exec.Command("xdg-open", d.Path)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// InstallApproval binds the handoff to the exact version and bytes explicitly
// approved by the user, even if another local updater replaces the receipt.
type InstallApproval struct {
	Version string
	SHA256  string
}

func (s *Service) readApprovedDownload(dir string, approved InstallApproval) (Download, error) {
	d, err := s.ReadDownload(dir)
	if err != nil {
		return d, err
	}
	if d.Check.Version != approved.Version || d.Check.Artifact.SHA256 != approved.SHA256 {
		return d, ErrInvalid
	}
	return d, nil
}

// LaunchInstallerHelper runs a copy of this host so the Windows installation
// path is no longer held open. The helper waits for the window process to exit,
// acquires the local core lock, verifies the receipt again, then runs NSIS/xdg.
// launch callback is host-specific and detached from the Wails lifetime.
func LaunchInstallerHelper(dir, executable string, approved InstallApproval, launch func(string, []string) error) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	root := filepath.Join(dir, "updates")
	f, err := os.CreateTemp(root, "anchor-update-helper-*"+filepath.Ext(executable))
	if err != nil {
		return err
	}
	name := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(name)
		}
	}()
	defer f.Close()
	source, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer source.Close()
	if _, err := io.Copy(f, source); err != nil {
		return err
	}
	if err := platform.SecureFile(name); err != nil {
		return err
	}
	if err := f.Chmod(0700); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := launch(name, []string{"--data-dir", dir, "--finish-update", strconv.Itoa(os.Getpid()), "--install-version", approved.Version, "--install-sha256", approved.SHA256}); err != nil {
		return err
	}
	ok = true
	return nil
}
func FinishInstaller(ctx context.Context, dir, version string, parent int, approved InstallApproval) error {
	if parent < 1 || parent == os.Getpid() {
		return ErrInvalid
	}
	if err := waitParent(ctx, parent); err != nil {
		return err
	}
	lock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	// Runtime activation has already restarted and verified the core. Reopening
	// the GUI must not require or acquire its lifetime lock.
	service := New("desktop", version)
	downloaded, err := service.readApprovedDownload(dir, approved)
	if err != nil {
		return err
	}
	if downloaded.Check.Artifact.Kind == "runtime" {
		active, err := ActiveRuntime(dir, "desktop", version)
		if err != nil {
			return err
		}
		if active.Version != approved.Version || active.Artifact.SHA256 != approved.SHA256 {
			return ErrInvalid
		}
		_, _, err = LaunchRuntime(active.Path, "--data-dir", dir)
		return err
	}
	coreLock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		return fmt.Errorf("%w: another core is running; install cancelled", ErrBusy)
	}
	defer coreLock.Close()
	s := New("desktop", version)
	d, err := s.readApprovedDownload(dir, approved)
	if err != nil {
		return err
	}
	return StartInstaller(ctx, d)
}
func sleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}
func removeHelper() {
	if p, err := os.Executable(); err == nil {
		_ = os.Remove(p)
	}
}

// A helper log survives the closing window and does not contain credentials.
func RunHelper(dir, version string, parent int, approved InstallApproval) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if filepath.Dir(exe) != filepath.Join(abs, "updates") || !strings.HasPrefix(filepath.Base(exe), "anchor-update-helper-") {
		return ErrInvalid
	}
	defer removeHelper() // Windows may keep its executing copy; it is safe to delete later.
	root := filepath.Join(dir, "updates")
	if err := platform.SecureDir(root); err != nil {
		return err
	}
	path := filepath.Join(root, "install.log")
	if err := platform.SecureFile(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	err = FinishInstaller(ctx, dir, version, parent, approved)
	if err != nil {
		_, _ = fmt.Fprintln(f, "Installation did not complete:", err)
	} else {
		_, _ = fmt.Fprintln(f, "Update handoff completed; native installers still require confirmation; runtime updates reopened the verified version.")
	}
	return err
}
