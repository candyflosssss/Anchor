// Package skills owns local Skill inventory, immutable snapshots and explicit,
// version-pinned mounts. Collection preserves sources; explicit cleanup replaces
// a verified, archived source with a managed link. Skill code is never executed.
package skills

import "errors"

var (
	ErrUnsafe  = errors.New("unsafe skill path or unsupported file type")
	ErrLimit   = errors.New("skill inventory limit exceeded")
	ErrChanged = errors.New("skill content or mount changed externally")
	ErrLink    = errors.New("symbolic links unavailable")
)

const MaxFiles = 4096
const MaxBytes = 64 << 20
const MaxText = 48 << 10
const maxInventory = 4096

type SkillFile struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}
type SkillVersion struct {
	Number    int         `json:"number"`
	Parent    int         `json:"parent"`
	Baseline  int         `json:"baseline"`
	Hash      string      `json:"hash"`
	CreatedAt string      `json:"created_at"`
	Origin    string      `json:"origin"`
	Files     []SkillFile `json:"files"`
}
type Skill struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Versions []SkillVersion `json:"versions"`
}
type SkillScope struct {
	ID      string `json:"id"`
	Agent   string `json:"agent"`
	Kind    string `json:"kind"`
	Root    string `json:"root"`
	Project string `json:"project"`
	Origin  string `json:"origin"`
}
type SkillSource struct {
	ID       string `json:"id"`
	ScopeID  string `json:"scope_id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	SkillID  string `json:"skill_id"`
	Baseline int    `json:"baseline"`
	Status   string `json:"status"`
}
type SkillMount struct {
	ID      string `json:"id"`
	SkillID string `json:"skill_id"`
	Version int    `json:"version"`
	ScopeID string `json:"scope_id"`
	Path    string `json:"path"`
	Target  string `json:"target"`
	Status  string `json:"status"`
}
type SkillWorkspace struct {
	Revision int64         `json:"revision"`
	Library  []Skill       `json:"library"`
	Scopes   []SkillScope  `json:"scopes"`
	Sources  []SkillSource `json:"sources"`
	Mounts   []SkillMount  `json:"mounts"`
	Warnings []string      `json:"warnings"`
}
type SkillEdit struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Executable bool   `json:"executable"`
	Delete     bool   `json:"delete"`
}

// Action is a closed, core-validated operation, not a general file API. Paths
// identify approved project roots only; file edits are confined to Skill trees.
type SkillAction struct {
	Action           string      `json:"action"`
	ExpectedRevision int64       `json:"expected_revision"`
	Approved         bool        `json:"approved"`
	SkillID          string      `json:"skill_id"`
	SourceID         string      `json:"source_id"`
	ScopeID          string      `json:"scope_id"`
	MountID          string      `json:"mount_id"`
	Name             string      `json:"name"`
	Version          int         `json:"version"`
	ProjectPath      string      `json:"project_path"`
	ExpectedHash     string      `json:"expected_hash"`
	Edits            []SkillEdit `json:"edits"`
}
type SkillQuery struct {
	SkillID        string `json:"skill_id"`
	Version        int    `json:"version"`
	SourceID       string `json:"source_id"`
	AgainstVersion int    `json:"against_version"`
	File           string `json:"file"`
}
type SkillDifference struct {
	Path      string `json:"path"`
	Change    string `json:"change"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
}
type SkillDetail struct {
	SkillID     string            `json:"skill_id"`
	Version     int               `json:"version"`
	SourceID    string            `json:"source_id"`
	Hash        string            `json:"hash"`
	Files       []SkillFile       `json:"files"`
	Differences []SkillDifference `json:"differences"`
	File        string            `json:"file"`
	Content     string            `json:"content"`
	Editable    bool              `json:"editable"`
}
type association struct {
	Path    string `json:"path"`
	SkillID string `json:"skill_id"`
	Version int    `json:"version"`
}
type manifest struct {
	Format   int            `json:"format"`
	Revision int64          `json:"revision"`
	Library  []Skill        `json:"library"`
	Projects []string       `json:"projects"`
	Sources  []association  `json:"sources"`
	Mounts   []SkillMount   `json:"mounts"`
	Pending  *intent        `json:"pending,omitempty"`
	Cleanup  *cleanupIntent `json:"cleanup,omitempty"`
	Excluded []string       `json:"excluded,omitempty"`
}

// Cleanup is journaled separately from ordinary link installation: the original
// is retained in Backup until the verified snapshot and new mount are durable.
type cleanupIntent struct {
	Mount        SkillMount `json:"mount"`
	Backup       string     `json:"backup"`
	Hash         string     `json:"hash"`
	OriginalLink string     `json:"original_link"`
	Published    bool       `json:"published"`
}

// Write-ahead intent makes crashes during link publication visible/recoverable.
type intent struct {
	Before *SkillMount `json:"before"`
	After  *SkillMount `json:"after"`
	Backup string      `json:"backup"`
}
