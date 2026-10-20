package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimePreparationAndActiveIntegrity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("shell fixture")
	}
	for _, mode := range []string{"ready", "wrong-version", "not-executable", "approval", "tamper", "link"} {
		t.Run(mode, func(t *testing.T) {
			service, cfg, manifest, payload := testFeed(t, "desktop")
			*payload = "#!/bin/sh\nprintf '%s\\n' '{\"version\":\"0.1.0-dev.desktop.10\"}'\n"
			if mode == "wrong-version" {
				*payload = "#!/bin/sh\nprintf '%s\\n' '{\"version\":\"0.1.0-dev.desktop.11\"}'\n"
			}
			if mode == "not-executable" {
				*payload = "not a program"
			}
			// Preserve the installer while selecting the separately verified runtime.
			a := manifest.Artifacts[0]
			a.Kind = "runtime"
			a.File = "anchor-runtime"
			a.Bytes = int64(len(*payload))
			a.SHA256 = runtimeHash([]byte(*payload))
			manifest.Artifacts[0].Runtime = &a
			checked, err := service.Check(context.Background(), cfg)
			if err != nil || checked.Artifact.Kind != "runtime" {
				t.Fatal(checked, err)
			}
			dir := t.TempDir()
			d, err := service.Download(context.Background(), dir, checked)
			if err != nil {
				t.Fatal(err)
			}
			approval := InstallApproval{Version: checked.Version, SHA256: a.SHA256}
			if mode == "approval" {
				approval.Version = "0.1.0-dev.desktop.11"
			}
			if mode == "tamper" {
				os.WriteFile(d.Path, []byte("tampered"), 0600)
			}
			if mode == "link" {
				os.Remove(d.Path)
				os.Symlink(filepath.Join(dir, "outside"), d.Path)
			}
			lock, err := Lock(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			prepared, err := service.PrepareRuntime(context.Background(), dir, approval)
			if mode != "ready" {
				if err == nil {
					t.Fatal("invalid runtime accepted")
				}
				if active, e := ActiveRuntime(dir, "desktop", service.Version); e != nil || active.Path != "" {
					t.Fatal("failed preparation activated runtime", active, e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := service.ActivateRuntime(dir, prepared); err != nil {
				t.Fatal(err)
			}
			active, err := ActiveRuntime(dir, "desktop", service.Version)
			if err != nil || active.Path != prepared.Path {
				t.Fatal(active, err)
			}
			if active, err := ActiveRuntime(dir, "desktop", prepared.Version); err != nil || active.Path != "" {
				t.Fatal("same-version forwarding loop", active, err)
			}
			os.WriteFile(prepared.Path, []byte("tampered"), 0700)
			if _, err := ActiveRuntime(dir, "desktop", service.Version); !errors.Is(err, ErrIntegrity) {
				t.Fatal("active tampering accepted", err)
			}
		})
	}
}

func runtimeHash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

func TestApplyBinaryDoesNotOverwriteRuntimeManagedPath(t *testing.T) {
	service, cfg, _, _ := testFeed(t, "cli")
	dir := t.TempDir()
	checked, err := service.Check(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Download(t.Context(), dir, checked); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "updates", "runtime", "existing")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "anchor")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, []byte("preserve"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyBinary(dir, path); !errors.Is(err, ErrInvalid) {
		t.Fatal("runtime path overwritten", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "preserve" {
		t.Fatal("runtime was modified", err)
	}
}
