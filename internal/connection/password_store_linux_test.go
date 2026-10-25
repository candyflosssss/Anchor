//go:build linux

package connection

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSystemPasswordVaultIsolatedSecretService(t *testing.T) {
	if os.Getenv("ANCHOR_TEST_OS_KEYRING") != "1" {
		t.Skip("requires isolated D-Bus and disposable GNOME keyring")
	}
	if !strings.HasPrefix(os.Getenv("XDG_DATA_HOME"), os.Getenv("TMPDIR")+"/") || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Fatal("keyring test isolation missing")
	}
	vault := systemPasswordVault{}
	service, account, secret := passwordService+".isolated-test", "fixture-account", "fixture-only-password"
	if err := vault.Set(service, account, secret); err != nil {
		t.Fatal("Secret Service write", err)
	}
	t.Cleanup(func() { _ = vault.Delete(service, account) })
	if actual, err := vault.Get(service, account); err != nil || actual != secret {
		t.Fatal("Secret Service read failed", err)
	}
	if err := vault.Delete(service, account); err != nil {
		t.Fatal("Secret Service forget", err)
	}
	if _, err := vault.Get(service, account); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatal("credential still present", err)
	}
}
