package desktop

import "golang.org/x/sys/windows"

var allowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// A second launch can delegate its foreground eligibility to the running GUI.
// Windows may deny this if the launch itself has no foreground eligibility;
// showing/restoring the window must still be attempted in that case.
func allowHostForeground(processID uint32) {
	_, _, _ = allowSetForegroundWindow.Call(uintptr(processID))
}
