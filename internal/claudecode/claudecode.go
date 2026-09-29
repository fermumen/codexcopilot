// Package claudecode points Claude Code's user settings.json at the local
// Copilot proxy and restores the previous values afterwards.
package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fermumen/codexcopilot/internal/copilot"
	"github.com/fermumen/codexcopilot/internal/paths"
)

const (
	// AuthTokenPlaceholder satisfies Claude Code's login check; the proxy strips
	// it and uses the saved GitHub token instead.
	AuthTokenPlaceholder = "codexcopilot"
	pickerDescription    = "GitHub Copilot"
)

// families are the Claude Code model slots, in model picker order.
var families = []string{"opus", "fable", "sonnet", "haiku"}

// defaultFamilies is the preference order for the default model alias.
var defaultFamilies = []string{"opus", "sonnet", "fable", "haiku"}

func slotEnvKey(family string) string {
	return "ANTHROPIC_DEFAULT_" + strings.ToUpper(family) + "_MODEL"
}

// clearedEnvKeys are removed while patched because they would bypass the proxy,
// pin a model id Copilot does not serve, or send another provider's
// credentials (custom headers are forwarded upstream by the proxy).
var clearedEnvKeys = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_CUSTOM_HEADERS",
	"ANTHROPIC_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
	"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	"CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_FOUNDRY",
	"CLAUDE_CODE_USE_GATEWAY",
	"CLAUDE_CODE_USE_MANTLE",
	"CLAUDE_CODE_USE_VERTEX",
}

func managedEnvKeys() []string {
	keys := []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"}
	for _, family := range families {
		keys = append(keys, slotEnvKey(family))
	}
	return append(keys, clearedEnvKeys...)
}

// managedRootKeys are top-level settings codexcopilot writes or removes.
var managedRootKeys = []string{"model", "modelPicker", "apiKeyHelper"}

// Slot is one Claude Code model alias mapped to a Copilot model id.
type Slot struct {
	Alias string
	Model string
	Label string
}

