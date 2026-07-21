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
// returns "" if none found (handlers then return a clear error rather than the
// process dying silently).
func readKeyFile(p string) string {
	if len(p) == 0 {
		return ""
	}

	if b, err := os.ReadFile(p); err == nil {
		return strings.TrimSpace(string(b))
	}

	return ""
}

func LoadAPIKey() string {
	if k := strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")); k != "" {
		return k
	}

	if k := readKeyFile(os.Getenv("OPENCODE_KEY_FILE")); k != "" {
		return k
	}

	if k := keyFromOpencodeAuth(); k != "" {
		return k
	}

	if exe, err := os.Executable(); err == nil {
		if k := readKeyFile(filepath.Join(filepath.Dir(exe), "opencode-key.txt")); k != "" {
			return k
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		if k := readKeyFile(filepath.Join(home, ".claude-code-router", "opencode-key.txt")); k != "" {
			return k
		}
	}

	return ""
}

// keyFromOpencodeAuth reads the api key opencode itself stores in auth.json,
// shaped as { "<provider>": { "type":"api", "key":"..." }, ... }.
// Prefers the "opencode-go" provider, then "opencode".
func keyFromOpencodeAuth() string {
	var paths []string

	p := os.Getenv("OPENCODE_AUTH_FILE")
	paths = append(paths, p)

	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		p := filepath.Join(x, "opencode", "auth.json")
		paths = append(paths, p)
	}

	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "share", "opencode", "auth.json") // real path on Win/Linux/mac today
		paths = append(paths, p)

		switch runtime.GOOS {
		case "windows":
			if ad := os.Getenv("APPDATA"); ad != "" {
				p := filepath.Join(ad, "opencode", "auth.json")
				paths = append(paths, p)
			}
		case "darwin":
			p := filepath.Join(home, "Library", "Application Support", "opencode", "auth.json")
			paths = append(paths, p)
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
