package cli

import (
	"context"
	"errors"
	"time"

	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) passwords() connection.Passwords { return connection.Passwords{Vault: a.PasswordVault} }

func (a App) contextPassword(ctx context.Context, s *state.Store, args []string) error {
	if len(args) != 2 || (args[0] != "save" && args[0] != "status" && args[0] != "forget") {
		return errors.New("context password requires save|status|forget NAME; save requires --ssh-password-stdin")
	}
	c, err := s.Context(args[1])
	if err != nil {
		return err
	}
	if c.Transport != "ssh" || c.SSHAuth != "password" {
		return errors.New("context password requires a password-authenticated SSH context")
	}
	p := a.passwords()
	switch args[0] {
	case "save":
		if a.sshPassword == "" {
			return errors.New("context password save requires --ssh-password-stdin")
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		live, err := p.Open(ctx, s, c, a.sshPassword)
		if err != nil {
			return err
		}
		defer live.Close()
		cl, _, err := live.Connect(ctx)
		if err != nil {
			return err
		}
		defer cl.Close()
		status, err := cl.Status(ctx)
		if err != nil {
			return err
		}
		if status["core"] != "ready" {
			return errors.New("remote core is not ready; password was not saved")
		}
		if err := p.Save(s, live.Profile, a.sshPassword); err != nil {
			return err
		}
	case "forget":
		if err := p.Forget(s, c); err != nil {
			return err
		}
	}
	saved, err := p.Saved(s, c)
	if err != nil {
		return err
	}
	return a.output(map[string]any{"name": c.Name, "password_saved": saved})
}
