//go:build !windows

package autostart

func nativeBackend() backend                   { return nil }
func desktopExecutable(string) (string, error) { return "", ErrUnsupported }
