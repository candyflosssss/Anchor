package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/candyflosssss/Anchor/internal/platform"
)

type tree struct {
	Files []SkillFile
	Data  map[string][]byte
	Hash  string
}

func digest(b []byte) string         { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func key(prefix, path string) string { return prefix + digest([]byte(filepath.Clean(path)))[:20] }
func validName(s string) bool {
	if len(s) < 1 || len(s) > 64 || s == "." || s == ".." || strings.HasSuffix(s, ".") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	switch strings.ToUpper(strings.Split(s, ".")[0]) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}
func validRelative(p string) bool {
	if p == "" || len(p) > 512 || strings.ContainsAny(p, "\\:\x00") || strings.HasPrefix(p, "/") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if !validName(part) {
			return false
		}
	}
	return true
}

// CheckManagedPath also checks ownership, reparse points and symlink ancestors.
// Missing directories are checked through their nearest existing ancestor.
func safeDir(path string, missing bool) error {
	if !absolutePath(path) {
		return ErrUnsafe
	}
	p := path
	for {
		st, err := os.Lstat(p)
		if err == nil {
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return ErrUnsafe
			}
			if platform.CheckManagedPath(p, filepath.Join(p, ".anchor-skill-path-check")) != nil {
				return ErrUnsafe
			}
			return nil
		}
		if !missing || !errors.Is(err, os.ErrNotExist) || filepath.Dir(p) == p {
			return ErrUnsafe
		}
		p = filepath.Dir(p)
	}
}
func readTree(root string) (tree, error) { return readTreeLimit(root, MaxBytes) }
func readTreeLimit(root string, limit int64) (tree, error) {
	t := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
	if err := safeDir(root, false); err != nil {
		return t, err
	}
	count, total := 0, int64(0)
	folded := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return ErrUnsafe
		}
		if path == root {
			return nil
		}
		count++
		if count > MaxFiles*2 {
			return ErrLimit
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return ErrUnsafe
		}
		rel = filepath.ToSlash(rel)
		if !validRelative(rel) || folded[strings.ToLower(rel)] {
			return ErrUnsafe
		}
		folded[strings.ToLower(rel)] = true
		if d.Type()&os.ModeSymlink != 0 {
			return ErrUnsafe
		}
		if d.IsDir() {
			return safeDir(path, false)
		}
		if !d.Type().IsRegular() || platform.CheckManagedPath(filepath.Dir(path), path) != nil {
			return ErrUnsafe
		}
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() {
			return ErrUnsafe
		}
		if st.Size() > limit || total+st.Size() > limit || len(t.Files) >= MaxFiles {
			return ErrLimit
		}
		f, e := os.Open(path)
		if e != nil {
			return ErrUnsafe
		}
		opened, e := f.Stat()
		if e != nil || !os.SameFile(st, opened) {
			f.Close()
			return ErrChanged
		}
		b, e := io.ReadAll(io.LimitReader(f, limit-total+1))
		f.Close()
		after, ae := os.Lstat(path)
		if e != nil || ae != nil || !os.SameFile(st, after) || after.Size() != int64(len(b)) || !after.ModTime().Equal(st.ModTime()) {
			return ErrChanged
		}
		total += int64(len(b))
		if total > limit {
			return ErrLimit
		}
		t.Files = append(t.Files, SkillFile{rel, digest(b), int64(len(b)), executable(st.Mode())})
		t.Data[rel] = b
		return nil
	})
	if err != nil {
		return t, err
	}
	if _, ok := t.Data["SKILL.md"]; !ok {
		return t, ErrUnsafe
	}
	t.finish()
	return t, nil
}
func (t *tree) finish() {
	sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })
	b, _ := json.Marshal(t.Files)
	t.Hash = digest(b)
}
func writeTree(root string, t tree) error {
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	for _, f := range t.Files {
		path := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		mode := os.FileMode(0400)
		if f.Executable {
			mode = 0500
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, err = out.Write(t.Data[f.Path])
		if err == nil {
			err = out.Sync()
		}
		ce := out.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
	}
	return platform.SyncManagedDir(root)
}
func edited(base tree, edits []SkillEdit) (tree, error) {
	if len(edits) == 0 || len(edits) > MaxFiles {
		return tree{}, ErrUnsafe
	}
	data := map[string][]byte{}
	exec := map[string]bool{}
	for _, f := range base.Files {
		data[f.Path] = base.Data[f.Path]
		exec[f.Path] = f.Executable
	}
	seen := map[string]bool{}
	for _, e := range edits {
		if !validRelative(e.Path) || seen[e.Path] || len(e.Content) > MaxText || !utf8.ValidString(e.Content) || strings.ContainsRune(e.Content, 0) {
			return tree{}, ErrUnsafe
		}
		seen[e.Path] = true
		if e.Delete {
			delete(data, e.Path)
			delete(exec, e.Path)
		} else {
			data[e.Path] = []byte(e.Content)
			exec[e.Path] = executableFlag(e.Executable)
		}
	}
	if _, ok := data["SKILL.md"]; !ok {
		return tree{}, ErrUnsafe
	}
	t := tree{Files: []SkillFile{}, Data: data}
	total := 0
	// A file cannot also be an ancestor of another file (portable across OSes).
	folded := map[string]bool{}
	for p, b := range data {
		total += len(b)
		if total > MaxBytes || len(data) > MaxFiles {
			return t, ErrLimit
		}
		low := strings.ToLower(p)
		if folded[low] {
			return t, ErrUnsafe
		}
		folded[low] = true
		t.Files = append(t.Files, SkillFile{p, digest(b), int64(len(b)), exec[p]})
	}
	for p := range folded {
		for dir := filepath.ToSlash(filepath.Dir(p)); dir != "."; dir = filepath.ToSlash(filepath.Dir(dir)) {
			if folded[dir] {
				return t, ErrUnsafe
			}
		}
	}
	t.finish()
	return t, nil
}
func text(b []byte) (string, bool, bool) {
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return "", true, false
	}
	if len(b) > 4096 {
		b = b[:4096]
		for !utf8.Valid(b) {
			b = b[:len(b)-1]
		}
		return string(b), false, true
	}
	return string(b), false, false
}
func differences(before, after tree) []SkillDifference {
	result := []SkillDifference{}
	all := map[string]bool{}
	bm := map[string]SkillFile{}
	am := map[string]SkillFile{}
	for _, f := range before.Files {
		all[f.Path] = true
		bm[f.Path] = f
	}
	for _, f := range after.Files {
		all[f.Path] = true
		am[f.Path] = f
	}
	paths := []string{}
	for p := range all {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	previewBudget := 128 << 10
	for _, p := range paths {
		b, bok := bm[p]
		a, aok := am[p]
		if bok && aok && b.Hash == a.Hash && b.Executable == a.Executable {
			continue
		}
		d := SkillDifference{Path: p, Change: "modified"}
		if !bok {
			d.Change = "added"
		}
		if !aok {
			d.Change = "deleted"
		}
		bt, bb, btr := text(before.Data[p])
		at, ab, atr := text(after.Data[p])
		if len(bt)+len(at) > previewBudget {
			bt, at, btr, atr = "", "", true, true
		}
		previewBudget -= len(bt) + len(at)
		d.Before = bt
		d.After = at
		d.Binary = bb || ab
		d.Truncated = btr || atr
		result = append(result, d)
	}
	return result
}