type savedValue struct {
	Present bool            `json:"present"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

type restoreState struct {
	FilePresent bool                  `json:"file_present"`
	EnvPresent  bool                  `json:"env_present"`
	Root        map[string]savedValue `json:"root"`
	Env         map[string]savedValue `json:"env"`
	RootOrder   []string              `json:"root_order,omitempty"`
	EnvOrder    []string              `json:"env_order,omitempty"`
}

// withManagedKeys returns the saved values with every managed key present, so
// keys written by newer versions are removed on restore even when the state
// file predates them.
func withManagedKeys(saved map[string]savedValue, keys []string) map[string]savedValue {
	out := map[string]savedValue{}
	for key, value := range saved {
		out[key] = value
	}
	for _, key := range keys {
		if _, ok := out[key]; !ok {
			out[key] = savedValue{}
		}
	}
	return out
}

// NormalizeBaseURL turns the proxy base URL into the form Claude Code expects:
// no trailing slash and no /v1 suffix, because Claude Code appends /v1/messages.
func NormalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.TrimSuffix(baseURL, "/v1")
}

type claudeID struct {
	Family  string
	Version []int
	Suffix  bool
}

func parseVersion(value string) ([]int, bool) {
	var parts []int
	for _, piece := range strings.Split(value, ".") {
		n, err := strconv.Atoi(piece)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, len(parts) > 0
}

func isFamily(value string) bool {
	for _, family := range families {
		if value == family {
			return true
		}
	}
	return false
}

// parseClaudeID accepts Copilot's dotted ids such as claude-opus-4.8 or
// claude-sonnet-5, and the older claude-3.5-sonnet ordering.
func parseClaudeID(id string) (claudeID, bool) {
	tokens := strings.Split(strings.ToLower(id), "-")
	if len(tokens) < 3 || tokens[0] != "claude" {
		return claudeID{}, false
	}
	family, version := tokens[1], tokens[2]
	if !isFamily(family) {
		family, version = tokens[2], tokens[1]
	}
	if !isFamily(family) {
		return claudeID{}, false
	}
	parts, ok := parseVersion(version)
	if !ok {
		return claudeID{}, false
	}
	return claudeID{Family: family, Version: parts, Suffix: len(tokens) > 3}, true
}

func compareVersion(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// Slots maps each Claude family Copilot serves on its Messages API to the
// newest model id in that family, preferring ids without extra suffixes.
func Slots(models []copilot.Model) []Slot {
	type candidate struct {
		id     string
		label  string
		parsed claudeID
	}
	best := map[string]candidate{}
	for _, model := range copilot.AnthropicMessagesModels(models) {
		id, _ := model["id"].(string)
		parsed, ok := parseClaudeID(id)
		if !ok {
			continue
		}
		label, _ := model["name"].(string)
		if label == "" {
			label = id
		}
		next := candidate{id: id, label: label, parsed: parsed}
		current, ok := best[parsed.Family]
		if !ok {
			best[parsed.Family] = next
			continue
		}
		cmp := compareVersion(parsed.Version, current.parsed.Version)
		if cmp > 0 ||
			(cmp == 0 && current.parsed.Suffix && !parsed.Suffix) ||
			(cmp == 0 && current.parsed.Suffix == parsed.Suffix && id < current.id) {
			best[parsed.Family] = next
		}
	}
	var slots []Slot
	for _, family := range families {
		if c, ok := best[family]; ok {
			slots = append(slots, Slot{Alias: family, Model: c.id, Label: c.label})
		}
	}
	return slots
}

// chooseModel resolves the Claude Code default model: a slot alias, or a
// Copilot Claude id served on the Messages API.
func chooseModel(slots []Slot, models []copilot.Model, requested string) (string, error) {
	if requested == "" {
		for _, family := range defaultFamilies {
			for _, slot := range slots {
				if slot.Alias == family {
					return slot.Alias, nil
				}
			}
		}
		return "", fmt.Errorf("GitHub Copilot returned no Claude models for the Anthropic Messages API")
	}
	for _, slot := range slots {
		if slot.Alias == requested {
			return requested, nil
		}
	}
	for _, model := range copilot.AnthropicMessagesModels(models) {
		if id, _ := model["id"].(string); id == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("Claude model %q was not returned by GitHub Copilot for the Anthropic Messages API", requested)
}

// Configure patches Claude Code settings to use the Copilot proxy at baseURL.
// Only managed keys change; everything else in settings.json is preserved.
// It returns the default model alias or id written to settings.
func Configure(p paths.Paths, models []copilot.Model, baseURL string, requested string) (string, error) {
	slots := Slots(models)
	if len(slots) == 0 {
		return "", fmt.Errorf("GitHub Copilot returned no Claude models for the Anthropic Messages API")
	}
	selected, err := chooseModel(slots, models, requested)
	if err != nil {
		return "", err
	}
	initialData, filePresent, err := readSettings(p.ClaudeSettings)
	if err != nil {
		return "", err
	}
	initial, err := parseObject(initialData)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", p.ClaudeSettings, err)
	}
	if err := saveRestoreState(p, initial, filePresent); err != nil {
		return "", err
	}
	if err := paths.BackupFile(p.ClaudeSettings, p.ClaudeBackupDir); err != nil {
		return "", err
	}

	// Claude Code writes settings.json itself (for example /model). Re-read just
	// before patching so a concurrent write is not replaced with the snapshot
	// used for restore state.
	latestData, _, err := readSettings(p.ClaudeSettings)
	if err != nil {
		return "", err
	}
	settings, err := parseObject(latestData)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", p.ClaudeSettings, err)
	}
	if err := patchSettings(&settings, slots, selected, NormalizeBaseURL(baseURL)); err != nil {
		return "", err
	}
	return selected, writeSettings(p.ClaudeSettings, settings)
}

func patchSettings(settings *object, slots []Slot, selected string, base string) error {
	env, err := settings.child("env")
	if err != nil {
		return err
	}
	env.set("ANTHROPIC_BASE_URL", jsonString(base))
	env.set("ANTHROPIC_AUTH_TOKEN", jsonString(AuthTokenPlaceholder))
	bySlot := map[string]Slot{}
	for _, slot := range slots {
		bySlot[slot.Alias] = slot
	}
	for _, family := range families {
		if slot, ok := bySlot[family]; ok {
			env.set(slotEnvKey(family), jsonString(slot.Model))
		} else {
			env.remove(slotEnvKey(family))
		}
	}
	for _, key := range clearedEnvKeys {
		env.remove(key)
	}
	envRaw, err := env.marshalCompact()
	if err != nil {
		return err
	}
	settings.set("env", envRaw)
	settings.set("model", jsonString(selected))
	settings.set("modelPicker", pickerJSON(slots, selected))
	settings.remove("apiKeyHelper")
	return nil
}

func pickerJSON(slots []Slot, selected string) json.RawMessage {
	type option struct {
		Model       string `json:"model"`
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	picker := struct {
		Options               []option `json:"options"`
		ReplaceBuiltInOptions bool     `json:"replaceBuiltInOptions"`
	}{ReplaceBuiltInOptions: true}
	listed := false
	for _, slot := range slots {
		description := pickerDescription + " · " + slot.Model
		if slot.Alias == selected {
			description += " · default"
			listed = true
		}
		picker.Options = append(picker.Options, option{Model: slot.Alias, Label: slot.Label, Description: description})
	}
	if !listed {
		picker.Options = append(picker.Options, option{Model: selected, Label: selected, Description: pickerDescription + " · default"})
	}
	return marshalNoEscape(picker)
}

func saveRestoreState(p paths.Paths, settings object, filePresent bool) error {
	if _, err := os.Stat(p.ClaudeRestoreFile); err == nil {
		return nil
	}
	state := restoreState{FilePresent: filePresent, Root: map[string]savedValue{}, Env: map[string]savedValue{}}
	for _, key := range managedRootKeys {
		state.Root[key] = savedOf(settings, key)
	}
	state.RootOrder = settings.keys()
	if raw, ok := settings.get("env"); ok {
		env, err := parseObject(raw)
		if err != nil {
			return fmt.Errorf("settings env must be a JSON object: %w", err)
		}
		state.EnvPresent = true
		state.EnvOrder = env.keys()
		for _, key := range managedEnvKeys() {
			state.Env[key] = savedOf(env, key)
		}
	} else {
		for _, key := range managedEnvKeys() {
			state.Env[key] = savedValue{}
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return paths.AtomicWrite(p.ClaudeRestoreFile, append(data, '\n'), 0o600)
}

func savedOf(o object, key string) savedValue {
	raw, ok := o.get(key)
	if !ok {
		return savedValue{}
	}
	return savedValue{Present: true, Raw: raw}
}

// Restore puts managed Claude Code settings back to the values saved by the
// first Configure. It returns false when there is no restore state.
func Restore(p paths.Paths) (bool, error) {
	stateData, err := os.ReadFile(p.ClaudeRestoreFile)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state restoreState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return false, err
	}
	data, _, err := readSettings(p.ClaudeSettings)
	if err != nil {
		return false, err
	}
	settings, err := parseObject(data)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", p.ClaudeSettings, err)
	}
	applyAllSaved(&settings, withManagedKeys(state.Root, managedRootKeys), state.RootOrder)
	env, err := settings.child("env")
	if err != nil {
		return false, err
	}
	applyAllSaved(&env, withManagedKeys(state.Env, managedEnvKeys()), state.EnvOrder)
	if len(env) == 0 && !state.EnvPresent {
		settings.remove("env")
	} else {
		envRaw, err := env.marshalCompact()
		if err != nil {
			return false, err
		}
		settings.set("env", envRaw)
	}
	if err := paths.BackupFile(p.ClaudeSettings, p.ClaudeBackupDir); err != nil {
		return false, err
	}
	if len(settings) == 0 && !state.FilePresent {
		if err := os.Remove(p.ClaudeSettings); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	} else if err := writeSettings(p.ClaudeSettings, settings); err != nil {
		return false, err
	}
	_ = os.Remove(p.ClaudeRestoreFile)
	return true, nil
}

// applyAllSaved puts saved values back. Keys that were removed while patched
// are re-inserted after their original predecessor so a restore reproduces the
// original key order.
func applyAllSaved(o *object, saved map[string]savedValue, order []string) {
	keys := []string{}
	seen := map[string]bool{}
	for _, key := range order {
		if _, ok := saved[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	for _, key := range sortedKeys(saved) {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		value := saved[key]
		if !value.Present {
			o.remove(key)
			continue
		}
		if _, ok := o.get(key); ok {
			o.set(key, value.Raw)
			continue
		}
		o.insertAfterPredecessor(key, value.Raw, order)
	}
}

func readSettings(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func writeSettings(path string, settings object) error {
	data, err := settings.marshalIndent()
	if err != nil {
		return err
	}
	// settings.json can hold credentials, so a new file is user-only.
	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return paths.AtomicWrite(path, data, perm)
}

// object is a JSON object that keeps key order and raw member values, so
// unmanaged settings round-trip unchanged apart from indentation.
type object []member

type member struct {
	Key   string
	Value json.RawMessage
}

func parseObject(data []byte) (object, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return object{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}
	out := object{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out.set(key, raw)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after JSON object")
	}
	return out, nil
}

func (o object) get(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.Key == key {
			return m.Value, true
		}
	}
	return nil, false
}

func (o *object) set(key string, value json.RawMessage) {
	for i, m := range *o {
		if m.Key == key {
			(*o)[i].Value = value
			return
		}
	}
	*o = append(*o, member{Key: key, Value: value})
}

// insertAfterPredecessor inserts key after the nearest key that preceded it in
// order and is still present, or appends when key was not in order.
func (o *object) insertAfterPredecessor(key string, value json.RawMessage, order []string) {
	idx := -1
	for i, k := range order {
		if k == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		o.set(key, value)
		return
	}
	at := 0
	for i := idx - 1; i >= 0; i-- {
		if pos := o.index(order[i]); pos >= 0 {
			at = pos + 1
			break
		}
	}
	*o = append(*o, member{})
	copy((*o)[at+1:], (*o)[at:])
	(*o)[at] = member{Key: key, Value: value}
}

func (o object) index(key string) int {
	for i, m := range o {
		if m.Key == key {
			return i
		}
	}
	return -1
}

func (o object) keys() []string {
	out := make([]string, 0, len(o))
	for _, m := range o {
		out = append(out, m.Key)
	}
	return out
}

func (o *object) remove(key string) {
	out := (*o)[:0]
	for _, m := range *o {
		if m.Key != key {
			out = append(out, m)
		}
	}
	*o = out
}

// child returns the nested object at key, or an empty object when absent.
func (o object) child(key string) (object, error) {
	raw, ok := o.get(key)
	if !ok {
		return object{}, nil
	}
	child, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("settings %s must be a JSON object: %w", key, err)
	}
	return child, nil
}

func (o object) marshalCompact() (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshalNoEscape(m.Key))
		buf.WriteByte(':')
		if err := json.Compact(&buf, m.Value); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (o object) marshalIndent() ([]byte, error) {
	compact, err := o.marshalCompact()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func jsonString(value string) json.RawMessage {
	return marshalNoEscape(value)
}

func marshalNoEscape(value any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func sortedKeys(values map[string]savedValue) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
