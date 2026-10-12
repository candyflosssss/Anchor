package updater

import (
	"context"
	"net/url"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var winhttp = windows.NewLazySystemDLL("winhttp.dll")
var winHTTPGetIEProxyConfig = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
var winHTTPOpen = winhttp.NewProc("WinHttpOpen")
var winHTTPGetProxyForURL = winhttp.NewProc("WinHttpGetProxyForUrl")
var winHTTPSetTimeouts = winhttp.NewProc("WinHttpSetTimeouts")
var winHTTPCloseHandle = winhttp.NewProc("WinHttpCloseHandle")
var globalFree = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")

// Layout follows winhttp.h; pointer alignment is supplied by Go on AMD64.
type ieProxyConfig struct {
	AutoDetect                   int32
	AutoConfigURL, Proxy, Bypass *uint16
}
type autoProxyOptions struct {
	Flags, AutoDetectFlags uint32
	AutoConfigURL          *uint16
	Reserved               unsafe.Pointer
	ReservedFlags          uint32
	AutoLogonIfChallenged  int32
}
type winProxyInfo struct {
	AccessType    uint32
	Proxy, Bypass *uint16
}

func freeProxyString(p *uint16) {
	if p != nil {
		globalFree.Call(uintptr(unsafe.Pointer(p)))
	}
}

// A native PAC discovery may outlive cancellation. Bound outstanding discovery
// to one call; its worker owns and releases the handle/returned strings.
var autoProxySlot = make(chan struct{}, 1)

func readSystemProxy(ctx context.Context, target *url.URL) (*url.URL, error) {
	var cfg ieProxyConfig
	ok, _, _ := winHTTPGetIEProxyConfig.Call(uintptr(unsafe.Pointer(&cfg)))
	if ok == 0 {
		return nil, nil
	}
	defer freeProxyString(cfg.AutoConfigURL)
	defer freeProxyString(cfg.Proxy)
	defer freeProxyString(cfg.Bypass)
	server, bypass, pac := windows.UTF16PtrToString(cfg.Proxy), windows.UTF16PtrToString(cfg.Bypass), windows.UTF16PtrToString(cfg.AutoConfigURL)
	if pac != "" || cfg.AutoDetect != 0 && server == "" {
		u, err := autoProxy(ctx, target, pac, cfg.AutoDetect != 0)
		if err == nil {
			return u, nil
		} // PAC's DIRECT is an intentional decision.
		if server == "" && pac != "" {
			return nil, err
		}
	}
	return pickSystemProxy(server, bypass, target)
}

func autoProxy(ctx context.Context, target *url.URL, pac string, detect bool) (*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	select {
	case autoProxySlot <- struct{}{}:
	case <-ctx.Done():
		return nil, errProxy
	}
	type result struct {
		u   *url.URL
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-autoProxySlot }()
		u, err := resolveAutoProxy(target, pac, detect)
		done <- result{u, err}
	}()
	select {
	case r := <-done:
		return r.u, r.err
	case <-ctx.Done():
		return nil, errProxy
	}
}

func resolveAutoProxy(target *url.URL, pac string, detect bool) (*url.URL, error) {
	agent, _ := windows.UTF16PtrFromString("Anchor-Updater")
	handle, _, _ := winHTTPOpen.Call(uintptr(unsafe.Pointer(agent)), 1, 0, 0, 0) // NO_PROXY for discovery session
	if handle == 0 {
		return nil, errProxy
	}
	defer winHTTPCloseHandle.Call(handle)
	winHTTPSetTimeouts.Call(handle, 5000, 5000, 5000, 5000)
	var options autoProxyOptions
	// Do not automatically send the user's Windows credentials to a PAC server.
	if pac != "" {
		p, err := windows.UTF16PtrFromString(pac)
		if err != nil {
			return nil, errProxy
		}
		options.AutoConfigURL = p
		options.Flags = 2
	} else if detect {
		options.Flags = 1
		options.AutoDetectFlags = 3
	}
	raw, err := windows.UTF16PtrFromString(target.String())
	if err != nil {
		return nil, errProxy
	}
	var info winProxyInfo
	ok, _, _ := winHTTPGetProxyForURL.Call(handle, uintptr(unsafe.Pointer(raw)), uintptr(unsafe.Pointer(&options)), uintptr(unsafe.Pointer(&info)))
	defer freeProxyString(info.Proxy)
	defer freeProxyString(info.Bypass)
	if ok == 0 {
		return nil, errProxy
	}
	if info.AccessType == 1 {
		return nil, nil
	}
	if info.AccessType != 3 || info.Proxy == nil {
		return nil, errProxy
	}
	return pickSystemProxy(windows.UTF16PtrToString(info.Proxy), windows.UTF16PtrToString(info.Bypass), target)
}
