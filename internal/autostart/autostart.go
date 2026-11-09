// Package autostart manages only Anchor's current-user desktop core login item.
package autostart

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

var (
	ErrUnsupported = errors.New("autostart is supported on Windows only")
	ErrInvalid     = errors.New("invalid autostart request or unavailable desktop installation")
	ErrConflict    = errors.New("autostart changed; read its status again")
	ErrUnknown     = errors.New("autostart write result unknown; read its status before retrying")
)

type AutostartState struct {
	Supported        bool   `json:"supported"`
	Enabled          bool   `json:"enabled"`
	MatchesCurrent   bool   `json:"matches_current"`
	DisabledBySystem bool   `json:"disabled_by_system"`
	CanEnable        bool   `json:"can_enable"`
	Revision         string `json:"revision"`
}
type Status = AutostartState
type SaveAutostart struct {
	Enabled          *bool  `json:"enabled"`
	ExpectedRevision string `json:"expected_revision"`
	Approved         bool   `json:"approved"`
}
type Save = SaveAutostart

func (in SaveAutostart) Validate() error {
	b, err := hex.DecodeString(in.ExpectedRevision)
	if in.Enabled == nil || !in.Approved || err != nil || len(b) != sha256.Size || strings.ToLower(in.ExpectedRevision) != in.ExpectedRevision {
		return ErrInvalid
	}
	return nil
}

type snapshot struct {
	Command  string
	Approval []byte
}
type backend interface {
	read() (snapshot, error)
	write(string) error
}
type Manager struct {
	Dir string
	// Executable is an optional CLI-only, explicit desktop path. The GUI never supplies a path.
	Executable      string
	store           backend
	commandOverride func() (string, error)
	lockOverride    func() (func(), error)
}

func New(dir, executable string) *Manager {
	return &Manager{Dir: dir, Executable: executable, store: nativeBackend()}
}

func revision(s snapshot) string {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (m *Manager) command() (string, error) {
	if m.commandOverride != nil {
		return m.commandOverride()
	}
	exe, err := desktopExecutable(m.Executable)
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(m.Dir)
	if err != nil {
		return "", ErrInvalid
	}
	command, err := windowsCommand(exe, dir)
	if err != nil {
		return "", err
	}
	st, err := state.Open(dir, false)
	if err != nil {
		return "", ErrInvalid
	}
	_, err = st.Settings()
	st.Close()
	if err != nil {
		return "", ErrInvalid
	}
	return command, nil
}
func (m *Manager) status(s snapshot) Status {
	command, err := m.command()
	blocked := len(s.Approval) > 0 && (s.Approval[0] == 3 || s.Approval[0] == 7)
	return Status{Supported: true, Enabled: s.Command != "", MatchesCurrent: err == nil && s.Command == command, DisabledBySystem: blocked, CanEnable: err == nil, Revision: revision(s)}
}
func (m *Manager) Status() (Status, error) {
	if m.store == nil {
		return Status{}, nil
	}
	s, err := m.store.read()
	if err != nil {
		return Status{}, err
	}
	return m.status(s), nil
}
func (m *Manager) lock() (func(), error) {
	if m.lockOverride != nil {
		return m.lockOverride()
	}
	// The Run value is per-user, not per-data-directory. All profiles share this lock.
	dir, err := state.DefaultDir()
	if err != nil {
		return nil, err
	}
	if err = platform.SecureDir(dir); err != nil {
		return nil, err
	}
	l, err := platform.AcquireLock(filepath.Join(dir, "autostart.lock"))
	if err != nil {
		return nil, ErrConflict
	}
	return func() { _ = l.Close() }, nil
}
func (m *Manager) Save(in Save) (Status, error) {
	if err := in.Validate(); err != nil {
		return Status{}, err
	}
	if m.store == nil {
		return Status{}, ErrUnsupported
	}
	unlock, err := m.lock()
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	old, err := m.store.read()
	if err != nil {
		return Status{}, err
	}
	if revision(old) != in.ExpectedRevision {
		return Status{}, ErrConflict
	}
	command := ""
	if *in.Enabled {
		command, err = m.command()
		if err != nil {
			return Status{}, err
		}
	}
	if command != old.Command {
		if err = m.store.write(command); err != nil {
			if errors.Is(err, os.ErrPermission) {
				return Status{}, os.ErrPermission
			}
			return Status{}, ErrUnknown
		}
	}
	current, err := m.store.read()
	if err != nil || current.Command != command {
		return Status{}, ErrUnknown
	}
	return m.status(current), nil
}

// Always quote paths, including the executable; use Windows CommandLineToArgvW rules.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		if r == '\\' {
			slashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat("\\", 2*slashes+1))
		} else {
			b.WriteString(strings.Repeat("\\", slashes))
		}
		b.WriteRune(r)
		slashes = 0
	}
	b.WriteString(strings.Repeat("\\", 2*slashes))
	b.WriteByte('"')
	return b.String()
}
func windowsCommand(exe, dir string) (string, error) {
	for _, p := range []string{exe, dir} {
		if !validWindowsPath(p) {
			return "", ErrInvalid
		}
	}
	command := quote(exe) + " --data-dir " + quote(dir) + " --core"
	// The Windows Run key contract limits command lines to 260 characters.
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", ErrInvalid
	}
	return command, nil
}

func validWindowsPath(p string) bool {
	return len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && (p[2] == '\\' || p[2] == '/') && !strings.ContainsAny(p, "\x00\r\n\"")
}
