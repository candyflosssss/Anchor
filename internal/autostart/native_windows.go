package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const approvalKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
const valueName = "AnchorDesktopCore"

type windowsBackend struct{}

func nativeBackend() backend { return windowsBackend{} }
func (windowsBackend) read() (snapshot, error) {
	var s snapshot
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err == nil {
		s.Command, _, err = k.GetStringValue(valueName)
		k.Close()
	}
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return s, err
	}
	k, err = registry.OpenKey(registry.CURRENT_USER, approvalKey, registry.QUERY_VALUE)
	if err == nil {
		s.Approval, _, err = k.GetBinaryValue(valueName)
		k.Close()
	}
	if errors.Is(err, registry.ErrNotExist) {
		err = nil
	}
	return s, err
}
func (windowsBackend) write(command string) error {
	if command == "" {
		k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer k.Close()
		err = k.DeleteValue(valueName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(valueName, command)
}
func desktopExecutable(explicit string) (string, error) {
	path := explicit
	if path == "" {
		// Keep the stable launcher across overlay updates. It forwards --core to
		// the verified active runtime without opening a GUI.
		k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\AnchorDesktop`, registry.QUERY_VALUE)
		if err == nil {
			dir, _, readErr := k.GetStringValue("InstallLocation")
			k.Close()
			if readErr == nil && dir != "" {
				path = filepath.Join(dir, "anchor-desktop.exe")
			}
		}
		if path == "" {
			exe, err := os.Executable()
			if err != nil {
				return "", ErrInvalid
			}
			if strings.EqualFold(filepath.Base(exe), "anchor-desktop.exe") {
				path = exe
			} else {
				path = filepath.Join(filepath.Dir(exe), "anchor-desktop.exe")
			}
		}
	}
	if !validWindowsPath(path) || !strings.EqualFold(filepath.Base(path), "anchor-desktop.exe") {
		return "", ErrInvalid
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrInvalid
	}
	return filepath.Clean(path), nil
}
