package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func linkMatches(path, target string) bool {
	if sourceDir(filepath.Dir(path)) != nil {
		return false
	}
	st, err := os.Lstat(path)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		return false
	}
	to, err := os.Readlink(path)
	return err == nil && to == target
}
func absent(path string) bool { _, err := os.Lstat(path); return errors.Is(err, os.ErrNotExist) }
func (m *Manager) mountAction(v *manifest, env managed.Environment, in SkillAction) error {
	var before, after *SkillMount
	if in.Action == "link" {
		s := findSkill(v, in.SkillID)
		if s == nil {
			return state.ErrNotFound
		}
		if _, err := m.version(s, in.Version); err != nil {
			return err
		}
		var scope *SkillScope
		for _, sc := range scopes(*v, env) {
			if sc.ID == in.ScopeID {
				cp := sc
				scope = &cp
				break
			}
		}
		if scope == nil {
			return state.ErrNotFound
		}
		name := s.Name
		if in.Name != "" {
			name = in.Name
		}
		if !validName(name) {
			return state.ErrInvalid
		}
		if len(v.Mounts) >= maxInventory {
			return ErrLimit
		}
		path := filepath.Join(scope.Root, name)
		for _, mt := range v.Mounts {
			if mt.Path == path {
				return state.ErrConflict
			}
		}
		if !absent(scope.Root) && sourceDir(scope.Root) != nil {
			return ErrUnsafe
		}
		if !absent(path) {
			return state.ErrConflict
		}
		if err := os.MkdirAll(scope.Root, 0700); err != nil {
			return err
		}
		if sourceDir(scope.Root) != nil {
			return ErrUnsafe
		}
		after = &SkillMount{ID: key("mount-", path), SkillID: s.ID, Version: in.Version, ScopeID: scope.ID, Path: path, Target: m.versionPath(s.ID, in.Version), Status: "linked"}
	} else {
		for _, mt := range v.Mounts {
			if mt.ID == in.MountID {
				cp := mt
				before = &cp
				break
			}
		}
		if before == nil {
			return state.ErrNotFound
		}
		if in.Action == "unlink" && absent(before.Path) {
			remaining := []SkillMount{}
			for _, mt := range v.Mounts {
				if mt.ID != before.ID {
					remaining = append(remaining, mt)
				}
			}
			v.Mounts = remaining
			v.Revision++
			return m.persist(*v)
		}
		if !linkMatches(before.Path, before.Target) {
			return ErrChanged
		}
		if in.Action == "update" {
			s := findSkill(v, before.SkillID)
			if _, err := m.version(s, in.Version); err != nil {
				return err
			}
			cp := *before
			cp.Version = in.Version
			cp.Target = m.versionPath(s.ID, in.Version)
			after = &cp
			if after.Target == before.Target {
				return nil
			}
		}
	}
	backup := ""
	if before != nil {
		backup = filepath.Join(filepath.Dir(before.Path), ".anchor-skills-"+randomID())
	}
	v.Pending = &intent{Before: before, After: after, Backup: backup}
	if err := m.persist(*v); err != nil {
		return err
	}
	return m.recover(v)
}

// recover completes only a previously approved transaction. Every publication
// uses create-if-absent; an unexpected file is preserved, including if raced
// into the old mount slot. A quarantined old entry is never blindly deleted.
func (m *Manager) recover(v *manifest) error {
	p := v.Pending
	if p == nil {
		return nil
	}
	var mt *SkillMount
	if p.Before != nil {
		mt = p.Before
	} else {
		mt = p.After
	}
	if mt == nil {
		return ErrUnsafe
	}
	root := filepath.Dir(mt.Path)
	if sourceDir(root) != nil {
		return ErrUnsafe
	}
	if p.Before != nil {
		if filepath.Dir(p.Backup) != root || !strings.HasPrefix(filepath.Base(p.Backup), ".anchor-skills-") {
			return ErrUnsafe
		}
		if absent(p.Backup) {
			if p.After != nil && linkMatches(mt.Path, p.After.Target) { /* final link already published */
			} else {
				if !linkMatches(mt.Path, p.Before.Target) {
					return ErrChanged
				}
				if err := os.Rename(mt.Path, p.Backup); err != nil {
					return err
				}
			}
		}
		if !absent(p.Backup) && !linkMatches(p.Backup, p.Before.Target) {
			return ErrChanged
		}
	}
	if p.After != nil {
		s := findSkill(v, p.After.SkillID)
		if _, err := m.version(s, p.After.Version); err != nil {
			return err
		}
		if !linkMatches(p.After.Path, p.After.Target) {
			if !absent(p.After.Path) {
				return ErrChanged
			}
			if err := os.Symlink(p.After.Target, p.After.Path); err != nil {
				// Restore the previous link only into an empty slot. Keep the intent if
				// restoration is not possible; never overwrite an external replacement.
				restored := p.Before == nil && absent(p.After.Path)
				if p.Before != nil && absent(p.Before.Path) && linkMatches(p.Backup, p.Before.Target) {
					if os.Symlink(p.Before.Target, p.Before.Path) == nil {
						restored = true
					}
				}
				if restored {
					v.Pending = nil
					if e := m.persist(*v); e != nil {
						return e
					}
					if p.Before != nil && linkMatches(p.Backup, p.Before.Target) {
						_ = os.Remove(p.Backup)
					}
				}
				return ErrLink
			}
		}
	} else if !absent(mt.Path) {
		return ErrChanged
	}
	if err := platform.SyncManagedDir(root); err != nil {
		return err
	}
	mounts := []SkillMount{}
	for _, old := range v.Mounts {
		if p.Before == nil || old.ID != p.Before.ID {
			mounts = append(mounts, old)
		}
	}
	if p.After != nil {
		mounts = append(mounts, *p.After)
	}
	v.Mounts = mounts
	v.Pending = nil
	v.Revision++
	if err := m.persist(*v); err != nil {
		v.Pending = p
		return err
	}
	// Only remove the exact old symlink. A foreign replacement is left alone.
	if p.Before != nil && linkMatches(p.Backup, p.Before.Target) {
		_ = os.Remove(p.Backup)
		_ = platform.SyncManagedDir(root)
	}
	return nil
}
