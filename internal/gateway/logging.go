package gateway

import (
	"log"
	"os"
	"path/filepath"
)

// openLogger builds the request logger from the GATEWAY_LOG spec:
//
//	""              -> nil (logging off, zero overhead)
//	"1"/"true"/"on" -> gateway.log next to the executable
//	any other value -> treated as a file path
//
// The file is opened in append mode. Returns nil if the file can't be opened
// (logging is a debugging aid; it must never take the gateway down).
func openLogger(spec string) *log.Logger {
	if spec == "" {
		return nil
	}
	switch spec {
	case "1", "true", "on":
		spec = "gateway.log"
		if exe, err := os.Executable(); err == nil {
			spec = filepath.Join(filepath.Dir(exe), spec)
		}
	}
	f, err := os.OpenFile(spec, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	return log.New(f, "", log.LstdFlags)
}
