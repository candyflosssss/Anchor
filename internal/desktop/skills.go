package desktop

import (
	"context"
	"time"

	"github.com/candyflosssss/Anchor/internal/skills"
)

// Canonical Skill DTOs are shared by the core, typed client and desktop bridge.
type SkillFile = skills.SkillFile
type SkillVersion = skills.SkillVersion
type Skill = skills.Skill
type SkillScope = skills.SkillScope
type SkillSource = skills.SkillSource
type SkillMount = skills.SkillMount
type SkillWorkspace = skills.SkillWorkspace
type SkillEdit = skills.SkillEdit
type SkillAction = skills.SkillAction
type SkillQuery = skills.SkillQuery
type SkillDifference = skills.SkillDifference
type SkillDetail = skills.SkillDetail
type SkillWorkspaceResult struct {
	Contract string          `json:"contract"`
	Data     *SkillWorkspace `json:"data"`
	Problem  *Problem        `json:"problem"`
}
type SkillDetailResult struct {
	Contract string       `json:"contract"`
	Data     *SkillDetail `json:"data"`
	Problem  *Problem     `json:"problem"`
}

func (b *Bridge) SkillWorkspace() SkillWorkspaceResult { return b.skillWorkspaceCall(nil) }
func (b *Bridge) ManageSkill(in SkillAction) SkillWorkspaceResult {
	if !in.Approved || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision {
		return SkillWorkspaceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.skillWorkspaceCall(&in)
}
func (b *Bridge) skillWorkspaceCall(in *SkillAction) SkillWorkspaceResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SkillWorkspaceResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	var data SkillWorkspace
	if in == nil {
		data, err = cl.SkillWorkspace(ctx)
	} else {
		data, err = cl.SkillAction(ctx, *in)
	}
	if err != nil {
		out.Problem = problem(err, in != nil)
		return out
	}
	out.Data = &data
	return out
}
func (b *Bridge) SkillDetail(in SkillQuery) SkillDetailResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SkillDetailResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.SkillDetail(ctx, in)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &data
	return out
}
