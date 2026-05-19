package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fermumen/codexcopilot/internal/catalog"
	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
)

const (
	ProfileName  = "codexcopilot-codex-app"
	ProviderName = "codexcopilot-codex-app"
)

var rootKeys = []string{"profile", "model", "model_provider", "model_catalog_json"}

type rootValue struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

type restoreState struct {
	Root map[string]rootValue `json:"root"`
}

func quote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func backupFile(path string, backupDir string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(backupDir, fmt.Sprintf("%s.%d", filepath.Base(path), time.Now().Unix()))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return err
	}
	matches, _ := filepath.Glob(filepath.Join(backupDir, filepath.Base(path)+".*"))
	sort.Slice(matches, func(i, j int) bool {
		ii, _ := os.Stat(matches[i])
		jj, _ := os.Stat(matches[j])
		return ii.ModTime().After(jj.ModTime())
	})
	for _, stale := range matches[5:] {
		_ = os.Remove(stale)
	}
	return nil
}

func parseRootValues(text string) map[string]rootValue {
	values := map[string]rootValue{}
	keyPattern := regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=\s*(.*)\s*$`)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			break
		}
		match := keyPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		key := match[1]
		for _, wanted := range rootKeys {
			if key != wanted {
				continue
			}
			var value string
			raw := strings.TrimSpace(match[2])
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				value = strings.Trim(raw, `"`)
			}
			values[key] = rootValue{Present: true, Value: value}
		}
	}
	return values
}

func saveRestoreState(p paths.Paths, text string) error {
	if _, err := os.Stat(p.RestoreFile); err == nil && strings.Contains(text, `profile = "`+ProfileName+`"`) {
		return nil
	}
	state := restoreState{Root: map[string]rootValue{}}
	values := parseRootValues(text)
	for _, key := range rootKeys {
		state.Root[key] = values[key]
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p.RestoreFile, append(data, '\n'), 0o600)
}

func sectionRange(lines []string, header string) (int, int, bool) {
	for start, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				end = i
				break
			}
		}
		return start, end, true
	}
	return 0, 0, false
}

func removeSection(text string, header string) string {
	lines := strings.SplitAfter(text, "\n")
	start, end, ok := sectionRange(lines, header)
	if !ok {
		return text
	}
	return strings.Join(append(lines[:start], lines[end:]...), "")
}

func upsertSection(text string, header string, body string) string {
	replacement := header + "\n" + strings.TrimRight(body, "\n") + "\n"
	lines := strings.SplitAfter(text, "\n")
	start, end, ok := sectionRange(lines, header)
	if ok {
		repLines := strings.SplitAfter(replacement, "\n")
		return strings.Join(append(append(lines[:start], repLines...), lines[end:]...), "")
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	return text + replacement
}

func setRootValues(text string, values map[string]string) string {
	lines := strings.SplitAfter(text, "\n")
	tableIndex := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			tableIndex = i
			break
		}
	}
	root := append([]string{}, lines[:tableIndex]...)
	rest := append([]string{}, lines[tableIndex:]...)
	pattern := regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=`)
	seen := map[string]bool{}
	for i, line := range root {
		match := pattern.FindStringSubmatch(line)
		if len(match) == 2 {
			if value, ok := values[match[1]]; ok {
				root[i] = match[1] + " = " + quote(value) + "\n"
				seen[match[1]] = true
			}
		}
	}
	for _, key := range rootKeys {
		value, ok := values[key]
		if ok && !seen[key] {
			root = append(root, key+" = "+quote(value)+"\n")
		}
	}
	return strings.Join(append(root, rest...), "")
}

func restoreRootValues(text string, saved map[string]rootValue) string {
	lines := strings.SplitAfter(text, "\n")
	tableIndex := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			tableIndex = i
			break
		}
	}
	root := append([]string{}, lines[:tableIndex]...)
	rest := append([]string{}, lines[tableIndex:]...)
	pattern := regexp.MustCompile(`^\s*([A-Za-z0-9_-]+)\s*=`)
	indexByKey := map[string]int{}
	for i, line := range root {
		match := pattern.FindStringSubmatch(line)
		if len(match) == 2 {
			indexByKey[match[1]] = i
		}
	}
	for _, key := range rootKeys {
		state := saved[key]
		if state.Present {
			line := key + " = " + quote(state.Value) + "\n"
			if index, ok := indexByKey[key]; ok {
				root[index] = line
			} else {
				root = append(root, line)
			}
		} else if index, ok := indexByKey[key]; ok {
			root[index] = ""
		}
	}
	return strings.Join(append(root, rest...), "")
}

func Configure(p paths.Paths, model string, models []copilot.Model, baseURL string) error {
	data, err := os.ReadFile(p.CodexConfig)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return err
	}
	text := string(data)
	if err := saveRestoreState(p, text); err != nil {
		return err
	}
	if err := backupFile(p.CodexConfig, p.BackupDir); err != nil {
		return err
	}
	catalogData, err := catalog.Build(models, model)
	if err != nil {
		return err
	}
	if err := atomicWrite(p.ModelCatalog, catalogData, 0o644); err != nil {
		return err
	}
	normalizedBase := NormalizeProviderBaseURL(baseURL)
	text = setRootValues(text, map[string]string{
		"profile":            ProfileName,
		"model":              model,
		"model_provider":     ProviderName,
		"model_catalog_json": p.ModelCatalog,
	})
	text = upsertSection(text, "[profiles."+ProfileName+"]", strings.Join([]string{
		"openai_base_url = " + quote(normalizedBase),
		"model = " + quote(model),
		"model_provider = " + quote(ProviderName),
		"model_catalog_json = " + quote(p.ModelCatalog),
	}, "\n"))
	text = upsertSection(text, "[model_providers."+ProviderName+"]", strings.Join([]string{
		"name = " + quote("GitHub Copilot"),
		"base_url = " + quote(normalizedBase),
		`wire_api = "responses"`,
	}, "\n"))
	return atomicWrite(p.CodexConfig, []byte(text), 0o644)
}

func Restore(p paths.Paths) (bool, error) {
	configData, configErr := os.ReadFile(p.CodexConfig)
	restoreData, restoreErr := os.ReadFile(p.RestoreFile)
	if errors.Is(configErr, os.ErrNotExist) && errors.Is(restoreErr, os.ErrNotExist) {
		return false, nil
	}
	if configErr != nil && !errors.Is(configErr, os.ErrNotExist) {
		return false, configErr
	}
	text := string(configData)
	if restoreErr == nil {
		var state restoreState
		if err := json.Unmarshal(restoreData, &state); err != nil {
			return false, err
		}
		text = restoreRootValues(text, state.Root)
	} else if strings.Contains(text, `profile = "`+ProfileName+`"`) {
		text = restoreRootValues(text, map[string]rootValue{})
	}
	text = removeSection(text, "[profiles."+ProfileName+"]")
	text = removeSection(text, "[model_providers."+ProviderName+"]")
	if err := backupFile(p.CodexConfig, p.BackupDir); err != nil {
		return false, err
	}
	if err := atomicWrite(p.CodexConfig, []byte(text), 0o644); err != nil {
		return false, err
	}
	_ = os.Remove(p.ModelCatalog)
	_ = os.Remove(p.RestoreFile)
	return true, nil
}

func NormalizeProviderBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/"
	}
	return baseURL + "/v1/"
}

func LaunchApp() error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Codex").Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", "Codex").Start()
	default:
		return errors.New("Codex App automatic launch is currently supported on macOS and Windows only")
	}
}
