package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
)

const (
	AppName         = "codexcopilot"
	CatalogName     = "codexcopilot-models.json"
	ConfigFile      = "config.toml"
	RestoreFileName = "codex-app-restore.json"
	AuthFileName    = "auth.json"

	ClaudeSettingsFile    = "settings.json"
	ClaudeRestoreFileName = "claude-code-restore.json"
)

type Paths struct {
	CodexDir      string
	CodexConfig   string
	ProfileConfig string
	ModelCatalog  string
	StateDir      string
	AuthFile      string
	RestoreFile   string
	BackupDir     string

	ClaudeDir         string
	ClaudeSettings    string
	ClaudeRestoreFile string
	ClaudeBackupDir   string
}

func ConfigHome() string {
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			return v
		}
		return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support")
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// CodexHome resolves the Codex config directory. Like the Codex CLI itself,
// a non-empty CODEX_HOME env var is used as the directory path directly;
// otherwise it defaults to ~/.codex.
func CodexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		// Not resolved to an absolute path: existing restore state is scoped
		// by a hash of this exact value.
		return filepath.Clean(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// ClaudeHome resolves the Claude Code config directory. Like Claude Code
// itself, a non-empty CLAUDE_CONFIG_DIR env var is used directly; otherwise it
// defaults to ~/.claude.
func ClaudeHome() string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return absPath(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// absPath resolves a relative config dir so restore state, which is scoped by a
// hash of the directory, cannot be shared between different working directories.
func absPath(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return filepath.Clean(dir)
}

func scopedStateDir(stateDir string, prefix string, dir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(dir)))
	return filepath.Join(stateDir, prefix+"-"+hex.EncodeToString(sum[:8]))
}

func Default() Paths {
	codexDir := CodexHome()
	stateDir := filepath.Join(ConfigHome(), AppName)
	codexStateDir := scopedStateDir(stateDir, "codex", codexDir)
	claudeDir := ClaudeHome()
	claudeStateDir := scopedStateDir(stateDir, "claude", claudeDir)
	return Paths{
		CodexDir:      codexDir,
		CodexConfig:   filepath.Join(codexDir, ConfigFile),
		ProfileConfig: filepath.Join(codexDir, "codexcopilot-codex-app.config.toml"),
		ModelCatalog:  filepath.Join(codexDir, CatalogName),
		StateDir:      stateDir,
		AuthFile:      filepath.Join(stateDir, AuthFileName),
		RestoreFile:   filepath.Join(codexStateDir, RestoreFileName),
		BackupDir:     filepath.Join(codexStateDir, "backup"),

		ClaudeDir:         claudeDir,
		ClaudeSettings:    filepath.Join(claudeDir, ClaudeSettingsFile),
		ClaudeRestoreFile: filepath.Join(claudeStateDir, ClaudeRestoreFileName),
		ClaudeBackupDir:   filepath.Join(claudeStateDir, "backup"),
	}
}
