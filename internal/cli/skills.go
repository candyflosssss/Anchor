package cli

import (
	"context"
	"errors"
	"io"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/skills"
)

func (a *App) skills(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("skill requires scan, list, inspect, diff, register-project, import, cleanup, create, save, link, update, unlink or delete")
	}
	action := args[0]
	if action == "scan" || action == "list" {
		if len(args) != 1 {
			return errors.New("skill scan/list takes no arguments")
		}
		v, e := c.SkillWorkspace(ctx)
		if e != nil {
			return e
		}
		return a.output(v)
	}
	if action == "inspect" || action == "diff" {
		f := a.flags("skill " + action)
		id := ""
		rest := args[1:]
		if len(rest) > 0 && (rest[0] == "" || rest[0][0] != '-') {
			id = rest[0]
			rest = rest[1:]
		}
		source := f.String("source", "", "discovered source ID (instead of Skill ID)")
		version := f.Int("version", 0, "library snapshot number")
		against := f.Int("against", 0, "comparison snapshot; source defaults to last imported baseline")
		file := f.String("file", "", "read one relative text file (inspect)")
		if e := parse(f, rest); e != nil {
			return e
		}
		if (id == "") == (*source == "") || id != "" && *version < 1 {
			return errors.New("provide one Skill ID with --version, or --source SOURCE")
		}
		v, e := c.SkillDetail(ctx, skills.SkillQuery{SkillID: id, Version: *version, SourceID: *source, AgainstVersion: *against, File: *file})
		if e != nil {
			return e
		}
		return a.output(v)
	}
	if len(args) < 2 {
		return errors.New("skill operation requires a target argument")
	}
	target := args[1]
	f := a.flags("skill " + action)
	revision := f.Int64("revision", 0, "expected Skill workspace revision (required)")
	approve := f.Bool("approve", false, "explicitly approve this operation (required)")
	version := f.Int("version", 0, "immutable version to read or mount")
	name := f.String("name", "", "mount directory name or imported Skill name")
	scope := f.String("scope", "", "destination scope ID from skill scan")
	id := f.String("skill", "", "existing Skill ID for an explicit import")
	hash := f.String("hash", "", "expected source content hash from scan/detail")
	file := f.String("file", "SKILL.md", "relative text file for save")
	stdin := f.Bool("content-stdin", false, "read UTF-8 file content from stdin, at most 48 KiB")
	remove := f.Bool("delete-file", false, "delete the selected file in a new snapshot")
	executable := f.Bool("executable", false, "set executable bit on the edited file")
	if e := parse(f, args[2:]); e != nil {
		return e
	}
	if !*approve || *revision < 1 {
		return errors.New("--revision and --approve are required")
	}
	in := skills.SkillAction{Action: action, ExpectedRevision: *revision, Approved: *approve, Version: *version, Name: *name, ScopeID: *scope, ExpectedHash: *hash, Edits: []skills.SkillEdit{}}
	switch action {
	case "register-project":
		in.ProjectPath = target
	case "import", "cleanup":
		in.SourceID = target
		in.SkillID = *id
		if *hash == "" {
			return errors.New("skill import/cleanup requires --hash from a fresh scan or detail")
		}
	case "create", "save":
		if action == "create" {
			in.Name = target
			if *file != "SKILL.md" {
				return errors.New("skill create writes SKILL.md; use save for additional files")
			}
		} else {
			in.SkillID = target
			if *version < 1 {
				return errors.New("skill save requires --version")
			}
		}
		if *stdin == *remove || action == "create" && *remove {
			return errors.New("provide --content-stdin or --delete-file (save only)")
		}
		content := ""
		if *stdin {
			b, e := io.ReadAll(io.LimitReader(a.In, skills.MaxText+1))
			if e != nil {
				return e
			}
			if len(b) > skills.MaxText {
				return errors.New("content exceeds 48 KiB")
			}
			content = string(b)
		}
		in.Edits = []skills.SkillEdit{{Path: *file, Content: content, Executable: *executable, Delete: *remove}}
	case "link":
		in.SkillID = target
		if *scope == "" || *version < 1 {
			return errors.New("skill link requires --scope and --version")
		}
	case "update":
		in.MountID = target
		if *version < 1 {
			return errors.New("skill update requires --version")
		}
	case "unlink":
		in.MountID = target
	case "delete":
		in.SkillID = target
	default:
		return errors.New("unknown skill operation")
	}
	v, e := c.SkillAction(ctx, in)
	if e != nil {
		return e
	}
	return a.output(v)
}
