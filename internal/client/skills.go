package client

import (
	"context"
	"errors"

	"github.com/candyflosssss/Anchor/internal/skills"
)

func (c *Client) SkillWorkspace(ctx context.Context) (skills.SkillWorkspace, error) {
	var v skills.SkillWorkspace
	err := c.callLimited(ctx, "GET", "/control/v1/skills", nil, &v, 64<<20)
	return v, err
}
func (c *Client) SkillAction(ctx context.Context, in skills.SkillAction) (skills.SkillWorkspace, error) {
	var v skills.SkillWorkspace
	err := c.callLimited(ctx, "POST", "/control/v1/skills/action", in, &v, 64<<20)
	return v, err
}
func (c *Client) SkillDetail(ctx context.Context, in skills.SkillQuery) (skills.SkillDetail, error) {
	var v skills.SkillDetail
	err := c.callLimited(ctx, "POST", "/control/v1/skills/detail", in, &v, 64<<20)
	// This POST is read-only. Transport errors are never unknown writes.
	var re *RequestError
	if errors.As(err, &re) {
		cp := *re
		cp.Unknown = false
		err = &cp
	}
	return v, err
}
