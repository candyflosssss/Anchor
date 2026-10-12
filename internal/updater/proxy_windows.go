package updater

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

var lookupEnv = os.Getenv

func readSystemProxy() (server, override string, ok bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return "", "", false
	}
	defer k.Close()
	if enabled, _, err := k.GetIntegerValue("ProxyEnable"); err != nil || enabled == 0 {
		return "", "", false
	}
	server, _, err = k.GetStringValue("ProxyServer")
	if err != nil || server == "" {
		return "", "", false
	}
	override, _, _ = k.GetStringValue("ProxyOverride")
	return server, override, true
}
