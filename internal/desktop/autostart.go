package desktop

import (
	"errors"
	"os"

	"github.com/candyflosssss/Anchor/internal/autostart"
)

type autostartManager interface {
	Status() (autostart.Status, error)
	Save(autostart.Save) (autostart.Status, error)
}

func (b *Bridge) Autostart() AutostartResult { return b.autostartCall(nil) }
func (b *Bridge) SaveAutostart(in SaveAutostart) AutostartResult {
	if in.Validate() != nil {
		return AutostartResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.autostartCall(&in)
}
func (b *Bridge) autostartCall(in *SaveAutostart) AutostartResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := AutostartResult{Contract: Contract}
	if s.ctx.Err() != nil {
		out.Problem = &Problem{"closed"}
		return out
	}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	// This is a host capability, even when the selected core is remote.
	var m autostartManager = autostart.New(s.options.Dir, "")
	if s.autostartManager != nil {
		m = s.autostartManager
	}
	var data AutostartState
	var err error
	if in == nil {
		data, err = m.Status()
	} else {
		data, err = m.Save(*in)
	}
	if err != nil {
		code := "read_error"
		switch {
		case errors.Is(err, autostart.ErrInvalid):
			code = "invalid"
		case errors.Is(err, autostart.ErrConflict):
			code = "conflict"
		case errors.Is(err, autostart.ErrUnsupported):
			code = "unsupported"
		case errors.Is(err, os.ErrPermission):
			code = "permission"
		case in != nil:
			code = "unknown"
		}
		out.Problem = &Problem{code}
		return out
	}
	out.Data = &data
	return out
}
