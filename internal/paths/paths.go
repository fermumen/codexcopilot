package paths

import (
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
		return filepath.Clean(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func Default() Paths {
	codexDir := CodexHome()
	stateDir := filepath.Join(ConfigHome(), AppName)
	return Paths{
		CodexDir:      codexDir,
		CodexConfig:   filepath.Join(codexDir, ConfigFile),
		ProfileConfig: filepath.Join(codexDir, "codexcopilot-codex-app.config.toml"),
		ModelCatalog:  filepath.Join(codexDir, CatalogName),
		StateDir:      stateDir,
		AuthFile:      filepath.Join(stateDir, AuthFileName),
		RestoreFile:   filepath.Join(stateDir, RestoreFileName),
		BackupDir:     filepath.Join(stateDir, "backup"),
	}
}
