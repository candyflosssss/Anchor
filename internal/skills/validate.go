package skills

import (
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

const maxIndexBytes = 32 << 20

func absolutePath(p string) bool {
	return len(p) <= 1024 && filepath.IsAbs(p) && filepath.Clean(p) == p && !strings.ContainsRune(p, 0)
}
func hexID(s string, size int) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == size && strings.ToLower(s) == s
}
func libraryID(s string) bool {
	return strings.HasPrefix(s, "skill-") && hexID(strings.TrimPrefix(s, "skill-"), 12)
}

// Disk metadata is an untrusted input too. In particular, never derive a
// library path or recover a pending link from unchecked IDs or target paths.
func (m *Manager) validate(v manifest) error {
	if v.Format != 1 || v.Revision < 1 || v.Revision >= 9007199254740991 ||
		v.Library == nil || v.Projects == nil || v.Sources == nil || v.Mounts == nil ||
		len(v.Library) > maxInventory || len(v.Projects) > 64 || len(v.Sources) > maxInventory || len(v.Mounts) > maxInventory {
		return ErrUnsafe
	}
	ids := map[string]bool{}
	for _, s := range v.Library {
		if !libraryID(s.ID) || ids[s.ID] || !validName(s.Name) || len(s.Versions) < 1 || len(s.Versions) > 64 {
			return ErrUnsafe
		}
		ids[s.ID] = true
		for i, ver := range s.Versions {
			if ver.Number != i+1 || ver.Parent < 0 || ver.Parent >= ver.Number || ver.Baseline < 0 || ver.Baseline >= ver.Number ||
				!hexID(ver.Hash, 32) || len(ver.Files) < 1 || len(ver.Files) > MaxFiles ||
				(ver.Origin != "import" && ver.Origin != "create" && ver.Origin != "save") {
				return ErrUnsafe
			}
			if _, err := time.Parse(time.RFC3339, ver.CreatedAt); err != nil {
				return ErrUnsafe
			}
			seen := map[string]bool{}
			total := int64(0)
			last := ""
			for _, f := range ver.Files {
				p := strings.ToLower(f.Path)
				if !validRelative(f.Path) || seen[p] || f.Path <= last || !hexID(f.Hash, 32) || f.Size < 0 || f.Size > MaxBytes {
					return ErrUnsafe
				}
				total += f.Size
				if total > MaxBytes {
					return ErrUnsafe
				}
				seen[p], last = true, f.Path
			}
			if !seen["skill.md"] {
				return ErrUnsafe
			}
			for _, f := range ver.Files {
				for p := filepath.ToSlash(filepath.Dir(strings.ToLower(f.Path))); p != "."; p = filepath.ToSlash(filepath.Dir(p)) {
					if seen[p] {
						return ErrUnsafe
					}
				}
			}
			t := tree{Files: append([]SkillFile{}, ver.Files...)}
			t.finish()
			if t.Hash != ver.Hash {
				return ErrUnsafe
			}
		}
	}
	paths := map[string]bool{}
	for _, p := range v.Projects {
		if !absolutePath(p) || p == filepath.VolumeName(p)+string(filepath.Separator) || paths[p] {
			return ErrUnsafe
		}
		paths[p] = true
	}
	paths = map[string]bool{}
	for _, a := range v.Sources {
		if !absolutePath(a.Path) || paths[a.Path] || findVersion(findSkill(&v, a.SkillID), a.Version) == nil {
			return ErrUnsafe
		}
		paths[a.Path] = true
	}
	checkMount := func(mt SkillMount) bool {
		return absolutePath(mt.Path) && validName(filepath.Base(mt.Path)) &&
			mt.ID == key("mount-", mt.Path) && mt.ScopeID == key("scope-", filepath.Dir(mt.Path)) &&
			findVersion(findSkill(&v, mt.SkillID), mt.Version) != nil && mt.Target == m.versionPath(mt.SkillID, mt.Version)
	}
	paths = map[string]bool{}
	for _, mt := range v.Mounts {
		if !checkMount(mt) || paths[mt.Path] {
			return ErrUnsafe
		}
		paths[mt.Path] = true
	}
	if p := v.Pending; p != nil {
		if p.Before == nil && p.After == nil {
			return ErrUnsafe
		}
		if p.Before != nil {
			if !checkMount(*p.Before) || !absolutePath(p.Backup) || filepath.Dir(p.Backup) != filepath.Dir(p.Before.Path) ||
				!strings.HasPrefix(filepath.Base(p.Backup), ".anchor-skills-") || !libraryID(strings.TrimPrefix(filepath.Base(p.Backup), ".anchor-skills-")) {
				return ErrUnsafe
			}
			found := false
			for _, mt := range v.Mounts {
				if mt == *p.Before {
					found = true
				}
			}
			if !found {
				return ErrUnsafe
			}
		} else if p.Backup != "" {
			return ErrUnsafe
		}
		if p.After != nil {
			if !checkMount(*p.After) {
				return ErrUnsafe
			}
			if p.Before == nil && paths[p.After.Path] {
				return ErrUnsafe
			}
			if p.Before != nil && (p.Before.Path != p.After.Path || p.Before.SkillID != p.After.SkillID) {
				return ErrUnsafe
			}
		}
	}
	if p := v.Cleanup; p != nil {
		if v.Pending != nil || !checkMount(p.Mount) || pathsOverlap(m.root, p.Mount.Path) || !hexID(p.Hash, 32) ||
			findVersion(findSkill(&v, p.Mount.SkillID), p.Mount.Version).Hash != p.Hash ||
			!absolutePath(p.Backup) || filepath.Dir(p.Backup) != filepath.Dir(p.Mount.Path) ||
			!strings.HasPrefix(filepath.Base(p.Backup), ".anchor-skills-cleanup-") ||
			!libraryID(strings.TrimPrefix(filepath.Base(p.Backup), ".anchor-skills-cleanup-")) {
			return ErrUnsafe
		}
		if paths[p.Mount.Path] != p.Published {
			return ErrUnsafe
		}
		if p.Published {
			found := false
			for _, mt := range v.Mounts {
				if mt == p.Mount {
					found = true
				}
			}
			if !found {
				return ErrUnsafe
			}
		} else {
			found := false
			for _, a := range v.Sources {
				if a.Path == p.Mount.Path && a.SkillID == p.Mount.SkillID && a.Version == p.Mount.Version {
					found = true
				}
			}
			if !found {
				return ErrUnsafe
			}
		}
	}
	for _, path := range v.Excluded {
		if !absolutePath(path) {
			return ErrUnsafe
		}
	}
	return nil
}

func indexBytes(v manifest) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > maxIndexBytes {
		return nil, ErrLimit
	}
	return b, nil
}
