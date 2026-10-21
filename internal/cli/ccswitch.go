package cli

import (
	"context"
	"errors"
	"github.com/candyflosssss/Anchor/internal/ccswitch"
	"github.com/candyflosssss/Anchor/internal/client"
)

func (a App) importCCSwitch(ctx context.Context, c *client.Client, args []string) error {
	f := a.flags("provider import-ccs")
	path := f.String("db", "", "CC Switch database path on the selected instance (default ~/.cc-switch/cc-switch.db)")
	fingerprint := f.String("fingerprint", "", "fingerprint from the reviewed preview (required with --approve)")
	approved := f.Bool("approve", false, "atomically import and enable the reviewed providers, Keys and mappings")
	if err := parse(f, args); err != nil {
		return err
	}
	if *approved != (*fingerprint != "") {
		return errors.New("apply requires both --approve and the preview --fingerprint; omit both to preview")
	}
	out, err := c.ImportCCSwitch(ctx, ccswitch.Request{DatabasePath: *path, Fingerprint: *fingerprint, Approved: *approved})
	if err != nil {
		return err
	}
	if err = a.output(out); err != nil {
		return err
	}
	if out.Failure != "" {
		return errors.New("CC Switch source unavailable: " + out.Failure)
	}
	return nil
}
