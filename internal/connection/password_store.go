package connection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/zalando/go-keyring"
)

const passwordService = "Anchor.SSH.Password.v1"

// PasswordVault exposes only individual credential operations. Get must return
// keyring.ErrNotFound for a missing item. Tests inject an isolated vault;
// production uses the current user's OS keyring.
type PasswordVault interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
	Delete(string, string) error
}
type systemPasswordVault struct{}

func (systemPasswordVault) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}
func (systemPasswordVault) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}
func (systemPasswordVault) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

type Passwords struct{ Vault PasswordVault }

func (p Passwords) vault() PasswordVault {
	if p.Vault != nil {
		return p.Vault
	}
	return systemPasswordVault{}
}
func passwordRef(s *state.Store, c state.Context) (string, error) {
	if c.Validate() != nil || c.Transport != "ssh" || c.SSHAuth != "password" || c.SSHHostKey == "" {
		return "", failure("invalid")
	}
	dir, err := filepath.Abs(s.Dir)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	port := c.SSHPort
	if port == 0 {
		port = 22
	}
	raw, _ := json.Marshal([]any{dir, c.Name, c.SSHTarget, port, c.SSHHostKey, c.InstanceID})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// Saved reads only non-secret metadata, so listing devices never unlocks a vault.
func (p Passwords) Saved(s *state.Store, c state.Context) (bool, error) {
	if c.SSHAuth != "password" {
		return false, nil
	}
	want, err := passwordRef(s, c)
	if err != nil {
		return false, err
	}
	ref, err := s.ContextPasswordRef(c.Name)
	return ref == want, err
}
func (p Passwords) Open(ctx context.Context, s *state.Store, c state.Context, password string) (*Live, error) {
	if password == "" && c.SSHAuth == "password" {
		saved, err := p.Saved(s, c)
		if err != nil {
			return nil, err
		}
		if saved {
			ref, _ := passwordRef(s, c)
			password, err = p.vault().Get(passwordService, ref)
			if errors.Is(err, keyring.ErrNotFound) {
				return nil, failure("ssh_auth")
			}
			if err != nil {
				return nil, failure("credential_store")
			}
		}
	}
	return OpenWithPassword(ctx, s, c, password)
}

// Save is called only after this password authenticated the pinned, ready target.
func (p Passwords) Save(s *state.Store, c state.Context, password string) error {
	if password == "" || len(password) > 8192 {
		return failure("invalid")
	}
	ref, err := passwordRef(s, c)
	if err != nil {
		return err
	}
	current, err := s.Context(c.Name)
	if err != nil {
		return err
	}
	currentRef, err := passwordRef(s, current)
	if err != nil || currentRef != ref {
		return failure("conflict")
	}
	if err := p.vault().Set(passwordService, ref, password); err != nil {
		return failure("credential_store")
	}
	// Some Secret Service providers complete a dismissed prompt without an
	// error. Verify the result before marking the password as saved.
	if stored, err := p.vault().Get(passwordService, ref); err != nil || stored != password {
		return failure("credential_store")
	}
	if err := s.SetContextPasswordRef(c.Name, ref); err != nil {
		// Fail closed: never fall back to plaintext, even if metadata cannot be saved.
		_ = p.vault().Delete(passwordService, ref)
		return failure("credential_store")
	}
	return nil
}
func (p Passwords) Forget(s *state.Store, c state.Context) error {
	ref, err := s.ContextPasswordRef(c.Name)
	if err != nil {
		return err
	}
	if ref == "" {
		return nil
	}
	if err := p.vault().Delete(passwordService, ref); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return failure("credential_store")
	}
	// A canceled system prompt must never be reported as a successful forget.
	if _, err := p.vault().Get(passwordService, ref); !errors.Is(err, keyring.ErrNotFound) {
		return failure("credential_store")
	}
	return s.SetContextPasswordRef(c.Name, "")
}
