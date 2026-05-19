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
	CodexDir     string
	CodexConfig  string
	ModelCatalog string
	StateDir     string
	AuthFile     string
	RestoreFile  string
	BackupDir    string
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

func Default() Paths {
	home, _ := os.UserHomeDir()
	codexDir := filepath.Join(home, ".codex")
	stateDir := filepath.Join(ConfigHome(), AppName)
	return Paths{
		CodexDir:     codexDir,
		CodexConfig:  filepath.Join(codexDir, ConfigFile),
		ModelCatalog: filepath.Join(codexDir, CatalogName),
		StateDir:     stateDir,
		AuthFile:     filepath.Join(stateDir, AuthFileName),
		RestoreFile:  filepath.Join(stateDir, RestoreFileName),
		BackupDir:    filepath.Join(stateDir, "backup"),
	}
}
