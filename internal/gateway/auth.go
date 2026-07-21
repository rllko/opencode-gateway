package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LoadAPIKey looks in, in order:
//  1. $OPENCODE_API_KEY               (explicit override)
//  2. $OPENCODE_KEY_FILE              (explicit file override)
//  3. opencode's own auth store       (single source of truth — auto-syncs with opencode)
//  4. opencode-key.txt next to the exe (portable fallback for sharing)
//  5. ~/.claude-code-router/opencode-key.txt
//
// Returns "" if none found (handlers then return a clear error rather than the
// process dying silently).
func LoadAPIKey() string {
	read := func(p string) string {
		if p == "" {
			return ""
		}
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
		return ""
	}
	if k := strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")); k != "" {
		return k
	}
	if k := read(os.Getenv("OPENCODE_KEY_FILE")); k != "" {
		return k
	}
	if k := keyFromOpencodeAuth(); k != "" {
		return k
	}
	if exe, err := os.Executable(); err == nil {
		if k := read(filepath.Join(filepath.Dir(exe), "opencode-key.txt")); k != "" {
			return k
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if k := read(filepath.Join(home, ".claude-code-router", "opencode-key.txt")); k != "" {
			return k
		}
	}
	return ""
}

// keyFromOpencodeAuth reads the api key opencode itself stores in auth.json,
// shaped as { "<provider>": { "type":"api", "key":"..." }, ... }.
// Prefers the "opencode-go" provider, then "opencode".
//
// opencode currently follows the XDG layout on ALL platforms, including Windows
// (see anomalyco/opencode#8235), so ~/.local/share/opencode/auth.json is the real
// location on Windows too. The OS-native dirs (%APPDATA%, macOS Application Support)
// are added as defensive fallbacks in case that changes.
func keyFromOpencodeAuth() string {
	var paths []string
	add := func(p string) {
		if p != "" {
			paths = append(paths, p)
		}
	}
	add(os.Getenv("OPENCODE_AUTH_FILE"))
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		add(filepath.Join(x, "opencode", "auth.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local", "share", "opencode", "auth.json")) // real path on Win/Linux/mac today
		switch runtime.GOOS {
		case "windows":
			if ad := os.Getenv("APPDATA"); ad != "" {
				add(filepath.Join(ad, "opencode", "auth.json"))
			}
		case "darwin":
			add(filepath.Join(home, "Library", "Application Support", "opencode", "auth.json"))
		}
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m map[string]struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		for _, name := range []string{"opencode-go", "opencode"} {
			if k := strings.TrimSpace(m[name].Key); k != "" {
				return k
			}
		}
	}
	return ""
}
