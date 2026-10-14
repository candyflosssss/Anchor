package cli

import (
	"context"
	"errors"
	"github.com/candyflosssss/Anchor/internal/connection"
	"time"
)

func (a App) bootstrap(ctx context.Context, dir string, args []string) error {
	f := a.flags("context bootstrap")
	port := f.Int("control-port", 17861, "remote loopback control port")
	show := f.Bool("show-once", false, "explicitly disclose new management token")
	if err := parse(f, args); err != nil {
		return err
	}
	if !*show {
		return errors.New("explicit --show-once is required; output contains a secret")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s, err := connection.EnsureLocal(ctx, dir, *port)
	if err != nil {
		return err
	}
	defer s.Close()
	cfg, err := s.Settings()
	if err != nil {
		return err
	}
	access, token, err := s.Grant("anchor-ssh", "manage", 720*time.Hour)
	if err != nil {
		return err
	}
	return a.output(connection.Bootstrap{Settings: cfg, Access: access, Token: token})
}
