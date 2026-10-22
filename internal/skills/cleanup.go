package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (m *Manager) cleanupSource(v *manifest, env managed.Environment, in SkillAction) error {
	ws := m.workspace(*v, env)
	var src *SkillSource
	for i := range ws.Sources {
		if ws.Sources[i].ID == in.SourceID {
			src = &ws.Sources[i]
			break
		}
	}
	if src == nil {
		return state.ErrNotFound
	}
	if len(v.Mounts) >= maxInventory {
		return ErrLimit
	}
	t, err := readSource(src.Path)
	if err != nil {
		return err
	}
	if in.ExpectedHash == "" || t.Hash != in.ExpectedHash {
		return ErrChanged
	}
	link := ""
	if st, e := os.Lstat(src.Path); e == nil && st.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(src.Path)
		if err != nil {
			return err
		}
	}
	// Neither the private store nor a directory containing it may be replaced.
	real, err := filepath.EvalSymlinks(src.Path)
	if err != nil {
		return ErrUnsafe
	}
	store, err := filepath.EvalSymlinks(m.root)
	if err != nil || pathsOverlap(m.root, src.Path) || link == "" && pathsOverlap(store, real) {
		return ErrUnsafe
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(src.Path))
	if err != nil || pathWithin(store, parent) {
		return ErrUnsafe
	}
	if err := m.importTree(v, *src, t, "", ""); err != nil {
		return err
	}
	var a association
	for _, item := range v.Sources {
		if item.Path == src.Path {
			a = item
			break
		}
	}
	// Verify the archived bytes from disk, not only the just-computed source hash.
	archived, err := m.version(findSkill(v, a.SkillID), a.Version)
	if err != nil {
		return err
	}
	if archived.Hash != t.Hash {
		return ErrChanged
	}
	v.Cleanup = &cleanupIntent{
		Mount:  SkillMount{ID: key("mount-", src.Path), SkillID: a.SkillID, Version: a.Version, ScopeID: src.ScopeID, Path: src.Path, Target: m.versionPath(a.SkillID, a.Version), Status: "linked"},
		Backup: filepath.Join(filepath.Dir(src.Path), ".anchor-skills-cleanup-"+randomID()), Hash: t.Hash, OriginalLink: link,
	}
	if err := m.persist(*v); err != nil {
		return err
	}
	return m.recoverCleanup(v)
}

func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cleanupOriginalMatches(path string, p *cleanupIntent) bool {
	st, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if p.OriginalLink != "" {
		target, err := os.Readlink(path)
		if err != nil || target != p.OriginalLink {
			return false
		}
	} else if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return false
	}
	t, err := readSource(path)
	return err == nil && t.Hash == p.Hash
}

func (m *Manager) recoverCleanup(v *manifest) error {
	p := v.Cleanup
	if p == nil {
		return nil
	}
	root := filepath.Dir(p.Mount.Path)
	if sourceDir(root) != nil {
		return ErrUnsafe
	}
	store, err := filepath.EvalSymlinks(m.root)
	if err != nil {
		return ErrUnsafe
	}
	parent, err := filepath.EvalSymlinks(root)
	if err != nil || pathWithin(store, parent) {
		return ErrUnsafe
	}
	archived, err := m.version(findSkill(v, p.Mount.SkillID), p.Mount.Version)
	if err != nil {
		return err
	}
	if archived.Hash != p.Hash {
		return ErrChanged
	}
	if !p.Published {
		if absent(p.Backup) {
			if !cleanupOriginalMatches(p.Mount.Path, p) {
				return ErrChanged
			}
			if err := os.Rename(p.Mount.Path, p.Backup); err != nil {
				return err
			}
			if err := platform.SyncManagedDir(root); err != nil {
				return err
			}
		}
		if !cleanupOriginalMatches(p.Backup, p) {
			// A writer changed the original during publication. Restore it only to
			// an empty slot; never discard either this copy or a foreign replacement.
			if absent(p.Mount.Path) {
				if err := os.Rename(p.Backup, p.Mount.Path); err == nil {
					v.Cleanup = nil
					v.Revision++
					if err := m.persist(*v); err != nil {
						return err
					}
				}
			}
			return ErrChanged
		}
		if !linkMatches(p.Mount.Path, p.Mount.Target) {
			if !absent(p.Mount.Path) {
				return ErrChanged
			}
			if err := os.Symlink(p.Mount.Target, p.Mount.Path); err != nil {
				if absent(p.Mount.Path) && os.Rename(p.Backup, p.Mount.Path) == nil {
					v.Cleanup = nil
					v.Revision++
					if err := m.persist(*v); err != nil {
						return err
					}
				}
				return ErrLink
			}
		}
		if err := platform.SyncManagedDir(root); err != nil {
			return err
		}
		v.Mounts = append(v.Mounts, p.Mount)
		sources := []association{}
		for _, a := range v.Sources {
			if a.Path != p.Mount.Path {
				sources = append(sources, a)
			}
		}
		v.Sources = sources
		p.Published = true
		v.Revision++
		if err := m.persist(*v); err != nil {
			return err
		}
	}
	// The link and mount index are now durable. Only delete the exact archived
	// original. For a source symlink this removes the link, never its target.
	if !linkMatches(p.Mount.Path, p.Mount.Target) {
		return ErrChanged
	}
	if !absent(p.Backup) {
		if !cleanupOriginalMatches(p.Backup, p) {
			return ErrChanged
		}
		if p.OriginalLink != "" {
			err = os.Remove(p.Backup)
		} else {
			err = os.RemoveAll(p.Backup)
		}
		if err != nil {
			return err
		}
		if err := platform.SyncManagedDir(root); err != nil {
			return err
		}
	}
	v.Cleanup = nil
	return m.persist(*v)
}
