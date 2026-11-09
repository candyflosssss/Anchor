package cli

import (
	"errors"
	"github.com/candyflosssss/Anchor/internal/autostart"
)

type autostartManager interface {
	Status() (autostart.Status, error)
	Save(autostart.Save) (autostart.Status, error)
}

func (a App) autostart(dir string, args []string) error {
	if len(args) == 0 {
		return errors.New("autostart requires status, enable or disable")
	}
	if args[0] != "status" && args[0] != "enable" && args[0] != "disable" {
		return errors.New("unknown autostart command")
	}
	f := a.flags("autostart " + args[0])
	exe := f.String("desktop-executable", "", "absolute installed anchor-desktop.exe path (optional)")
	var revision string
	var approve bool
	if args[0] != "status" {
		f.StringVar(&revision, "revision", "", "revision from autostart status")
		f.BoolVar(&approve, "approve", false, "approve current-user login item change; does not start or stop the core")
	}
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	var m autostartManager = autostart.New(dir, *exe)
	if a.autostartManager != nil {
		m = a.autostartManager
	}
	if args[0] == "status" {
		out, err := m.Status()
		if err != nil {
			return err
		}
		return a.output(out)
	}
	enabled := args[0] == "enable"
	in := autostart.Save{Enabled: &enabled, ExpectedRevision: revision, Approved: approve}
	if err := in.Validate(); err != nil {
		return err
	}
	out, err := m.Save(in)
	if err != nil {
		return err
	}
	return a.output(out)
}
