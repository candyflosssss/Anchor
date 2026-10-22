package skills

import (
	"context"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func setAssociation(v *manifest, path, id string, version int) {
	for i := range v.Sources {
		if v.Sources[i].Path == path {
			v.Sources[i] = association{path, id, version}
			return
		}
	}
	v.Sources = append(v.Sources, association{path, id, version})
}

func (m *Manager) importTree(v *manifest, src SkillSource, t tree, id, name string) error {
	known := src.SkillID != ""
	if !known && len(v.Sources) >= maxInventory {
		return ErrLimit
	}
	if id == "" {
		id = src.SkillID
	}
	s := findSkill(v, id)
	if s == nil {
		if id != "" {
			return state.ErrNotFound
		}
		if len(v.Library) >= maxInventory {
			return ErrLimit
		}
		if name == "" {
			name = src.Name
			if real, err := filepath.EvalSymlinks(src.Path); err == nil {
				name = filepath.Base(real)
			}
		}
		if !validName(name) {
			return state.ErrInvalid
		}
		v.Library = append(v.Library, Skill{ID: randomID(), Name: name, Versions: []SkillVersion{}})
		s = &v.Library[len(v.Library)-1]
	}
	if known && src.SkillID != s.ID {
		return state.ErrConflict
	}
	parent := 0
	if len(s.Versions) > 0 {
		parent = s.Versions[len(s.Versions)-1].Number
	}
	n, err := m.addVersion(v, s, t, parent, src.Baseline, "import")
	if err != nil {
		return err
	}
	setAssociation(v, src.Path, s.ID, n)
	remaining := v.Excluded[:0]
	for _, path := range v.Excluded {
		if path != src.Path {
			remaining = append(remaining, path)
		}
	}
	v.Excluded = remaining
	return nil
}

// Collection is an idempotent archive of previously unknown sources. It never
// edits an original, follows up a deletion, or auto-merges changed sources.
func (m *Manager) collect(ctx context.Context, v *manifest, env managed.Environment) []string {
	warnings := []string{}
	if v.Pending != nil || v.Cleanup != nil {
		return warnings
	}
	ws := m.workspaceContext(ctx, *v, env)
	for _, src := range ws.Sources {
		if ctx.Err() != nil {
			break
		}
		if src.SkillID != "" || src.Hash == "" || src.Status == "excluded" {
			continue
		}
		if len(v.Sources) >= maxInventory {
			warnings = append(warnings, "collect_limit:"+src.ID)
			continue
		}
		t, err := readSourceContext(ctx, src.Path)
		if err != nil || t.Hash != src.Hash {
			warnings = append(warnings, "collect_changed:"+src.ID)
			continue
		}
		// Work on a private index so a failed source cannot poison later sources.
		candidate := *v
		candidate.Library = append([]Skill{}, v.Library...)
		for i := range candidate.Library {
			candidate.Library[i].Versions = append([]SkillVersion{}, v.Library[i].Versions...)
		}
		candidate.Sources = append([]association{}, v.Sources...)
		candidate.Excluded = append([]string{}, v.Excluded...)
		real, _ := filepath.EvalSymlinks(src.Path)
		matched := false
		for _, a := range v.Sources {
			other, e := filepath.EvalSymlinks(a.Path)
			if e == nil && real != "" && other == real {
				src.Baseline = a.Version
				err = m.importTree(&candidate, src, t, a.SkillID, "")
				matched = true // A corrupt snapshot must not create a second identity.
				break
			}
		}
		if !matched {
			for _, s := range v.Library {
				for _, ver := range s.Versions {
					if ver.Hash == t.Hash && (s.Name == filepath.Base(real) || real == m.versionPath(s.ID, ver.Number)) {
						matched = true
						if _, err = m.version(&s, ver.Number); err == nil {
							setAssociation(&candidate, src.Path, s.ID, ver.Number)
						}
						break
					}
				}
				if matched {
					break
				}
			}
		}
		if !matched {
			err = m.importTree(&candidate, src, t, "", "")
		}
		if err != nil {
			warnings = append(warnings, "collect_failed:"+src.ID)
			continue
		}
		candidate.Revision++
		if err = m.persist(candidate); err != nil {
			warnings = append(warnings, "collect_failed:"+src.ID)
			continue
		}
		*v = candidate
	}
	return warnings
}
