package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func DefaultConfig() Config { return Config{Channel: "alpha"} }
func ReadConfig(dir string) (Config, error) {
	c := DefaultConfig()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return c, err
	}
	dir = abs
	b, err := readFile(filepath.Join(dir, "updates", "config.json"), maxMetadata)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if json.Unmarshal(b, &c) != nil || !ValidChannel(c.Channel) {
		return c, ErrInvalid
	}
	if c.ManifestURL != "" {
		if err := ValidateURL(c.ManifestURL); err != nil {
			return c, err
		}
	}
	return c, nil
}
func Configure(dir string, c Config) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	dir = abs
	if !ValidChannel(c.Channel) {
		return ErrInvalid
	}
	if c.ManifestURL != "" {
		if err := ValidateURL(c.ManifestURL); err != nil {
			return err
		}
	}
	lock, err := Lock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	return writeJSON(filepath.Join(dir, "updates", "config.json"), c)
}
func Lock(dir string) (*platform.Lock, error) {
	root := filepath.Join(dir, "updates")
	if err := platform.SecureDir(root); err != nil {
		return nil, err
	}
	l, err := platform.AcquireLock(filepath.Join(root, "update.lock"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	return l, nil
}
func readFile(path string, limit int64) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := platform.CheckManagedPath(filepath.Dir(path), path); err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrInvalid
	}
	return b, nil
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".update-*.partial")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := platform.SecureFile(f.Name()); err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return ErrInvalid
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return platform.ReplaceManagedFile(f.Name(), path)
}

type Download struct {
	Check Check  `json:"check"`
	Path  string `json:"path"`
}

func receiptPath(dir, product string) string {
	return filepath.Join(dir, "updates", product+"-download.json")
}
func artifactPath(dir string, c Check) string {
	// File names from the server never determine a directory or executable path.
	return filepath.Join(dir, "updates", c.Product+"-"+c.Version+"-"+c.Artifact.SHA256[:16]+filepath.Ext(c.Artifact.File))
}
func verify(path string, a Artifact) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() != a.Bytes {
		return ErrIntegrity
	}
	if err := platform.CheckManagedPath(filepath.Dir(path), path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, a.Bytes+1))
	if err != nil {
		return err
	}
	if n != a.Bytes || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return ErrIntegrity
	}
	return nil
}
func (s *Service) Download(ctx context.Context, dir string, c Check) (Download, error) {
	d := Download{Check: c}
	if c.Product != s.Product || c.Artifact == nil || !c.Available {
		return d, ErrUnavailable
	}
	if _, err := s.validate(Manifest{1, c.Product, c.Version, c.Channel, []Artifact{*c.Artifact}}, Config{Channel: c.Channel}); err != nil {
		return d, err
	}
	order, err := compareVersions(c.Version, s.Version)
	if err != nil || order <= 0 {
		return d, ErrUnavailable
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return d, err
	}
	lock, err := Lock(dir)
	if err != nil {
		return d, err
	}
	defer lock.Close()
	d.Path = artifactPath(dir, c)
	// Resume completed downloads only, never unverified partial content.
	if verify(d.Path, *c.Artifact) == nil {
		return d, writeJSON(receiptPath(dir, s.Product), d)
	}
	resp, err := s.get(ctx, c.Artifact.URL)
	if err != nil {
		return d, err
	}
	defer resp.Body.Close()
	if resp.ContentLength >= 0 && resp.ContentLength != c.Artifact.Bytes {
		return d, ErrIntegrity
	}
	f, err := os.CreateTemp(filepath.Join(dir, "updates"), ".download-*.partial")
	if err != nil {
		return d, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := platform.SecureFile(f.Name()); err != nil {
		return d, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, c.Artifact.Bytes+1))
	if err != nil {
		return d, errors.New("update download interrupted")
	}
	if n != c.Artifact.Bytes {
		return d, ErrIntegrity
	}
	if err := f.Sync(); err != nil {
		return d, err
	}
	if err := f.Close(); err != nil {
		return d, err
	}
	if err := verify(f.Name(), *c.Artifact); err != nil {
		return d, err
	}
	if st, err := os.Lstat(d.Path); err == nil && !st.Mode().IsRegular() {
		return d, ErrInvalid
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return d, err
	}
	if err := platform.ReplaceManagedFile(f.Name(), d.Path); err != nil {
		return d, err
	}
	return d, writeJSON(receiptPath(dir, s.Product), d)
}

// ReadDownload revalidates all metadata and hashes on every installation. A
// stale receipt, modified package, foreign platform, link, or downgrade fails.
func (s *Service) ReadDownload(dir string) (Download, error) {
	var d Download
	abs, err := filepath.Abs(dir)
	if err != nil {
		return d, err
	}
	b, err := readFile(receiptPath(abs, s.Product), maxMetadata)
	if err != nil {
		return d, err
	}
	if json.Unmarshal(b, &d) != nil || d.Check.Artifact == nil || !d.Check.Available {
		return d, ErrInvalid
	}
	c := d.Check
	if _, err := s.validate(Manifest{1, c.Product, c.Version, c.Channel, []Artifact{*c.Artifact}}, Config{Channel: c.Channel}); err != nil {
		return d, err
	}
	order, err := compareVersions(c.Version, s.Version)
	if err != nil || order <= 0 {
		return d, ErrUnavailable
	}
	if d.Path != artifactPath(abs, c) {
		return d, ErrInvalid
	}
	return d, verify(d.Path, *c.Artifact)
}
