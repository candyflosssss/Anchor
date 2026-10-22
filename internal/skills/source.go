package skills

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Sources are readable user content, not Anchor's private configuration. Do not
// chmod them or apply the private-store ownership/group-write requirements.
func sourceDir(path string) error {
	if !absolutePath(path) {
		return ErrUnsafe
	}
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return ErrUnsafe
	}
	return nil
}

func readSource(root string) (tree, error) { return readSourceContext(context.Background(), root) }

func readSourceContext(ctx context.Context, root string) (tree, error) {
	t := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
	if err := sourceDir(root); err != nil {
		return t, err
	}
	count, total := 0, int64(0)
	folded, stack := map[string]bool{}, map[string]bool{}
	var walk func(string, string, int) error
	walk = func(path, rel string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 64 {
			return ErrLimit
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return ErrUnsafe
		}
		st, err := os.Stat(resolved)
		if err != nil {
			return ErrUnsafe
		}
		if rel != "" {
			count++
			if count > MaxFiles*2 {
				return ErrLimit
			}
			if !validRelative(rel) || folded[strings.ToLower(rel)] {
				return ErrUnsafe
			}
			folded[strings.ToLower(rel)] = true
		}
		if st.IsDir() {
			if stack[resolved] {
				return ErrUnsafe
			}
			stack[resolved] = true
			defer delete(stack, resolved)
			f, err := os.Open(resolved)
			if err != nil {
				return ErrUnsafe
			}
			entries, readErr := f.ReadDir(MaxFiles*2 + 1)
			f.Close()
			if len(entries) > MaxFiles*2 {
				return ErrLimit
			}
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return ErrUnsafe
			}
			for _, entry := range entries {
				child := entry.Name()
				if rel != "" {
					child = rel + "/" + child
				}
				if err := walk(filepath.Join(resolved, entry.Name()), child, depth+1); err != nil {
					return err
				}
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(st, after) || !st.ModTime().Equal(after.ModTime()) {
				return ErrChanged
			}
			return nil
		}
		if !st.Mode().IsRegular() {
			return ErrUnsafe
		}
		if st.Size() > MaxBytes-total || len(t.Files) >= MaxFiles {
			return ErrLimit
		}
		f, err := os.Open(resolved)
		if err != nil {
			return ErrUnsafe
		}
		opened, err := f.Stat()
		if err != nil || !os.SameFile(st, opened) {
			f.Close()
			return ErrChanged
		}
		b, err := io.ReadAll(io.LimitReader(contextReader{ctx, f}, MaxBytes-total+1))
		f.Close()
		after, ae := os.Stat(path)
		if err != nil || ae != nil || !os.SameFile(st, after) || int64(len(b)) != after.Size() || !st.ModTime().Equal(after.ModTime()) {
			return ErrChanged
		}
		total += int64(len(b))
		if total > MaxBytes {
			return ErrLimit
		}
		t.Files = append(t.Files, SkillFile{rel, digest(b), int64(len(b)), executable(st.Mode())})
		t.Data[rel] = b
		return nil
	}
	if err := walk(root, "", 0); err != nil {
		return t, err
	}
	if _, ok := t.Data["SKILL.md"]; !ok {
		return t, ErrUnsafe
	}
	t.finish()
	return t, nil
}

// Check cancellation between bounded file reads, including a core shutdown.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(b) > 64<<10 {
		b = b[:64<<10]
	}
	return r.r.Read(b)
}
