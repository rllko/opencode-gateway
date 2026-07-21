package gateway

import (
	"io"
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
// The file is opened in append mode. It also returns the file as an io.Closer so
// the caller owns its lifetime; on Windows an open handle blocks deletion, so the
// file must be closed on shutdown (and in tests before temp-dir cleanup). Returns
// (nil, nil) if logging is off or the file can't be opened (logging is a debugging
// aid; it must never take the gateway down).
func openLogger(spec string) (*log.Logger, io.Closer) {
	if spec == "" {
		return nil, nil
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
		return nil, nil
	}
	return log.New(f, "", log.LstdFlags), f
}
