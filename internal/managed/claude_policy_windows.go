package managed

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

func claudeDesktopPolicyPresent(string) bool {
	for _, hive := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(hive, `SOFTWARE\Policies\Claude`, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return true
		}
		info, err := key.Stat()
		_ = key.Close()
		if err != nil || info.ValueCount > 0 {
			return true
		}
	}
	return false
}
