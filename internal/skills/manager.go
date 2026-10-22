package skills

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

type Manager struct {
	root string
	mu   sync.Mutex
}

func New(dataDir string) *Manager { return &Manager{root: filepath.Join(dataDir, "skills")} }
func (m *Manager) load() (manifest, error) {
	v := manifest{Format: 1, Revision: 1, Library: []Skill{}, Projects: []string{}, Sources: []association{}, Mounts: []SkillMount{}}
	if err := safeDir(m.root, true); err != nil {
		return v, err
	}
	path := filepath.Join(m.root, "index.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if platform.CheckManagedPath(m.root, path) != nil {
		return v, ErrUnsafe
	}
	f, err := os.Open(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxIndexBytes+1))
	if err != nil || len(b) > maxIndexBytes {
		return v, ErrLimit
	}
	if json.Unmarshal(b, &v) != nil || m.validate(v) != nil {
		return v, ErrUnsafe
	}
	return v, nil
}
func (m *Manager) persist(v manifest) error {
	if err := platform.SecureDir(m.root); err != nil {
		return err
	}
	path := filepath.Join(m.root, "index.json")
	if platform.CheckManagedPath(m.root, path) != nil {
		return ErrUnsafe
	}
	if err := m.validate(v); err != nil {
		return err
	}
	b, err := indexBytes(v)
	if err != nil {
		return err
	}
	if len(b) > maxIndexBytes {
		return ErrLimit
	}
	f, err := os.CreateTemp(m.root, ".index-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = platform.ReplaceManagedFile(name, path); err != nil {
		return err
	}
	return platform.SyncManagedDir(m.root)
}
func (m *Manager) versionPath(id string, n int) string {
	return filepath.Join(m.root, "library", id, versionName(n))
}
func versionName(n int) string { return "v" + itoa(n) }
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := [20]byte{}
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte(n%10) + '0'
		n /= 10
	}
	return string(b[i:])
}
func randomID() string {
	b := make([]byte, 12)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return "skill-" + hex.EncodeToString(b)
}
func findSkill(v *manifest, id string) *Skill {
	for i := range v.Library {
		if v.Library[i].ID == id {
			return &v.Library[i]
		}
	}
	return nil
}
func findVersion(s *Skill, n int) *SkillVersion {
	if s == nil {
		return nil
	}
	for i := range s.Versions {
		if s.Versions[i].Number == n {
			return &s.Versions[i]
		}
	}
	return nil
}
func (m *Manager) version(s *Skill, n int) (tree, error) {
	v := findVersion(s, n)
	if v == nil {
		return tree{}, state.ErrNotFound
	}
	t, err := readTree(m.versionPath(s.ID, n))
	if err == nil && t.Hash != v.Hash {
		err = ErrChanged
	}
	return t, err
}
func (m *Manager) addVersion(v *manifest, s *Skill, t tree, parent, baseline int, origin string) (int, error) {
	// Reuse exact snapshots, including executable bits.
	for _, v := range s.Versions {
		if v.Hash == t.Hash {
			if _, err := m.version(s, v.Number); err != nil {
				return 0, err
			}
			return v.Number, nil
		}
	}
	if len(s.Versions) >= 64 {
		return 0, ErrLimit
	}
	n := len(s.Versions) + 1
	candidate := SkillVersion{Number: n, Parent: parent, Baseline: baseline, Hash: t.Hash, CreatedAt: time.Now().UTC().Format(time.RFC3339), Origin: origin, Files: t.Files}
	s.Versions = append(s.Versions, candidate)
	b, sizeErr := indexBytes(*v)
	s.Versions = s.Versions[:len(s.Versions)-1]
	// Reserve room for an import association/revision before publishing files.
	if sizeErr != nil || len(b) > maxIndexBytes-8192 {
		return 0, ErrLimit
	}
	dir := filepath.Dir(m.versionPath(s.ID, n))
	if err := platform.SecureDir(dir); err != nil {
		return 0, err
	}
	stage, err := os.MkdirTemp(dir, ".snapshot-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(stage)
	content := filepath.Join(stage, "content")
	if err = writeTree(content, t); err != nil {
		return 0, err
	}
	// A previous crash may leave an unpublished version. Never overwrite it;
	// reuse only if the content is identical, otherwise report integrity conflict.
	dst := m.versionPath(s.ID, n)
	if _, err = os.Lstat(dst); err == nil {
		existing, e := readTree(dst)
		if e != nil || existing.Hash != t.Hash {
			return 0, ErrChanged
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err = os.Rename(content, dst); err != nil {
			return 0, err
		}
	} else {
		return 0, err
	}
	if err = platform.SyncManagedDir(dir); err != nil {
		return 0, err
	}
	s.Versions = append(s.Versions, candidate)
	return n, nil
}
func (m *Manager) Workspace(env managed.Environment) (SkillWorkspace, error) {
	return m.WorkspaceContext(context.Background(), env)
}

func (m *Manager) WorkspaceContext(ctx context.Context, env managed.Environment) (SkillWorkspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.load()
	if err != nil {
		return SkillWorkspace{}, err
	}
	if v.Pending != nil || v.Cleanup != nil {
		return m.workspaceContext(ctx, v, env), nil
	}
	lock, err := m.acquireWriteLock()
	if err != nil {
		return SkillWorkspace{}, err
	}
	defer lock.Close()
	v, err = m.load()
	if err != nil {
		return SkillWorkspace{}, err
	}
	warnings := m.collect(ctx, &v, env)
	if err := ctx.Err(); err != nil {
		return SkillWorkspace{}, err
	}
	ws := m.workspaceContext(ctx, v, env)
	ws.Warnings = append(ws.Warnings, warnings...)
	return ws, nil
}
func (m *Manager) acquireWriteLock() (*platform.Lock, error) {
	if err := platform.SecureDir(m.root); err != nil {
		return nil, err
	}
	if err := platform.SecureFile(filepath.Join(m.root, "write.lock")); err != nil {
		return nil, err
	}
	lock, err := platform.AcquireLock(filepath.Join(m.root, "write.lock"))
	if err != nil {
		return nil, state.ErrConflict
	}
	return lock, nil
}
func (m *Manager) Action(env managed.Environment, in SkillAction) (SkillWorkspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !in.Approved || in.ExpectedRevision < 1 || in.ExpectedRevision >= 9007199254740991 {
		return SkillWorkspace{}, state.ErrInvalid
	}
	if err := platform.SecureDir(m.root); err != nil {
		return SkillWorkspace{}, err
	}
	if err := platform.SecureFile(filepath.Join(m.root, "write.lock")); err != nil {
		return SkillWorkspace{}, err
	}
	lock, err := platform.AcquireLock(filepath.Join(m.root, "write.lock"))
	if err != nil {
		return SkillWorkspace{}, state.ErrConflict
	}
	defer lock.Close()
	v, err := m.load()
	if err != nil {
		return SkillWorkspace{}, err
	}
	if v.Pending != nil {
		if err = m.recover(&v); err != nil {
			return SkillWorkspace{}, err
		}
	}
	if v.Cleanup != nil {
		if err = m.recoverCleanup(&v); err != nil {
			return SkillWorkspace{}, err
		}
	}
	if v.Revision != in.ExpectedRevision {
		return SkillWorkspace{}, state.ErrConflict
	}
	switch in.Action {
	case "register-project":
		p := filepath.Clean(in.ProjectPath)
		if in.ProjectPath != p || sourceDir(p) != nil || p == filepath.VolumeName(p)+string(filepath.Separator) {
			return SkillWorkspace{}, state.ErrInvalid
		}
		exists := false
		for _, q := range v.Projects {
			if q == p {
				exists = true
			}
		}
		if !exists {
			if len(v.Projects) >= 64 {
				return SkillWorkspace{}, ErrLimit
			}
			v.Projects = append(v.Projects, p)
		}
	case "import":
		ws := m.workspace(v, env)
		var src *SkillSource
		for i := range ws.Sources {
			if ws.Sources[i].ID == in.SourceID {
				src = &ws.Sources[i]
				break
			}
		}
		if src == nil {
			return SkillWorkspace{}, state.ErrNotFound
		}
		t, e := readSource(src.Path)
		if e != nil {
			return SkillWorkspace{}, e
		}
		if in.ExpectedHash == "" || t.Hash != in.ExpectedHash {
			return SkillWorkspace{}, ErrChanged
		}
		if e = m.importTree(&v, *src, t, in.SkillID, in.Name); e != nil {
			return SkillWorkspace{}, e
		}
	case "create", "save":
		s := findSkill(&v, in.SkillID)
		base := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
		parent := 0
		if in.Action == "create" {
			if in.SkillID != "" || !validName(in.Name) {
				return SkillWorkspace{}, state.ErrInvalid
			}
			if len(v.Library) >= maxInventory {
				return SkillWorkspace{}, ErrLimit
			}
			v.Library = append(v.Library, Skill{ID: randomID(), Name: in.Name, Versions: []SkillVersion{}})
			s = &v.Library[len(v.Library)-1]
		} else {
			if s == nil {
				return SkillWorkspace{}, state.ErrNotFound
			}
			parent = in.Version
			if parent != s.Versions[len(s.Versions)-1].Number {
				return SkillWorkspace{}, state.ErrConflict
			}
			base, err = m.version(s, parent)
			if err != nil {
				return SkillWorkspace{}, err
			}
		}
		t, e := edited(base, in.Edits)
		if e != nil {
			return SkillWorkspace{}, e
		}
		if _, e = m.addVersion(&v, s, t, parent, parent, in.Action); e != nil {
			return SkillWorkspace{}, e
		}
	case "cleanup":
		if err = m.cleanupSource(&v, env, in); err != nil {
			return SkillWorkspace{}, err
		}
		return m.workspace(v, env), nil
	case "link", "update", "unlink":
		if err = m.mountAction(&v, env, in); err != nil {
			return SkillWorkspace{}, err
		}
		return m.workspace(v, env), nil
	case "delete":
		s := findSkill(&v, in.SkillID)
		if s == nil {
			return SkillWorkspace{}, state.ErrNotFound
		}
		for _, mt := range v.Mounts {
			if mt.SkillID == s.ID {
				return SkillWorkspace{}, state.ErrReferenced
			}
		}
		// Remove the index entry, not arbitrary paths. Unreferenced immutable files
		// remain as recovery data; this first version has no automatic garbage collector.
		for i := range v.Library {
			if v.Library[i].ID == s.ID {
				v.Library = append(v.Library[:i], v.Library[i+1:]...)
				break
			}
		}
		sources := v.Sources[:0]
		for _, a := range v.Sources {
			if a.SkillID == s.ID {
				v.Excluded = append(v.Excluded, a.Path)
			}
			if a.SkillID != s.ID {
				sources = append(sources, a)
			}
		}
		v.Sources = sources
	default:
		return SkillWorkspace{}, state.ErrInvalid
	}
	v.Revision++
	if err = m.persist(v); err != nil {
		return SkillWorkspace{}, err
	}
	warnings := m.collect(context.Background(), &v, env)
	ws := m.workspace(v, env)
	ws.Warnings = append(ws.Warnings, warnings...)
	return ws, nil
}
func (m *Manager) Detail(env managed.Environment, in SkillQuery) (SkillDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, err := m.load()
	if err != nil {
		return SkillDetail{}, err
	}
	out := SkillDetail{SkillID: in.SkillID, Version: in.Version, SourceID: in.SourceID, Files: []SkillFile{}, Differences: []SkillDifference{}}
	var current tree
	before := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
	if in.SourceID != "" {
		ws := m.workspace(v, env)
		var src *SkillSource
		for i := range ws.Sources {
			if ws.Sources[i].ID == in.SourceID {
				src = &ws.Sources[i]
				break
			}
		}
		if src == nil {
			return out, state.ErrNotFound
		}
		current, err = readSource(src.Path)
		if err != nil {
			return out, err
		}
		out.SkillID = src.SkillID
		if src.SkillID != "" {
			n := src.Baseline
			if in.AgainstVersion > 0 {
				n = in.AgainstVersion
			}
			before, err = m.version(findSkill(&v, src.SkillID), n)
			if err != nil {
				return out, err
			}
			out.Version = n
		}
	} else {
		s := findSkill(&v, in.SkillID)
		current, err = m.version(s, in.Version)
		if err != nil {
			return out, err
		}
		if in.AgainstVersion > 0 {
			before, err = m.version(s, in.AgainstVersion)
			if err != nil {
				return out, err
			}
		}
	}
	out.Hash = current.Hash
	out.Files = current.Files
	out.Differences = differences(before, current)
	if in.File != "" {
		if !validRelative(in.File) {
			return out, state.ErrInvalid
		}
		b, ok := current.Data[in.File]
		if !ok {
			return out, state.ErrNotFound
		}
		out.File = in.File
		if len(b) <= MaxText {
			s, binary, _ := text(b)
			_ = s
			if !binary {
				out.Content = string(b)
				out.Editable = true
			}
		}
	}
	return out, nil
}
